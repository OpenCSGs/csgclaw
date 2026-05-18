package bot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeCreateRequestDefaultsChannel(t *testing.T) {
	got, err := NormalizeCreateRequest(CreateRequest{
		Name:        " Alice ",
		Description: " test lead ",
		Role:        " WORKER ",
	})
	if err != nil {
		t.Fatalf("NormalizeCreateRequest() error = %v", err)
	}
	if got.Name != "Alice" {
		t.Fatalf("Name = %q, want %q", got.Name, "Alice")
	}
	if got.Description != "test lead" {
		t.Fatalf("Description = %q, want %q", got.Description, "test lead")
	}
	if got.Role != string(RoleWorker) {
		t.Fatalf("Role = %q, want %q", got.Role, RoleWorker)
	}
	if got.Channel != string(ChannelCSGClaw) {
		t.Fatalf("Channel = %q, want %q", got.Channel, ChannelCSGClaw)
	}
}

func TestNormalizeCreateRequestRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name string
		req  CreateRequest
		want string
	}{
		{
			name: "empty name",
			req:  CreateRequest{Role: string(RoleWorker)},
			want: "name is required",
		},
		{
			name: "invalid role",
			req:  CreateRequest{Name: "alice", Role: "agent"},
			want: "role must be one of",
		},
		{
			name: "invalid channel",
			req:  CreateRequest{Name: "alice", Role: string(RoleWorker), Channel: "slack"},
			want: "channel must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeCreateRequest(tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NormalizeCreateRequest() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestStoreSaveListAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "bots.json")

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	first := Bot{
		ID:          "bot-2",
		Name:        "Bob",
		Description: " handles deployments ",
		Role:        "WORKER",
		Channel:     "FEISHU",
		AgentID:     "agent-2",
		UserID:      "user-2",
		CreatedAt:   time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}
	second := Bot{
		ID:        "bot-1",
		Name:      "Manager",
		Role:      string(RoleManager),
		AgentID:   "agent-1",
		UserID:    "user-1",
		CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}
	if err := store.Save(first); err != nil {
		t.Fatalf("Save(first) error = %v", err)
	}
	if err := store.Save(second); err != nil {
		t.Fatalf("Save(second) error = %v", err)
	}

	got := store.List()
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
	if got[0].ID != "bot-1" || got[1].ID != "bot-2" {
		t.Fatalf("List() order = %+v, want bot-1 then bot-2", got)
	}
	if got[0].Channel != string(ChannelCSGClaw) {
		t.Fatalf("List()[0].Channel = %q, want default %q", got[0].Channel, ChannelCSGClaw)
	}
	if got[1].Role != string(RoleWorker) || got[1].Channel != string(ChannelFeishu) {
		t.Fatalf("List()[1] = %+v, want normalized worker/feishu", got[1])
	}

	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore(reload) error = %v", err)
	}
	loaded, ok := reloaded.Get("bot-2")
	if !ok {
		t.Fatal("Get(bot-2) ok = false, want true")
	}
	if loaded.Role != string(RoleWorker) || loaded.Channel != string(ChannelFeishu) {
		t.Fatalf("Get(bot-2) = %+v, want normalized worker/feishu", loaded)
	}
	if loaded.Description != "handles deployments" {
		t.Fatalf("Get(bot-2).Description = %q, want trimmed description", loaded.Description)
	}
}

func TestStoreAllowsSameBotIDAcrossChannels(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}

	csgclawBot := Bot{
		ID:        "u-alice",
		Name:      "Alice",
		Role:      string(RoleWorker),
		Channel:   string(ChannelCSGClaw),
		AgentID:   "u-alice",
		UserID:    "u-alice",
		CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}
	feishuBot := csgclawBot
	feishuBot.Channel = string(ChannelFeishu)
	if err := store.Save(csgclawBot); err != nil {
		t.Fatalf("Save(csgclaw) error = %v", err)
	}
	if err := store.Save(feishuBot); err != nil {
		t.Fatalf("Save(feishu) error = %v", err)
	}

	all := store.List()
	if len(all) != 2 {
		t.Fatalf("List() = %+v, want two channel-scoped bots", all)
	}
	if _, ok, err := store.GetByChannelID(string(ChannelCSGClaw), "u-alice"); err != nil || !ok {
		t.Fatalf("GetByChannelID(csgclaw, u-alice) = ok %v err %v, want ok", ok, err)
	}
	if _, ok, err := store.GetByChannelID(string(ChannelFeishu), "u-alice"); err != nil || !ok {
		t.Fatalf("GetByChannelID(feishu, u-alice) = ok %v err %v, want ok", ok, err)
	}
}

func TestStoreDeleteByChannelIDRemovesOnlyMatchingChannel(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}

	csgclawBot := Bot{
		ID:        "u-alice",
		Name:      "Alice",
		Role:      string(RoleWorker),
		Channel:   string(ChannelCSGClaw),
		AgentID:   "u-alice",
		UserID:    "u-alice",
		CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}
	feishuBot := csgclawBot
	feishuBot.Channel = string(ChannelFeishu)
	if err := store.Save(csgclawBot); err != nil {
		t.Fatalf("Save(csgclaw) error = %v", err)
	}
	if err := store.Save(feishuBot); err != nil {
		t.Fatalf("Save(feishu) error = %v", err)
	}

	deleted, ok, err := store.DeleteByChannelID("feishu", "u-alice")
	if err != nil {
		t.Fatalf("DeleteByChannelID() error = %v", err)
	}
	if !ok || deleted.Channel != string(ChannelFeishu) {
		t.Fatalf("DeleteByChannelID() = %+v, %v; want feishu bot", deleted, ok)
	}
	if _, ok, err := store.GetByChannelID(string(ChannelFeishu), "u-alice"); err != nil || ok {
		t.Fatalf("GetByChannelID(feishu) = ok %v err %v, want deleted", ok, err)
	}
	if _, ok, err := store.GetByChannelID(string(ChannelCSGClaw), "u-alice"); err != nil || !ok {
		t.Fatalf("GetByChannelID(csgclaw) = ok %v err %v, want retained", ok, err)
	}
}

func TestStoreSaveRejectsInvalidBot(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if err := store.Save(Bot{Name: "Alice", Role: string(RoleWorker)}); err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("Save() error = %v, want id is required", err)
	}
	if err := store.Save(Bot{ID: "bot-1", Role: string(RoleWorker)}); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("Save() error = %v, want name is required", err)
	}
}

func TestNewStoreRejectsInvalidState(t *testing.T) {
	for _, tc := range []struct {
		name string
		role string
	}{
		{name: "agent", role: "agent"},
		{name: "notifier", role: "notifier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bots.json")
			raw := `{"bots":[{"id":"bot-1","name":"alice","role":"` + tc.role + `","channel":"csgclaw"}]}`
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatalf("os.WriteFile() error = %v", err)
			}
			if _, err := NewStore(path); err == nil || !strings.Contains(err.Error(), "role must be one of") {
				t.Fatalf("NewStore() error = %v, want role validation error", err)
			}
		})
	}
}
