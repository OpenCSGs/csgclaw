package api

import (
	"bytes"
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/participant"
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
)

type compactAPIEngine struct {
	agentengine.Interface
	agents       compactAPIAgents
	conversation *compactAPIConversation
}

func (e *compactAPIEngine) Agents() agentengine.AgentInterface { return e.agents }
func (e *compactAPIEngine) Conversations(string) agentengine.ConversationInterface {
	return e.conversation
}

type compactAPIAgents struct{ agentengine.AgentInterface }

func (compactAPIAgents) Get(_ context.Context, id string, _ agentengine.AgentGetOptions) (agentengine.Agent, error) {
	return agentengine.Agent{ID: id, Spec: agentengine.AgentSpec{Name: id, Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}}, nil
}

type compactAPIConversation struct {
	agentengine.ConversationInterface
	calls  atomic.Int32
	gate   chan struct{}
	result *contract.CompactResult
}

func (c *compactAPIConversation) Compact(ctx context.Context, req contract.CompactRequest) (contract.CompactResult, error) {
	c.calls.Add(1)
	if req.OnStarted != nil && (c.result == nil || c.result.Status == "failed" || c.result.Status == "completed") {
		if err := req.OnStarted(); err != nil {
			return contract.CompactResult{}, err
		}
	}
	gate := c.gate
	if gate == nil {
		gate = make(chan struct{})
		go func() { time.Sleep(700 * time.Millisecond); close(gate) }()
	}
	select {
	case <-gate:
	case <-ctx.Done():
		return contract.CompactResult{}, ctx.Err()
	}
	if c.result != nil {
		return *c.result, nil
	}
	if req.Usage.SessionID == "failure" {
		return contract.CompactResult{Status: "failed"}, nil
	}
	usage := req.Usage
	n := int64(1000)
	usage.UsedTokens = &n
	return contract.CompactResult{Status: "completed", Usage: &usage}, nil
}
func TestRoomManualCompactionHTTPDedupeAndClear(t *testing.T) {
	used := int64(50000)
	usage := modelcap.ContextUsage{SessionID: "s", ModelID: "m", UsedTokens: &used, ContextWindow: 100000, ContextSource: "default"}
	service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []im.Room{{ID: "r", Members: []string{"user-admin", "user-worker"}, ContextUsage: map[string]modelcap.ContextUsage{"user-worker": usage}}}})
	native := &compactAPIConversation{gate: make(chan struct{})}
	engine := &compactAPIEngine{conversation: native}
	h := &Handler{im: service, agentEngine: engine, participant: participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "p", Channel: "csgclaw", Type: "agent", AgentID: "worker", ChannelUserRef: "user-worker"}}))}
	router := h.Routes()
	post := func(body string, intended int) modelcap.CompactionOperation {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/r/context:compact", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != intended {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
		var op modelcap.CompactionOperation
		_ = json.Unmarshal(w.Body.Bytes(), &op)
		return op
	}
	post(`{"request_id":"one","agent_ids":["worker"]}`, 202)
	deadline := time.Now().Add(time.Second)
	for native.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	post(`{"request_id":"one","agent_ids":["worker"]}`, 200)
	if native.calls.Load() != 1 {
		t.Fatal("duplicate started another native compaction")
	}
	post(`{"request_id":"one","agent_ids":["outsider"]}`, 403)
	if _, err := service.ClearRoomMessages("r"); err != nil {
		t.Fatal(err)
	}
	close(native.gate)
	time.Sleep(20 * time.Millisecond)
	room, _ := service.Room("r")
	if len(room.Messages) != 0 || len(room.ContextUsage) != 0 {
		t.Fatal("completion repopulated cleared room")
	}
}

