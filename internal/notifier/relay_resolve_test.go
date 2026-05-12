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
