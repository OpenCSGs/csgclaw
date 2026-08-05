package runtimecatalog

import (
	"context"
	"errors"
	"testing"
)

type fakeCodexResolver struct {
	path string
	err  error
}

func (f fakeCodexResolver) Ensure(context.Context) (string, error) {
	return f.path, f.err
}

func TestServiceListReportsBundledCodex(t *testing.T) {
	service := NewService(
		WithCodexResolver(fakeCodexResolver{path: "/opt/csgclaw/bin/codex"}),
		WithPlatform("darwin", "arm64"),
	)

	runtimes := service.List()
	if len(runtimes) != 2 {
		t.Fatalf("List() length = %d, want 2: %+v", len(runtimes), runtimes)
	}
	if got := runtimes[0]; got.Name != RuntimeCodex || !got.Supported || !got.Installed || got.Installable || got.Status != "installed" || got.Path != "/opt/csgclaw/bin/codex" {
		t.Fatalf("Codex runtime = %+v, want installed bundled Codex", got)
	}
	if got := runtimes[1]; got.Name != RuntimeClaudeCode || got.Supported || got.Installed || got.Installable || got.Status != StatusComingSoon {
		t.Fatalf("Claude Code runtime = %+v, want coming soon", got)
	}
	for _, got := range runtimes {
		if got.OS != "darwin" || got.Arch != "arm64" {
			t.Fatalf("runtime platform = %s/%s, want darwin/arm64: %+v", got.OS, got.Arch, got)
		}
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
