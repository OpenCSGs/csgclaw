package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

func TestCloudTaskReportsExpiredRecoveryAsUnconfirmed(t *testing.T) {
	const roomID = "cloud-task"
	now := time.Now()
	state := im.Bootstrap{
		CurrentUserID: im.AdminUserID,
		Users:         []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}, {ID: "cloud", Name: "cloud", Role: "cloud"}},
		Rooms: []im.Room{{
			ID: roomID, CreatedBy: im.AdminUserID, Locale: "en",
			CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", UserID: "cloud"},
			Members:    []string{im.AdminUserID, "cloud"},
			Messages: []im.Message{{
				ID: "pending", SenderID: im.AdminUserID, Content: "Continue", CreatedAt: now.Add(-time.Hour),
				Metadata: map[string]any{
					cloudTaskRecoveryKey:                "Saved recovery input",
					"relay_context_recovery_started_at": now.Add(-11 * time.Minute).Format(time.RFC3339Nano),
				},
			}},
		}},
	}
	svc := im.NewServiceFromBootstrap(state)
	h := &Handler{im: svc}
	h.runCloudTask(roomID)
	room, _ := svc.Room(roomID)
	if len(room.Messages) != 2 {
		t.Fatalf("expected an unconfirmed-result reply, got %d messages", len(room.Messages))
	}
	reply := room.Messages[1]
	if reply.Metadata["relay_error_code"] != "relay_task_result_unavailable" || !strings.Contains(reply.Content, "Check whether it completed") {
		t.Fatalf("expected an unconfirmed-result warning, got %q", reply.Content)
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
	oversized, err := svc.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: im.AdminUserID, Content: strings.Repeat("x", 1<<20)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.awaitCloudTask(ctx, room, oversized, ""); !errors.Is(err, relay.ErrRequestTooLarge) || submits.Load() != 0 {
		t.Fatalf("oversized task was retried or submitted: %v", err)
	}
	response, err := h.awaitCloudTask(ctx, room, message, "")
	if err != nil || response.Status != "completed" || submits.Load() != 2 {
		t.Fatalf("retry failed: status=%s submissions=%d error=%v", response.Status, submits.Load(), err)
	}
	saved, _ := svc.Room(room.ID)
	if saved.Messages[len(saved.Messages)-2].Metadata["relay_pending_response_id"] != "resp_existing" {
		t.Fatal("response identity was not persisted")
	}
}

