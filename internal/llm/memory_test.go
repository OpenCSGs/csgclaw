package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentmemory"
	"csgclaw/internal/cliproxy"
	"csgclaw/internal/config"
)

func TestMemoryReferencesKeepPersistentIDsOutOfModelRequestsAndRestoreValidatedSources(t *testing.T) {
	for _, protocol := range []string{"responses", "chat_completions"} {
		for _, invalid := range []string{"", "message", "entry"} {
			t.Run(protocol+"/"+invalid, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body struct {
						Messages []struct{ Role, Content string } `json:"messages"`
						Input    []struct{ Role, Content string } `json:"input"`
						Stream   bool                             `json:"stream"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						http.Error(w, "bad request", 400)
						return
					}
					var input memoryRequest
					if !body.Stream {
						t.Error("memory did not use streaming generation")
					}
					var text string
					if protocol == "responses" {
						if len(body.Input) != 1 {
							t.Error("missing model evidence")
							return
						}
						text = body.Input[0].Content
					} else {
						if len(body.Messages) != 2 {
							t.Error("missing model instructions or evidence")
							return
						}
						text = body.Messages[1].Content
					}
					if err := json.Unmarshal([]byte(text), &input); err != nil {
						t.Error(err)
						return
					}
					for _, forbidden := range []string{"persistent-entry-id", "previous-page-source-id", "long-message-id-1", "long-message-id-2", "private-room", "updated_at", "room_id"} {
						if strings.Contains(text, forbidden) {
							t.Errorf("persistent metadata leaked into model evidence: %s", forbidden)
						}
					}
					if len(input.Messages) != 2 || input.Messages[0].MessageID != "m1" || input.Messages[1].MessageID != "m2" || len(input.Entries) != 1 || input.Entries[0].ID != "e1" {
						t.Errorf("request references not bounded: %+v", input)
					}
					source, entry := "m2", "e1"
					if invalid == "message" {
						source = "m99"
					}
					if invalid == "entry" {
						entry = "e99"
					}
					content, _ := json.Marshal(map[string]any{"changes": []map[string]string{{"entry_id": entry, "kind": "preference", "text": "Updated report preference", "source_message_id": source}}})
					writeMemoryTestStream(w, protocol, string(content))
				}))
				t.Cleanup(upstream.Close)
				ctx := WithTaskProfile(t.Context(), agent.AgentProfile{Provider: agent.ProviderAPI, APIProtocol: protocol, BaseURL: upstream.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model", ProfileComplete: true})
				input := agentmemory.Extraction{History: true, Entries: []agentmemory.Entry{{ID: "persistent-entry-id", Kind: "preference", Text: "Keep existing scope", Source: &agentmemory.Source{RoomID: "private-room", MessageID: "previous-page-source-id"}}}, Messages: []agentmemory.Message{
					{Source: agentmemory.Source{RoomID: "private-room", MessageID: "long-message-id-1"}, Text: "First request"},
					{Source: agentmemory.Source{RoomID: "private-room", MessageID: "long-message-id-2"}, Text: "Updated report preference"},
				}}
				changes, err := NewService(config.ModelConfig{}, nil).ExtractAgentMemory(ctx, "agent-test", input)
				if invalid != "" {
					if !errors.Is(err, agentmemory.ErrInvalid) {
						t.Fatalf("invalid reference accepted: %+v, %v", changes, err)
					}
					return
				}
				if err != nil || len(changes) != 1 || changes[0].SourceMessageID != "long-message-id-2" || changes[0].EntryID != "persistent-entry-id" {
					t.Fatalf("references not restored: %+v, %v", changes, err)
				}
				if input.Messages[1].MessageID != "long-message-id-2" || input.Entries[0].Source.MessageID != "previous-page-source-id" {
					t.Fatal("model reference preparation mutated stored evidence")
				}
			})
		}
	}
}

func writeMemoryTestStream(w http.ResponseWriter, protocol, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	if protocol == "responses" {
		text, _ := json.Marshal(content)
		fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":%s}]}]}}\n\n", text)
	} else {
		chunk, _ := json.Marshal(map[string]any{"id": "fixture", "model": "fixture-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": content}, "finish_reason": "stop"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
	}
}

func TestExtractAgentMemoryUsesNativeCodexResponses(t *testing.T) {
	oldBase, oldAuth := embeddedCLIProxyProviderBaseURL, embeddedCLIProxyAuthStatus
	t.Cleanup(func() {
		embeddedCLIProxyProviderBaseURL, embeddedCLIProxyAuthStatus = oldBase, oldAuth
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Instructions string `json:"instructions"`
			Input        []struct {
				Content string `json:"content"`
			} `json:"input"`
			Stream    bool `json:"stream"`
			Store     bool `json:"store"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.URL.Path != "/v1/responses" ||
			!body.Stream || body.Store || body.Reasoning.Effort != "low" || body.Instructions != memoryPolicy || len(body.Input) != 1 ||
			!strings.Contains(body.Input[0].Content, "Show a risk matrix in every report.") {
			t.Errorf("unexpected native extraction request: %s, %v, %+v", r.URL.Path, err, body)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		result, _ := json.Marshal(`{"changes":[{"entry_id":"","kind":"preference","text":"Use a risk matrix","source_message_id":"m1"}]}`)
		fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"reasoning\"},{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":%s}]}]}}\n\n", result)
	}))
	t.Cleanup(upstream.Close)
	embeddedCLIProxyProviderBaseURL = func(context.Context, string) (string, error) { return upstream.URL + "/v1", nil }
	embeddedCLIProxyAuthStatus = func(context.Context, string) (cliproxy.AuthStatus, error) {
		return cliproxy.AuthStatus{Authenticated: true}, nil
	}
	ctx := WithTaskProfile(context.Background(), agent.AgentProfile{Provider: agent.ProviderCodex, ModelID: "test-model", APIProtocol: "chat_completions", ReasoningEffort: "low"})
	changes, err := NewService(config.ModelConfig{}, nil).ExtractAgentMemory(ctx, "agent-test", agentmemory.Extraction{
		Messages: []agentmemory.Message{{Source: agentmemory.Source{RoomID: "private", MessageID: "m1"}, Text: "Show a risk matrix in every report."}},
	})
	if err != nil || len(changes) != 1 || changes[0].SourceMessageID != "m1" {
		t.Fatalf("changes = %+v, error = %v", changes, err)
	}
}

func TestDecodeMemoryChangesAcceptsEnclosingJSONFence(t *testing.T) {
	for _, value := range []string{"```json\n{\"changes\":[]}\n```", "```JSON\r\n{\"changes\":[]}\r\n```", "{\"changes\":[]}"} {
		if _, err := decodeMemoryChanges(value); err != nil {
			t.Fatalf("response = %q, error = %v", value, err)
		}
	}
	err := memoryHTTPError(429, []byte(`{"error":{"code":"rate_limit_exceeded","message":"secret debug body"}}`))
	if !strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "secret debug") {
		t.Fatalf("provider error = %v", err)
	}
}
