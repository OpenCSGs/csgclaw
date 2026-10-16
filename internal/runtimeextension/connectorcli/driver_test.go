package connectorcli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestDriverKeepsResourceProjectionsIndependent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture")
	}
	home := t.TempDir()
	executable := filepath.Join(home, "fixture")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'fixture 2.0.0\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	driver := Driver{ResolveHome: func(string) (string, error) { return home, nil }, RuntimeLoaded: func(_ string, p agentruntime.ExtensionProjection) bool { return p.Name == "connector-two" }}
	ctx := context.Background()
	desired := func(resource string) agentruntime.ExtensionDesired {
		payload, _ := json.Marshal(Payload{AgentID: "worker", ResourceID: resource, Executable: executable, VersionArgs: []string{"--version"}, Environment: map[string]string{"FIXTURE_CONFIG": resource}})
		return agentruntime.ExtensionDesired{Name: "connector-" + resource, Kind: Kind, Generation: 1, SourceRevision: "1", Payload: payload}
	}
	for _, id := range []string{"one", "two"} {
		change, _, err := driver.PrepareExtension(ctx, "worker", desired(id))
		if err != nil {
			t.Fatal(err)
		}
		if err := change.Activate(ctx); err != nil {
			t.Fatal(err)
		}
		if err := change.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
	}
	store, err := driver.store("worker")
	if err != nil {
		t.Fatal(err)
	}
	change, err := store.Delete("connector-one")
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := change.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	observed, err := driver.ObserveExtension(ctx, "worker", desired("two"))
	if err != nil || !observed.RuntimeLoaded || observed.State != agentruntime.ExtensionStateConfigured {
		t.Fatalf("observed=%+v err=%v", observed, err)
	}
	if _, _, err := driver.PrepareExtension(ctx, "another-worker", desired("two")); err == nil {
		t.Fatal("accepted another worker's configuration")
	}
}
