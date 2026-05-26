package upgrade

import (
	"context"
	"sync"
	"time"

	"csgclaw/internal/apitypes"
)

const defaultCheckInterval = time.Hour

type Checker interface {
	Check(ctx context.Context, currentVersion string) (CheckResult, error)
}

type ManagerOptions struct {
	CheckInterval  time.Duration
	Now            func() time.Time
	OnStatusChange func(apitypes.UpgradeStatus)
}

type Manager struct {
	checker        Checker
	currentVersion string
	checkInterval  time.Duration
	now            func() time.Time
	onStatusChange func(apitypes.UpgradeStatus)
	stickyError    string

	mu     sync.RWMutex
	status apitypes.UpgradeStatus
}

func NewManager(checker Checker, currentVersion string, opts ManagerOptions) *Manager {
	interval := opts.CheckInterval
	if interval <= 0 {
		interval = defaultCheckInterval
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	return &Manager{
		checker:        checker,
		currentVersion: currentVersion,
		checkInterval:  interval,
		now:            now,
		onStatusChange: opts.OnStatusChange,
		status: apitypes.UpgradeStatus{
			CurrentVersion: currentVersion,
		},
	}
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil || m.checker == nil {
		return
	}

	m.Refresh(ctx)

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Refresh(ctx)
		}
	}
}

func (m *Manager) Refresh(ctx context.Context) {
	if m == nil || m.checker == nil {
		return
	}

	m.mu.Lock()
	previous := copyStatus(m.status)
	m.status.Checking = true
	m.status.CurrentVersion = m.currentVersion
	m.mu.Unlock()

	result, err := m.checker.Check(ctx, m.currentVersion)
	checkedAt := m.now().UTC()

	m.mu.Lock()
	m.status.Checking = false
	m.status.CurrentVersion = m.currentVersion
	m.status.LastCheckedAt = &checkedAt

	if err != nil {
		if m.stickyError != "" {
			m.status.LastError = m.stickyError
		} else {
			m.status.LastError = err.Error()
		}
		updated := copyStatus(m.status)
		notify := shouldNotifyStatusChange(previous, updated)
		callback := m.onStatusChange
		m.mu.Unlock()
		if notify && callback != nil {
			callback(updated)
		}
		return
	}

	m.status.LatestVersion = result.LatestVersion
	m.status.UpdateAvailable = result.UpdateAvailable
	m.status.LastError = m.stickyError
	updated := copyStatus(m.status)
	notify := shouldNotifyStatusChange(previous, updated)
	callback := m.onStatusChange
	m.mu.Unlock()
	if notify && callback != nil {
		callback(updated)
	}
}

func (m *Manager) Status() apitypes.UpgradeStatus {
	if m == nil {
		return apitypes.UpgradeStatus{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	status := m.status
	if status.LastCheckedAt != nil {
		t := *status.LastCheckedAt
		status.LastCheckedAt = &t
	}
	return status
}

func (m *Manager) MarkUpgrading() apitypes.UpgradeStatus {
	if m == nil {
		return apitypes.UpgradeStatus{}
	}

	m.mu.Lock()
	previous := copyStatus(m.status)
	m.status.Upgrading = true
	m.status.ManualRestartRequired = false
	m.stickyError = ""
	m.status.LastError = ""
	updated := copyStatus(m.status)
	notify := shouldNotifyStatusChange(previous, updated)
	callback := m.onStatusChange
	m.mu.Unlock()
	if notify && callback != nil {
		callback(updated)
	}
	return updated
}

func (m *Manager) MarkUpgradeFailed(err error) apitypes.UpgradeStatus {
	if m == nil {
		return apitypes.UpgradeStatus{}
	}

	m.mu.Lock()
	previous := copyStatus(m.status)
	m.status.Upgrading = false
	m.status.ManualRestartRequired = false
	m.stickyError = ""
	if err != nil {
		m.stickyError = err.Error()
		m.status.LastError = m.stickyError
	}
	updated := copyStatus(m.status)
	notify := shouldNotifyStatusChange(previous, updated)
	callback := m.onStatusChange
	m.mu.Unlock()
	if notify && callback != nil {
		callback(updated)
	}
	return updated
}

func (m *Manager) MarkManualRestartRequired() apitypes.UpgradeStatus {
	if m == nil {
		return apitypes.UpgradeStatus{}
	}

	m.mu.Lock()
	previous := copyStatus(m.status)
	m.status.Upgrading = false
	m.status.ManualRestartRequired = true
	m.stickyError = ""
	m.status.LastError = ""
	updated := copyStatus(m.status)
	notify := shouldNotifyStatusChange(previous, updated)
	callback := m.onStatusChange
	m.mu.Unlock()
	if notify && callback != nil {
		callback(updated)
	}
	return updated
}

func copyStatus(status apitypes.UpgradeStatus) apitypes.UpgradeStatus {
	if status.LastCheckedAt != nil {
		t := *status.LastCheckedAt
		status.LastCheckedAt = &t
	}
	return status
}

func shouldNotifyStatusChange(previous, current apitypes.UpgradeStatus) bool {
	return previous.CurrentVersion != current.CurrentVersion ||
		previous.LatestVersion != current.LatestVersion ||
		previous.UpdateAvailable != current.UpdateAvailable ||
		previous.Upgrading != current.Upgrading ||
		previous.ManualRestartRequired != current.ManualRestartRequired ||
		previous.LastError != current.LastError
}
