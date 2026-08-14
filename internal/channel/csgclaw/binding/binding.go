package binding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/execution"
	"csgclaw/internal/channel/csgclaw/ingress"
	"csgclaw/internal/channelbridge"
)

const workerCloseTimeout = 5 * time.Second

// Manager owns Binding-scoped ingress workers. Agent Start/Stop/Recreate must
// not call Ensure or Stop; only Binding create/update/delete should.
type Manager struct {
	adapter *execution.Adapter

	mu      sync.Mutex
	workers map[channel.BindingID]*ingress.Worker
}

func NewManager(adapter *execution.Adapter) (*Manager, error) {
	if adapter == nil {
		return nil, fmt.Errorf("built-in IM adapter is required")
	}
	return &Manager{adapter: adapter, workers: make(map[channel.BindingID]*ingress.Worker)}, nil
}

func (m *Manager) Ensure(value channel.Binding) error {
	if m == nil || m.adapter == nil {
		return fmt.Errorf("built-in IM worker manager is not configured")
	}
	bindingID := value.StableID()
	if bindingID == "" || strings.TrimSpace(value.AgentID) == "" {
		return fmt.Errorf("binding id and agent id are required")
	}
	normalized := channel.Binding{
		ID:            string(bindingID),
		Channel:       channel.ChannelCSGClaw,
		ParticipantID: strings.TrimSpace(value.ParticipantID),
		AgentID:       strings.TrimSpace(value.AgentID),
		Enabled:       true,
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.workers[bindingID]; existing != nil {
		if existing.SameBinding(normalized) {
			return nil
		}
		delete(m.workers, bindingID)
		if err := closeWorker(existing); err != nil {
			return fmt.Errorf("replace built-in IM binding %q: %w", bindingID, err)
		}
	}
	worker, err := ingress.NewWorker(m.adapter, normalized)
	if err != nil {
		return err
	}
	m.workers[bindingID] = worker
	return nil
}

func (m *Manager) Stop(bindingID channel.BindingID) {
	if m == nil {
		return
	}
	bindingID = channel.BindingID(strings.TrimSpace(string(bindingID)))
	m.mu.Lock()
	worker := m.workers[bindingID]
	delete(m.workers, bindingID)
	m.mu.Unlock()
	if worker != nil {
		if err := closeWorker(worker); err != nil {
			slog.Warn("stop built-in IM binding failed", "binding_id", bindingID, "error", err)
		}
	}
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	workers := make([]*ingress.Worker, 0, len(m.workers))
	for id, worker := range m.workers {
		workers = append(workers, worker)
		delete(m.workers, id)
	}
	m.mu.Unlock()
	for _, worker := range workers {
		if err := closeWorker(worker); err != nil {
			slog.Warn("close built-in IM binding failed", "error", err)
		}
	}
}

// Submit enqueues an already-routed source event onto the binding worker.
func (m *Manager) Submit(value channel.Binding, event channelbridge.BotEvent) error {
	if m == nil {
		return fmt.Errorf("built-in IM worker manager is not configured")
	}
	bindingID := value.StableID()
	m.mu.Lock()
	worker := m.workers[bindingID]
	m.mu.Unlock()
	if worker == nil {
		return fmt.Errorf("binding worker %q is not running", bindingID)
	}
	return worker.Submit(event)
}

// IsCurrent checks detached continuation freshness without changing the
// Binding worker lifecycle.
func (m *Manager) IsCurrent(bindingID channel.BindingID, key agentengine.ConversationKey, sourceMessageID string) bool {
	if m == nil {
		return false
	}
	bindingID = channel.BindingID(strings.TrimSpace(string(bindingID)))
	m.mu.Lock()
	worker := m.workers[bindingID]
	m.mu.Unlock()
	return worker != nil && worker.IsCurrent(key, sourceMessageID)
}

func closeWorker(worker *ingress.Worker) error {
	if worker == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), workerCloseTimeout)
	defer cancel()
	return worker.Close(ctx)
}