// An opt-in HTTP fixture uses the production handler and persistence for browser acceptance.
func TestManualContextBrowserServer(t *testing.T) {
	root := os.Getenv("CSGCLAW_MANUAL_CONTEXT_FIXTURE")
	if root == "" {
		t.Skip("set CSGCLAW_MANUAL_CONTEXT_FIXTURE")
	}
	var users = []im.User{{ID: "user-admin", Name: "User", Role: "admin"}}
	var members = []string{"user-admin"}
	var participants = []apitypes.Participant{}
	var usages = map[string]modelcap.ContextUsage{}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("agent-%d", i)
		user := fmt.Sprintf("user-%d", i)
		members = append(members, user)
		users = append(users, im.User{ID: user, Name: id, Role: "agent"})
		participants = append(participants, apitypes.Participant{ID: "binding-" + id, Channel: "csgclaw", Type: "agent", AgentID: id, ChannelUserRef: user})
		n := int64(50000)
		session := "success"
		if i == 1 {
			session = "failure"
		}
		if i == 2 {
			n = 2000
		}
		usages[user] = modelcap.ContextUsage{SessionID: session, ModelID: "fixture-model", ContextWindow: 100000, ContextSource: "default", UsedTokens: &n}
	}
	path := filepath.Join(root, "http-state.json")
	if err := im.SaveBootstrap(path, im.Bootstrap{CurrentUserID: "user-admin", Users: users, Rooms: []im.Room{{ID: "r", Members: members, ContextUsage: usages}}}); err != nil {
		t.Fatal(err)
	}
	bus := im.NewBus()
	svc, err := im.NewServiceFromPathWithBus(path, bus)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: svc, imBus: bus, agentEngine: &compactAPIEngine{conversation: &compactAPIConversation{}}, participant: participant.NewService(participant.NewMemoryStore(participants))}
	router := h.Routes()
	router.Get("/fixture/state", func(w http.ResponseWriter, r *http.Request) {
		room, _ := svc.Room("r")
		_ = json.NewEncoder(w).Encode(room)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	if err := os.WriteFile(filepath.Join(root, "http-ready.json"), []byte(fmt.Sprintf(`{"url":%q}`, server.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("browser fixture timed out")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(root, "http-stop")); err == nil {
				return
			}
		}
	}
}

func TestManualSingleThenBatchKeepsContextAfterRefreshAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	used := int64(50000)
	usage := modelcap.ContextUsage{SessionID: "s", ModelID: "m", UsedTokens: &used, ContextWindow: 100000, ContextSource: "default"}
	initial := im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin"}, {ID: "user-worker"}, {ID: "user-qa"}}, Rooms: []im.Room{{ID: "r", Members: []string{"user-admin", "user-worker", "user-qa"}, ContextUsage: map[string]modelcap.ContextUsage{"user-worker": usage, "user-qa": usage}}}}
	if err := im.SaveBootstrap(path, initial); err != nil {
		t.Fatal(err)
	}
	service, err := im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	close(gate)
	native := &compactAPIConversation{gate: gate}
	h := &Handler{im: service, agentEngine: &compactAPIEngine{conversation: native}, participant: participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "p", Channel: "csgclaw", Type: "agent", AgentID: "worker", ChannelUserRef: "user-worker"}, {ID: "q", Channel: "csgclaw", Type: "agent", AgentID: "qa", ChannelUserRef: "user-qa"}}))}
	router := h.Routes()
	post := func(body string) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/rooms/r/context:compact", strings.NewReader(body)))
		if w.Code != 202 {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
	}
	wait := func(id string) *modelcap.CompactionOperation {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			room, _ := service.Room("r")
			for _, m := range room.Messages {
				if op := im.MessageCompaction(m); op != nil && op.ID == id && !op.Running {
					return op
				}
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("compaction did not finish")
		return nil
	}
	post(`{"request_id":"single","agent_ids":["worker"]}`)
	wait("single")
	post(`{"request_id":"batch","agent_ids":["worker","qa"]}`)
	op := wait("batch")
	if op.Targets[0].Status != "unnecessary" || op.Targets[1].Status != "completed" || native.calls.Load() != 2 {
		t.Fatalf("batch did not skip already-small context: %+v calls=%d", op, native.calls.Load())
	}
	// The UI's refresh route reloads disk, then a new service exercises restart.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/bootstrap", nil))
	if w.Code != 200 {
		t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
	}
	restarted, err := im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, _ := restarted.Room("r")
	for _, user := range []string{"user-worker", "user-qa"} {
		if snapshot := room.ContextUsage[user]; snapshot.UsedTokens == nil || *snapshot.UsedTokens != 1000 {
			t.Fatalf("refresh/restart lost %s: %+v", user, snapshot)
		}
	}
}

