package binding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
)

type testAgents struct {
	mu    sync.Mutex
	items []agentengine.Agent
	err   error
}

func (a *testAgents) List(context.Context, agentengine.AgentListOptions) ([]agentengine.Agent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]agentengine.Agent(nil), a.items...), a.err
}

func hostedAgent(id string) agentengine.Agent {
	return agentengine.Agent{ID: id, Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}}
}

type testProvider struct {
	mu      sync.Mutex
	err     error
	entries map[string]struct {
		participant string
		app         feishu.AppConfig
	}
}

func (p *testProvider) BotConfigForAgent(agentID string) (string, feishu.AppConfig, bool) {
	participantID, app, ok, _ := p.BotConfigForAgentWithError(agentID)
	return participantID, app, ok
}

func (p *testProvider) BotConfigForAgentWithError(agentID string) (string, feishu.AppConfig, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return "", feishu.AppConfig{}, false, p.err
	}
	entry, ok := p.entries[agentID]
	return entry.participant, entry.app, ok, nil
}

type testWorker struct {
	mu     sync.Mutex
	starts int
	closes int
	local  []feishu.MessageEvent
}

func (w *testWorker) Start(context.Context) error { w.mu.Lock(); w.starts++; w.mu.Unlock(); return nil }
func (w *testWorker) Close(context.Context) error { w.mu.Lock(); w.closes++; w.mu.Unlock(); return nil }
func (w *testWorker) HandleLocalMessage(_ context.Context, event feishu.MessageEvent) error {
	w.mu.Lock()
	w.local = append(w.local, event)
	w.mu.Unlock()
	return nil
}

type testWorkerFactory struct {
	mu      sync.Mutex
	workers []*testWorker
}

type blockingWorker struct {
	started chan struct{}
	release chan struct{}
	closed  chan struct{}
}

func (w *blockingWorker) Start(context.Context) error {
	close(w.started)
	<-w.release
	return nil
}

func (w *blockingWorker) Close(context.Context) error {
	close(w.closed)
	return nil
}

type blockingWorkerFactory struct{ worker *blockingWorker }

func (f blockingWorkerFactory) NewWorker(Resolved) (Worker, error) { return f.worker, nil }

func (f *testWorkerFactory) NewWorker(Resolved) (Worker, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := &testWorker{}
	f.workers = append(f.workers, w)
	return w, nil
}

func TestManagerKeepsBindingAcrossAgentRuntimeStateChanges(t *testing.T) {
	agentItem := hostedAgent("agent-1")
	agentItem.Status = agentengine.AgentStatus{State: agentengine.AgentStateStopped, RuntimeID: "runtime-old"}
	agents := &testAgents{items: []agentengine.Agent{agentItem}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{"agent-1": {participant: "bot-1", app: feishu.AppConfig{AppID: "app", AppSecret: "secret"}}}}
	factory := &testWorkerFactory{}
	manager, err := NewManager(ManagerOptions{Agents: agents, Provider: provider, Workers: factory, ReconcileInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitForWorkers(t, factory, 1)

	agents.mu.Lock()
	agents.items[0].Status = agentengine.AgentStatus{State: agentengine.AgentStateRunning, RuntimeID: "runtime-new", Ready: true}
	agents.mu.Unlock()
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(factory.workers) != 1 {
		t.Fatalf("workers = %d, want one stable binding worker", len(factory.workers))
	}
}

func TestManagerRoutesLocalMessageToMentionedBinding(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{hostedAgent("agent-1")}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{"agent-1": {participant: "bot-1", app: feishu.AppConfig{AppID: "app", AppSecret: "secret"}}}}
	factory := &testWorkerFactory{}
	bus := feishu.NewMessageBus()
	manager, err := NewManager(ManagerOptions{
		Agents: agents, Provider: provider, Workers: factory, Messages: bus, ReconcileInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		active := len(manager.active)
		manager.mu.Unlock()
		if active == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	bus.Publish(feishu.MessageEvent{
		Type: feishu.MessageEventTypeMessageCreated, RoomID: "oc-room",
		MentionBotID: "bot-1",
		Message:      &im.Message{ID: "om-local", SenderID: "ou-manager", Content: "hello"},
	})

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		factory.mu.Lock()
		var worker *testWorker
		if len(factory.workers) > 0 {
			worker = factory.workers[0]
		}
		factory.mu.Unlock()
		if worker != nil {
			worker.mu.Lock()
			count := len(worker.local)
			worker.mu.Unlock()
			if count == 1 {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("local Feishu message was not routed to the mentioned binding")
}

func TestManagerStopsOnlyWhenBindingIsRemoved(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{hostedAgent("agent-1")}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{"agent-1": {participant: "bot-1", app: feishu.AppConfig{AppID: "app", AppSecret: "secret"}}}}
	factory := &testWorkerFactory{}
	manager, err := NewManager(ManagerOptions{Agents: agents, Provider: provider, Workers: factory, ReconcileInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForWorkers(t, factory, 1)

	provider.mu.Lock()
	delete(provider.entries, "agent-1")
	provider.mu.Unlock()
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager.Close()

	factory.workers[0].mu.Lock()
	closes := factory.workers[0].closes
	factory.workers[0].mu.Unlock()
	if closes != 1 {
		t.Fatalf("worker closes = %d, want 1", closes)
	}
}

func TestResolverLeavesRuntimeNativeFeishuBindingsAlone(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{
		hostedAgent("codex"),
		{ID: "picoclaw", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "picoclaw", Sandboxed: true}}},
		{ID: "openclaw", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "openclaw", Sandboxed: true}}},
	}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{
		"codex":    {participant: "bot-codex", app: feishu.AppConfig{AppID: "app-codex", AppSecret: "secret"}},
		"picoclaw": {participant: "bot-pico", app: feishu.AppConfig{AppID: "app-pico", AppSecret: "secret"}},
		"openclaw": {participant: "bot-open", app: feishu.AppConfig{AppID: "app-open", AppSecret: "secret"}},
	}}
	resolved, err := NewResolver(agents, provider).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].Binding.AgentID != "codex" {
		t.Fatalf("resolved bindings = %+v, want only the CSGClaw-hosted Codex binding", resolved)
	}
}

