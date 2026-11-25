package api

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/relay"
	agentruntime "csgclaw/internal/runtime"
	hub "csgclaw/internal/template"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestCreateCSGClawAgentParticipantViaAPI(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	agentSvc.Models().SetLLMConfig(config.LLMConfig{Default: "service.chat", Providers: map[string]config.ProviderConfig{"service": {BaseURL: "https://model.example/v1", APIKey: "test-key", Models: []string{"chat"}, ImageModels: []string{"image"}, VideoModels: []string{"video"}}}})
	imSvc := im.NewService()
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentEngine(agentengine.New(agentSvc)),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		participant: participantSvc, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}

	body := `{
		"id": "qa",
		"type": "agent",
		"name": "QA-Display-Name",
		"avatar": "avatar/3D-5.png",
		"channel_user": {
			"ref": "u-qa",
			"kind": "local_user_id"
		},
		"agent_binding": {
			"mode": "create",
			"agent": {
				"name": "QA-Display-Name",
				"role": "worker",
				"runtime_kind": "codex",
				"image": "agent-image:test",
                "model_config": {"model_provider_id":"service","model_id":"chat","image_generation":{"provider_id":"service","model_id":"image"},"video_generation":{"provider_id":"service","model_id":"video"}},
				"avatar": "avatar/3D-5.png"
			}
		}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(body))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created apitypes.Participant
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != "pt-qa" || created.Channel != "csgclaw" || created.Type != "agent" || created.AgentID != "agent-qa" {
		t.Fatalf("created participant = %+v, want csgclaw agent qa bound to agent-qa", created)
	}
	if created.Avatar != "" {
		t.Fatalf("participant avatar = %q, want empty", created.Avatar)
	}
	createdAgent, ok := agentSvc.Agent("agent-qa")
	if !ok {
		t.Fatal("agent agent-qa was not created")
	}
	profile := createdAgent.AgentProfile
	if profile.ImageGeneration == nil || profile.ImageGeneration.ModelID != "image" || profile.VideoGeneration == nil || profile.VideoGeneration.ModelID != "video" {
		t.Fatalf("participant creation discarded generation models: %+v", profile)
	}
	defaults := agentSvc.Models().ProfileDefaultsView()
	if defaults.ImageGeneration == nil || defaults.ImageGeneration.ModelID != "image" || defaults.VideoGeneration == nil || defaults.VideoGeneration.ModelID != "video" {
		t.Fatalf("creation did not remember all model choices: %+v", defaults)
	}

	if createdAgent.Avatar != "" {
		t.Fatalf("agent avatar = %q, want empty", createdAgent.Avatar)
	}
	if _, ok := agentSvc.Agent("u-qa-display-name"); ok {
		t.Fatal("agent ID was derived from display name")
	}
	if user, ok := imSvc.User("u-qa"); !ok || !strings.EqualFold(user.Name, "QA-Display-Name") {
		t.Fatalf("channel user = %+v, ok=%v; want u-qa display user", user, ok)
	} else if user.Avatar == "" {
		t.Fatalf("channel user avatar is empty, want user-owned default")
	}
}

func TestCreateCSGClawAgentParticipantViaAPIUsesRequestHub(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	requestHub := mustNewLocalTemplateHubService(t, "request-worker", hub.Template{
		ID:          "request-worker",
		Name:        "request-worker",
		Description: "request-scoped template",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeNameCodex,
		Image:       "agent-image:request",
	})
	imSvc := im.NewService()
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentEngine(agentengine.New(agentSvc)),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		hub:         requestHub,
		im:          imSvc,
		participant: participantSvc, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	body := `{
		"id": "request-worker",
		"type": "agent",
		"name": "request-worker",
		"channel_user": {"ref": "u-request-worker", "kind": "local_user_id"},
		"agent_binding": {
			"mode": "create",
			"agent": {
				"name": "request-worker",
				"role": "worker",
				"from_template": "local.request-worker"
			}
		}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(body))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	created, ok := agentSvc.Agent("agent-request-worker")
	if !ok {
		t.Fatal("request-scoped template agent was not created")
	}
	if created.Description != "request-scoped template" || created.Image != "" {
		t.Fatalf("created agent = %+v, want request-scoped template defaults", created)
	}
}

