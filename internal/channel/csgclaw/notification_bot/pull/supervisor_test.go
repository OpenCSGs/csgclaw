package pull

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/bot"
	"csgclaw/internal/channel/csgclaw/notification_bot"
)

type stubBotLister struct {
	flat map[string]any
}

func (s *stubBotLister) Reload() error { return nil }

func (s *stubBotLister) ListNotificationBots(string) ([]bot.Bot, error) {
	// API list view: secrets redacted (matches presentNotificationBot).
	return []bot.Bot{{
		ID:   "u-test",
		Type: bot.BotTypeNotification,
		RuntimeOptions: map[string]any{
			"delivery_mode":    "remote_pull",
			"remote_url":       s.flat["remote_url"],
			"remote_token_set": true,
		},
	}}, nil
}

func (s *stubBotLister) LookupNotificationBotForDelivery(string, string) (map[string]any, string, bool) {
	return s.flat, "u-test", true
}

func TestDesiredPullBotIDsUsesStoredTokenNotAPIView(t *testing.T) {
	t.Parallel()
	sup := &Supervisor{
		Bots: &stubBotLister{
			flat: map[string]any{
				"delivery_mode": "remote_pull",
				"remote_url":    "https://relay.example.com",
				"remote_token":  "secret-token",
			},
		},
	}
	got := sup.desiredPullBotIDs()
	if len(got) != 1 {
		t.Fatalf("desiredPullBotIDs() = %v, want one bot", got)
	}
	if _, ok := got["u-test"]; !ok {
		t.Fatalf("desiredPullBotIDs() missing u-test, got %v", got)
	}
}

func TestDesiredPullBotIDsSkipsWhenStoredTokenMissing(t *testing.T) {
	t.Parallel()
	sup := &Supervisor{
		Bots: &stubBotLister{flat: map[string]any{
			"delivery_mode": "remote_pull",
			"remote_url":    "https://relay.example.com",
		}},
	}
	if len(sup.desiredPullBotIDs()) != 0 {
		t.Fatal("desiredPullBotIDs() should be empty without stored remote_token")
	}
}

func TestSupervisorPullUsesStoredTokenNotAPIView(t *testing.T) {
	t.Parallel()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[],"next_cursor":null}`))
	}))
	defer srv.Close()

	relay := &notification_bot.RelayClient{HTTP: srv.Client()}
	sup := &Supervisor{
		Bots: &stubBotLister{
			flat: map[string]any{
				"delivery_mode":          "remote_pull",
				"remote_url":             srv.URL,
				"remote_token":           "secret-token",
				"remote_subscription_id": "sub-1",
				"poll_interval":          "5s",
			},
		},
		Relay: relay,
	}
	b, cfg, ok := sup.lookupBot("u-test")
	if !ok {
		t.Fatal("lookupBot() = false")
	}
	if err := sup.pullBot(context.Background(), b, cfg); err != nil {
		t.Fatalf("pullBot() error = %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want Bearer secret-token", gotAuth)
	}
}

type stubFanouter struct {
	calls    int
	failFrom int
}

func (f *stubFanouter) DeliverFanout(string, string) error {
	f.calls++
	if f.failFrom > 0 && f.calls >= f.failFrom {
		return errDeliverFail
	}
	return nil
}

var errDeliverFail = &deliverFailError{}

type deliverFailError struct{}

func (e *deliverFailError) Error() string { return "deliver failed" }

func TestPullBotAcksDeliveredBeforeFailure(t *testing.T) {
	t.Parallel()
	var acked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1","payload_content_type":"text/plain","payload_base64":"aGVsbG8="},{"id":"m2","payload_content_type":"text/plain","payload_base64":"d29ybGQ="}],"next_cursor":null}`))
		case http.MethodPost:
			acked = append(acked, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	relay := &notification_bot.RelayClient{HTTP: srv.Client()}
	deliver := &stubFanouter{failFrom: 2}
	sup := &Supervisor{
		Bots: &stubBotLister{
			flat: map[string]any{
				"delivery_mode":          "remote_pull",
				"remote_url":             srv.URL,
				"remote_token":           "secret-token",
				"remote_subscription_id": "sub-1",
			},
		},
		Relay:   relay,
		Deliver: deliver,
	}
	b, cfg, ok := sup.lookupBot("u-test")
	if !ok {
		t.Fatal("lookupBot() = false")
	}
	err := sup.pullBot(context.Background(), b, cfg)
	if err == nil {
		t.Fatal("pullBot() error = nil, want deliver failure")
	}
	if deliver.calls != 2 {
		t.Fatalf("DeliverFanout calls = %d, want 2", deliver.calls)
	}
	if len(acked) != 1 {
		t.Fatalf("ack POST count = %d, want 1", len(acked))
	}
}