func TestResolverExcludesDuplicateHostedAppOwners(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{
		hostedAgent("codex-a"),
		hostedAgent("codex-b"),
		hostedAgent("codex-safe"),
	}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{
		"codex-a":    {participant: "bot-a", app: feishu.AppConfig{AppID: " shared-app ", AppSecret: "secret-a"}},
		"codex-b":    {participant: "bot-b", app: feishu.AppConfig{AppID: "shared-app", AppSecret: "secret-b"}},
		"codex-safe": {participant: "bot-safe", app: feishu.AppConfig{AppID: "safe-app", AppSecret: "secret-safe"}},
	}}

	resolved, err := NewResolver(agents, provider).All(context.Background())
	if !errors.Is(err, ErrAppOwnershipConflict) || !errors.Is(err, ErrAuthoritativeBindingConflict) {
		t.Fatalf("All() error = %v, want authoritative AppID conflict", err)
	}
	var conflict *AppOwnershipConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("All() error = %T, want *AppOwnershipConflictError", err)
	}
	if conflict.AppID != "shared-app" || len(conflict.Owners) != 2 {
		t.Fatalf("conflict = %+v, want shared-app with two owners", conflict)
	}
	if len(resolved) != 1 || resolved[0].Binding.AgentID != "codex-safe" {
		t.Fatalf("resolved bindings = %+v, want only conflict-free Codex binding", resolved)
	}
}

func TestManagerStopsRunningWorkerWhenAppOwnershipBecomesConflicted(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{hostedAgent("codex")}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{
		"codex": {participant: "bot-codex", app: feishu.AppConfig{AppID: "shared-app", AppSecret: "codex-secret"}},
	}}
	factory := &testWorkerFactory{}
	manager, err := NewManager(ManagerOptions{Agents: agents, Provider: provider, Workers: factory})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(factory.workers) != 1 {
		t.Fatalf("workers = %d, want one initial worker", len(factory.workers))
	}

	agents.mu.Lock()
	agents.items = append(agents.items, hostedAgent("codex-2"))
	agents.mu.Unlock()
	provider.mu.Lock()
	provider.entries["codex-2"] = struct {
		participant string
		app         feishu.AppConfig
	}{participant: "bot-codex-2", app: feishu.AppConfig{AppID: "shared-app", AppSecret: "codex-secret-2"}}
	provider.mu.Unlock()

	err = manager.Reconcile(context.Background())
	if !errors.Is(err, ErrAppOwnershipConflict) {
		t.Fatalf("Reconcile() error = %v, want AppID ownership conflict", err)
	}
	factory.workers[0].mu.Lock()
	closes := factory.workers[0].closes
	factory.workers[0].mu.Unlock()
	if closes != 1 {
		t.Fatalf("conflicted worker closes = %d, want 1", closes)
	}
	manager.mu.Lock()
	active := len(manager.active)
	manager.mu.Unlock()
	if active != 0 {
		t.Fatalf("active workers = %d, want fail-closed empty set", active)
	}
}