func TestRetryExplicitParticipantBinding(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	store := participant.NewMemoryStore(nil)
	engine := agentengine.New(agentSvc)
	imSvc := im.NewService()
	srv := &Handler{
		svc: agentSvc, im: imSvc, agentEngine: engine, workspace: agentSvc.Workspace(),
		agentModels: agentSvc.Models(), agentRuntime: agentSvc,
		participant: participant.NewService(store, participant.WithAgentEngine(engine), participant.WithIMService(imSvc)),
		hub: mustNewLocalTemplateHubService(t, "request-worker", hub.Template{
			ID: "request-worker", Name: "request-worker", Role: hub.TemplateRoleWorker,
			RuntimeKind: agent.RuntimeNameCodex, Image: "agent-image:test",
		}),
	}
	routes := srv.Routes()
	body := `{"id":"pt-qa","name":"QA","type":"agent","agent_binding":{"mode":"create","agent_id":"agent-qa","agent":{"from_template":"local.request-worker"}}}`
	post := func(payload string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(payload)))
		return rec
	}
	results := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { results <- post(body) }()
	}
	first, second := <-results, <-results
	if first.Code == http.StatusOK {
		first, second = second, first
	}
	if first.Code != http.StatusCreated || second.Code != http.StatusOK || first.Body.String() != second.Body.String() {
		t.Fatalf("concurrent create: %d %s; %d %s", first.Code, first.Body, second.Code, second.Body)
	}
	if rec := post(strings.Replace(body, "agent-qa", "agent-other", 1)); rec.Code != http.StatusOK || rec.Body.String() != first.Body.String() {
		t.Fatalf("historical binding: %d %s", rec.Code, rec.Body)
	}
	alias := strings.NewReplacer("pt-qa", "pt-alias", `"name":"QA"`, `"name":"Alias"`).Replace(body)
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		rec := post(alias)
		var p apitypes.Participant
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || rec.Code != status ||
			p.AgentID != "agent-qa" || p.AgentName != "QA" || p.Name != "Alias" || p.UserName != "Alias" {
			t.Fatalf("shared worker binding: %d %s", rec.Code, rec.Body)
		}
	}
	renamed := strings.Replace(body, `"name":"QA"`, `"name":"Renamed"`, 1)
	rec := post(strings.Replace(renamed, `"agent_id":"agent-qa",`, "", 1))
	var p apitypes.Participant
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || rec.Code != http.StatusOK ||
		p.Name != "Renamed" || p.UserName != p.Name || p.AgentName != p.Name || imSvc.ResolveUserID("Renamed") != "user-qa" {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if rec := post(`{"id":"pt-human","type":"human","name":"Reviewer"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create human: %d %s", rec.Code, rec.Body)
	}
	for _, taken := range []string{"reviewer", "user-human"} {
		if rec := post(strings.Replace(renamed, "Renamed", taken, 1)); rec.Code != http.StatusConflict ||
			!strings.Contains(rec.Body.String(), "agent_name_conflict") {
			t.Fatalf("IM name conflict: %d %s", rec.Code, rec.Body)
		}
	}
	stored, _ := store.Get(participant.ChannelCSGClaw, "pt-qa")
	if stored.Name != "Renamed" || imSvc.ResolveUserID("reviewer") != "user-human" || imSvc.ResolveUserID("Renamed") != "user-qa" {
		t.Fatal("name conflict changed the participant or IM name ownership")
	}
	if worker, ok := agentSvc.AgentByName("Renamed"); !ok || worker.ID != "agent-qa" {
		t.Fatal("name does not select the bound worker")
	}
	otherUser := strings.Replace(body, `"name":"QA"`, `"name":"QA","channel_user":{"ref":"user-other"}`, 1)
	if rec := post(otherUser); rec.Code != http.StatusConflict {
		t.Fatalf("identity conflict: %d %s", rec.Code, rec.Body)
	}
	if _, err := agentSvc.CreateRecord(t.Context(), agent.CreateRequest{Spec: agent.CreateAgentSpec{ID: "agent-taken", Name: "Taken", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeNameCodex, Image: "agent-image:test"}}); err != nil {
		t.Fatal(err)
	}
	if rec := post(strings.Replace(renamed, "Renamed", "Taken", 1)); rec.Code != http.StatusConflict {
		t.Fatalf("name conflict: %d %s", rec.Code, rec.Body)
	}
	afterConflict, _ := store.Get(participant.ChannelCSGClaw, "pt-qa")
	if afterConflict.Name != stored.Name || !afterConflict.UpdatedAt.Equal(stored.UpdatedAt) || imSvc.ResolveUserID("Renamed") != "user-qa" {
		t.Fatal("worker name conflict changed the participant or IM user")
	}
	if worker, ok := agentSvc.AgentByName("Renamed"); !ok || worker.ID != "agent-qa" {
		t.Fatal("conflict changed the bound worker")
	}
	worker, err := engine.Agents().Get(t.Context(), p.AgentID, agentengine.AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Agents().Update(t.Context(), worker.ID, agentengine.AgentUpdateRequest{
		Spec: agentengine.AgentSpec{Name: "PreviousWorker"}, FieldMask: []string{"name"}, ResourceVersion: worker.ResourceVersion,
	}); err != nil {
		t.Fatal(err)
	}
	if rec := post(renamed); rec.Code != http.StatusOK {
		t.Fatalf("restore worker name with another participant bound: %d %s", rec.Code, rec.Body)
	}
	if _, ok := agentSvc.AgentByName("Renamed"); !ok {
		t.Fatal("an unrelated alias prevented restoration of the worker name")
	}
	// Recover identities left under the old name when the worker already has
	// the requested name.
	previous := "Previous"
	if _, _, err := srv.participant.Update(t.Context(), participant.ChannelCSGClaw, "pt-qa", participant.UpdateRequest{Name: &previous}); err != nil {
		t.Fatal(err)
	}
	rec = post(renamed)
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || rec.Code != http.StatusOK ||
		p.Name != "Renamed" || p.UserName != p.Name || p.AgentName != p.Name || imSvc.ResolveUserID("Renamed") != "user-qa" {
		t.Fatalf("repair existing rename: %d %s", rec.Code, rec.Body)
	}
	if _, err := engine.Agents().SetArchived(context.Background(), "agent-qa", true); err != nil {
		t.Fatal(err)
	}
	if err := engine.Agents().Delete(context.Background(), "agent-qa"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.participant.RepairDanglingCSGClawAgentParticipants(); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get(participant.ChannelCSGClaw, "pt-qa"); !ok {
		t.Fatal("startup removed recoverable participant")
	}
	if user, ok := imSvc.User("user-qa"); !ok || user.AgentState == "deleted" {
		t.Fatal("startup retired recoverable user")
	}
	if _, err := agentSvc.CreateRecord(t.Context(), agent.CreateRequest{Spec: agent.CreateAgentSpec{ID: "agent-other", Name: "agent-qa", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeNameCodex, Image: "agent-image:test"}}); err != nil {
		t.Fatal(err)
	}
	if rec := post(body); rec.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	if _, ok := agentSvc.Agent("agent-qa"); !ok {
		t.Fatal("worker was not restored")
	}
}

type pausedAgentLookup struct {
	agentengine.AgentInterface
	resume chan struct{}
}

func (p pausedAgentLookup) Get(context.Context, string, agentengine.AgentGetOptions) (agentengine.Agent, error) {
	<-p.resume
	return agentengine.Agent{ID: "agent-qa", Spec: agentengine.AgentSpec{Name: "QA", Role: agentengine.AgentRoleWorker}}, nil
}

func TestRetryAndConcurrentParticipantPatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := participant.NewMemoryStore([]apitypes.Participant{{
			ID: "pt-qa", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, Name: "QA",
			AgentID: "agent-qa", ChannelUserRef: "user-qa", ChannelUserKind: participant.ChannelUserKindLocalUserID,
		}})
		imSvc := im.NewServiceFromBootstrap(im.Bootstrap{Users: []im.User{{ID: "user-qa", Name: "QA", Role: "worker"}}})
		resume := make(chan struct{})
		engine := getErrorEngine{agents: pausedAgentLookup{resume: resume}}
		svc := participant.NewService(store, participant.WithAgentEngine(engine), participant.WithIMService(imSvc))
		routes := (&Handler{im: imSvc, participant: svc}).Routes()
		const collection = "/api/v1/channels/csgclaw/participants"
		request := func(method, path, body string) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
			return rec
		}
		responses := make(chan *httptest.ResponseRecorder, 2)
		go func() {
			responses <- request(http.MethodPost, collection, `{"id":"pt-qa","type":"agent","name":"QA","agent_binding":{"mode":"create","agent_id":"agent-qa"}}`)
		}()
		synctest.Wait() // The retry is waiting for the worker lookup.
		go func() { responses <- request(http.MethodPatch, collection+"/pt-qa", `{"name":"QA-Renamed"}`) }()
		synctest.Wait()
		close(resume)
		for range 2 {
			if rec := <-responses; rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
		}
		rec := request(http.MethodGet, collection+"/pt-qa", "")
		var item apitypes.Participant
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		user, ok := imSvc.User("user-qa")
		if rec.Code != http.StatusOK || item.Name != "QA-Renamed" || !ok || user.Name != item.Name {
			t.Fatalf("GET status %d, participant %q, IM user %q", rec.Code, item.Name, user.Name)
		}
	})
}

func TestParticipantCreatedEventOnPublicationRetry(t *testing.T) {
	root := t.TempDir()
	local := httptest.NewServer(http.NotFoundHandler())
	defer local.Close()
	manager, err := relay.NewManager(t.Context(), root, apiclient.New(local.URL, "", local.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manifest := filepath.Join(root, "publications.json")
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manifest, 0700); err != nil {
		t.Fatal(err)
	}
	agents, _ := mustNewSeededServiceWithPath(t, []agent.Agent{{ID: "agent-qa", Name: "QA", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex}})
	engine := agentengine.New(agents)
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()
	h := &Handler{svc: agents, agentEngine: engine, imBus: bus, relayManager: manager, participant: participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(engine))}
	routes := h.Routes()
	post := func(want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(`{"id":"pt-qa","name":"QA","type":"agent","agent_binding":{"mode":"create","agent_id":"agent-qa"},"auto_publish":true}`))
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("status %d, want %d: %s", rec.Code, want, rec.Body)
		}
	}
	post(http.StatusBadGateway)
	if evt := mustReceiveIMEvent(t, events); evt.Type != im.EventTypeParticipantCreated || evt.Participant.ID != "pt-qa" {
		t.Fatalf("unexpected creation event: %+v", evt)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	post(http.StatusOK)
	select {
	case evt := <-events:
		t.Fatalf("duplicate event: %+v", evt)
	default:
	}
}

func TestCreateCSGClawAgentParticipantViaAPIRequiresAgentBinding(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	imSvc := im.NewService()
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentEngine(agentengine.New(agentSvc)),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		participant: participantSvc, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}

	body := `{
		"id": "gitlab-worker",
		"type": "agent",
		"name": "gitlab-worker",
		"channel_user": {
			"ref": "gitlab-worker",
			"kind": "local_user_id"
		}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(body))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent_binding.mode") {
		t.Fatalf("body = %s, want agent_binding.mode validation", rec.Body.String())
	}
	if got := participantSvc.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}); len(got) != 0 {
		t.Fatalf("participants = %+v, want none after missing agent binding", got)
	}
	if _, ok := imSvc.User("user-gitlab-worker"); ok {
		t.Fatal("channel user was created despite missing agent binding")
	}
	if _, ok := agentSvc.Agent("agent-gitlab-worker"); ok {
		t.Fatal("agent was created despite missing agent binding")
	}
}

