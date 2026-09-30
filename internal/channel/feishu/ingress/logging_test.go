package ingress

import (
	"fmt"
	"strings"
	"testing"

	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/transport"
)

func TestControlLoggingExcludesArbitraryPayloads(t *testing.T) {
	event := transport.Event{Kind: transport.EventCardAction, CardAction: &transport.CardAction{
		Token:       "private-callback-token",
		ActionValue: map[string]any{"operation": "private-action-value", "cmd": "stop", "extra": "private-extra-value"},
		FormValue:   map[string]any{"answer": "private-form-answer"},
	}}
	attrs := fmt.Sprint(eventLogAttrs(channel.Binding{}, event))
	if strings.Contains(attrs, "private-") {
		t.Fatalf("private payload in logs: %s", attrs)
	}
	if !strings.Contains(attrs, "unrecognized") || !strings.Contains(attrs, "stop") {
		t.Fatal("missing bounded control classification")
	}
}
