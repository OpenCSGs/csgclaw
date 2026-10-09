package api

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

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
	"csgclaw/internal/taskcore"
	"github.com/go-chi/chi/v5"
)

type roomScopeRuntimeRecords struct {
	appTaskTestRecords
	runtime *runtimecodex.Runtime
}

func (r roomScopeRuntimeRecords) Runtime(string) (agentruntime.Runtime, error) {
	return r.runtime, nil
}

type roomScopeBinary string

func (b roomScopeBinary) Ensure(context.Context) (string, error) { return string(b), nil }

// Force the three observed wrong choices through the real Codex MCP transport.
// Model responses are local fixtures, so the result is independent of sampling.
func TestAppRoomToolScopeBundledCodexE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY to the bundled Codex binary")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	h := newAppTaskTestHandler(t)
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Room work", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Ask dev to participate"})
	if err != nil {
		t.Fatal(err)
	}
	wrongRoom := "room-" + strings.TrimPrefix(source.ID, "msg-")
	calls := []struct {
		name string
		args map[string]any
	}{
		{"agent_task_create", map[string]any{"agent_id": "agent-dev", "title": "Participation"}},
		{"message_send", map[string]any{"room_id": room.ID, "mention_id": "pt-dev", "content": "Participate"}},
		{"room_task_create", map[string]any{"room_id": wrongRoom, "source_message_id": source.ID, "title": "Participation"}},
		{"room_task_plan", map[string]any{"room_id": wrongRoom, "task_id": "task-1", "auto_start": true, "tasks": []map[string]any{{"id_ref": "dev", "title": "Participate", "assigned_to": "pt-dev"}}}},
		{"room_task_dispatch", map[string]any{"room_id": wrongRoom, "task_id": "task-2"}},
	}
	var requests atomic.Int32
	mux := chi.NewRouter()
	mux.HandleFunc("/api/v1/agents/{id}/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-agent-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.apps.Handler("agent-manager").ServeHTTP(w, r)
	})
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "invalid input", 400)
			return
		}
		n := int(requests.Add(1))
		input, _ := json.Marshal(body["input"])
		if n == 2 && !strings.Contains(string(input), "agent_task_create delivers to a private chat") {
			t.Error("private task error did not reach the model")
		}
		if n == 3 && !strings.Contains(string(input), "ordinary mentions do not dispatch Workers") {
			t.Error("ordinary mention error did not reach the model")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(value any) { raw, _ := json.Marshal(value); fmt.Fprintf(w, "data: %s\n\n", raw) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprintf("response-%d", n)}})
		if n <= len(calls) {
			call := calls[n-1]
			args, _ := json.Marshal(call.args)
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "call_id": fmt.Sprintf("call-%d", n), "namespace": "mcp__csgclaw", "name": call.name, "arguments": string(args)}})
		} else {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": "done", "content": []map[string]any{{"type": "output_text", "text": "Dispatched"}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprintf("response-%d", n), "usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host-codex"))
	profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-model", APIKey: "fixture-agent-token", BaseURL: server.URL + "/v1", Env: map[string]string{
		"CSGCLAW_CALLER_AGENT_ID": "agent-manager", "CSGCLAW_BASE_URL": server.URL, "CSGCLAW_ACCESS_TOKEN": "fixture-agent-token",
	}}
	rt := runtimecodex.New(runtimecodex.Dependencies{
		BinaryProvider: roomScopeBinary(binary),
		AgentHome:      func(string) (string, error) { return filepath.Join(home, "agent-manager"), nil },
		ResolveAgent: func(agentruntime.Handle) (runtimecodex.AgentRef, error) {
			return runtimecodex.AgentRef{ID: "agent-manager", Name: "manager", Profile: profile, MCPServers: map[string]any{}, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
		},
	})
	defer rt.Close()
	h.svc = roomScopeRuntimeRecords{appTaskTestRecords: h.svc.(appTaskTestRecords), runtime: rt}
	h.registerAppPlatformTaskTools("agent-manager")
	h.registerAppRoomTools("agent-manager")
	rt.SetAgentMCPRevisionSource(h.apps.Revision)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-agent-manager", AgentID: "agent-manager", AgentName: "manager", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, roomToolConversation(t, room.ID, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Prompt(ctx, handle.RuntimeID, thread, "Coordinate the current room"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != int32(len(calls)+1) || len(h.agentTaskSvc.List()) != 0 {
		t.Fatal("wrong tool path created personal work or did not recover", requests.Load())
	}
	tasks := h.roomTaskSvc.List(room.ID)
	if len(tasks) != 2 || len(h.roomTaskSvc.List(wrongRoom)) != 0 {
		t.Fatal("room tasks were not bound to the active room", tasks)
	}
	child, ok := h.roomTaskSvc.Get(room.ID, "task-2")
	if !ok || child.ParentID != "task-1" || child.AssignedTo != "pt-dev" || child.Status != taskcore.StatusAssigned {
		t.Fatal("Worker child was not dispatched", child)
	}
}
