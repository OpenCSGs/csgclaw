package llm

import (
	"context"
	"csgclaw/internal/activity"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/modelpreset"
	"csgclaw/internal/modelusage"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type nativeProtocolBinary string

func (b nativeProtocolBinary) Ensure(context.Context) (string, error) { return string(b), nil }

type switchingProtocolProfile struct {
	mu sync.RWMutex
	p  agent.AgentProfile
}

func (p *switchingProtocolProfile) ResolvedAgentProfile(string) (agent.AgentProfile, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.p, nil
}

// This opt-in test runs the bundled Codex process through the actual LLM bridge
// and changes the upstream wire protocol between turns in the same native thread.
func TestBundledCodexProtocolSwitchE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, _ = filepath.Abs(binary)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	var mu sync.Mutex
	counts := map[string]int{}
	histories := map[string]string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			http.Error(w, "bad body", 400)
			return
		}
		serialized, _ := json.Marshal(payload)
		mu.Lock()
		counts[r.URL.Path]++
		histories[r.URL.Path] = string(serialized)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		text := "MODEL_PROTOCOL_SENTINEL done"
		switch r.URL.Path {
		case "/v1/chat/completions":
			fmt.Fprintf(w, "data: %s\n\n", `{"id":"chat-native","object":"chat.completion.chunk","model":"fixture-model","choices":[{"index":0,"delta":{"role":"assistant","content":"`+text+`"},"finish_reason":null}]}`)
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", `{"id":"chat-native","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}`)
		case "/v1/messages":
			for _, event := range []string{`{"type":"message_start","message":{"id":"msg-native","type":"message","role":"assistant","model":"fixture-model","content":[],"usage":{"input_tokens":100,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + text + `"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}`, `{"type":"message_stop"}`} {
				fmt.Fprintf(w, "data: %s\n\n", event)
			}
		case "/v1/responses":
			for _, event := range []string{`{"type":"response.created","response":{"id":"response-native"}}`, `{"type":"response.output_item.done","item":{"type":"message","role":"assistant","id":"message-native","content":[{"type":"output_text","text":"` + text + `"}]}}`, `{"type":"response.completed","response":{"id":"response-native","status":"completed","usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110}}}`} {
				fmt.Fprintf(w, "data: %s\n\n", event)
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	reader := &switchingProtocolProfile{p: agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "fixture-model", BaseURL: upstream.URL + "/v1", APIKey: "fixture-key", ProfileComplete: true}}
	svc := NewService(config.ModelConfig{}, reader)
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		resp, err := svc.Responses(r.Context(), "alice", body)
		if err != nil {
			t.Error(err)
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()
		for key, values := range resp.Header {
			w.Header()[key] = values
		}
		w.WriteHeader(resp.StatusCode)
		buf := make([]byte, 8192)
		for {
			n, e := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				w.(http.Flusher).Flush()
			}
			if e != nil {
				break
			}
		}
	}))
	defer bridge.Close()
	profile := agentruntime.Profile{Provider: "api", BaseURL: bridge.URL + "/v1", APIKey: "local", ModelID: "fixture-model"}
	rt := codex.New(codex.Dependencies{BinaryProvider: nativeProtocolBinary(binary), EventSink: codex.NewEventSink(), AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil }, ResolveAgent: func(agentruntime.Handle) (codex.AgentRef, error) {
		return codex.AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
	}})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "room:model-switch")
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{modelpreset.ChatCompletions, modelpreset.AnthropicMessages, modelpreset.Responses} {
		events, unsubscribe := rt.SubscribeSession(handle.RuntimeID, thread)
		reader.mu.Lock()
		reader.p.APIProtocol = protocol
		reader.mu.Unlock()
		if err := rt.Prompt(ctx, handle.RuntimeID, thread, "TURN_HISTORY_SENTINEL Reply done."); err != nil {
			t.Fatalf("%s: %v", protocol, err)
		}
		var usage modelusage.Usage
	drain:
		for {
			select {
			case event := <-events:
				if event.Kind == activity.RuntimeEventTokenUsage {
					usage, _ = event.Payload.(modelusage.Usage)
				}
			default:
				break drain
			}
		}
		unsubscribe()
		if !usage.Known || usage.Input != 100 || usage.Output != 10 || usage.Total != 110 {
			t.Fatalf("%s usage=%+v", protocol, usage)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses"} {
		if counts[path] != 1 {
			t.Errorf("%s requests=%d", path, counts[path])
		}
		if !strings.Contains(histories[path], "TURN_HISTORY_SENTINEL") {
			t.Errorf("missing user context for %s", path)
		}
	}
	if !strings.Contains(histories["/v1/messages"], "MODEL_PROTOCOL_SENTINEL") || !strings.Contains(histories["/v1/responses"], "MODEL_PROTOCOL_SENTINEL") {
		t.Fatal("conversation history was lost when switching protocols")
	}
}
