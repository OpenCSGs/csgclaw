package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"

	toml "github.com/pelletier/go-toml/v2"
)

func TestMemoryPipelineConfigOverridesHostDefaults(t *testing.T) {
	for _, initial := range []string{
		"memories.version = \"v2\"\nmemories.dual_write = true\nmemories.dedicated_tools = true\n",
		"[memories]\nversion = \"v2\"\ndual_write = true\ndedicated_tools = true\n",
		`[memories]
"version" = "v2"
'dual_write' = true
"dedicated_tools" = true
`,
		`"memories".'version' = "v2"
'memories'."dual_write" = true
memories."dedicated_tools" = true
`,
		`["memories"]
"ver\u0073ion" = "v2"
'dual_write' = true
"dedicated_tools" = true
`,
		`[memories]
"version" = """
v2"""
`,
		"memories.version = '''\nv2'''\n",
		"memories.version = \"\"\"\r\nv2\"\"\"",
	} {
		config := configureCodexHomeConfig(initial, agentruntime.Profile{}, nil)
		var parsed struct {
			Memories struct {
				Version        string `toml:"version"`
				DualWrite      bool   `toml:"dual_write"`
				DedicatedTools bool   `toml:"dedicated_tools"`
			}
		}
		if err := toml.Unmarshal([]byte(config), &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed.Memories.Version != "v1" || parsed.Memories.DualWrite || parsed.Memories.DedicatedTools {
			t.Fatalf("unexpected memory pipeline: %+v", parsed.Memories)
		}
		if next := configureCodexHomeConfig(config, agentruntime.Profile{}, nil); next != config {
			t.Fatal("memory pipeline configuration is not idempotent")
		}
	}
}

func TestStripMemoryDirectivesPreservesUnmanagedContent(t *testing.T) {
	input := `# Keep host configuration and literal dotted names.
"memories.version" = "literal"
['features']
'memories' = false
"other" = true
["memories"]
"ver\u0073ion" = "v2" # managed
'use_memories' = false
# Keep the following host setting.
"custom" = "keep"
[project]
"version" = "keep"
text = """
[memories]
version = "example"
"""
`
	if err := toml.Unmarshal([]byte(input), new(map[string]any)); err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(input, `"ver\u0073ion" = "v2" # managed`+"\n", "")
	want = strings.ReplaceAll(want, "['features']", "[features]")
	want = strings.ReplaceAll(want, "'memories' = false\n", "")
	want = strings.ReplaceAll(want, `["memories"]`, "[memories]")
	want = strings.ReplaceAll(want, "'use_memories' = false\n", "")
	if got := stripUserMemoryDirectives(input); got != want {
		t.Fatalf("normalized config = %s, want %s", got, want)
	}
}

// Exercise the real runtime so a changed Codex default cannot silently split
// new and existing agents between different memory stores.
func TestMemoryPipelineBundledCodexE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, _ = filepath.Abs(binary)
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("CODEX_HOME", filepath.Join(root, "host"))
			home := filepath.Join(root, "agent")
			memoryDir := filepath.Join(home, hostStateDirName, homeDirName, "memories")
			if existing {
				if err := os.MkdirAll(memoryDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(memoryDir, readableMemoryFileName), []byte("EXISTING_MEMORY_SENTINEL\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			host := filepath.Join(root, "host")
			if err := os.MkdirAll(host, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(host, "config.toml"), []byte("[memories]\n\"version\" = \"\"\"\nv2\"\"\"\n'dual_write' = true\n\"dedicated_tools\" = true\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var activeThread atomic.Value
			var sawConversation atomic.Bool
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				input, _ := json.Marshal(payload["input"])
				var metadata struct {
					ThreadID string `json:"thread_id"`
				}
				_ = json.Unmarshal([]byte(r.Header.Get("X-Codex-Turn-Metadata")), &metadata)
				if thread, ok := activeThread.Load().(string); ok && metadata.ThreadID == thread {
					sawConversation.Store(true)
					if existing && !strings.Contains(string(input), "EXISTING_MEMORY_SENTINEL") {
						t.Error("existing memory was not injected into the conversation")
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, event := range []any{
					map[string]any{"type": "response.created", "response": map[string]any{"id": "memory"}},
					map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": "answer", "content": []any{map[string]any{"type": "output_text", "text": "OK"}}}},
					map[string]any{"type": "response.completed", "response": map[string]any{"id": "memory", "usage": map[string]any{"input_tokens": 100, "output_tokens": 1, "total_tokens": 101}}},
				} {
					raw, _ := json.Marshal(event)
					_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
				}
			}))
			defer endpoint.Close()
			profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-memory", BaseURL: endpoint.URL + "/v1", APIKey: "fixture", ModelMetadata: modelcap.Resolved{ContextWindow: 32768, ContextSource: "provider"}}
			rt := New(Dependencies{
				BinaryProvider: fakeBinaryProvider{path: binary}, EventSink: NewEventSink(),
				AgentHome: func(string) (string, error) { return home, nil },
				ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
					return AgentRef{ID: "alice", Name: "alice", RuntimeID: h.RuntimeID, Profile: profile}, nil
				},
			})
			defer rt.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:memory")
			if err != nil {
				t.Fatal(err)
			}
			activeThread.Store(thread)
			if err := rt.Prompt(ctx, handle.RuntimeID, thread, "Reply OK."); err != nil {
				t.Fatal(err)
			}
			if !sawConversation.Load() {
				t.Fatal("conversation did not reach the inference fixture")
			}
			if _, err := os.Stat(memoryDir); err != nil {
				t.Fatalf("memory directory was not created: %v", err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(memoryDir), "memories_v2")); !os.IsNotExist(err) {
				t.Fatalf("unexpected second memory pipeline: %v", err)
			}
			if existing {
				if err := rt.Delete(ctx, handle); err != nil {
					metadata, _ := rt.readRuntimeMetadata(handle.RuntimeID)
					t.Fatalf("delete runtime pid=%d: %v", metadata.ProcessID, err)
				}
				memory, err := rt.ReadMemoryDocument(ctx, home, nil)
				if err != nil || !memory.Ready || memory.Content != "EXISTING_MEMORY_SENTINEL\n" {
					t.Fatalf("memory after recreation: %+v, %v", memory, err)
				}
			}
		})
	}
}
