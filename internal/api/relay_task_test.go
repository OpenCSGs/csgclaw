package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	"csgclaw/internal/relay"
)

func newCloudTaskTestRelay(t *testing.T, ctx context.Context, agentID string, responses http.HandlerFunc) (*relay.Manager, string) {
	t.Helper()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/agents":
			_ = json.NewEncoder(w).Encode(relay.Publication{Agent: relay.Agent{ID: agentID}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/"+agentID:
			_ = json.NewEncoder(w).Encode(relay.Agent{ID: agentID, Status: "online"})
		case strings.HasPrefix(r.URL.Path, "/v1/responses"):
			responses(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(remote.Close)
	manager, err := relay.NewManager(t.Context(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if err := manager.Configure(remote.URL, "test-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Publish(ctx, relay.PublishRequest{SourceAgentID: "local", Visibility: "public"}); err != nil {
		t.Fatal(err)
	}
	entries := manager.Catalog(ctx)
	if len(entries) != 1 {
		t.Fatalf("expected one published Relay agent, got %d", len(entries))
	}
	return manager, entries[0].ConnectionID
}

func cloudTaskTestBootstrap(room im.Room) im.Bootstrap {
	room.CreatedBy = im.AdminUserID
	room.Members = []string{im.AdminUserID, room.CloudAgent.UserID}
	return im.Bootstrap{
		CurrentUserID: im.AdminUserID,
		Users: []im.User{
			{ID: im.AdminUserID, Name: "admin", Role: "admin"},
			{ID: room.CloudAgent.UserID, Name: "cloud", Role: "cloud"},
		},
		Rooms: []im.Room{room},
	}
}

func newPersistedCloudTaskTestService(t *testing.T, path string, room im.Room) *im.Service {
	t.Helper()
	if err := im.SaveBootstrap(path, cloudTaskTestBootstrap(room)); err != nil {
		t.Fatal(err)
	}
	svc, err := im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestCloudTaskRecoversKnownResponseAfterRestart(t *testing.T) {
	var calls atomic.Int32
	var polls atomic.Int32
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	m, connectionID := newCloudTaskTestRelay(t, ctx, "agt_cloud", func(w http.ResponseWriter, r *http.Request) {
		switch {
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
	})
	// Keep the on-disk field name literal to verify compatibility on restart.
	room := im.Room{ID: "cloud-task", Title: "Task", TaskSession: true, IsDirect: true, CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", ConnectionID: connectionID, UserID: "relay-user"}, Messages: []im.Message{{ID: "pending", SenderID: im.AdminUserID, Content: "hello", CreatedAt: time.Now().Add(-time.Hour), Metadata: map[string]any{"relay_pending_response_id": "resp_saved"}}}}
	path := filepath.Join(t.TempDir(), "state.json")
	svc := newPersistedCloudTaskTestService(t, path, room)
	h := &Handler{im: svc, relayManager: m, relayContext: ctx}
	h.startCloudTask(room.ID)
	h.startCloudTask(room.ID)
	done := make(chan struct{})
	go func() { h.cloudTaskWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("recovery timed out")
	}
	saved, _ := svc.Room(room.ID)
	if len(saved.Messages) != 2 || saved.Messages[1].Content != "recovered result" {
		t.Fatal("recovery failed")
	}
	if _, _, pending := cloudTaskPending(saved); pending {
		t.Fatal("reply did not consume pending message")
	}
	if got := polls.Load(); got != 2 {
		t.Fatalf("expected one poll and one retry, got %d polls", got)
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

func TestCloudTaskResumesAfterTemporaryWriteFailure(t *testing.T) {
	for _, phase := range []string{"checkpoint", "reply"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			const roomID = "cloud-task"
			path := filepath.Join(t.TempDir(), "state.json")
			submitted := make(chan struct{}, 2)
			var submissions atomic.Int32
			manager, connectionID := newCloudTaskTestRelay(t, ctx, "agt_cloud", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
					http.NotFound(w, r)
					return
				}
				submissions.Add(1)
				select {
				case submitted <- struct{}{}:
				default:
				}
				_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_done", Model: "agt_cloud", Status: "completed"})
			})
			svc := newPersistedCloudTaskTestService(t, path, im.Room{
				ID:         roomID,
				CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", UserID: "cloud", ConnectionID: connectionID},
				Messages: []im.Message{{
					ID: "pending", SenderID: im.AdminUserID, Content: "Continue", CreatedAt: time.Now(),
					Metadata: map[string]any{cloudTaskUnsubmittedKey: true},
				}},
			})
			// The reply writes the room index, while the checkpoint only writes
			// the room's message log. Block the file needed for this phase.
			blockedPath := path
			if phase == "checkpoint" {
				blockedPath = filepath.Join(filepath.Dir(path), "sessions", roomID+".jsonl")
			}
			if err := os.Rename(blockedPath, blockedPath+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(blockedPath, 0o700); err != nil {
				t.Fatal(err)
			}
			h := &Handler{im: svc, relayManager: manager, relayContext: ctx}
			h.startCloudTask(roomID)
			done := make(chan struct{})
			go func() { h.cloudTaskWG.Wait(); close(done) }()
			defer func() { cancel(); <-done }()
			if phase == "reply" {
				select {
				case <-submitted:
				case <-ctx.Done():
					t.Fatal("task did not submit before the reply write")
				}
			}
			select {
			case <-done:
				t.Fatal("worker exited on a temporary write failure")
			case <-submitted:
				t.Fatal("task submitted with a blocked checkpoint or repeated its POST")
			case <-time.After(100 * time.Millisecond):
			}
			if err := os.Remove(blockedPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(blockedPath+".saved", blockedPath); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("worker did not resume after storage was restored")
			}
			room, _ := svc.Room(roomID)
			if submissions.Load() != 1 || len(room.Messages) != 2 || room.Messages[1].Metadata["relay_response_id"] != "resp_done" {
				t.Fatalf("expected one submission and its reply, got %d submissions and %d messages", submissions.Load(), len(room.Messages))
			}
		})
	}
}

func TestCloudTaskReportsExpiredRecoveryAsUnconfirmed(t *testing.T) {
	const roomID = "cloud-task"
	now := time.Now()
	svc := im.NewServiceFromBootstrap(cloudTaskTestBootstrap(im.Room{
		ID: roomID, Locale: "en",
		CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", UserID: "cloud"},
		Messages: []im.Message{{
			ID: "pending", SenderID: im.AdminUserID, Content: "Continue", CreatedAt: now.Add(-time.Hour),
			Metadata: map[string]any{
				cloudTaskRecoveryKey:      "Saved recovery input",
				cloudTaskRecoveryStartKey: now.Add(-11 * time.Minute).Format(time.RFC3339Nano),
			},
		}},
	}))
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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	m, connection := newCloudTaskTestRelay(t, ctx, "agt_review", func(w http.ResponseWriter, r *http.Request) {
		switch {
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
	})
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
	if saved.Messages[len(saved.Messages)-2].Metadata[cloudTaskResponseKey] != "resp_existing" {
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
	unauthorized := failure(401, "unauthorized", "")
	restored := response("resp_restored", "completed", "")
	rotatedOriginal := rotate(original, "resp_admitted")
	rotatedBusy := rotate(recovery, "resp_busy")
	rotatedOffline := rotate(recovery, "resp_offline")
	recoveryRejections := map[string]string{rotatedBusy: "resp_busy", rotatedOffline: "resp_offline"}
	for _, tc := range []struct {
		name, pending        string
		restore, old, legacy bool
		before, after        []exchange
		errorCode            string
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
		{name: "unauthorized poll", pending: "resp_pending", before: []exchange{{poll("resp_pending"), unauthorized}}},
		{name: "interrupted execution", pending: "resp_pending", before: []exchange{{poll("resp_pending"), response("resp_pending", "failed", "execution_interrupted")}}},
		{name: "execution timeout", pending: "resp_pending", before: []exchange{{poll("resp_pending"), response("resp_pending", "failed", "execution_timeout")}}},
		{name: "unconfirmed cancellation", pending: "resp_pending", before: []exchange{{poll("resp_pending"), response("resp_pending", "failed", "cancellation_unconfirmed")}}},
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
		{name: "unauthorized retry after lost response", before: []exchange{{original, nil}, {original, unauthorized}}},
		{name: "unauthorized retry after restart", before: []exchange{{original, nil}}, after: []exchange{{original, unauthorized}}},
		{name: "unauthorized first submission", before: []exchange{{original, unauthorized}}, errorCode: "execution_failed"},
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
		{
			name: "recovery response without model", restore: true, before: []exchange{{original, missing}},
			after: []exchange{
				{recovery, func(w http.ResponseWriter) {
					_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_restored", Status: "completed"})
				}},
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
			m, connectionID := newCloudTaskTestRelay(t, ctx, "agt_cloud", func(w http.ResponseWriter, r *http.Request) {
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
							pending.Metadata[cloudTaskRecoveryRetryKey] != rejectedID ||
							pending.Metadata[cloudTaskResponseKey] != nil {
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
			})
			message := im.Message{ID: "followup", SenderID: im.AdminUserID, Content: "Where is it?", CreatedAt: time.Now()}
			switch {
			case tc.pending != "":
				message.Metadata = map[string]any{cloudTaskResponseKey: tc.pending}
			case !tc.legacy:
				message.Metadata = map[string]any{cloudTaskUnsubmittedKey: true}
			}
			if tc.old {
				message.CreatedAt = time.Now().Add(-11 * time.Minute)
			}
			room := im.Room{
				ID:         "room",
				CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt_cloud", ConnectionID: connectionID, UserID: "cloud"},
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
			svc := newPersistedCloudTaskTestService(t, path, room)
			h := &Handler{im: svc, relayManager: m, relayContext: ctx}
			h.runCloudTask(room.ID)
			if len(after) != 0 {
				if int(calls.Load()) != len(before) {
					t.Fatal("did not stop at the expected restart point")
				}
				reloaded, err := im.NewServiceFromPath(path)
				if err != nil {
					t.Fatal(err)
				}
				svc = reloaded
				saved, _ := svc.Room(room.ID)
				pending, _, found := cloudTaskPending(saved)
				if !found || pending.Metadata[cloudTaskUnsubmittedKey] != nil || pending.Metadata[cloudTaskResponseKey] != nil {
					t.Fatal("submission checkpoint did not survive restart")
				}
				if tc.restore && (pending.Metadata[cloudTaskRecoveryKey] == nil || pending.Metadata[cloudTaskRecoveryStartKey] == nil) {
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
				return
			}
			wantCode := tc.errorCode
			if wantCode == "" {
				wantCode = "relay_task_result_unavailable"
			}
			if len(saved.Messages) != 6 {
				t.Fatalf("expected one failure reply, got %d messages", len(saved.Messages))
			}
			reply := saved.Messages[5]
			if got := reply.Metadata["relay_error_code"]; got != wantCode || reply.Content != cloudTaskFailureText(saved.Locale, wantCode) {
				t.Fatalf("expected %s failure reply, got %v: %q", wantCode, got, reply.Content)
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
	room := im.Room{ID: "cloud-shutdown", Title: "Pending", CreatedBy: im.AdminUserID, Members: []string{im.AdminUserID, "user-cloud"}, CloudAgent: &apitypes.CloudAgentReference{AgentID: "agt", ConnectionID: "conn", UserID: "user-cloud"}, Messages: []im.Message{{ID: "job", SenderID: im.AdminUserID, Content: "Work", CreatedAt: time.Now(), Metadata: map[string]any{cloudTaskResponseKey: "resp_pending"}}}}
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
	var slowLookup atomic.Bool
	var delayedLookups, submissions atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agent/instances":
			if r.URL.Query().Get("registered") != "true" {
				t.Error("missing registered discovery filter")
			}
			if slowLookup.Load() {
				delayedLookups.Add(1)
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Second):
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": 42, "kind": "runtime", "source": "external", "type": "csgclaw", "public": true, "name": "Remote worker", "relay": map[string]string{"agent_id": id}}}, "total": 1})
		case "/api/v1/relay/v1/agents/" + id:
			_ = json.NewEncoder(w).Encode(relay.Agent{ID: id, Name: "Remote worker", Status: "online", Visibility: "public"})
		case "/api/v1/relay/v1/responses":
			submissions.Add(1)
			_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_late", Model: id, Status: "completed"})
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
	t.Run("lookup cannot outlast submission window", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
		defer cancel()
		slowLookup.Store(true)
		now := time.Now()
		message := im.Message{
			ID: "pending", SenderID: im.AdminUserID, Content: "Continue", CreatedAt: now,
			Metadata: map[string]any{
				cloudTaskRecoveryKey:      "Saved recovery input",
				cloudTaskRecoveryStartKey: now.Add(-10*time.Minute + 500*time.Millisecond).Format(time.RFC3339Nano),
			},
		}
		room := im.Room{ID: "community-task", CloudAgent: req.CloudAgent, Messages: []im.Message{message}}
		svc := im.NewServiceFromBootstrap(cloudTaskTestBootstrap(room))
		h := &Handler{im: svc, relayManager: m}
		_, err := h.awaitCloudTask(ctx, room, message, "")
		if delayedLookups.Load() != 1 || submissions.Load() != 0 || !errors.Is(err, errCloudTaskResultUnavailable) {
			t.Fatalf("lookups=%d submissions=%d error=%v", delayedLookups.Load(), submissions.Load(), err)
		}
	})
}
