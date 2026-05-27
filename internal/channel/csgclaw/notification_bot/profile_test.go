package notification_bot

import "testing"

func TestProfileDeliveryComplete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		flat map[string]any
		want bool
	}{
		{
			name: "webhook missing token",
			flat: map[string]any{"delivery_mode": "webhook"},
			want: false,
		},
		{
			name: "webhook ok",
			flat: map[string]any{"delivery_mode": "webhook", "webhook_token": "tok"},
			want: true,
		},
		{
			name: "pull url only",
			flat: map[string]any{
				"delivery_mode": "remote_pull",
				"remote_url":    "https://relay.example.com",
			},
			want: false,
		},
		{
			name: "pull ok",
			flat: map[string]any{
				"delivery_mode": "remote_pull",
				"remote_url":    "https://relay.example.com",
				"remote_token":  "secret",
			},
			want: true,
		},
		{
			name: "both partial webhook only",
			flat: map[string]any{
				"delivery_mode": "both",
				"webhook_token": "tok",
				"remote_url":    "https://relay.example.com",
			},
			want: false,
		},
		{
			name: "both ok",
			flat: map[string]any{
				"delivery_mode": "both",
				"webhook_token": "tok",
				"remote_url":    "https://relay.example.com",
				"remote_token":  "secret",
			},
			want: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ProfileDeliveryComplete(tc.flat); got != tc.want {
				t.Fatalf("ProfileDeliveryComplete() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPullDeliveryComplete(t *testing.T) {
	t.Parallel()
	cfg := ConfigFromStored(map[string]any{
		"delivery_mode": "remote_pull",
		"remote_url":    "https://relay.example.com",
	})
	if cfg.PullDeliveryComplete() {
		t.Fatal("PullDeliveryComplete() = true without token")
	}
	cfg.RemoteToken = "secret"
	if !cfg.PullDeliveryComplete() {
		t.Fatal("PullDeliveryComplete() = false with url and token")
	}
}
