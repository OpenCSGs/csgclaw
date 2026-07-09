package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
)

func TestCreateCSGClawAgentParticipantViaAPI(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	imSvc := im.NewService()
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentService(agentSvc),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		participant: participantSvc,
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
				"runtime_kind": "picoclaw_sandbox",
				"image": "agent-image:test",
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

func TestCreateCSGClawAgentParticipantViaAPIReturnsConflictForDuplicateAgentName(t *testing.T) {
	agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{{
		ID:          "u-existing",
		Name:        "generic-assistant-lite",
		Role:        agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindPicoClawSandbox,
		Image:       "agent-image:test",
	}})
	imSvc := im.NewService()
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentService(agentSvc),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		participant: participantSvc,
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
				"runtime_kind": "picoclaw_sandbox",
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
	if !strings.Contains(rec.Body.String(), `agent name "generic-assistant-lite" already exists`) {
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
		RuntimeKind: agent.RuntimeKindPicoClawSandbox,
		Image:       "agent-image:test",
	}})
	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentService(agentSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		participant: participantSvc,
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

func TestDeleteFeishuAgentParticipantRecreatesBoundAgent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		agentID       string
		agentName     string
		role          string
		participantID string
		wantRecreate  bool
	}{
		{
			name:          "worker",
			agentID:       "agent-dev",
			agentName:     "dev",
			role:          agent.RoleWorker,
			participantID: "pt-dev",
			wantRecreate:  true,
		},
		{
			name:          "manager",
			agentID:       agent.ManagerUserID,
			agentName:     "manager",
			role:          agent.RoleManager,
			participantID: "pt-manager",
			wantRecreate:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var recreated []string
			target := completeWorkerAgent(tc.agentID, tc.agentName)
			target.Role = tc.role
			agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{target},
				agent.WithRuntime(fakeCompatRuntime{
					kind: agent.RuntimeKindPicoClawSandbox,
					new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
						recreated = append(recreated, spec.AgentID)
						return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-" + spec.AgentName}, nil
					},
				}),
			)
			participantSvc := participant.NewService(
				participant.NewMemoryStore([]apitypes.Participant{{
					ID:              tc.participantID,
					Channel:         participant.ChannelFeishu,
					Type:            participant.TypeAgent,
					Name:            tc.agentName,
					ChannelUserKind: participant.ChannelUserKindAppID,
					ChannelAppConfig: map[string]any{
						"app_id":     "cli_" + tc.agentName,
						"app_secret": tc.agentName + "-secret",
					},
					AgentID:         tc.agentID,
					LifecycleStatus: participant.LifecycleStatusActive,
					Mentionable:     true,
				}}),
				participant.WithAgentService(agentSvc),
			)
			srv := &Handler{
				svc:         agentSvc,
				participant: participantSvc,
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/feishu/participants/"+tc.participantID, nil)
			srv.Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			if _, ok := participantSvc.Get(participant.ChannelFeishu, tc.participantID); ok {
				t.Fatalf("participant feishu:%s still exists after delete", tc.participantID)
			}
			if tc.wantRecreate {
				if len(recreated) != 1 || recreated[0] != tc.agentID {
					t.Fatalf("recreated = %#v, want only %q", recreated, tc.agentID)
				}
			} else if len(recreated) != 0 {
				t.Fatalf("recreated = %#v, want none for codex manager", recreated)
			}
			got, ok := agentSvc.Agent(tc.agentID)
			if !ok {
				t.Fatalf("agent %q not found after recreate", tc.agentID)
			}
			if got.BoxID != "box-"+tc.agentName {
				t.Fatalf("agent BoxID = %q, want recreated runtime handle %q", got.BoxID, "box-"+tc.agentName)
			}
		})
	}
}

func TestDeleteFeishuAgentParticipantRemovesSameAgentFeishuBotsBeforeRecreate(t *testing.T) {
	var recreated []string
	target := completeWorkerAgent("u-dev", "dev")
	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{target},
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindPicoClawSandbox,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				recreated = append(recreated, spec.AgentID)
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-" + spec.AgentName}, nil
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
		participant.WithAgentService(agentSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		participant: participantSvc,
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
	if len(recreated) != 1 || recreated[0] != "agent-dev" {
		t.Fatalf("recreated = %#v, want only %q", recreated, "agent-dev")
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
		participant: participantSvc,
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

func TestParticipantDeleteWithAgentCleanupRemovesCSGClawUser(t *testing.T) {
	agentSvc := mustNewService(t)
	if _, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-qa",
			Name:        "qa",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
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
	}}), participant.WithAgentService(agentSvc), participant.WithIMService(imSvc))
	srv := &Handler{
		svc:         agentSvc,
		im:          imSvc,
		participant: participantSvc,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/participants/qa?delete_agent=if_unreferenced", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := participantSvc.Get(participant.ChannelCSGClaw, "qa"); ok {
		t.Fatal("participant csgclaw:qa still exists after delete")
	}
	if _, ok := agentSvc.Agent("agent-qa"); ok {
		t.Fatal("agent u-qa still exists after delete")
	}
	if _, ok := imSvc.User("u-qa"); ok {
		t.Fatal("user u-qa still exists after participant agent cleanup")
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
	rec := httptest.NewRecorder()
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
		return strings.Contains(rec.Body.String(), `"message_id":"msg-1"`)
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
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/qa/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(rec.Body.String(), ": connected")
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
		return strings.Contains(rec.Body.String(), `"id":"om_qa"`)
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
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/dev/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	waitForCondition(t, time.Second, 10*time.Millisecond, func() bool {
		return strings.Contains(rec.Body.String(), ": connected")
	})
	feishuSvc.MessageBus().Publish(feishu.MessageEvent{
		Type:        feishu.MessageEventTypeMessageCreated,
		RoomID:      "oc_alpha",
		SenderBotID: "manager",
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
		return strings.Contains(rec.Body.String(), `"id":"om_dev"`)
	})
	cancelReq()
	<-done
}

type noopFanouter struct{}

func (noopFanouter) DeliverFanout(string, string) error { return nil }
