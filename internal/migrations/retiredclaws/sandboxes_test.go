package retiredclaws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/localstore"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
)

func TestMigrationStopsProviderSandboxBeforeReadingWorkspace(t *testing.T) {
	for _, kind := range []string{"openclaw_sandbox", "picoclaw_sandbox"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, localstore.RootStateFileName)
			// Even a saved stopped state must be checked against the provider.
			original := `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"` + kind + `","state":"stopped","sandbox_id":"old-box"}}]}}`
			writeFile(t, path, original)
			workspace := filepath.Join(root, "agents", "agent-old", "."+retiredRuntimeName(kind), "workspace")
			filename := "AGENTS.md"
			if strings.HasPrefix(kind, "picoclaw") {
				filename = "AGENT.md"
			}
			instructions := filepath.Join(workspace, filename)
			writeFile(t, instructions, "Still changing")
			instance := sandboxtest.NewInstance(sandbox.Info{ID: "old-box", State: sandbox.StateRunning})
			instance.StopFunc = func(_ context.Context, opts sandbox.StopOptions) error {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != original {
					t.Fatal("record overwritten before sandbox stopped")
				}
				if opts.Force || opts.Timeout != 0 {
					t.Fatalf("stop options: %+v", opts)
				}
				writeFile(t, instructions, "Final instructions after shutdown")
				instance.InfoValue.State = sandbox.StateExited
				return nil
			}
			runtime := sandboxtest.NewRuntime()
			runtime.Instances["old-box"] = instance
			opens := 0
			open := func(context.Context, string) (sandbox.Runtime, error) { opens++; return runtime, nil }
			if err := MigrateAgents(context.Background(), root, open); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if instance.InfoValue.State != sandbox.StateExited || !bytes.Contains(data, []byte("Final instructions after shutdown")) || bytes.Contains(data, []byte("sandbox_id")) {
				t.Fatalf("unsafe migration: %s", data)
			}
			if err := MigrateAgents(context.Background(), root, open); err != nil {
				t.Fatal(err)
			}
			if opens != 1 {
				t.Fatalf("migrated sandbox reopened: %d", opens)
			}
		})
	}
}

func TestMigrationContinuesWhenOneSandboxCannotBeStopped(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, localstore.RootStateFileName)
	original := `{"version":1,"agents":{"items":[{"id":"agent-first","runtime":{"kind":"openclaw_sandbox","sandbox_id":"first"}},{"id":"agent-second","runtime":{"kind":"picoclaw_sandbox","sandbox_id":"second"}}]}}`
	writeFile(t, path, original)
	runtime := sandboxtest.NewRuntime()
	first := sandboxtest.NewInstance(sandbox.Info{ID: "first", State: sandbox.StateRunning})
	second := sandboxtest.NewInstance(sandbox.Info{ID: "second", State: sandbox.StateRunning})
	runtime.Instances["first"] = first
	runtime.Instances["second"] = second
	second.StopFunc = func(context.Context, sandbox.StopOptions) error { return errors.New("stop refused") }
	open := func(context.Context, string) (sandbox.Runtime, error) { return runtime, nil }
	if err := MigrateAgents(context.Background(), root, open); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.Contains(data, []byte("sandbox_id")) || bytes.Count(data, []byte(`"kind": "codex"`)) != 2 {
		t.Fatalf("Agents did not migrate: %s, %v", data, err)
	}
	if first.InfoValue.State != sandbox.StateStopped || second.InfoValue.State != sandbox.StateRunning {
		t.Fatal("fixture did not preserve the unresponsive sandbox")
	}
	backups, _ := filepath.Glob(root + "_backup_*")
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	backup, _ := os.ReadFile(filepath.Join(backups[0], localstore.RootStateFileName))
	if string(backup) != original {
		t.Fatal("original sandbox records were not backed up")
	}
}

