package notification

import "testing"

func TestResolveRelayRoutes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		in          string
		wantMsg     string
		wantAck     string
		wantIngress string
	}{
		{
			name:        "origin_only",
			in:          "https://relay.example.com",
			wantMsg:     "https://relay.example.com/inbox/messages",
			wantAck:     "https://relay.example.com/inbox/ack",
			wantIngress: "https://relay.example.com/webhooks/ingress",
		},
		{
			name:        "origin_trailing_slash",
			in:          "https://relay.example.com/",
			wantMsg:     "https://relay.example.com/inbox/messages",
			wantAck:     "https://relay.example.com/inbox/ack",
			wantIngress: "https://relay.example.com/webhooks/ingress",
		},
		{
			name:        "relay_service_base",
			in:          "http://opencsg-stg.com/api/v1/csgbot/notification-relay/",
			wantMsg:     "http://opencsg-stg.com/api/v1/csgbot/notification-relay/inbox/messages",
			wantAck:     "http://opencsg-stg.com/api/v1/csgbot/notification-relay/inbox/ack",
			wantIngress: "http://opencsg-stg.com/api/v1/csgbot/notification-relay/webhooks/ingress",
		},
		{
			name:        "strips_query_on_base",
			in:          "https://relay.example.com/api/v1/foo?subscription_id=sub-1",
			wantMsg:     "https://relay.example.com/api/v1/foo/inbox/messages",
			wantAck:     "https://relay.example.com/api/v1/foo/inbox/ack",
			wantIngress: "https://relay.example.com/api/v1/foo/webhooks/ingress",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotMsg, gotAck, gotIngress, err := ResolveRelayRoutes(tc.in)
			if err != nil {
				t.Fatalf("ResolveRelayRoutes: %v", err)
			}
			if gotMsg != tc.wantMsg {
				t.Fatalf("messages URL\ngot  %q\nwant %q", gotMsg, tc.wantMsg)
			}
			if gotAck != tc.wantAck {
				t.Fatalf("ack URL\ngot  %q\nwant %q", gotAck, tc.wantAck)
			}
			if gotIngress != tc.wantIngress {
				t.Fatalf("ingress URL\ngot  %q\nwant %q", gotIngress, tc.wantIngress)
			}
		})
	}
}

func TestNormalizeRemoteURLForStorage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{
			in:   "http://opencsg-stg.com/api/v1/csgbot/notification-relay/",
			want: "http://opencsg-stg.com/api/v1/csgbot/notification-relay",
		},
		{
			in:   "http://opencsg-stg.com/api/v1/csgbot/notification-relay/webhooks/ingress",
			want: "http://opencsg-stg.com/api/v1/csgbot/notification-relay",
		},
		{
			in:   "http://opencsg-stg.com/api/v1/csgbot/notification-relay/inbox/messages",
			want: "http://opencsg-stg.com/api/v1/csgbot/notification-relay",
		},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeRemoteURLForStorage(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestResolvePullEndpointsOverrides(t *testing.T) {
	t.Parallel()
	cfg := Config{
		RemoteURL: "https://relay.example.com/api/v1/csgbot/notification-relay",
	}
	msg, ack, err := ResolvePullEndpoints(cfg)
	if err != nil {
		t.Fatalf("ResolvePullEndpoints: %v", err)
	}
	if msg != "https://relay.example.com/api/v1/csgbot/notification-relay/inbox/messages" ||
		ack != "https://relay.example.com/api/v1/csgbot/notification-relay/inbox/ack" {
		t.Fatalf("defaults: msg=%q ack=%q", msg, ack)
	}
	cfg2 := Config{
		RemoteURL:         "https://relay.example.com/api/v1/csgbot/notification-relay",
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
}
