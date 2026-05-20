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
	botStore, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewService(botStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	botSvc.SetDependencies(nil, imSvc)

	srv := &Handler{botSvc: botSvc, im: imSvc}
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
	if created.AgentID != "" {
		t.Fatalf("created.AgentID = %q, want empty", created.AgentID)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET bots status = %d", rec.Code)
	}
	var normalBots []apitypes.Bot
	if err := json.Unmarshal(rec.Body.Bytes(), &normalBots); err != nil {
		t.Fatalf("decode bots: %v", err)
	}
	for _, b := range normalBots {
		if b.Type == bot.BotTypeNotification {
			t.Fatalf("GET /bots included notification bot %q", b.ID)
		}
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots?type=notification", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET notification-bots status = %d", rec.Code)
	}
	var nbots []apitypes.Bot
	if err := json.Unmarshal(rec.Body.Bytes(), &nbots); err != nil {
		t.Fatalf("decode notification-bots: %v", err)
	}
	if len(nbots) != 1 || nbots[0].ID != created.ID {
		t.Fatalf("notification-bots = %+v, want one bot id %q", nbots, created.ID)
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