func TestManagerReconcilesConflictFreeBindingFromAuthoritativePartialSnapshot(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{
		hostedAgent("codex-a"),
		hostedAgent("codex-b"),
		hostedAgent("codex-safe"),
	}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{
		"codex-a":    {participant: "bot-a", app: feishu.AppConfig{AppID: "shared-app", AppSecret: "secret-a"}},
		"codex-b":    {participant: "bot-b", app: feishu.AppConfig{AppID: "shared-app", AppSecret: "secret-b"}},
		"codex-safe": {participant: "bot-safe", app: feishu.AppConfig{AppID: "safe-app", AppSecret: "secret-safe"}},
	}}
	factory := &testWorkerFactory{}
	manager, err := NewManager(ManagerOptions{Agents: agents, Provider: provider, Workers: factory})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	err = manager.Reconcile(context.Background())
	if !errors.Is(err, ErrAppOwnershipConflict) {
		t.Fatalf("Reconcile() error = %v, want AppID ownership conflict", err)
	}
	manager.mu.Lock()
	active := make(map[string]*activeWorker, len(manager.active))
	for id, worker := range manager.active {
		active[id] = worker
	}
	manager.mu.Unlock()
	if len(active) != 1 || active[stableBindingID("bot-safe")] == nil {
		t.Fatalf("active workers = %+v, want only conflict-free binding", active)
	}
}

func TestSafeHostedDesiredRejectsDuplicateAppID(t *testing.T) {
	resolved := []Resolved{
		{
			Binding: channelBinding("codex-a", "bot-a"),
			App:     feishu.AppConfig{AppID: " shared-app ", AppSecret: "secret-a"},
		},
		{
			Binding: channelBinding("codex-b", "bot-b"),
			App:     feishu.AppConfig{AppID: "shared-app", AppSecret: "secret-b"},
		},
	}
	desired, err := safeHostedDesired(resolved)
	if !errors.Is(err, ErrAppOwnershipConflict) {
		t.Fatalf("safeHostedDesired() error = %v, want AppID conflict", err)
	}
	if len(desired) != 0 {
		t.Fatalf("safeHostedDesired() = %+v, want no conflicted bindings", desired)
	}
}

func TestManagerKeepsWorkersOnCredentialSnapshotFailure(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{hostedAgent("agent-1")}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{"agent-1": {participant: "bot-1", app: feishu.AppConfig{AppID: "app", AppSecret: "secret"}}}}
	factory := &testWorkerFactory{}
	manager, err := NewManager(ManagerOptions{Agents: agents, Provider: provider, Workers: factory})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	provider.err = errors.New("temporary store failure")
	provider.mu.Unlock()
	if err := manager.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil during credential snapshot failure")
	}
	factory.workers[0].mu.Lock()
	closes := factory.workers[0].closes
	factory.workers[0].mu.Unlock()
	if closes != 0 {
		t.Fatalf("worker closes = %d, want healthy worker preserved", closes)
	}
	manager.Close()
}

func TestManagerKeepsWorkersOnAgentEngineSnapshotFailure(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{hostedAgent("agent-1")}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{"agent-1": {participant: "bot-1", app: feishu.AppConfig{AppID: "app", AppSecret: "secret"}}}}
	factory := &testWorkerFactory{}
	manager, err := NewManager(ManagerOptions{Agents: agents, Provider: provider, Workers: factory})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	agents.mu.Lock()
	agents.err = errors.New("temporary Agent Engine read failure")
	agents.mu.Unlock()
	if err := manager.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil during Agent Engine snapshot failure")
	}
	factory.workers[0].mu.Lock()
	closes := factory.workers[0].closes
	factory.workers[0].mu.Unlock()
	if closes != 0 {
		t.Fatalf("worker closes = %d, want healthy worker preserved", closes)
	}
}

func TestManagerCloseWaitsForReconcileAndClosesNewWorker(t *testing.T) {
	agents := &testAgents{items: []agentengine.Agent{hostedAgent("agent-1")}}
	provider := &testProvider{entries: map[string]struct {
		participant string
		app         feishu.AppConfig
	}{"agent-1": {participant: "bot-1", app: feishu.AppConfig{AppID: "app", AppSecret: "secret"}}}}
	worker := &blockingWorker{started: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
	manager, err := NewManager(ManagerOptions{Agents: agents, Provider: provider, Workers: blockingWorkerFactory{worker: worker}})
	if err != nil {
		t.Fatal(err)
	}
	reconciled := make(chan error, 1)
	go func() { reconciled <- manager.Reconcile(context.Background()) }()
	select {
	case <-worker.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	closed := make(chan struct{})
	go func() {
		manager.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while Reconcile was still starting a worker")
	case <-time.After(20 * time.Millisecond):
	}
	close(worker.release)
	if err := <-reconciled; err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish")
	}
	select {
	case <-worker.closed:
	case <-time.After(time.Second):
		t.Fatal("worker installed by Reconcile was not closed")
	}
}

func waitForWorkers(t *testing.T, factory *testWorkerFactory, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		factory.mu.Lock()
		got := len(factory.workers)
		factory.mu.Unlock()
		if got >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	factory.mu.Lock()
	got := len(factory.workers)
	factory.mu.Unlock()
	t.Fatalf("workers = %d, want at least %d", got, want)
}
