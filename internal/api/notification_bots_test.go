package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/bot"
	"csgclaw/internal/im"
)

func TestNotificationBotsCRUDAndListBotsFilter(t *testing.T) {
	imSvc := im.NewService()
	bus := im.NewBus()
	botStore, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewService(botStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	botSvc.SetDependencies(nil, imSvc)

	srv := &Handler{botSvc: botSvc, im: imSvc, imBus: bus}
	router := srv.Routes()

	createBody, _ := json.Marshal(apitypes.CreateBotRequest{
		Name: "notify-1",
		Type: "notification",
		Role: "worker",
		RuntimeOptions: map[string]any{
			"delivery_mode": "webhook",
			"webhook_token": "secret-token",
		},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots", bytes.NewReader(createBody)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST notification-bots status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created apitypes.Bot
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.Type != bot.BotTypeNotification {
		t.Fatalf("created.Type = %q, want %q", created.Type, bot.BotTypeNotification)
	}
	if created.ID != "n-notify-1" {
		t.Fatalf("created.ID = %q, want n-notify-1", created.ID)
	}
	if created.AgentID != "" {
		t.Fatalf("created.AgentID = %q, want empty", created.AgentID)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET bots status = %d", rec.Code)
	}
	var listed []apitypes.Bot
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode bots: %v", err)
	}
	var found bool
	for _, b := range listed {
		if b.ID == created.ID && b.Type == bot.BotTypeNotification {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GET /bots = %+v, want notification bot %q", listed, created.ID)
	}

	events, cancel := bus.Subscribe()
	defer cancel()
	patchBody, _ := json.Marshal(apitypes.PatchNotificationBotRequest{
		Avatar: "avatar/cartoon-4.png",
	})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/channels/csgclaw/bots/"+created.ID, bytes.NewReader(patchBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH notification-bots status = %d, body = %s", rec.Code, rec.Body.String())
	}
	user, ok := imSvc.User(created.UserID)
	if !ok {
		t.Fatalf("User(%q) ok = false, want true", created.UserID)
	}
	if user.Avatar != "avatar/cartoon-4.png" {
		t.Fatalf("user avatar = %q, want avatar/cartoon-4.png", user.Avatar)
	}
	evt := mustReceiveIMEvent(t, events)
	if evt.Type != im.EventTypeUserUpdated || evt.User == nil || evt.User.Avatar != "avatar/cartoon-4.png" {
		t.Fatalf("event = %+v, want user.updated with updated avatar", evt)
	}

	push := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots/"+created.ID+"/notifications", bytes.NewReader([]byte(`{"hello":"world"}`)))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")
	srv.SetNotificationDeliver(&noopFanouter{})
	router.ServeHTTP(push, req)
	if push.Code != http.StatusAccepted {
		t.Fatalf("POST notifications status = %d, body = %s", push.Code, push.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/bots/"+created.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE notification-bots status = %d, body = %s", rec.Code, rec.Body.String())
	}
	_ = context.Background()
}

type noopFanouter struct{}

func (noopFanouter) DeliverFanout(string, string) error { return nil }
