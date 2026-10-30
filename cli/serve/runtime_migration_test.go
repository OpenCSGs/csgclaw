package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetiredSandboxMigrationUsesConfiguredProviderOutsideStateLock(t *testing.T) {
	const helperEnv = "CSGCLAW_TEST_RETIRED_SANDBOX_MIGRATION"
	if os.Getenv(helperEnv) == "1" {
		if err := migrateLocalStore(context.Background(), os.Getenv("CSGCLAW_TEST_MIGRATION_CONFIG")); err != nil {
			t.Fatal(err)
		}
		return
	}

	for _, test := range []struct {
		name, initial string
		rejectFirst   bool
	}{
		{name: "running", initial: "running"},
		{name: "starting", initial: "starting"},
		{name: "deploying", initial: "deploying"},
		{name: "creating", initial: "creating"},
		{name: "pending", initial: "pending"},
		{name: "created", initial: "created"},
		{name: "configured", initial: "configured"},
		{name: "starting-stop-failure", initial: "starting", rejectFirst: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".csgclaw")
			path := filepath.Join(root, "state.json")
			if err := os.MkdirAll(root, 0755); err != nil {
				t.Fatal(err)
			}
			original := `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"openclaw_sandbox","state":"running","sandbox_id":"old-box"}}]}}`
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			var stopped, rejectStop atomic.Bool
			var stopCalls, postStopReads atomic.Int32
			rejectStop.Store(test.rejectFirst)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/old-box/status/stop":
					data, err := os.ReadFile(path)
					if err != nil || string(data) != original {
						t.Error("Agent overwritten before provider shutdown")
					}
					stopCalls.Add(1)
					if rejectStop.Load() {
						http.Error(w, "fixture stop rejected", http.StatusInternalServerError)
						return
					}
					stopped.Store(true)
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/old-box":
					state := test.initial
					if stopped.Load() && postStopReads.Add(1) >= 2 {
						state = "stopped"
					}
					fmt.Fprintf(w, `{"spec":{"sandbox_name":"old-box"},"state":{"status":%q}}`, state)
				default:
					t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			configPath := filepath.Join(home, "custom-config.toml")
			if err := os.WriteFile(configPath, []byte("[server]\nlisten_addr=\"127.0.0.1:18080\"\n[sandbox]\nprovider=\"csghub\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// A child process makes a lock re-entry regression time out without wedging
			// the parent test process's shared root-state lock.
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			values := map[string]string{"HOME": home, helperEnv: "1", "CSGCLAW_TEST_MIGRATION_CONFIG": configPath, "CSGHUB_API_BASE_URL": server.URL, "CSGHUB_USER_TOKEN": "fixture-only", "CSGCLAW_NAME": "", "CSGCLAW_RESOURCE_ID": "", "CSGCLAW_SANDBOX_PORT": "", "CSGCLAW_SANDBOX_TIMEOUT": "", "CSGCLAW_SANDBOX_POLL_INTERVAL": "500ms"}
			runMigration := func() ([]byte, error) {
				cmd := exec.CommandContext(ctx, binary, "-test.run=^TestRetiredSandboxMigrationUsesConfiguredProviderOutsideStateLock$")
				for _, item := range os.Environ() {
					key, _, _ := strings.Cut(item, "=")
					if _, override := values[key]; !override {
						cmd.Env = append(cmd.Env, item)
					}
				}
				for key, value := range values {
					cmd.Env = append(cmd.Env, key+"="+value)
				}
				return cmd.CombinedOutput()
			}
			output, err := runMigration()
			if err != nil {
				t.Fatalf("migration startup failed (context=%v): %v\n%s", ctx.Err(), err, output)
			}
			if test.rejectFirst {
				if !strings.Contains(string(output), "forcing Agent migration to Codex") {
					t.Fatalf("missing forced migration warning: %s", output)
				}
			} else if stopCalls.Load() == 0 || postStopReads.Load() < 2 || !stopped.Load() {
				t.Fatalf("starting sandbox was not stopped and confirmed: stops=%d, post-stop reads=%d", stopCalls.Load(), postStopReads.Load())
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var state struct {
				Agents struct {
					Items []struct {
						DesiredState string `json:"desired_state"`
						Runtime      struct {
							Kind, State string
							SandboxID   string `json:"sandbox_id"`
						}
					}
				}
			}
			if err := json.Unmarshal(data, &state); err != nil {
				t.Fatal(err)
			}
			rt := state.Agents.Items[0].Runtime
			if rt.Kind != "codex" || rt.State != "stopped" || rt.SandboxID != "" || state.Agents.Items[0].DesiredState != "running" {
				t.Fatalf("migration result: %s", data)
			}
		})
	}
}

func TestConfigMigrationFailureKeepsLoadedConfiguration(t *testing.T) {
	for _, stage := range []string{"backup", "rewrite"} {
		t.Run(stage, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			root := filepath.Join(home, ".csgclaw")
			if err := os.MkdirAll(root, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "config.toml")
			original := "[server]\nlisten_addr=\"127.0.0.1:19091\"\n[bootstrap]\ndefault_worker_template=\"builtin.openclaw-worker\"\nruntime_kind=\"picoclaw_sandbox\"\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if stage == "backup" {
				if err := os.Chmod(root, 0555); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(root, 0755) })
				if err := os.WriteFile(filepath.Join(root, "probe"), nil, 0600); err == nil {
					t.Skip("filesystem does not enforce directory permissions")
				}
			} else {
				if err := os.Chmod(path, 0400); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(path, 0600) })
				if file, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
					file.Close()
					t.Skip("filesystem does not enforce file permissions")
				}
			}
			cfg, err := loadConfig(path)
			if err != nil {
				t.Fatalf("migration %s blocked usable configuration: %v", stage, err)
			}
			if cfg.Server.ListenAddr != "127.0.0.1:19091" || cfg.Bootstrap.ResolvedDefaultWorkerTemplate() != "builtin.codex-worker" {
				t.Fatalf("loaded config lost normalization: %+v", cfg.Bootstrap)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != original {
				t.Fatal("failed rewrite changed source configuration")
			}
		})
	}
}
