package bot

import (
	"context"
	"strings"
	"testing"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
)

func TestNotificationBotIDUsesDedicatedPrefix(t *testing.T) {
	t.Parallel()
	got := notificationBotID(CreateRequest{Name: "alerts"})
	if got != "n-alerts" {
		t.Fatalf("notificationBotID() = %q, want n-alerts", got)
	}
	if got := notificationBotID(CreateRequest{ID: "custom-id", Name: "alerts"}); got != "custom-id" {
		t.Fatalf("explicit id = %q, want custom-id", got)
	}
}

func TestCreateNotificationBotRejectsChannelBotIDConflict(t *testing.T) {
	imSvc := im.NewService()
	botStore, err := NewMemoryStore([]Bot{{
		ID:      "u-alice",
		Name:    "alice",
		Type:    BotTypeNormal,
		Role:    string(RoleWorker),
		Channel: string(ChannelCSGClaw),
		AgentID: "u-alice",
		UserID:  "u-alice",
	}})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	botSvc, err := NewService(botStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	botSvc.SetDependencies(nil, imSvc)

	_, err = botSvc.CreateNotificationBot(context.Background(), CreateRequest{
		ID:   "u-alice",
		Name: "notify-alice",
		RuntimeOptions: map[string]any{
			"delivery_mode": "webhook",
			"webhook_token": "tok",
		},
	})
	if err == nil || (!strings.Contains(err.Error(), "conflicts with existing channel bot") && !strings.Contains(err.Error(), "already exists")) {
		t.Fatalf("CreateNotificationBot() error = %v, want id conflict", err)
	}
}

func TestCreateNotificationBotUsesDedicatedIMUser(t *testing.T) {
	imSvc := im.NewService()
	botStore, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	botSvc, err := NewService(botStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	botSvc.SetDependencies(nil, imSvc)

	created, err := botSvc.CreateNotificationBot(context.Background(), CreateRequest{
		Name: "alice",
		RuntimeOptions: map[string]any{
			"delivery_mode": "webhook",
			"webhook_token": "tok",
		},
	})
	if err != nil {
		t.Fatalf("CreateNotificationBot() error = %v", err)
	}
	if created.ID != "n-alice" {
		t.Fatalf("created.ID = %q, want n-alice", created.ID)
	}
	if created.UserID != "n-alice" {
		t.Fatalf("created.UserID = %q, want dedicated IM user n-alice", created.UserID)
	}
}

func TestDeleteNotificationBotSkipsSharedIMUser(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     "u-shared",
		Name:   "shared-worker",
		Handle: "shared-worker",
		Role:   "worker",
	}); err != nil {
		t.Fatalf("EnsureAgentUser(worker) error = %v", err)
	}

	botStore, err := NewMemoryStore([]Bot{{
		ID:      "n-notify",
		Name:    "notify",
		Type:    BotTypeNotification,
		Role:    string(RoleWorker),
		Channel: string(ChannelCSGClaw),
		UserID:  "u-shared",
	}})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	botSvc, err := NewService(botStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	botSvc.SetDependencies(nil, imSvc)

	if err := botSvc.Delete(context.Background(), string(ChannelCSGClaw), "n-notify"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := imSvc.User("u-shared"); !ok {
		t.Fatal("IM user u-shared was deleted but is still referenced by a worker")
	}
}

func TestServiceListIncludesNotificationBotsForCSGClawOnly(t *testing.T) {
	botStore, err := NewMemoryStore([]Bot{
		{ID: "u-worker", Name: "worker", Type: BotTypeNormal, Role: string(RoleWorker), Channel: string(ChannelCSGClaw), AgentID: "u-worker", UserID: "u-worker"},
		{ID: "n-notify", Name: "notify", Type: BotTypeNotification, Role: string(RoleWorker), Channel: string(ChannelCSGClaw), UserID: "n-notify"},
		{ID: "n-feishu", Name: "feishu-notify", Type: BotTypeNotification, Role: string(RoleWorker), Channel: string(ChannelFeishu), UserID: "n-feishu"},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	botSvc, err := NewService(botStore)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	csgclaw, err := botSvc.List(string(ChannelCSGClaw), "", "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(csgclaw) != 2 {
		t.Fatalf("List(csgclaw) len = %d, want worker + notification", len(csgclaw))
	}
	feishu, err := botSvc.List(string(ChannelFeishu), "", "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	for _, b := range feishu {
		if IsNotificationBot(b) {
			t.Fatalf("List(feishu) included notification bot %q", b.ID)
		}
	}

	notifyOnly, err := botSvc.List(string(ChannelCSGClaw), "", BotTypeNotification)
	if err != nil {
		t.Fatalf("List(csgclaw, notification) error = %v", err)
	}
	if len(notifyOnly) != 1 || notifyOnly[0].ID != "n-notify" {
		t.Fatalf("List(csgclaw, notification) = %+v, want n-notify only", notifyOnly)
	}
	normalOnly, err := botSvc.List(string(ChannelCSGClaw), "", BotTypeNormal)
	if err != nil {
		t.Fatalf("List(csgclaw, normal) error = %v", err)
	}
	if len(normalOnly) != 1 || normalOnly[0].ID != "u-worker" {
		t.Fatalf("List(csgclaw, normal) = %+v, want u-worker only", normalOnly)
	}
}

func TestServiceListFeishuSkipsConfiguredBotsForNotificationType(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	feishuSvc := feishu.NewServiceWithBotOpenIDResolver(
		map[string]feishu.AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		},
		func(_ context.Context, app feishu.AppConfig) (feishu.BotInfo, error) {
			return feishu.BotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
		},
	)
	svc, err := NewServiceWithDependencies(store, nil, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.List(string(ChannelFeishu), "", BotTypeNotification)
	if err != nil {
		t.Fatalf("List(feishu, notification) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List(feishu, notification) = %+v, want empty (no configured feishu bots)", got)
	}
}