func TestCreateCSGClawAgentParticipantViaAPIReturnsConflictForDuplicateAgentName(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{{
		ID:          "u-existing",
		Name:        "generic-assistant-lite",
		Role:        agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
		Image:       "agent-image:test",
	}})
	imSvc := im.NewService()
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentEngine(agentengine.New(agentSvc)),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		participant: participantSvc, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}

	body := `{
		"type": "agent",
		"name": "generic-assistant-lite",
		"channel_user": {
			"ref": "u-generic-assistant-lite",
			"kind": "local_user_id"
		},
		"agent_binding": {
			"mode": "create",
			"agent": {
				"name": "generic-assistant-lite",
				"role": "worker",
				"runtime_kind": "codex",
				"image": "agent-image:test"
			}
		}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(body))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"agent_name_conflict"`) {
		t.Fatalf("body = %q, want duplicate agent name error", rec.Body.String())
	}
}

func TestHandleParticipantsPublishesLifecycleEvents(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	imSvc := im.NewService()
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		im:          imSvc,
		imBus:       bus,
		participant: participantSvc,
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants", strings.NewReader(`{
		"id": "qa",
		"type": "human",
		"name": "QA",
		"channel_user": {
			"ref": "u-qa",
			"kind": "local_user_id"
		}
	}`))
	createRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	createdEvent := mustReceiveIMEvent(t, events)
	if createdEvent.Type != im.EventTypeParticipantCreated || createdEvent.Participant == nil || createdEvent.Participant.ID != "pt-qa" {
		t.Fatalf("created event = %+v, want participant.created for pt-qa", createdEvent)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/channels/csgclaw/participants/pt-qa", strings.NewReader(`{"name":"QAUpdated"}`))
	updateRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	updatedEvent := mustReceiveIMEvent(t, events)
	if updatedEvent.Type != im.EventTypeParticipantUpdated || updatedEvent.Participant == nil || updatedEvent.Participant.Name != "QAUpdated" {
		t.Fatalf("updated event = %+v, want participant.updated for QAUpdated", updatedEvent)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/participants/pt-qa", nil)
	deleteRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d; body=%s", deleteRec.Code, http.StatusNoContent, deleteRec.Body.String())
	}
	deletedEvent := mustReceiveIMEvent(t, events)
	if deletedEvent.Type != im.EventTypeParticipantDeleted || deletedEvent.Participant == nil || deletedEvent.Participant.ID != "pt-qa" {
		t.Fatalf("deleted event = %+v, want participant.deleted for pt-qa", deletedEvent)
	}
}

func TestCreateFeishuAgentParticipantViaAPIReusesExistingAgent(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{{
		ID:          "u-qa",
		Name:        "QA-Runtime",
		Role:        agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
		Image:       "agent-image:test",
	}})
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentEngine(agentengine.New(agentSvc)),
	)
	srv := &Handler{
		svc:         agentSvc,
		participant: participantSvc, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}

	body := `{
		"id": "test",
		"type": "agent",
		"name": "QA Feishu",
		"channel_app_ref": "cli_xxx",
		"channel_user": {
			"ref": "ou_xxx",
			"kind": "open_id"
		},
		"agent_binding": {
			"mode": "reuse",
			"agent_id": "u-qa"
		}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/participants", strings.NewReader(body))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created apitypes.Participant
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != "pt-test" || created.Channel != "feishu" || created.AgentID != "agent-qa" {
		t.Fatalf("created participant = %+v, want feishu:pt-test bound to agent-qa", created)
	}
	if created.ChannelUserRef != "ou_xxx" || created.ChannelUserKind != "open_id" || created.ChannelAppRef != "cli_xxx" {
		t.Fatalf("created channel identity = %+v, want Feishu app/open_id identity", created)
	}
}