func TestCompactionFailureReasonSurvivesHTTPRefreshAndRestart(t *testing.T) {
	for _, reason := range []modelcap.CompactionFailureReason{modelcap.CompactionBindingMissing, modelcap.CompactionRestoreFailed, modelcap.CompactionIdentityChanged, modelcap.CompactionHistoryUnreadable, modelcap.CompactionHistoryInvalid, modelcap.CompactionAgentUnavailable, modelcap.CompactionMemberRemoved} {
		t.Run(string(reason), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			used := int64(50000)
			usage := modelcap.ContextUsage{SessionID: "saved", UsedTokens: &used, ContextWindow: 100000}
			if err := im.SaveBootstrap(path, im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []im.Room{{ID: "r", Members: []string{"user-admin", "user-worker"}, ContextUsage: map[string]modelcap.ContextUsage{"user-worker": usage}}}}); err != nil {
				t.Fatal(err)
			}
			service, err := im.NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			gate := make(chan struct{})
			close(gate)
			native := &compactAPIConversation{gate: gate, result: &contract.CompactResult{Status: "unavailable", Reason: reason}}
			h := &Handler{im: service, agentEngine: &compactAPIEngine{conversation: native}, participant: participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "p", Channel: "csgclaw", Type: "agent", AgentID: "worker", ChannelUserRef: "user-worker"}}))}
			router := h.Routes()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/rooms/r/context:compact", strings.NewReader(`{"request_id":"reason","agent_ids":["worker"],"batch":true}`)))
			if w.Code != 202 {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				room, _ := service.Room("r")
				op := im.MessageCompaction(room.Messages[0])
				if !op.Running {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("compaction did not finish")
				}
				time.Sleep(time.Millisecond)
			}
			w = httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/bootstrap", nil))
			if w.Code != 200 || !strings.Contains(w.Body.String(), string(reason)) {
				t.Fatalf("refresh lost reason: %d", w.Code)
			}
			restarted, err := im.NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			room, _ := restarted.Room("r")
			op := im.MessageCompaction(room.Messages[0])
			if op.Targets[0].Reason != reason || op.Running {
				t.Fatalf("saved result=%+v", op)
			}
			if *room.ContextUsage["user-worker"].UsedTokens != used {
				t.Fatal("failed compaction changed usage")
			}
		})
	}
}

func TestSingleCompactionPreStartSkipHasNoMessage(t *testing.T) {
	for _, tc := range []contract.CompactResult{
		{Status: "unsupported", Reason: modelcap.CompactionScopedSessionUnsupported},
		{Status: "busy"}, {Status: "no_history"}, {Status: "unknown"}, {Status: "unnecessary"},
		{Status: "unavailable", Reason: modelcap.CompactionRestoreFailed},
	} {
		t.Run(tc.Status, func(t *testing.T) {
			used := int64(50000)
			service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin"}, {ID: "user-worker"}}, Rooms: []im.Room{{ID: "r", Members: []string{"user-admin", "user-worker"}, ContextUsage: map[string]modelcap.ContextUsage{"user-worker": {SessionID: "saved", UsedTokens: &used, ContextWindow: 100000}}}}})
			gate := make(chan struct{})
			close(gate)
			native := &compactAPIConversation{gate: gate, result: &tc}
			h := &Handler{im: service, agentEngine: &compactAPIEngine{conversation: native}, participant: participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "p", Channel: "csgclaw", Type: "agent", AgentID: "worker", ChannelUserRef: "user-worker"}}))}
			router := h.Routes()
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/rooms/r/context:compact", strings.NewReader(fmt.Sprintf(`{"request_id":"skip-%d","agent_ids":["worker"]}`, i))))
				var op modelcap.CompactionOperation
				_ = json.Unmarshal(w.Body.Bytes(), &op)
				if w.Code != 200 || op.StartedAt != "" || op.Running || op.Targets[0].Status != tc.Status || op.Targets[0].Reason != tc.Reason {
					t.Fatalf("HTTP %d result=%+v", w.Code, op)
				}
				room, _ := service.Room("r")
				if len(room.Messages) != 0 {
					t.Fatal("pre-start skip created a message")
				}
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/rooms/r/context:compact", strings.NewReader(`{"request_id":"batch","agent_ids":["worker"],"batch":true}`)))
			if w.Code != 202 {
				t.Fatalf("batch HTTP %d: %s", w.Code, w.Body.String())
			}
			deadline := time.Now().Add(time.Second)
			for {
				room, _ := service.Room("r")
				if len(room.Messages) != 1 {
					t.Fatal("batch did not create one card")
				}
				op := im.MessageCompaction(room.Messages[0])
				if !op.Running {
					if !op.Batch || op.Targets[0].Status != tc.Status || op.Targets[0].Reason != tc.Reason {
						t.Fatalf("batch=%+v", op)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("batch stuck")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}
