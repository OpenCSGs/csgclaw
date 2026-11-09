package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/relay"
)

func TestCloudTaskRecoversKnownResponseAfterRestart(t *testing.T) {
	var calls atomic.Int32
	var polls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/agents":
			_ = json.NewEncoder(w).Encode(relay.Publication{Agent: relay.Agent{ID: "agt_cloud"}})
		case r.Method == "GET" && r.URL.Path == "/v1/responses/resp_saved":
			if polls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "resp_saved", "model": "agt_cloud", "status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": "recovered result"}}}}})
		case r.URL.Path == "/v1/responses":
			calls.Add(1)
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	m, err := relay.NewManager(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Configure(remote.URL, "test-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Publish(context.Background(), relay.PublishRequest{SourceAgentID: "local", Visibility: "public"}); err != nil {
		t.Fatal(err)
	}
	connectionID := m.Catalog(context.Background())[0].ConnectionID
	room := im.Room{ID: "cloud-task", Title: "Task", TaskSession: true, IsDirect: true, CreatedBy: im.AdminUserID, Members: []string{im.AdminUserID, "relay-user"}, CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", ConnectionID: connectionID, UserID: "relay-user"}, Messages: []im.Message{{ID: "pending", SenderID: im.AdminUserID, Content: "hello", CreatedAt: time.Now().Add(-time.Hour), Metadata: map[string]any{"relay_pending_response_id": "resp_saved"}}}}
	state := im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}, {ID: "relay-user", Name: "cloud", Role: "cloud"}}, Rooms: []im.Room{room}}
	path := filepath.Join(t.TempDir(), "state.json")
	if err = im.SaveBootstrap(path, state); err != nil {
		t.Fatal(err)
	}
	svc, err := im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &Handler{im: svc, relayManager: m, relayContext: ctx}
	h.startCloudTask(room.ID)
	h.startCloudTask(room.ID)
	deadline := time.Now().Add(3 * time.Second)
	for {
		saved, _ := svc.Room(room.ID)
		if len(saved.Messages) == 2 {
			if saved.Messages[1].Content != "recovered result" {
				t.Fatal("recovery failed")
			}
			if _, _, pending := cloudTaskPending(saved); pending {
				t.Fatal("reply did not consume pending message")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovery timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if polls.Load() < 2 {
		t.Fatal("did not recover from temporary polling failure")
	}
	if calls.Load() != 0 {
		t.Fatal("reissued an already accepted operation")
	}
	reloaded, err := im.LoadBootstrap(path)
	if err != nil || len(reloaded.Rooms) != 1 || reloaded.Rooms[0].CloudAgent == nil || reloaded.Rooms[0].CloudAgent.ConnectionID != connectionID {
		t.Fatal("cloud task reference did not survive persistence")
	}
}

func TestCloudTaskModelFailureIsActionable(t *testing.T) {
	for _, locale := range []string{"zh", "en"} {
		text := cloudTaskFailureText(locale, "model_protocol_unsupported")
		if !strings.Contains(text, "Responses") {
			t.Fatal("lost actionable model protocol diagnosis")
		}
		if text == cloudTaskFailureText(locale, "execution_failed") {
			t.Fatal("model failure rendered as generic execution failure")
		}
	}
}

func TestCloudTaskManagementGuardsPersistedPendingWork(t *testing.T) {
	h, svc, _, _ := newAgentSessionTestHandler(t, nil, &fakeSessionEngine{}, "")
	user, _, err := svc.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "relay-review", Name: "CloudReview", Role: "cloud", SkipDirectRoom: true})
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "Pending cloud task", Type: apitypes.RoomTypeFree, IsDirect: true, TaskSession: true, CreatorID: im.AdminUserID, MemberIDs: []string{user.ID}, CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_review", ConnectionID: "connection", UserID: user.ID}})
	if err != nil {
		t.Fatal(err)
	}
	message, err := svc.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: im.AdminUserID, Content: "Still running"})
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"delete", "archive", "clear"} {
		t.Run(operation, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			switch operation {
			case "delete":
				h.handleLocalRoomByID(recorder, httptest.NewRequest(http.MethodDelete, "/", nil), room.ID)
			case "archive":
				h.handleLocalRoomByID(recorder, httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"archived":true}`)), room.ID)
			case "clear":
				req := httptest.NewRequest(http.MethodDelete, "/", nil)
				req.SetPathValue("id", room.ID)
				h.handleClearRoomMessages(recorder, req)
			}
			if recorder.Code != http.StatusConflict {
				t.Fatalf("pending cloud task was not protected: %d", recorder.Code)
			}
			saved, found := svc.Room(room.ID)
			if !found || saved.Archived || len(saved.Messages) != 2 {
				t.Fatal("pending room changed")
			}
		})
	}
	if _, err = svc.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: user.ID, Content: "Done", ClientMessageID: "relay-reply:" + message.ID}); err != nil {
		t.Fatal(err)
	}
	if !h.allowRoomManagement(httptest.NewRecorder(), room.ID) {
		t.Fatal("completed task remained locked")
	}
}

func TestCloudTaskSubmissionRetriesKeepIdentity(t *testing.T) {
	var submits atomic.Int32
	var firstKey string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/agents":
			_ = json.NewEncoder(w).Encode(relay.Publication{Agent: relay.Agent{ID: "agt_review"}})
		case r.Method == "POST" && r.URL.Path == "/v1/responses":
			if submits.Add(1) == 1 {
				firstKey = r.Header.Get("Idempotency-Key")
				w.WriteHeader(503)
				return
			}
			if firstKey == "" || r.Header.Get("Idempotency-Key") != firstKey {
				t.Error("changed submission key")
			}
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": relay.ProtocolError{Code: "response_in_progress", ResponseID: "resp_existing"}})
		case r.Method == "GET" && r.URL.Path == "/v1/responses/resp_existing":
			_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_existing", Model: "agt_review", Status: "completed"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := relay.NewManager(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Configure(remote.URL, "test-token"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Publish(ctx, relay.PublishRequest{SourceAgentID: "local", Visibility: "public"}); err != nil {
		t.Fatal(err)
	}
	connection := m.Catalog(ctx)[0].ConnectionID
	svc := im.NewService()
	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "Review", CreatorID: im.AdminUserID, Type: apitypes.RoomTypeFree})
	if err != nil {
		t.Fatal(err)
	}
	message, err := svc.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: im.AdminUserID, Content: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	room.CloudAgent = &apitypes.CloudAgentReference{AgentID: "agt_review", ConnectionID: connection}
	h := &Handler{im: svc, relayManager: m}
	response, err := h.awaitCloudTask(ctx, room, message, "")
	if err != nil || response.Status != "completed" || submits.Load() != 2 {
		t.Fatalf("retry failed: status=%s submissions=%d error=%v", response.Status, submits.Load(), err)
	}
	saved, _ := svc.Room(room.ID)
	if saved.Messages[len(saved.Messages)-1].Metadata["relay_pending_response_id"] != "resp_existing" {
		t.Fatal("response identity was not persisted")
	}
}

func TestCloudTaskRestoresLostContextInSameRoom(t *testing.T) {
	for _, rejection := range []string{"missing parent", "missing checkpoint", "missing submitted result", "missing polled result"} {
		t.Run(rejection, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var recoveryCalls atomic.Int32
			firstRecoveryInput := make(chan string, 1)
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/v1/agents":
					_ = json.NewEncoder(w).Encode(relay.Publication{Agent: relay.Agent{ID: "agt_cloud"}})
				case r.URL.Path == "/v1/agents/agt_cloud":
					_ = json.NewEncoder(w).Encode(relay.Agent{ID: "agt_cloud", Status: "online"})
				case r.URL.Path == "/v1/agents/agt_cloud/connect":
					w.WriteHeader(http.StatusNotFound)
				case r.URL.Path == "/v1/responses/resp_pending":
					if rejection == "missing checkpoint" {
						_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_pending", Model: "agt_cloud", Status: "failed", Error: &relay.ProtocolError{Code: "relay_branch_unsupported"}})
					} else {
						w.WriteHeader(http.StatusNotFound)
						_ = json.NewEncoder(w).Encode(map[string]any{"error": relay.ProtocolError{Code: "response_not_found"}})
					}
				case r.URL.Path == "/v1/responses" && r.Method == http.MethodPost:
					var req relay.CallRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					var input string
					_ = json.Unmarshal(req.Input, &input)
					switch r.Header.Get("Idempotency-Key") {
					case "task:room:followup":
						if req.PreviousResponseID != "resp_old" || input != "Where is it?" {
							t.Errorf("unexpected continuation: %+v, %q", req, input)
						}
						if rejection == "missing submitted result" {
							w.Header().Set("X-Response-Id", "resp_admitted")
						}
						w.WriteHeader(http.StatusNotFound)
						_ = json.NewEncoder(w).Encode(map[string]any{"error": relay.ProtocolError{Code: "response_not_found"}})
					case "task-recovery:room:followup":
						if req.PreviousResponseID != "" || !strings.Contains(input, "Remember the report") ||
							!strings.Contains(input, "Saved in the project folder") || !strings.HasSuffix(input, "Where is it?") ||
							strings.Contains(input, "Failed request") || strings.Contains(input, "Future message") {
							t.Errorf("unexpected recovery input: %q, parent %q", input, req.PreviousResponseID)
						}
						attempt := recoveryCalls.Add(1)
						if attempt == 1 {
							firstRecoveryInput <- input
							cancel()
							return
						}
						original := <-firstRecoveryInput
						firstRecoveryInput <- original
						if input != original {
							t.Error("recovery input changed after restart")
						}
						if attempt == 2 && rejection == "missing checkpoint" {
							w.WriteHeader(http.StatusConflict)
							_ = json.NewEncoder(w).Encode(map[string]any{"error": relay.ProtocolError{Code: "agent_busy"}})
							return
						}
						_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_restored", Model: "agt_cloud", Status: "completed"})
					case "task:room:next":
						if req.PreviousResponseID != "resp_restored" || input != "Future message" {
							t.Errorf("unexpected follow-up: parent %q, input %q", req.PreviousResponseID, input)
						}
						_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_next", Model: "agt_cloud", Status: "completed"})
					default:
						t.Errorf("unexpected key %q", r.Header.Get("Idempotency-Key"))
						w.WriteHeader(http.StatusBadRequest)
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer remote.Close()
			m, err := relay.NewManager(context.Background(), t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if err := m.Configure(remote.URL, "test-token"); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Publish(ctx, relay.PublishRequest{SourceAgentID: "local", Visibility: "public"}); err != nil {
				t.Fatal(err)
			}
			room := im.Room{
				ID: "room", CreatedBy: im.AdminUserID, Members: []string{im.AdminUserID, "cloud"},
				CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", ConnectionID: m.Catalog(ctx)[0].ConnectionID, UserID: "cloud"},
				Messages: []im.Message{
					{ID: "prior", SenderID: im.AdminUserID, Content: "Remember the report"},
					{ID: "failed", SenderID: im.AdminUserID, Content: "Failed request"},
					{ID: "followup", SenderID: im.AdminUserID, Content: "Where is it?", CreatedAt: time.Now()},
					{ID: "prior-reply", SenderID: "cloud", ClientMessageID: "relay-reply:prior", Content: "Saved in the project folder", Metadata: map[string]any{"relay_response_id": "resp_old"}},
					{ID: "failed-reply", SenderID: "cloud", ClientMessageID: "relay-reply:failed", Content: "Failed", Metadata: map[string]any{"relay_task_failed": true}},
				},
			}
			if rejection == "missing checkpoint" || rejection == "missing polled result" {
				room.Messages[2].Metadata = map[string]any{"relay_pending_response_id": "resp_pending"}
			}
			if !strings.HasSuffix(rejection, "result") {
				room.Messages = append(room.Messages, im.Message{ID: "next", SenderID: im.AdminUserID, Content: "Future message", CreatedAt: time.Now()})
			}
			path := filepath.Join(t.TempDir(), "state.json")
			if err := im.SaveBootstrap(path, im.Bootstrap{
				CurrentUserID: im.AdminUserID,
				Users:         []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}, {ID: "cloud", Name: "cloud", Role: "cloud"}},
				Rooms:         []im.Room{room},
			}); err != nil {
				t.Fatal(err)
			}
			svc, err := im.NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			h := &Handler{im: svc, relayManager: m, relayContext: ctx}
			h.runCloudTask(room.ID)
			if strings.HasSuffix(rejection, "result") {
				saved, _ := svc.Room(room.ID)
				if len(saved.Messages) != 6 || saved.Messages[5].Metadata["relay_error_code"] != "relay_task_result_unavailable" || recoveryCalls.Load() != 0 {
					t.Fatal("replayed a request with an unknown execution outcome")
				}
				return
			}
			if recoveryCalls.Load() != 1 {
				t.Fatal("did not start recovery")
			}
			svc, err = im.NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			saved, _ := svc.Room(room.ID)
			pending, _, found := cloudTaskPending(saved)
			if !found || pending.Metadata["relay_context_recovery_input"] == nil || pending.Metadata["relay_pending_response_id"] != nil {
				t.Fatal("recovery attempt was not persisted before submission")
			}
			h = &Handler{im: svc, relayManager: m, relayContext: context.Background()}
			h.runCloudTask(room.ID)
			saved, _ = svc.Room(room.ID)
			wantCalls := int32(2)
			if rejection == "missing checkpoint" {
				wantCalls++
			}
			if recoveryCalls.Load() != wantCalls || len(saved.Messages) != 8 || saved.Messages[6].Metadata["relay_response_id"] != "resp_restored" {
				t.Fatal("did not finish the recovered turn in the original room")
			}
			if saved.Messages[7].Metadata["relay_response_id"] != "resp_next" {
				t.Fatal("next turn did not continue from the restored response")
			}
		})
	}
}

func TestCloudTaskShutdownDoesNotConsumePendingMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	room := im.Room{ID: "cloud-shutdown", Title: "Pending", CreatedBy: im.AdminUserID, Members: []string{im.AdminUserID, "user-cloud"}, CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt", ConnectionID: "conn", UserID: "user-cloud"}, Messages: []im.Message{{ID: "job", SenderID: im.AdminUserID, Content: "Work", CreatedAt: time.Now(), Metadata: map[string]any{"relay_pending_response_id": "resp_pending"}}}}
	svc := im.NewServiceFromBootstrap(im.Bootstrap{Users: []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}, {ID: "user-cloud", Name: "cloud", Role: "cloud"}}, Rooms: []im.Room{room}})
	h := &Handler{im: svc, relayContext: ctx}
	h.runCloudTask(room.ID)
	saved, _ := svc.Room(room.ID)
	if len(saved.Messages) != 1 {
		t.Fatal("shutdown wrote a terminal failure")
	}
	if _, _, pending := cloudTaskPending(saved); !pending {
		t.Fatal("shutdown lost the recoverable request")
	}
	if h.allowRoomManagement(httptest.NewRecorder(), room.ID) {
		t.Fatal("unconfirmed work became deletable after shutdown")
	}
}

func TestCloudTaskAcceptsRegisteredCommunityAgentWithoutLocalPublication(t *testing.T) {
	const id = "agt_0123456789abcdef01234567"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agent/instances":
			if r.URL.Query().Get("registered") != "true" {
				t.Error("missing registered discovery filter")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": 42, "kind": "runtime", "source": "external", "type": "csgclaw", "public": true, "name": "Remote worker", "relay": map[string]string{"agent_id": id}}}, "total": 1})
		case "/api/v1/relay/v1/agents/" + id:
			_ = json.NewEncoder(w).Encode(relay.Agent{ID: id, Name: "Remote worker", Status: "online", Visibility: "public"})
		default:
			t.Error("unexpected cloud task discovery request")
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	source := func() (relay.SiteCredentials, error) {
		return relay.SiteCredentials{Site: remote.URL, Token: "caller", User: "caller"}, nil
	}
	m, err := relay.NewManager(context.Background(), t.TempDir(), nil, relay.WithCommunity(relay.HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	entries, err := m.Discover(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatalf("discovery failed: %v", err)
	}
	if len(m.Catalog(context.Background())) != 0 {
		t.Fatal("unexpected local publication")
	}
	h := &Handler{im: im.NewService(), relayManager: m}
	req := &apitypes.CreateRoomRequest{CloudAgent: &apitypes.CloudAgentReference{AgentID: id, ConnectionID: entries[0].ConnectionID}}
	if err = h.prepareCloudTask(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if req.CloudAgent.UserID == "" || !req.TaskSession || len(req.MemberIDs) != 1 {
		t.Fatal("did not prepare the remote cloud task")
	}
}
