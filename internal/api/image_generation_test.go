package api

import (
	"bytes"
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/modelprovider"
	"csgclaw/internal/participant"
	"encoding/base64"
	"image"
	"image/png"
	"path/filepath"

	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
)

func TestImageModelProfileHTTPRoundTripKeepsChatModel(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{{ID: "u-alice", Name: "Alice", Role: agent.RoleWorker, AgentProfile: agent.AgentProfile{Provider: agent.ProviderCodex, ModelProviderID: "codex", ModelID: "chat-model"}}})
	h := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	for _, image := range []string{`{"provider_id":"codex","model_id":"gpt-image-2"}`, `null`} {
		body := `{"model_provider_id":"codex","model_id":"chat-model","image_generation":` + image + `}`
		for _, method := range []string{http.MethodPut, http.MethodGet} {
			recorder := httptest.NewRecorder()
			h.Routes().ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/agents/alice/profile", strings.NewReader(body)))
			if recorder.Code != 200 {
				t.Fatalf("%s: %d %s", method, recorder.Code, recorder.Body.String())
			}
			var profile apitypes.AgentProfile
			if err := json.Unmarshal(recorder.Body.Bytes(), &profile); err != nil {
				t.Fatal(err)
			}
			if profile.ModelID != "chat-model" || (profile.ImageGeneration == nil) != (image == "null") {
				t.Fatalf("profile did not round trip: %+v", profile)
			}
		}
	}
}

func TestImageRetryHTTPUsesCapturedModelAndPersistsAttachment(t *testing.T) {
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fail := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("wrong endpoint %s", r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["prompt"] != "exact captured prompt" || request["model"] != "vendor-image" || request["response_format"] != "b64_json" || request["output_format"] != nil {
			t.Errorf("retry changed original request: %v", request)
		}
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "moderation_blocked", "message": "The image request was rejected by the provider safety system.", "moderation_details": map[string]string{"moderation_stage": "output"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(pixels.Bytes())}}})
	}))
	defer provider.Close()
	svc := mustNewSeededService(t, []agent.Agent{{ID: "agent-alice", Name: "Alice", Role: agent.RoleWorker, AgentProfile: agent.AgentProfile{Provider: agent.ProviderCodex, ModelID: "chat-model"}}})
	svc.Models().SetLLMConfig(config.LLMConfig{Providers: map[string]config.ProviderConfig{"fixture": {BaseURL: provider.URL + "/v1", APIKey: "test-secret", ImageModels: []string{"vendor-image"}}}})
	engine := agentengine.New(svc)
	imSvc, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-alice", Name: "Alice", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := imSvc.CreateRoom(im.CreateRoomRequest{Title: "Image retry", CreatorID: "user-admin", MemberIDs: []string{"user-alice"}})
	if err != nil {
		t.Fatal(err)
	}
	participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-alice", Channel: "csgclaw", Type: participant.TypeAgent, AgentID: "agent-alice", ChannelUserRef: "user-alice", ChannelUserKind: participant.ChannelUserKindLocalUserID, LifecycleStatus: participant.LifecycleStatusActive}}))
	store, err := delivery.NewIMTranscriptStore(imSvc, participants, engine)
	if err != nil {
		t.Fatal(err)
	}
	task := contract.ImageGenerationTask{ID: "call-image", Prompt: "exact captured prompt", State: "failed", Model: &modelprovider.ImageGenerationConfig{ProviderID: "fixture", ModelID: "vendor-image"}, Error: "image_generation_failed"}
	turn := channel.TurnContext{AgentID: "agent-alice", ParticipantID: "pt-alice", RoomID: room.ID, ConversationKey: "room-image", TurnID: "turn-original"}
	if err := store.DeliverImageGeneration(context.Background(), turn, task); err != nil {
		t.Fatal(err)
	}
	messages, _ := imSvc.ListMessages(room.ID)
	messageID := messages[len(messages)-1].ID
	h := &Handler{im: imSvc, participant: participants, agentEngine: engine}
	route := "/api/v1/messages/" + messageID + "/image-generation/retry"
	for attempt, wantStatus := range []int{http.StatusOK, http.StatusOK, http.StatusConflict} {
		response := httptest.NewRecorder()
		h.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"room_id":"`+room.ID+`"}`)))
		if response.Code != wantStatus {
			t.Fatalf("retry HTTP %d, want %d: %s", response.Code, wantStatus, response.Body.String())
		}
		if attempt == 0 {
			var outcome agentengine.TurnResult
			if err := json.Unmarshal(response.Body.Bytes(), &outcome); err != nil {
				t.Fatal(err)
			}
			if outcome.Status != agentengine.TurnFailed {
				t.Fatalf("expected provider failure: %s", response.Body.String())
			}
			listed := httptest.NewRecorder()
			h.Routes().ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/api/v1/messages?room_id="+room.ID, nil))
			if listed.Code != http.StatusOK {
				t.Fatalf("read failed task: %s", listed.Body.String())
			}
			var saved []im.Message
			if err := json.Unmarshal(listed.Body.Bytes(), &saved); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(saved[len(saved)-1].Metadata["image_generation"])
			var failedTask contract.ImageGenerationTask
			_ = json.Unmarshal(raw, &failedTask)
			if failedTask.ErrorDetails == nil || failedTask.ErrorDetails.Code != "moderation_blocked" || failedTask.ErrorDetails.HTTPStatus != 400 || failedTask.ErrorDetails.Stage != "output" || failedTask.Error != "image_generation_output_blocked" {
				t.Fatalf("provider reason missing from transcript: %+v", failedTask)
			}
			if failedTask.State != "failed" {
				t.Fatalf("failed image remains %s", failedTask.State)
			}
			fail = false
			continue
		}
		if wantStatus == 200 && !strings.Contains(response.Body.String(), `"status":"succeeded"`) {
			t.Fatalf("retry failed: %s", response.Body.String())
		}
	}
	messages, _ = imSvc.ListMessages(room.ID)
	if calls != 2 || len(messages) != 2 || len(messages[1].Attachments) != 1 {
		t.Fatalf("calls=%d, messages=%d, output=%+v", calls, len(messages), messages)
	}
	current, ok := svc.Agent("agent-alice")
	if !ok || current.AgentProfile.ModelID != "chat-model" {
		t.Fatal("retry changed chat model")
	}
}

func TestImageModelProviderReferenceCountsAsInUse(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{{ID: "agent-alice", Name: "Alice", Role: agent.RoleWorker, AgentProfile: agent.AgentProfile{Provider: agent.ProviderCodex, ModelID: "chat-model", ImageGeneration: &modelprovider.ImageGenerationConfig{ProviderID: "image-provider", ModelID: "vendor-image"}}}})
	h := &Handler{agentEngine: agentengine.New(svc)}
	if !h.modelProviderInUse(config.LLMConfig{}, "image-provider") {
		t.Fatal("image provider could be deleted while an Agent references it")
	}
}
