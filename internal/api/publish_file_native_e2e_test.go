package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

// Real Codex and HTTP bridge: a bad publication must be a tool error the model
// can repair, and an acknowledged file must survive subsequent source removal.
func TestNativeCodexFilePublicationRecoveryAndDelivery(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	const game = "<!doctype html><html><title>Tank battle</title><canvas id=game></canvas></html>"
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		n := calls.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		input, _ := json.Marshal(payload["input"])
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(value any) { raw, _ := json.Marshal(value); fmt.Fprintf(w, "data: %s\n\n", raw) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprint(n)}})
		switch n {
		case 1, 2:
			path := "missing/game.html"
			if n == 2 {
				if !strings.Contains(string(input), "unavailable") && !strings.Contains(string(input), "failed") {
					t.Errorf("model did not receive publication failure: %s", input)
				}
				if err := os.WriteFile(filepath.Join(workspace, "game.html"), []byte(game), 0600); err != nil {
					t.Error(err)
				}
				path = "game.html"
			}
			args, _ := json.Marshal(map[string]string{"path": path, "mimeType": "text/html"})
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": fmt.Sprint(n), "call_id": fmt.Sprint(n), "name": "csgclaw_publish_file", "arguments": string(args)}})
		default:
			if n == 3 {
				if err := os.Remove(filepath.Join(workspace, "game.html")); err != nil {
					t.Error(err)
				}
			}
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprint(n), "content": []any{map[string]any{"type": "output_text", "text": "Game ready."}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprint(n), "status": "completed", "usage": map[string]any{"input_tokens": 100, "output_tokens": 10, "total_tokens": 110}}})
	}))
	defer upstream.Close()
	h := &Handler{llm: llm.NewService(config.ModelConfig{}, usageProfile{agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "fixture-model", BaseURL: upstream.URL + "/v1", APIKey: "fixture"}}), serverAccessToken: "fixture-token"}
	bridge := httptest.NewServer(h.Routes())
	defer bridge.Close()
	profile := agentruntime.Profile{Provider: "api", BaseURL: bridge.URL + "/api/v1/agents/dev/llm/v1", APIKey: "fixture-token", ModelID: "fixture-model"}
	rt := codex.New(codex.Dependencies{
		BinaryProvider: usageNativeBinary(binary), EventSink: codex.NewEventSink(),
		AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil },
		ResolveAgent: func(agentruntime.Handle) (codex.AgentRef, error) {
			return codex.AgentRef{ID: "dev", Name: "dev", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled", "local_workspace_dir": workspace}}, nil
		},
	})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-dev", AgentID: "dev", AgentName: "dev", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	request := agentengine.TurnRequest{ID: "game-turn", ConversationKey: "game-task", Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "Build and publish the tank game."}}}
	result := rt.Conversation(handle.RuntimeID).Run(ctx, request, agentengine.EventSinkFunc(func(context.Context, agentengine.TurnEvent) error { return nil }))
	if result.Status != agentengine.TurnSucceeded || len(result.RuntimeFiles) != 1 || calls.Load() != 3 {
		t.Fatalf("publication should recover within the same turn: status=%s error=%+v files=%d calls=%d", result.Status, result.Error, len(result.RuntimeFiles), calls.Load())
	}
	defer result.RuntimeFiles[0].Cleanup()
	fileStore := agentengine.NewFileStore()
	result.Files, result.Error = fileStore.RegisterTurnFiles("dev", request.ConversationKey, request.ID, result.RuntimeFiles)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	statePath := filepath.Join(home, "im", "state.json")
	messages, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatal(err)
	}
	worker, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "dev", Name: "dev", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Tank game", CreatorID: "user-admin", MemberIDs: []string{worker.ID}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := delivery.NewIMTranscriptStore(messages, downstreamParticipant{worker.ID}, downstreamFiles{files: fileStore.Scope("dev")})
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.NewTranscriptRenderer(store).Complete(ctx, channel.TurnContext{AgentID: "dev", ParticipantID: "dev", RoomID: room.ID, TurnID: request.ID}, result); err != nil {
		t.Fatal(err)
	}
	fileStore.DeleteTurn("dev", request.ConversationKey, request.ID)
	messages, err = im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := messages.Room(room.ID)
	var downloadURL string
	for _, message := range saved.Messages {
		for _, file := range message.Attachments {
			if file.Name == "game.html" {
				downloadURL = file.DownloadURL
			}
		}
	}
	if downloadURL == "" {
		t.Fatal("game attachment was not delivered")
	}
	filesServer := httptest.NewServer((&Handler{im: messages}).Routes())
	defer filesServer.Close()
	response, err := http.Get(filesServer.URL + downloadURL)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || string(content) != game {
		t.Fatalf("download status=%d bytes=%d error=%v", response.StatusCode, len(content), readErr)
	}
	request.ID = "next-turn"
	next := rt.Conversation(handle.RuntimeID).Run(ctx, request, nil)
	if next.Status != agentengine.TurnSucceeded || len(next.RuntimeFiles) != 0 {
		t.Fatalf("next turn = %+v", next)
	}
}
