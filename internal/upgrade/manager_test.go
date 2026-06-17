package upgrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
)

type fakeChecker struct {
	check func(context.Context, string) (CheckResult, error)
}

func (f fakeChecker) Check(ctx context.Context, currentVersion string) (CheckResult, error) {
	return f.check(ctx, currentVersion)
}

func TestManagerRefreshUpdatesStatus(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, currentVersion string) (CheckResult, error) {
			if got, want := currentVersion, "v0.2.5"; got != want {
				t.Fatalf("currentVersion = %q, want %q", got, want)
			}
			return CheckResult{
				CurrentVersion:  "v0.2.5",
				LatestVersion:   "v0.2.7",
				UpdateAvailable: true,
			}, nil
		},
	}, "v0.2.5", ManagerOptions{
		Now: func() time.Time { return now },
	})

	manager.Refresh(context.Background())

	status := manager.Status()
	if !status.UpdateAvailable {
		t.Fatal("UpdateAvailable = false, want true")
	}
	if got, want := status.LatestVersion, "v0.2.7"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	if status.Checking {
		t.Fatal("Checking = true, want false")
	}
	if status.LastCheckedAt == nil || !status.LastCheckedAt.Equal(now) {
		t.Fatalf("LastCheckedAt = %v, want %v", status.LastCheckedAt, now)
	}
	if status.LastError != "" {
		t.Fatalf("LastError = %q, want empty", status.LastError)
	}
}

func TestManagerIncludesAutoUpgradeSupport(t *testing.T) {
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, _ string) (CheckResult, error) {
			return CheckResult{
				CurrentVersion:  "v0.2.5",
				LatestVersion:   "v0.2.7",
				UpdateAvailable: true,
			}, nil
		},
	}, "v0.2.5", ManagerOptions{
		AutoUpgradeSupported:         false,
		AutoUpgradeUnsupportedReason: "not_official_bundle",
	})

	manager.Refresh(context.Background())

	status := manager.Status()
	if status.AutoUpgradeSupported {
		t.Fatal("AutoUpgradeSupported = true, want false")
	}
	if got, want := status.AutoUpgradeUnsupportedReason, "not_official_bundle"; got != want {
		t.Fatalf("AutoUpgradeUnsupportedReason = %q, want %q", got, want)
	}
}

func TestManagerRefreshKeepsLastKnownVersionOnError(t *testing.T) {
	now := time.Date(2026, 5, 6, 13, 0, 0, 0, time.UTC)
	call := 0
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, _ string) (CheckResult, error) {
			call++
			if call == 1 {
				return CheckResult{
					CurrentVersion:  "v0.2.5",
					LatestVersion:   "v0.2.7",
					UpdateAvailable: true,
				}, nil
			}
			return CheckResult{}, errors.New("network unavailable")
		},
	}, "v0.2.5", ManagerOptions{
		Now: func() time.Time { return now },
	})

	manager.Refresh(context.Background())
	manager.Refresh(context.Background())

	status := manager.Status()
	if got, want := status.LatestVersion, "v0.2.7"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	if !status.UpdateAvailable {
		t.Fatal("UpdateAvailable = false, want true")
	}
	if got, want := status.LastError, "network unavailable"; got != want {
		t.Fatalf("LastError = %q, want %q", got, want)
	}
}

func TestManagerRefreshPreservesUpgradeFailure(t *testing.T) {
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, _ string) (CheckResult, error) {
			return CheckResult{
				CurrentVersion:  "v0.2.5",
				LatestVersion:   "v0.2.7",
				UpdateAvailable: true,
			}, nil
		},
	}, "v0.2.5", ManagerOptions{})

	manager.MarkUpgradeFailed(errors.New("restart daemon: boom"))
	manager.Refresh(context.Background())

	status := manager.Status()
	if got, want := status.LastError, "restart daemon: boom"; got != want {
		t.Fatalf("LastError = %q, want %q", got, want)
	}
}

func TestManagerRefreshSkipsLocalDevVersionWithoutError(t *testing.T) {
	now := time.Date(2026, 5, 6, 16, 0, 0, 0, time.UTC)
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, currentVersion string) (CheckResult, error) {
			if got, want := currentVersion, "dev"; got != want {
				t.Fatalf("currentVersion = %q, want %q", got, want)
			}
			return CheckResult{CurrentVersion: currentVersion}, nil
		},
	}, "dev", ManagerOptions{
		Now: func() time.Time { return now },
	})

	manager.Refresh(context.Background())

	status := manager.Status()
	if got, want := status.CurrentVersion, "dev"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if got := status.LatestVersion; got != "" {
		t.Fatalf("LatestVersion = %q, want empty", got)
	}
	if status.UpdateAvailable {
		t.Fatal("UpdateAvailable = true, want false")
	}
	if got := status.LastError; got != "" {
		t.Fatalf("LastError = %q, want empty", got)
	}
	if status.LastCheckedAt == nil || !status.LastCheckedAt.Equal(now) {
		t.Fatalf("LastCheckedAt = %v, want %v", status.LastCheckedAt, now)
	}
}