func TestCloudTaskRestoresLostContextInSameRoom(t *testing.T) {
	const original = "task:room:followup"
	const recovery = "task-recovery:room:followup"
	const next = "task:room:next"
	type exchange struct {
		request string // POST idempotency key or GET path
		reply   func(http.ResponseWriter)
	}
	poll := func(id string) string { return "/v1/responses/" + id }
	rotate := func(key, id string) string {
		return fmt.Sprintf("task-recovery:%x", sha256.Sum256([]byte(key+"\x00"+id)))
	}
	failure := func(status int, code, id string) func(http.ResponseWriter) {
		return func(w http.ResponseWriter) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": relay.ProtocolError{Code: code, ResponseID: id}})
		}
	}
	response := func(id, status, code string) func(http.ResponseWriter) {
		return func(w http.ResponseWriter) {
			result := relay.Response{ID: id, Model: "agt_cloud", Status: status}
			if code != "" {
				result.Error = &relay.ProtocolError{Code: code}
			}
			_ = json.NewEncoder(w).Encode(result)
		}
	}
	missing := failure(404, "response_not_found", "")
	offline := failure(503, "agent_offline", "")
	restored := response("resp_restored", "completed", "")
	rotatedOriginal := rotate(original, "resp_admitted")
	rotatedBusy := rotate(recovery, "resp_busy")
	rotatedOffline := rotate(recovery, "resp_offline")
	recoveryRejections := map[string]string{rotatedBusy: "resp_busy", rotatedOffline: "resp_offline"}
	for _, tc := range []struct {
		name, pending        string
		restore, old, legacy bool
		before, after        []exchange
	}{
		{name: "missing parent", restore: true, old: true, before: []exchange{{original, missing}}},
		{
			name: "missing checkpoint", pending: "resp_pending", restore: true, old: true,
			before: []exchange{{poll("resp_pending"), response("resp_pending", "failed", "relay_branch_unsupported")}},
			after: []exchange{
				{recovery, failure(409, "agent_busy", "")},
				{recovery, failure(409, "agent_busy", "resp_busy")},
				{poll("resp_busy"), response("resp_busy", "failed", "agent_busy")},
				{rotatedBusy, restored},
			},
		},
		{
			name: "missing submitted result", before: []exchange{
				{original, func(w http.ResponseWriter) { w.Header().Set("X-Response-Id", "resp_admitted"); missing(w) }},
				{poll("resp_admitted"), missing},
			},
		},
		{name: "missing polled result", pending: "resp_pending", before: []exchange{{poll("resp_pending"), missing}}},
		{
			name: "persistent invalid poll responses", pending: "resp_pending", old: true,
			before: []exchange{
				{poll("resp_pending"), func(w http.ResponseWriter) { _, _ = fmt.Fprint(w, "{}") }},
				{poll("resp_pending"), func(w http.ResponseWriter) { _, _ = fmt.Fprint(w, "{") }},
				{poll("resp_pending"), response("resp_pending", "", "")},
			},
		},
		{name: "lost response", before: []exchange{{original, nil}, {original, offline}, {original, missing}}},
		{name: "lost response before restart", before: []exchange{{original, nil}}, after: []exchange{{original, missing}}},
		{name: "legacy pending message", legacy: true, before: []exchange{{original, missing}}},
		{name: "agent offline", restore: true, before: []exchange{{original, offline}, {original, missing}}},
		{
			name: "agent offline with response ID", restore: true,
			before: []exchange{
				{original, failure(503, "agent_offline", "resp_admitted")},
				{poll("resp_admitted"), response("resp_admitted", "failed", "agent_offline")},
				{rotatedOriginal, missing},
			},
		},
		{
			name: "agent unhealthy", restore: true,
			before: []exchange{{original, failure(503, "agent_unhealthy", "")}, {original, missing}},
		},
		{
			name: "lost offline response", before: []exchange{
				{original, failure(503, "agent_offline", "resp_admitted")}, {poll("resp_admitted"), missing},
			},
		},
		{
			name: "recorded recovery offline", restore: true, before: []exchange{{original, missing}},
			after: []exchange{
				{recovery, failure(503, "agent_offline", "resp_offline")},
				{poll("resp_offline"), response("resp_offline", "failed", "agent_offline")},
				{rotatedOffline, restored},
			},
		},
		{
			name: "truncated recovery response", restore: true, before: []exchange{{original, missing}},
			after: []exchange{
				{recovery, func(w http.ResponseWriter) {
					w.Header().Set("X-Response-Id", "resp_restored")
					_, _ = fmt.Fprint(w, "{")
				}},
				{poll("resp_restored"), response("resp_restored", "", "")},
				{poll("resp_restored"), restored},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, after := tc.before, tc.after
			if tc.restore {
				before = append(before, exchange{recovery, nil})
				if len(after) == 0 {
					after = []exchange{{recovery, restored}}
				}
				after = append(after, exchange{next, response("resp_next", "completed", "")})
			}
			script := append(before, after...)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "state.json")
			var calls atomic.Int32
			var firstRecoveryInput atomic.Value
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/agents":
					_ = json.NewEncoder(w).Encode(relay.Publication{Agent: relay.Agent{ID: "agt_cloud"}})
					return
				case "/v1/agents/agt_cloud":
					_ = json.NewEncoder(w).Encode(relay.Agent{ID: "agt_cloud", Status: "online"})
					return
				case "/v1/agents/agt_cloud/connect":
					w.WriteHeader(http.StatusNotFound)
					return
				}
				request := r.URL.Path
				if r.Method == http.MethodPost && request == "/v1/responses" {
					request = r.Header.Get("Idempotency-Key")
				}
				index := int(calls.Add(1)) - 1
				if index >= len(script) || script[index].request != request {
					t.Errorf("unexpected Relay request %d: %s", index, request)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.Method == http.MethodPost {
					var req relay.CallRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					var input string
					_ = json.Unmarshal(req.Input, &input)
					switch request {
					case original, rotatedOriginal:
						if req.PreviousResponseID != "resp_old" || input != "Where is it?" {
							t.Errorf("unexpected continuation: parent %q, input %q", req.PreviousResponseID, input)
						}
					case next:
						if req.PreviousResponseID != "resp_restored" || input != "Future message" {
							t.Errorf("unexpected next turn: parent %q, input %q", req.PreviousResponseID, input)
						}
					default:
						if req.PreviousResponseID != "" || !strings.Contains(input, "Remember the report") ||
							!strings.Contains(input, "Saved in the project folder") || !strings.HasSuffix(input, "Where is it?") ||
							strings.Contains(input, "Failed request") || strings.Contains(input, "Future message") {
							t.Errorf("unexpected recovery input: parent %q, input %q", req.PreviousResponseID, input)
						}
						if first := firstRecoveryInput.Load(); first == nil {
							firstRecoveryInput.Store(input)
						} else if first != input {
							t.Error("recovery input changed after restart or retry")
						}
					}
					if rejectedID := recoveryRejections[request]; rejectedID != "" {
						state, err := im.LoadBootstrap(path)
						if err != nil || len(state.Rooms) != 1 {
							t.Errorf("could not read persisted recovery state: %v", err)
						} else if pending, _, found := cloudTaskPending(state.Rooms[0]); !found ||
							pending.Metadata["relay_context_recovery_after"] != rejectedID ||
							pending.Metadata["relay_pending_response_id"] != nil {
							t.Error("replacement recovery attempt was not saved before submission")
						}
					}
				}
				if reply := script[index].reply; reply != nil {
					reply(w)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				if len(after) != 0 && index+1 == len(before) {
					cancel()
				}
				_ = conn.Close()
			}))
			defer remote.Close()
			m, err := relay.NewManager(t.Context(), t.TempDir(), nil)
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
			message := im.Message{ID: "followup", SenderID: im.AdminUserID, Content: "Where is it?", CreatedAt: time.Now()}
			switch {
			case tc.pending != "":
				message.Metadata = map[string]any{"relay_pending_response_id": tc.pending}
			case !tc.legacy:
				message.Metadata = map[string]any{cloudTaskUnsubmittedKey: true}
			}
			if tc.old {
				message.CreatedAt = time.Now().Add(-11 * time.Minute)
			}
			room := im.Room{
				ID: "room", CreatedBy: im.AdminUserID, Members: []string{im.AdminUserID, "cloud"},
				CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", ConnectionID: m.Catalog(ctx)[0].ConnectionID, UserID: "cloud"},
				Messages: []im.Message{
					{ID: "prior", SenderID: im.AdminUserID, Content: "Remember the report"},
					{ID: "failed", SenderID: im.AdminUserID, Content: "Failed request"},
					message,
					{ID: "prior-reply", SenderID: "cloud", ClientMessageID: "relay-reply:prior", Content: "Saved in the project folder", Metadata: map[string]any{"relay_response_id": "resp_old"}},
					{ID: "failed-reply", SenderID: "cloud", ClientMessageID: "relay-reply:failed", Content: "Failed", Metadata: map[string]any{"relay_task_failed": true}},
				},
			}
			if tc.restore {
				room.Messages = append(room.Messages, im.Message{ID: "next", SenderID: im.AdminUserID, Content: "Future message", CreatedAt: time.Now()})
			}
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
			if len(after) != 0 {
				if int(calls.Load()) != len(before) {
					t.Fatal("did not stop at the expected restart point")
				}
				svc, err = im.NewServiceFromPath(path)
				if err != nil {
					t.Fatal(err)
				}
				saved, _ := svc.Room(room.ID)
				pending, _, found := cloudTaskPending(saved)
				if !found || pending.Metadata[cloudTaskUnsubmittedKey] != nil || pending.Metadata["relay_pending_response_id"] != nil {
					t.Fatal("submission checkpoint did not survive restart")
				}
				if tc.restore && (pending.Metadata["relay_context_recovery_input"] == nil || pending.Metadata["relay_context_recovery_started_at"] == nil) {
					t.Fatal("recovery checkpoint was not saved before submission")
				}
				resumed, stop := context.WithTimeout(t.Context(), 10*time.Second)
				defer stop()
				h = &Handler{im: svc, relayManager: m, relayContext: resumed}
				h.runCloudTask(room.ID)
			}
			if int(calls.Load()) != len(script) {
				t.Fatalf("Relay requests = %d, want %d", calls.Load(), len(script))
			}
			saved, _ := svc.Room(room.ID)
			if tc.restore {
				if len(saved.Messages) != 8 || saved.Messages[6].Metadata["relay_response_id"] != "resp_restored" ||
					saved.Messages[7].Metadata["relay_response_id"] != "resp_next" {
					t.Fatal("recovered turn or its follow-up did not finish in the original room")
				}
			} else if len(saved.Messages) != 6 || saved.Messages[5].Metadata["relay_error_code"] != "relay_task_result_unavailable" {
				t.Fatal("did not report the unknown execution outcome")
			}
		})
	}
}