func TestMigrationForcesConversionWhenSandboxDependenciesFail(t *testing.T) {
	for _, failure := range []string{"unavailable", "inspect", "stop", "shutdown timeout", "active without id", "starting without id", "no provider"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, localstore.RootStateFileName)
			runtime := sandboxtest.NewRuntime()
			instance := sandboxtest.NewInstance(sandbox.Info{ID: "old-box", State: sandbox.StateRunning})
			runtime.Instances["old-box"] = instance
			original := `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"openclaw_sandbox","state":"running","sandbox_id":"old-box"},"desired_state":"running","model_config":{"model_provider_id":"fixture","model_id":"fixture-model"}}]}}`
			open := func(context.Context, string) (sandbox.Runtime, error) { return runtime, nil }
			switch failure {
			case "unavailable":
				open = func(context.Context, string) (sandbox.Runtime, error) { return nil, sandbox.ErrUnavailable }
			case "inspect":
				instance.InfoFunc = func(context.Context) (sandbox.Info, error) { return sandbox.Info{}, errors.New("inspect failed") }
			case "stop":
				instance.StopFunc = func(context.Context, sandbox.StopOptions) error { return errors.New("stop failed") }
			case "shutdown timeout":
				instance.StopFunc = func(context.Context, sandbox.StopOptions) error {
					instance.InfoFunc = func(context.Context) (sandbox.Info, error) { return sandbox.Info{}, context.DeadlineExceeded }
					return nil
				}
			case "active without id", "starting without id":
				original = strings.ReplaceAll(original, `,"sandbox_id":"old-box"`, ``)
				if failure == "starting without id" {
					original = strings.ReplaceAll(original, `"state":"running"`, `"state":"starting"`)
				}
			case "no provider":
				open = nil
			}
			writeFile(t, path, original)
			if err := backupOnce(root); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, "new-notes.txt"), "user data created after the earlier backup")
			if err := MigrateAgents(context.Background(), root, open); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			for _, want := range []string{`"kind": "codex"`, `"state": "stopped"`, `"desired_state": "running"`, `"model_id": "fixture-model"`} {
				if !bytes.Contains(data, []byte(want)) {
					t.Fatalf("missing %s: %s", want, data)
				}
			}
			backups, _ := filepath.Glob(root + "_backup_*")
			if len(backups) != 2 {
				t.Fatalf("forced migration did not take a fresh snapshot: %v", backups)
			}
			fresh := false
			for _, backup := range backups {
				if _, err := os.Stat(filepath.Join(backup, "new-notes.txt")); err == nil {
					before, _ := os.ReadFile(filepath.Join(backup, localstore.RootStateFileName))
					fresh = string(before) == original
				}
			}
			if !fresh {
				t.Fatal("current user data and original runtime handles were not preserved")
			}
			if err := MigrateAgents(context.Background(), root, open); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(path)
			backups, _ = filepath.Glob(root + "_backup_*")
			if !bytes.Equal(data, after) || len(backups) != 2 {
				t.Fatal("completed forced migration was not idempotent")
			}
		})
	}
}

func TestCanceledMigrationDoesNotForceConversion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, localstore.RootStateFileName)
	original := `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"openclaw_sandbox","state":"running","sandbox_id":"old-box"}}]}}`
	writeFile(t, path, original)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := MigrateAgents(ctx, root, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled migration returned %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("canceled migration rewrote original records")
	}
}

func TestRetiredSandboxHandlesIncludeLegacyFieldsAndIgnoreOnlyConfirmedMissing(t *testing.T) {
	for _, state := range []sandbox.State{sandbox.StateCreated, sandbox.StateStarting, sandbox.StateStopped, sandbox.StateExited, sandbox.StateDead, sandbox.StateRunning, sandbox.StatePaused, sandbox.StateRestarting, sandbox.StateRemoving} {
		t.Run(string(state), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, localstore.RootStateFileName)
			writeFile(t, path, `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"picoclaw_sandbox","sandbox_id":"nested"},"box_id":"legacy","sandbox_id":"missing"}]}}`)
			runtime := sandboxtest.NewRuntime()
			for _, id := range []string{"nested", "legacy"} {
				runtime.Instances[id] = sandboxtest.NewInstance(sandbox.Info{ID: id, State: state})
			}
			if err := MigrateAgents(context.Background(), root, func(context.Context, string) (sandbox.Runtime, error) { return runtime, nil }); err != nil {
				t.Fatal(err)
			}
			for _, instance := range runtime.Instances {
				if state == sandbox.StateCreated && len(instance.StopCalls) != 0 {
					t.Fatal("inactive created instance was stopped unnecessarily")
				}
				if state == sandbox.StateStarting && len(instance.StopCalls) != 1 {
					t.Fatal("starting instance was not stopped")
				}
				if !retiredSandboxStopped(instance.InfoValue.State) {
					t.Fatal("sandbox left active")
				}
			}
			data, _ := os.ReadFile(path)
			var decoded any
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("sandbox_id")) || bytes.Contains(data, []byte("box_id")) {
				t.Fatalf("retired handle retained: %s", data)
			}
		})
	}
}

func TestRetiredSandboxCanDisappearWhenStopped(t *testing.T) {
	runtime := sandboxtest.NewRuntime()
	instance := sandboxtest.NewInstance(sandbox.Info{State: sandbox.StateRunning})
	runtime.Instances["auto-remove"] = instance
	instance.StopFunc = func(context.Context, sandbox.StopOptions) error {
		instance.InfoFunc = func(context.Context) (sandbox.Info, error) { return sandbox.Info{}, sandbox.ErrNotFound }
		return nil
	}
	if err := stopRetiredSandbox(context.Background(), runtime, "auto-remove"); err != nil {
		t.Fatal(err)
	}
}
