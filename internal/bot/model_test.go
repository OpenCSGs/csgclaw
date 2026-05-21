package bot

import "testing"

func TestNormalizeBotTypeDefaultsToNormal(t *testing.T) {
	if got := NormalizeBotType(""); got != BotTypeNormal {
		t.Fatalf("NormalizeBotType(\"\") = %q, want %q", got, BotTypeNormal)
	}
	if got := NormalizeBotType("notification"); got != BotTypeNotification {
		t.Fatalf("NormalizeBotType(notification) = %q, want %q", got, BotTypeNotification)
	}
}

func TestNormalizeBotSetsTypeDefault(t *testing.T) {
	b, err := NormalizeBot(Bot{
		ID:      "u-test",
		Name:    "test",
		Role:    "worker",
		Channel: "csgclaw",
	})
	if err != nil {
		t.Fatalf("NormalizeBot() error = %v", err)
	}
	if b.Type != BotTypeNormal {
		t.Fatalf("Type = %q, want %q", b.Type, BotTypeNormal)
	}
}

func TestShouldIncludeBotInList(t *testing.T) {
	t.Parallel()
	notify := Bot{ID: "n-a", Type: BotTypeNotification, Channel: string(ChannelCSGClaw)}
	worker := Bot{ID: "u-a", Type: BotTypeNormal, Channel: string(ChannelCSGClaw)}

	if !shouldIncludeBotInList(worker, string(ChannelCSGClaw), "") {
		t.Fatal("normal bot should appear in csgclaw list")
	}
	if !shouldIncludeBotInList(notify, string(ChannelCSGClaw), "") {
		t.Fatal("notification bot should appear in csgclaw list")
	}
	if shouldIncludeBotInList(notify, string(ChannelFeishu), "") {
		t.Fatal("notification bot should be excluded for feishu list")
	}
	if !shouldIncludeBotInList(notify, "", "") {
		t.Fatal("notification bot should appear when listing all channels")
	}
	notifyFeishu := Bot{ID: "n-f", Type: BotTypeNotification, Channel: string(ChannelFeishu)}
	if shouldIncludeBotInList(notifyFeishu, "", "") {
		t.Fatal("feishu notification bot should be excluded when listing all channels")
	}
	if !shouldIncludeBotInList(notify, string(ChannelCSGClaw), BotTypeNotification) {
		t.Fatal("type=notification should include csgclaw notification bot")
	}
	if shouldIncludeBotInList(worker, string(ChannelCSGClaw), BotTypeNotification) {
		t.Fatal("type=notification should exclude normal bot")
	}
	if !shouldIncludeBotInList(worker, string(ChannelCSGClaw), BotTypeNormal) {
		t.Fatal("type=normal should include worker bot")
	}
	if shouldIncludeBotInList(notify, string(ChannelCSGClaw), BotTypeNormal) {
		t.Fatal("type=normal should exclude notification bot")
	}
}
