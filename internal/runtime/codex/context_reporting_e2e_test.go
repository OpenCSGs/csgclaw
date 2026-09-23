package codex

import (
	"bufio"
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

	"csgclaw/internal/codexmodel"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
)

// Launch Codex directly, without the CSGClaw runtime, usage adapter or LLM proxy.
// Capture its raw notifications before comparing them with our usage projection.
func TestNativeCodexContextReportingE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, _ = filepath.Abs(binary)
	for _, scenario := range []struct {
		name  string
		input int64
		turns int
	}{
		{"compaction_boundary", 150000, 1},
		{"reported_overflow", 1730000, 1},
		{"cumulative_is_not_occupancy", 60000, 5},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			window := int64(200000)
			if scenario.name == "reported_overflow" {
				window = 1000000
			}
			var calls atomic.Int64
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/responses" {
					http.NotFound(w, r)
					return
				}
				n := calls.Add(1)
				tokens := scenario.input
				if scenario.name == "reported_overflow" && n > 1 {
					tokens = 1000
				}
				w.Header().Set("Content-Type", "text/event-stream")
				emit := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b) }
				emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprint(n)}})
				emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprint(n), "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
				emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprint(n), "usage": map[string]any{"input_tokens": tokens, "output_tokens": 20, "total_tokens": tokens + 20}}})
			}))
			defer endpoint.Close()
			home := t.TempDir()
			profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-context", BaseURL: endpoint.URL + "/v1", ModelMetadata: modelcap.Resolved{ContextWindow: window, ContextSource: "user"}}
			catalog, _ := json.Marshal(codexmodel.Catalog(codexmodel.Profile{ModelID: profile.ModelID, Metadata: profile.ModelMetadata}))
			if err := os.WriteFile(filepath.Join(home, modelCatalogFileName), catalog, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(buildProviderConfigBlock(profile)), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "app-server")
			cmd.Dir = home
			for _, e := range os.Environ() {
				if !strings.HasPrefix(e, "CODEX_HOME=") {
					cmd.Env = append(cmd.Env, e)
				}
			}
			cmd.Env = append(cmd.Env, "CODEX_HOME="+home)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stderr, err := os.Create(filepath.Join(home, "stderr.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			cmd.Stderr = stderr
			client := newAppServerClient(stdin, nil)
			notes := make(chan appServerNotification, 128)
			client.onNotification = func(n appServerNotification) {
				select {
				case notes <- n:
				case <-ctx.Done():
				}
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			go func() {
				scanner := bufio.NewScanner(stdout)
				scanner.Buffer(make([]byte, 4096), 4<<20)
				for scanner.Scan() {
					client.handleLine(scanner.Text())
				}
			}()
			request := func(method string, params any) json.RawMessage {
				t.Helper()
				result, err := client.request(ctx, method, params)
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			request("initialize", map[string]any{"clientInfo": map[string]any{"name": "context-reporting-test", "version": "1"}, "capabilities": map[string]any{"experimentalApi": true}})
			client.notify("initialized")
			raw := request("thread/start", map[string]any{"model": profile.ModelID, "cwd": home, "approvalPolicy": "never"})
			thread, err := appServerThreadIDFromResult(raw)
			if err != nil {
				t.Fatal(err)
			}
			manager := newAppServerManager(managerDeps{EventSink: &recordingSink{}})
			live := &liveSession{spec: SessionSpec{Profile: profile}}
			var nativeLast, nativeTotal, nativeWindow int64
			sawExpected := false
			for i := 0; i < scenario.turns; i++ {
				result := request("turn/start", map[string]any{"threadId": thread, "input": []any{map[string]any{"type": "text", "text": "Reply done"}}})
				turn := appServerTurnIDFromResult(result)
			waitTurn:
				for {
					select {
					case n := <-notes:
						var params map[string]any
						if err := json.Unmarshal(n.Params, &params); err != nil {
							t.Fatal(err)
						}
						if n.Method == "thread/tokenUsage/updated" {
							usage := params["tokenUsage"].(map[string]any)
							nativeLast, _ = contextTokenNumber(usage["last"].(map[string]any)["totalTokens"])
							nativeTotal, _ = contextTokenNumber(usage["total"].(map[string]any)["totalTokens"])
							nativeWindow, _ = contextTokenNumber(usage["modelContextWindow"])
							manager.publishContextUsage("isolated", live, thread, params, n.Method)
							projected := live.contextUsage[thread]
							if projected.UsedTokens == nil || *projected.UsedTokens != nativeLast {
								t.Fatalf("adapter changed native usage: %+v, native=%d", projected, nativeLast)
							}
							if projected.ContextWindow != window {
								t.Fatalf("display capacity=%d, configured=%d, native usable=%d", projected.ContextWindow, window, nativeWindow)
							}
							if nativeLast == scenario.input+20 {
								sawExpected = true
								t.Logf("upstream=%d native_last=%d native_total=%d native_usable=%d displayed_capacity=%d displayed_percent=%.2f", scenario.input+20, nativeLast, nativeTotal, nativeWindow, projected.ContextWindow, float64(nativeLast)*100/float64(projected.ContextWindow))
							}
						}
						if n.Method == "turn/completed" && appServerNotificationTurnID(params) == turn {
							break waitTurn
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
			}
			if !sawExpected {
				t.Fatal("expected provider usage missing from native notification")
			}
			if nativeWindow != window*95/100 {
				t.Fatalf("native usable window=%d", nativeWindow)
			}
			if scenario.name == "cumulative_is_not_occupancy" && (nativeTotal <= window || nativeLast >= window) {
				t.Fatalf("last=%d total=%d", nativeLast, nativeTotal)
			}
		})
	}
}
