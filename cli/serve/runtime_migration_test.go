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
			if test.rejectFirst {
				output, err := runMigration()
				if err == nil {
					t.Fatalf("migration accepted failed shutdown: %s", output)
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil || string(data) != original {
					t.Fatalf("failed shutdown discarded the starting sandbox record: %s, %v", data, readErr)
				}
				rejectStop.Store(false)
			}
			output, err := runMigration()
			if err != nil {
				t.Fatalf("migration startup failed (context=%v): %v\n%s", ctx.Err(), err, output)
			}
			if stopCalls.Load() == 0 || postStopReads.Load() < 2 {
				t.Fatalf("starting sandbox was not stopped and confirmed: stops=%d, post-stop reads=%d", stopCalls.Load(), postStopReads.Load())
			}
			if !stopped.Load() {
				t.Fatal("configured provider was not stopped")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var state struct {
				Agents struct {
					Items []struct {
						Runtime struct {
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
			if rt.Kind != "codex" || rt.State != "stopped" || rt.SandboxID != "" {
				t.Fatalf("migration result: %s", data)
			}
		})
	}
}