func TestCreateFeishuBotParticipantRedactsChannelAppConfigSecretViaAPI(t *testing.T) {
	participantSvc := participant.NewService(participant.NewMemoryStore(nil))
	srv := &Handler{participant: participantSvc}

	body := `{
		"id": "dev",
		"type": "agent",
		"name": "Dev",
		"channel_user": {
			"kind": "app_id"
		},
		"channel_app_config": {
			"app_id": "cli_dev",
			"app_secret": "dev-secret"
		}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/participants", strings.NewReader(body))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dev-secret") {
		t.Fatalf("response leaked app_secret: %s", rec.Body.String())
	}
	var created apitypes.Participant
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := created.ChannelAppConfig["app_id"], "cli_dev"; got != want {
		t.Fatalf("response channel_app_config.app_id = %#v, want %q", got, want)
	}
	if got, want := created.ChannelAppConfig["app_secret"], participant.RedactedSecretValue; got != want {
		t.Fatalf("response channel_app_config.app_secret = %#v, want %q", got, want)
	}
	stored, ok := participantSvc.Get(participant.ChannelFeishu, "dev")
	if !ok {
		t.Fatal("stored participant not found")
	}
	if got, want := stored.ChannelAppConfig["app_secret"], "dev-secret"; got != want {
		t.Fatalf("stored channel_app_config.app_secret = %#v, want %q", got, want)
	}
}

func TestDeleteFeishuCodexAgentParticipantRefreshesBridgeWithoutRecreate(t *testing.T) {
	var runtimeCreates int
	target := completeWorkerAgent("u-dev", "dev")
	target.RuntimeKind = agent.RuntimeKindCodex
	target.RuntimeID = "rt-dev"
	target.BoxID = "codex-session-dev"
	target.AgentProfile = agent.AgentProfile{
		Name:            "dev",
		Provider:        agent.ProviderCodex,
		ModelID:         "gpt-5.4",
		ProfileComplete: true,
	}
	target.ProfileComplete = true
	bridge := &fakeCodexBridgeController{}
	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{target},
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				runtimeCreates++
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-new"}, nil
			},
		}),
	)
	participantSvc := participant.NewService(
		participant.NewMemoryStore([]apitypes.Participant{
			{
				ID:              "dev",
				Channel:         participant.ChannelFeishu,
				Type:            participant.TypeAgent,
				Name:            "dev",
				ChannelUserKind: participant.ChannelUserKindAppID,
				ChannelAppConfig: map[string]any{
					"app_id":     "cli_dev",
					"app_secret": "dev-secret",
				},
				AgentID:         "u-dev",
				LifecycleStatus: participant.LifecycleStatusActive,
				Mentionable:     true,
			},
			{
				ID:              "legacy-dev",
				Channel:         participant.ChannelFeishu,
				Type:            participant.TypeAgent,
				Name:            "legacy dev",
				ChannelUserKind: participant.ChannelUserKindAppID,
				ChannelAppConfig: map[string]any{
					"app_id":     "cli_legacy_dev",
					"app_secret": "legacy-dev-secret",
				},
				AgentID:         "u-dev",
				LifecycleStatus: participant.LifecycleStatusActive,
				Mentionable:     true,
			},
		}),
		participant.WithAgentEngine(agentengine.New(agentSvc)),
	)
	srv := &Handler{
		svc:             agentSvc,
		participant:     participantSvc,
		channelBindings: bridge, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/feishu/participants/dev", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	for _, id := range []string{"dev", "legacy-dev"} {
		if _, ok := participantSvc.Get(participant.ChannelFeishu, id); ok {
			t.Fatalf("participant feishu:%s still exists after disconnect", id)
		}
	}
	if runtimeCreates != 0 {
		t.Fatalf("runtime New() calls = %d, want 0", runtimeCreates)
	}
	if len(bridge.refreshCalls) != 1 {
		t.Fatalf("RefreshAgentChannel() calls = %+v, want one call", bridge.refreshCalls)
	}
	if bridge.refreshCalls[0].agent.ID != "agent-dev" || bridge.refreshCalls[0].channel != participant.ChannelFeishu {
		t.Fatalf("RefreshAgentChannel() call = %+v, want agent-dev feishu", bridge.refreshCalls[0])
	}
}

func TestCreateFeishuHumanParticipantViaAPI(t *testing.T) {
	participantSvc := participant.NewService(participant.NewMemoryStore(nil))
	srv := &Handler{participant: participantSvc}

	body := `{
		"id": "alice",
		"type": "human",
		"name": "Alice",
		"channel_app_ref": "cli_xxx",
		"channel_user": {
			"ref": "ou_alice",
			"kind": "open_id"
		}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/participants", strings.NewReader(body))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created apitypes.Participant
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != "pt-alice" || created.Type != "human" || created.AgentID != "" {
		t.Fatalf("created participant = %+v, want unbound human pt-alice", created)
	}
	if created.ChannelUserRef != "ou_alice" || created.ChannelUserKind != "open_id" || created.ChannelAppRef != "cli_xxx" {
		t.Fatalf("created channel identity = %+v, want Feishu human open_id identity", created)
	}
}

func TestListAgentsIncludesParticipantsWhenRequested(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{{
		ID:   "u-qa",
		Name: "QA-Runtime",
		Role: agent.RoleWorker,
	}})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:             "qa",
		Channel:        "csgclaw",
		Type:           "agent",
		Name:           "QA",
		ChannelUserRef: "u-qa",
		AgentID:        "u-qa",
		Mentionable:    true,
	}}))
	srv := &Handler{
		svc:         agentSvc,
		participant: participantSvc, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?include_participants=true", nil)

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var agents []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("agents = %+v, want one agent", agents)
	}
	participants, ok := agents[0]["participants"].([]any)
	if !ok || len(participants) != 1 {
		t.Fatalf("participants = %#v, want one participant", agents[0]["participants"])
	}
	got, ok := participants[0].(map[string]any)
	if !ok || got["id"] != "qa" || got["channel"] != "csgclaw" {
		t.Fatalf("participant expansion = %#v, want csgclaw qa", participants[0])
	}
}