func TestManagerRefreshSkipsLocalVersionWithoutError(t *testing.T) {
	now := time.Date(2026, 5, 26, 16, 0, 0, 0, time.UTC)
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, currentVersion string) (CheckResult, error) {
			if got, want := currentVersion, "v0.3.5-test6+local"; got != want {
				t.Fatalf("currentVersion = %q, want %q", got, want)
			}
			return CheckResult{CurrentVersion: currentVersion}, nil
		},
	}, "v0.3.5-test6+local", ManagerOptions{
		Now: func() time.Time { return now },
	})

	manager.Refresh(context.Background())

	status := manager.Status()
	if got, want := status.CurrentVersion, "v0.3.5-test6+local"; got != want {
		t.Fatalf("CurrentVersion = %q, want %q", got, want)
	}
	if got := status.LatestVersion; got != "" {
		t.Fatalf("LatestVersion = %q, want empty", got)
	}
	if status.UpdateAvailable {
		t.Fatal("UpdateAvailable = true, want false")
	}
	if got := status.LastError; got != "" {
		t.Fatalf("LastError = %q, want empty", got)
	}
	if status.LastCheckedAt == nil || !status.LastCheckedAt.Equal(now) {
		t.Fatalf("LastCheckedAt = %v, want %v", status.LastCheckedAt, now)
	}
}

func TestManagerStartChecksImmediatelyAndPeriodically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := make(chan int, 2)
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, _ string) (CheckResult, error) {
			select {
			case calls <- 1:
			default:
			}
			return CheckResult{LatestVersion: "v0.2.5"}, nil
		},
	}, "v0.2.5", ManagerOptions{
		CheckInterval: 10 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		manager.Start(ctx)
		close(done)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-calls:
		case <-time.After(time.Second):
			t.Fatalf("check call %d was not observed", i+1)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not exit after context cancellation")
	}
}

func TestManagerRefreshNotifiesOnStatusChange(t *testing.T) {
	now := time.Date(2026, 5, 6, 15, 0, 0, 0, time.UTC)
	notifications := make(chan apitypes.UpgradeStatus, 1)
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, _ string) (CheckResult, error) {
			return CheckResult{
				CurrentVersion:  "v0.2.5",
				LatestVersion:   "v0.2.7",
				UpdateAvailable: true,
			}, nil
		},
	}, "v0.2.5", ManagerOptions{
		Now: func() time.Time { return now },
		OnStatusChange: func(status apitypes.UpgradeStatus) {
			notifications <- status
		},
	})

	manager.Refresh(context.Background())

	select {
	case status := <-notifications:
		if !status.UpdateAvailable {
			t.Fatal("UpdateAvailable = false, want true")
		}
		if got, want := status.LatestVersion, "v0.2.7"; got != want {
			t.Fatalf("LatestVersion = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("expected upgrade status notification")
	}
}

func TestManagerRefreshSkipsNotificationWhenOnlyCheckedAtChanges(t *testing.T) {
	call := 0
	notifications := make(chan apitypes.UpgradeStatus, 2)
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, _ string) (CheckResult, error) {
			call++
			return CheckResult{
				CurrentVersion:  "v0.2.5",
				LatestVersion:   "v0.2.7",
				UpdateAvailable: true,
			}, nil
		},
	}, "v0.2.5", ManagerOptions{
		Now: func() time.Time {
			return time.Date(2026, 5, 6, 15, call, 0, 0, time.UTC)
		},
		OnStatusChange: func(status apitypes.UpgradeStatus) {
			notifications <- status
		},
	})

	manager.Refresh(context.Background())
	manager.Refresh(context.Background())

	select {
	case <-notifications:
	case <-time.After(time.Second):
		t.Fatal("expected first upgrade status notification")
	}

	select {
	case status := <-notifications:
		t.Fatalf("unexpected second notification: %+v", status)
	default:
	}
}

func TestManagerMarkManualRestartRequired(t *testing.T) {
	notifications := make(chan apitypes.UpgradeStatus, 2)
	manager := NewManager(fakeChecker{
		check: func(_ context.Context, _ string) (CheckResult, error) {
			return CheckResult{}, nil
		},
	}, "v0.2.5", ManagerOptions{
		OnStatusChange: func(status apitypes.UpgradeStatus) {
			notifications <- status
		},
	})

	manager.MarkUpgrading()
	status := manager.MarkManualRestartRequired()

	if status.Upgrading {
		t.Fatal("Upgrading = true, want false")
	}
	if !status.ManualRestartRequired {
		t.Fatal("ManualRestartRequired = false, want true")
	}

	select {
	case <-notifications:
	case <-time.After(time.Second):
		t.Fatal("expected upgrading notification")
	}

	select {
	case got := <-notifications:
		if !got.ManualRestartRequired {
			t.Fatal("notification ManualRestartRequired = false, want true")
		}
	case <-time.After(time.Second):
		t.Fatal("expected manual restart notification")
	}
}
