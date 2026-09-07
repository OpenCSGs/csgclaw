package larkcli

import (
	"context"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
)

type sourceParticipantReader struct {
	item apitypes.Participant
}

func (r *sourceParticipantReader) Get(channel, id string) (apitypes.Participant, bool) {
	return r.item, channel == participant.ChannelFeishu && id == r.item.ID
}

func TestSourceResolvesParticipantWithoutCopyingAppSecret(t *testing.T) {
	reader := &sourceParticipantReader{item: apitypes.Participant{
		ID: "pt-dev", Channel: participant.ChannelFeishu, Type: participant.TypeAgent, AgentID: "agent-dev",
		ChannelAppConfig: map[string]any{"app_id": "cli_dev", "app_secret": "app-secret"},
	}}
	source, err := NewSource(Options{
		Participants: reader,
		BaseURL:      func() string { return "http://csgclaw.test/" },
		AccessToken:  func(string, string, string) (string, error) { return "source-token", nil },
		HelperPath:   func() (string, error) { return "/opt/csgclaw", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := source.Resolve(context.Background(), "agent-dev", "pt-dev")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := larkextension.Decode(first.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if payload.AppID != "cli_dev" || payload.AgentID != "agent-dev" || payload.AccessToken != "source-token" || payload.BaseURL != "http://csgclaw.test" {
		t.Fatalf("payload = %+v", payload)
	}
	if strings.Contains(string(first.Payload), "app-secret") {
		t.Fatalf("resolved payload copied App Secret: %s", first.Payload)
	}
	reader.item.ChannelAppConfig[participant.ChannelAppConfigAppSecretKey] = "rotated-secret"
	second, err := source.Resolve(context.Background(), "agent-dev", "pt-dev")
	if err != nil {
		t.Fatal(err)
	}
	if second.SourceRevision == first.SourceRevision {
		t.Fatal("source revision did not change after App Secret rotation")
	}
}