func TestParticipantMessageRouteSendsAsParticipantChannelUser(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-bob", Name: "bob"},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", "u-bob"},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "bob",
		Channel:         "csgclaw",
		Type:            "human",
		Name:            "Bob",
		ChannelUserRef:  "u-bob",
		ChannelUserKind: "local_user_id",
		LifecycleStatus: "active",
		Mentionable:     true,
	}}))
	srv := &Handler{
		im:                imSvc,
		participant:       participantSvc,
		participantBridge: im.NewParticipantBridge("secret"),
		serverAccessToken: "secret",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/bob/messages", strings.NewReader(`{
		"room_id": "room-1",
		"content": "hello from participant route"
	}`))
	req.Header.Set("Authorization", "Bearer secret")

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	messages, err := imSvc.ListMessages("room-1")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %+v, want one delivered message", messages)
	}
	if messages[0].SenderID != "user-bob" || messages[0].Content != "hello from participant route" {
		t.Fatalf("delivered message = %+v, want sender user-bob with posted content", messages[0])
	}
}

func TestParticipantMessageRouteCanonicalizesAgentIDAlias(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: agent.ManagerParticipantID, Name: "manager"},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", agent.ManagerParticipantID},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              agent.ManagerParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            agent.ManagerName,
		ChannelUserRef:  agent.ManagerParticipantID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	srv := &Handler{
		im:                imSvc,
		participant:       participantSvc,
		participantBridge: im.NewParticipantBridge(""),
		serverNoAuth:      true,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/u-manager/messages", strings.NewReader(`{
		"room_id": "room-1",
		"text": "hello from manager alias"
	}`))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	messages, err := imSvc.ListMessages("room-1")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %+v, want one delivered message", messages)
	}
	if messages[0].SenderID != im.ManagerUserID || messages[0].Content != "hello from manager alias" {
		t.Fatalf("delivered message = %+v, want canonical manager user sender", messages[0])
	}
}

func TestParticipantDeleteCannotBypassArchivedAgentConfirmation(t *testing.T) {
	agentSvc := mustNewService(t)
	if _, err := agentSvc.CreateRecord(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-qa",
			Name:        "qa",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindCodex,
			Image:       "agent-image:test",
		},
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-qa", Name: "qa"},
		},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "qa",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		ChannelUserRef:  "u-qa",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         "u-qa",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}), participant.WithAgentEngine(agentengine.New(agentSvc)), participant.WithIMService(imSvc))
	srv := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		participant: participantSvc, agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}

	if _, err := agentSvc.SetArchived(context.Background(), "u-qa", true); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/participants/qa?delete_agent=if_unreferenced", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := participantSvc.Get(participant.ChannelCSGClaw, "qa"); !ok {
		t.Fatal("delete shortcut removed participant")
	}
	if _, ok := agentSvc.Agent("agent-qa"); !ok {
		t.Fatal("delete shortcut removed agent")
	}
}

func TestParticipantNotificationRouteAcceptsNotificationParticipant(t *testing.T) {
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "alerts",
		Channel:         "csgclaw",
		Type:            "notification",
		Name:            "Alerts",
		ChannelUserRef:  "n-alerts",
		ChannelUserKind: "local_user_id",
		LifecycleStatus: "active",
		Mentionable:     true,
		Metadata: map[string]any{
			"delivery_mode": "webhook",
			"webhook_token": "secret-token",
		},
	}}))
	srv := &Handler{participant: participantSvc}
	srv.SetNotificationDeliver(&noopFanouter{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/alerts/notifications", strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
}

func TestParticipantEventsRouteRequiresAuthorization(t *testing.T) {
	srv := &Handler{
		im:                im.NewService(),
		participantBridge: im.NewParticipantBridge("secret"),
		serverAccessToken: "secret",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/participants/u-manager/events", nil)

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestParticipantEventsRouteCanonicalizesAgentIDAlias(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-agent-hhtz4b", Name: "qa"},
		},
		Rooms: []im.Room{{
			ID:       "room-qa",
			IsDirect: true,
			Members:  []string{"u-admin", "u-agent-hhtz4b"},
			Messages: []im.Message{{
				ID:        "msg-qa",
				SenderID:  "u-admin",
				Content:   "qa only",
				CreatedAt: now,
			}},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "agent-hhtz4b",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		ChannelUserRef:  "u-agent-hhtz4b",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         "u-agent-hhtz4b",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	srv := &Handler{
		im:                imSvc,
		participant:       participantSvc,
		participantBridge: im.NewParticipantBridge(""),
		serverNoAuth:      true,
	}

	writer := &recordingFailingEventWriter{header: make(http.Header)}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/participants/u-agent-hhtz4b/events", nil).WithContext(ctx)

	srv.Routes().ServeHTTP(writer, req)

	if got := writer.String(); !strings.Contains(got, `"message_id":"msg-qa"`) || !strings.Contains(got, `"account":"pt-hhtz4b"`) {
		t.Fatalf("event stream = %q, want replay delivered on canonical participant id pt-hhtz4b", got)
	}
}

type recordingFailingEventWriter struct {
	header http.Header
	body   strings.Builder
}

func (w *recordingFailingEventWriter) Header() http.Header {
	return w.header
}

func (w *recordingFailingEventWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	if strings.Contains(string(data), "event: message") {
		return 0, errors.New("stop after message event")
	}
	return len(data), nil
}

func (w *recordingFailingEventWriter) WriteHeader(int) {}

func (w *recordingFailingEventWriter) Flush() {}

func (w *recordingFailingEventWriter) String() string {
	return w.body.String()
}

func TestCreateMessageResolvesCSGClawParticipantMentionToBridgeID(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: agent.ManagerParticipantID, Name: "manager", Role: agent.RoleManager},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", agent.ManagerParticipantID},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              agent.ManagerParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            agent.ManagerName,
		ChannelUserRef:  agent.ManagerParticipantID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	bridge := im.NewParticipantBridge("secret")
	bus := im.NewBus()
	events, cancel := bridge.Subscribe(agent.ManagerParticipantID)
	defer cancel()
	srv := &Handler{
		im:                imSvc,
		csgclaw:           csgclawchannel.NewService(imSvc),
		imBus:             bus,
		participant:       participantSvc,
		participantBridge: bridge,
		serverNoAuth:      true,
	}
	busEvents, cancelBus := bus.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for evt := range busEvents {
			srv.PublishParticipantEvent(evt)
		}
	}()
	defer func() {
		cancelBus()
		<-done
	}()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", strings.NewReader(`{
		"room_id": "room-1",
		"sender_id": "u-admin",
		"mention_id": "manager",
		"content": "hello manager"
	}`))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	select {
	case evt := <-events:
		if evt.Context.Account != agent.ManagerParticipantID || len(evt.Mentions) != 1 || evt.Mentions[0] != im.ManagerUserID {
			t.Fatalf("event = %+v, want bridge delivery for participant %q", evt, agent.ManagerParticipantID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for manager bridge event")
	}
}

func TestPublishParticipantEventDeliversToParticipantIDWhenRoomUsesChannelUserRef(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-agent-hhtz4b", Name: "qa"},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", "u-agent-hhtz4b"},
			Messages: []im.Message{{
				ID:        "msg-1",
				SenderID:  "u-admin",
				Content:   "hello qa",
				CreatedAt: time.Now().UTC(),
			}},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "agent-hhtz4b",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		ChannelUserRef:  "u-agent-hhtz4b",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         "u-agent-hhtz4b",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	bridge := im.NewParticipantBridge("secret")
	events, cancel := bridge.Subscribe("agent-hhtz4b")
	defer cancel()
	srv := &Handler{
		im:                imSvc,
		participant:       participantSvc,
		participantBridge: bridge,
	}
	room, ok := imSvc.Room("room-1")
	if !ok || len(room.Messages) != 1 {
		t.Fatalf("room = %+v, want one message", room)
	}
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("missing admin sender")
	}

	srv.PublishParticipantEvent(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  "room-1",
		Sender:  &sender,
		Message: &room.Messages[0],
	})

	select {
	case evt := <-events:
		if evt.MessageID != "msg-1" || evt.Context.Account != "pt-hhtz4b" {
			t.Fatalf("event = %+v, want participant-keyed delivery for pt-hhtz4b", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for participant-keyed bridge event")
	}
}

func TestPublishParticipantEventDoesNotDispatchUserInputAnswerTranscript(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-agent-hhtz4b", Name: "qa"},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", "u-agent-hhtz4b"},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "agent-hhtz4b",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		ChannelUserRef:  "u-agent-hhtz4b",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         "u-agent-hhtz4b",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	bridge := im.NewParticipantBridge("secret")
	events, cancel := bridge.Subscribe("agent-hhtz4b")
	defer cancel()
	srv := &Handler{
		im:                imSvc,
		participant:       participantSvc,
		participantBridge: bridge,
	}
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("missing admin sender")
	}
	message := im.Message{
		ID:        "answer-request-1",
		SenderID:  sender.ID,
		Content:   "## Answers\n\n- kind：Standard (Normal checks.)",
		CreatedAt: time.Now().UTC(),
		Metadata: map[string]any{
			userInputTranscriptMetadataKey: map[string]any{
				userInputTranscriptRequestKey: map[string]any{
					"kind":       userInputTranscriptAnswerKind,
					"request_id": "request-1",
				},
			},
		},
	}

	srv.PublishParticipantEvent(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  "room-1",
		Sender:  &sender,
		Message: &message,
	})

	select {
	case evt := <-events:
		t.Fatalf("unexpected participant event for transcript: %+v", evt)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCSGClawMessageRouteDeliversToCanonicalParticipantWhenRoomUsesAgentID(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-agent-hhtz4b", Name: "qa"},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", "u-agent-hhtz4b"},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "agent-hhtz4b",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		ChannelUserRef:  "u-agent-hhtz4b",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         "u-agent-hhtz4b",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	bridge := im.NewParticipantBridge("secret")
	bus := im.NewBus()
	events, cancel := bridge.Subscribe("agent-hhtz4b")
	defer cancel()
	srv := &Handler{
		im:                imSvc,
		csgclaw:           csgclawchannel.NewService(imSvc),
		imBus:             bus,
		participant:       participantSvc,
		participantBridge: bridge,
		serverNoAuth:      true,
	}
	busEvents, cancelBus := bus.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for evt := range busEvents {
			srv.PublishParticipantEvent(evt)
		}
	}()
	defer func() {
		cancelBus()
		<-done
	}()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", strings.NewReader(`{
		"room_id": "room-1",
		"sender_id": "u-admin",
		"content": "hello qa"
	}`))

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	select {
	case evt := <-events:
		if evt.MessageID == "" || evt.RoomID != "room-1" || evt.Context.Account != "pt-hhtz4b" || evt.Text != "hello qa" {
			t.Fatalf("event = %+v, want route-created message delivered to canonical participant", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for route-created canonical participant bridge event")
	}
}

func TestParticipantEventsRouteReceivesParticipantIDQueue(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: agent.ManagerParticipantID, Name: "manager", Role: agent.RoleManager},
			{ID: agent.ManagerUserID, Name: "manager", Role: agent.RoleManager},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", agent.ManagerParticipantID},
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              agent.ManagerParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            agent.ManagerName,
		ChannelUserRef:  agent.ManagerParticipantID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	bridge := im.NewParticipantBridge("secret")
	srv := &Handler{
		im:                imSvc,
		participant:       participantSvc,
		participantBridge: bridge,
		serverAccessToken: "secret",
	}
	ctx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	rec := newSSERecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/participants/manager/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()
	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return bridge.SubscriberCount(agent.ManagerParticipantID) > 0
	})
	if got := bridge.SubscriberCount(agent.ManagerUserID); got == 0 {
		t.Fatalf("u-manager subscriber count = %d, want alias to resolve to participant ID subscription", got)
	}

	room := im.Room{ID: "room-1", IsDirect: true, Members: []string{"u-admin", agent.ManagerParticipantID}}
	sender := im.User{ID: "u-admin", Name: "admin"}
	message := im.Message{
		ID:        "msg-1",
		SenderID:  "u-admin",
		Content:   "hello manager",
		CreatedAt: time.Now().UTC(),
	}
	bridge.PublishMessageEvent(room, sender, message)
	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(rec.BodyString(), `"message_id":"msg-1"`)
	})
	cancelReq()
	<-done
}

func TestFeishuParticipantEventsRouteUsesParticipantChannelUserRef(t *testing.T) {
	feishuSvc := feishu.NewService()
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "qa",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeAgent,
		Name:            "QA",
		ChannelUserRef:  "ou_qa",
		ChannelUserKind: participant.ChannelUserKindOpenID,
		AgentID:         "u-qa",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	srv := &Handler{
		feishu:            feishuSvc,
		participant:       participantSvc,
		serverAccessToken: "secret",
	}

	ctx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	rec := newSSERecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/qa/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(rec.BodyString(), ": connected")
	})
	feishuSvc.MessageBus().Publish(feishu.MessageEvent{
		Type:   feishu.MessageEventTypeMessageCreated,
		RoomID: "oc_alpha",
		Message: &im.Message{
			ID:       "om_qa",
			SenderID: "ou_user",
			Content:  "hello qa",
			Mentions: []im.Mention{
				{ID: "ou_qa"},
			},
		},
	})
	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(rec.BodyString(), `"id":"om_qa"`)
	})
	cancelReq()
	<-done
}

func TestFeishuParticipantEventsRouteUsesBotOpenIDForAppIDParticipant(t *testing.T) {
	feishuSvc := feishu.NewServiceWithBotOpenIDResolver(
		map[string]feishu.AppConfig{
			"dev": {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, app feishu.AppConfig) (feishu.BotInfo, error) {
			if app.AppID != "cli_dev" {
				t.Fatalf("resolve app_id = %q, want cli_dev", app.AppID)
			}
			return feishu.BotInfo{OpenID: "ou_dev"}, nil
		},
	)
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "dev",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeAgent,
		Name:            "Dev",
		ChannelUserKind: participant.ChannelUserKindAppID,
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_dev",
			"app_secret": "dev-secret",
		},
		AgentID:         "u-dev",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	srv := &Handler{
		feishu:            feishuSvc,
		participant:       participantSvc,
		serverAccessToken: "secret",
	}

	ctx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	rec := newSSERecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/dev/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(rec.BodyString(), ": connected")
	})
	feishuSvc.MessageBus().Publish(feishu.MessageEvent{
		Type:   feishu.MessageEventTypeMessageCreated,
		RoomID: "oc_alpha",
		Message: &im.Message{
			ID:       "om_dev",
			SenderID: "ou_manager",
			Content:  "hello dev",
			Mentions: []im.Mention{
				{ID: "ou_dev", Name: "dev"},
			},
		},
	})
	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(rec.BodyString(), `"id":"om_dev"`)
	})
	cancelReq()
	<-done
}

type noopFanouter struct{}

func (noopFanouter) DeliverFanout(string, string) error { return nil }