func TestCloudTaskRecoveryInputSkipsOversizedTurns(t *testing.T) {
	message := im.Message{ID: "current", SenderID: im.AdminUserID, Content: "What is the project called?"}
	room := im.Room{
		CreatedBy:  im.AdminUserID,
		CloudAgent: &apitypes.CloudAgentReference{UserID: "cloud"},
		Messages: []im.Message{
			{ID: "prior", SenderID: im.AdminUserID, Content: "The project is called Oak."},
			{ID: "prior-reply", SenderID: "cloud", ClientMessageID: "relay-reply:prior", Content: "Understood.", Metadata: map[string]any{"relay_response_id": "resp_prior"}},
			{ID: "report", SenderID: im.AdminUserID, Content: "Print the detailed report."},
			{ID: "report-reply", SenderID: "cloud", ClientMessageID: "relay-reply:report", Metadata: map[string]any{"relay_response_id": "resp_report"}},
			message,
		},
	}
	for _, content := range []string{strings.Repeat("report ", 3000), strings.Repeat("<", 3000)} {
		room.Messages[3].Content = content
		input := cloudTaskRecoveryInput(room, message)
		if !strings.Contains(input, "The project is called Oak.") || !strings.Contains(input, "Understood.") ||
			strings.Contains(input, "Print the detailed report.") || !strings.HasSuffix(input, message.Content) {
			t.Fatal("recovery did not retain the earlier complete turn within the size budget")
		}
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
