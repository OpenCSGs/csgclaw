package feishubind

import (
	"errors"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
)

func TestValidateBotAppIDExclusive(t *testing.T) {
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:      "pt-dev",
		Channel: participant.ChannelFeishu,
		Type:    participant.TypeAgent,
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_shared",
			"app_secret": "dev-secret",
		},
		AgentID: "agent-dev",
	}}))

	if err := ValidateBotAppIDExclusive(participantSvc, "u-dev", "cli_shared"); err != nil {
		t.Fatalf("same worker validation error = %v", err)
	}
	if err := ValidateBotAppIDExclusive(participantSvc, "u-qa", "cli_shared"); !errors.Is(err, ErrBotAppIDConflict) {
		t.Fatalf("other worker validation error = %v, want ErrBotAppIDConflict", err)
	} else if !strings.Contains(err.Error(), "disconnect Feishu") || !strings.Contains(err.Error(), "agent-dev") {
		t.Fatalf("conflict guidance = %q", err)
	}
}
