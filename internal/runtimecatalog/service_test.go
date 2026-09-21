package runtimecatalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/dshcli"
)

type fakeCodexResolver struct {
	path string
	err  error
}

type fakeDSHResolver struct {
	path    string
	version string
	err     error
}

func (f fakeDSHResolver) Ensure(context.Context) (string, error) { return f.path, nil }

func (f fakeDSHResolver) Resolve(context.Context) (dshcli.Info, error) {
	return dshcli.Info{Path: f.path, Version: f.version}, f.err
}

type mutableDSHResolver struct {
	info dshcli.Info
	err  error
}

func (f *mutableDSHResolver) Ensure(context.Context) (string, error)       { return f.info.Path, f.err }
func (f *mutableDSHResolver) Resolve(context.Context) (dshcli.Info, error) { return f.info, f.err }

type fakeDSHInstaller struct {
	install func() dshcli.Info
	err     error
}

type progressDSHInstaller struct {
	resolver *mutableDSHResolver
	started  chan struct{}
	release  chan struct{}
}

func (i progressDSHInstaller) Install(context.Context) (dshcli.Info, error) {
	return dshcli.Info{}, errors.New("InstallWithProgress should be used")
}

func (i progressDSHInstaller) InstallWithProgress(
	_ context.Context,
	report dshcli.ProgressReporter,
) (dshcli.Info, error) {
	report(dshcli.InstallProgress{Stage: dshcli.InstallStageInstalling, ActivityCount: 42})
	close(i.started)
	<-i.release
	i.resolver.err = nil
	i.resolver.info = dshcli.Info{Path: "/home/test/.local/bin/dsh", Version: "0.1.5-rc.2"}
	report(dshcli.InstallProgress{Stage: dshcli.InstallStageCompleted, ActivityCount: 42})
	return i.resolver.info, nil
}

func (f fakeDSHInstaller) Install(context.Context) (dshcli.Info, error) {
	if f.err != nil {
		return dshcli.Info{}, f.err
	}
	return f.install(), nil
}

func (f fakeCodexResolver) Ensure(context.Context) (string, error) {
	return f.path, f.err
}

func TestServiceListReportsBundledCodex(t *testing.T) {
	service := NewService(
		WithCodexResolver(fakeCodexResolver{path: "/opt/csgclaw/bin/codex"}),
		WithDSHResolver(fakeDSHResolver{path: "/usr/local/bin/dsh", version: "0.1.6-alpha.2"}),
		WithPlatform("darwin", "arm64"),
	)

	runtimes := service.List()
	if len(runtimes) != 3 {
		t.Fatalf("List() length = %d, want 3: %+v", len(runtimes), runtimes)
	}
	if got := runtimes[0]; got.Name != RuntimeCodex || !got.Supported || !got.Installed || got.Installable || got.Status != "installed" || got.Path != "/opt/csgclaw/bin/codex" {
		t.Fatalf("Codex runtime = %+v, want installed bundled Codex", got)
	}
	if got := runtimes[1]; got.Name != RuntimeDSH || !got.Supported || !got.Installed || !got.Installable || got.Path != "/usr/local/bin/dsh" || got.Version != "0.1.6-alpha.2" {
		t.Fatalf("DSH runtime = %+v, want installed external DSH", got)
	}
	if got := runtimes[2]; got.Name != RuntimeClaudeCode || got.Supported || got.Installed || got.Installable || got.Status != StatusComingSoon {
		t.Fatalf("Claude Code runtime = %+v, want coming soon", got)
	}
	for _, got := range runtimes {
		if got.OS != "darwin" || got.Arch != "arm64" {
			t.Fatalf("runtime platform = %s/%s, want darwin/arm64: %+v", got.OS, got.Arch, got)
		}
	}
}

func TestServiceInstallDSHRefreshesRuntime(t *testing.T) {
	resolver := &mutableDSHResolver{err: errors.New("missing")}
	service := NewService(
		WithDSHResolver(resolver),
		WithDSHInstaller(fakeDSHInstaller{install: func() dshcli.Info {
			resolver.err = nil
			resolver.info = dshcli.Info{Path: "/home/test/.local/bin/dsh", Version: "0.1.5-rc.2"}
			return resolver.info
		}}),
	)

	got, err := service.Install(context.Background(), RuntimeDSH)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !got.Installed || got.Status != "installed" || got.Path != resolver.info.Path || got.Version != resolver.info.Version {
		t.Fatalf("Install() = %+v, want installed DSH", got)
	}
}

func TestServiceStartInstallReportsLiveProgress(t *testing.T) {
	resolver := &mutableDSHResolver{err: errors.New("missing")}
	started := make(chan struct{})
	release := make(chan struct{})
	service := NewService(
		WithDSHResolver(resolver),
		WithDSHInstaller(progressDSHInstaller{resolver: resolver, started: started, release: release}),
	)

	installation, err := service.StartInstall(RuntimeDSH)
	if err != nil {
		t.Fatalf("StartInstall() error = %v", err)
	}
	if installation.Status != InstallStatusRunning || installation.Stage != dshcli.InstallStageChecking {
		t.Fatalf("StartInstall() = %+v, want checking installation", installation)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("installer did not start")
	}
	installation, err = service.Installation(RuntimeDSH)
	if err != nil {
		t.Fatalf("Installation() error = %v", err)
	}
	if installation.Stage != dshcli.InstallStageInstalling || installation.ActivityCount != 42 {
		t.Fatalf("Installation() = %+v, want live npm progress", installation)
	}
	close(release)

	deadline := time.Now().Add(time.Second)
	for installation.Status == InstallStatusRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		installation, err = service.Installation(RuntimeDSH)
		if err != nil {
			t.Fatal(err)
		}
	}
	if installation.Status != InstallStatusSucceeded || installation.Runtime == nil || !installation.Runtime.Installed {
		t.Fatalf("Installation() = %+v, want completed runtime", installation)
	}
}

func TestServiceReportsMissingDSHAsInstallable(t *testing.T) {
	service := NewService(WithDSHResolver(fakeDSHResolver{err: errors.New("missing")}))
	got := service.List()[1]
	if got.Installed || !got.Installable || got.Status != "missing" || got.MessageCode != "dsh_not_installed" {
		t.Fatalf("DSH runtime = %+v, want installable missing runtime", got)
	}
}

func TestServiceListReportsMissingBundledCodex(t *testing.T) {
	service := NewService(WithCodexResolver(fakeCodexResolver{err: errors.New("bundle missing")}))

	got := service.List()[0]
	if got.Name != RuntimeCodex || got.Installed || got.Installable || got.Status != "failed" {
		t.Fatalf("Codex runtime = %+v, want failed non-installable bundled runtime", got)
	}
	if got.Message == "" {
		t.Fatal("Codex missing bundle message is empty")
	}
}
