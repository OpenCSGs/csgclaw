package notifier

import "testing"

func TestResolveRelayEndpoints(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		wantMsg string
		wantAck string
	}{
		{
			name:    "legacy_origin",
			in:      "https://relay.example.com",
			wantMsg: "https://relay.example.com/api/v1/inbox/messages",
			wantAck: "https://relay.example.com/api/v1/inbox/ack",
		},
		{
			name:    "legacy_origin_slash",
			in:      "https://relay.example.com/",
			wantMsg: "https://relay.example.com/api/v1/inbox/messages",
			wantAck: "https://relay.example.com/api/v1/inbox/ack",
		},
		{
			name:    "full_messages_url",
			in:      "https://relay.example.com/custom/api/v1/inbox/messages",
			wantMsg: "https://relay.example.com/custom/api/v1/inbox/messages",
			wantAck: "https://relay.example.com/custom/api/v1/inbox/ack",
		},
		{
			name:    "full_messages_url_with_query",
			in:      "https://relay.example.com/p/messages?x=1",
			wantMsg: "https://relay.example.com/p/messages?x=1",
			wantAck: "https://relay.example.com/p/ack",
		},
		{
			name:    "ingress_paste_url_rewrites_to_inbox",
			in:      "https://opencsg-stg.com/api/v1/csgbot/notification-relay/webhooks/ingress",
			wantMsg: "https://opencsg-stg.com/api/v1/csgbot/notification-relay/inbox/messages",
			wantAck: "https://opencsg-stg.com/api/v1/csgbot/notification-relay/inbox/ack",
		},
		{
			name:    "ingress_with_query_subscription",
			in:      "https://relay.example.com/api/v1/foo/webhooks/ingress?subscription_id=sub-1",
			wantMsg: "https://relay.example.com/api/v1/foo/inbox/messages?subscription_id=sub-1",
			wantAck: "https://relay.example.com/api/v1/foo/inbox/ack",
		},
		{
			name:    "default_api_v1_webhooks_ingress",
			in:      "https://relay.example.com/api/v1/webhooks/ingress",
			wantMsg: "https://relay.example.com/api/v1/inbox/messages",
			wantAck: "https://relay.example.com/api/v1/inbox/ack",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotMsg, gotAck, err := resolveRelayEndpoints(tc.in)
			if err != nil {
				t.Fatalf("resolveRelayEndpoints: %v", err)
			}
			if gotMsg != tc.wantMsg {
				t.Fatalf("messages URL\ngot  %q\nwant %q", gotMsg, tc.wantMsg)
			}
			if gotAck != tc.wantAck {
				t.Fatalf("ack URL\ngot  %q\nwant %q", gotAck, tc.wantAck)
			}
		})
	}
}

func TestResolvePullEndpointsOverrides(t *testing.T) {
	t.Parallel()
	cfg := Config{
		RemoteURL: "https://relay.example.com/api/v1/webhooks/ingress",
	}
	msg, ack, err := ResolvePullEndpoints(cfg)
	if err != nil {
		t.Fatalf("ResolvePullEndpoints: %v", err)
	}
	if msg != "https://relay.example.com/api/v1/inbox/messages" || ack != "https://relay.example.com/api/v1/inbox/ack" {
		t.Fatalf("defaults: msg=%q ack=%q", msg, ack)
	}
	cfg2 := Config{
		RemoteURL:         "https://relay.example.com/api/v1/webhooks/ingress",
		RemoteMessagesURL: "https://other.example/list",
		RemoteAckURL:      "https://other.example/done",
	}
	msg2, ack2, err := ResolvePullEndpoints(cfg2)
	if err != nil {
		t.Fatalf("ResolvePullEndpoints: %v", err)
	}
	if msg2 != "https://other.example/list" || ack2 != "https://other.example/done" {
		t.Fatalf("overrides: msg=%q ack=%q", msg2, ack2)
	}
	cfg3 := Config{
		RemoteURL:         "https://relay.example.com/x",
		RemoteMessagesURL: "https://custom.example/inbox",
	}
	msg3, ack3, err := ResolvePullEndpoints(cfg3)
	if err != nil {
		t.Fatalf("ResolvePullEndpoints: %v", err)
	}
	if msg3 != "https://custom.example/inbox" {
		t.Fatalf("partial msg override: %q", msg3)
	}
	if want := "https://relay.example.com/ack"; ack3 != want {
		t.Fatalf("ack partial: got %q want %q", ack3, want)
	}
}
