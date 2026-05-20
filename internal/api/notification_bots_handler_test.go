package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/bot"
	"csgclaw/internal/im"
)

func TestHandleBotByIDRejectsGetPatchForNormalBot(t *testing.T) {
	imSvc := im.NewService()
	botStore, err := bot.NewMemoryStore([]bot.Bot{{
		ID:      "u-worker",
		Name:    "worker",
		Type:    bot.BotTypeNormal,
		Role:    string(bot.RoleWorker),
		Channel: string(bot.ChannelCSGClaw),
		AgentID: "u-worker",
		UserID:  "u-worker",
	}})
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

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots/u-worker", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET normal bot status = %d, want 405", rec.Code)
	}
}
