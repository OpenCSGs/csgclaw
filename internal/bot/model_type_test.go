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
