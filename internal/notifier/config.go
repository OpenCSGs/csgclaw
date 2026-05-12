package notifier

import (
	"fmt"
	"strings"
	"time"
)

const (
	DeliveryWebhook    = "webhook"
	DeliveryRemotePull = "remote_pull"
	DeliveryBoth       = "both"
)

// Config is parsed from agent_profile.request_options["notifier"].
type Config struct {
	DeliveryMode         string
	WebhookToken         string
	RemoteURL            string
	RemoteSubscriptionID string
	PollInterval         string
	RemoteToken          string
}

func ParseConfigFromRequestOptions(ro map[string]any) Config {
	if ro == nil {
		return Config{}
	}
	raw, ok := ro["notifier"]
	if !ok || raw == nil {
		return Config{}
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return Config{}
	}
	return Config{
		DeliveryMode:         strings.TrimSpace(toString(m["delivery_mode"])),
		WebhookToken:         strings.TrimSpace(toString(m["webhook_token"])),
		RemoteURL:            strings.TrimSpace(toString(m["remote_url"])),
		RemoteSubscriptionID: strings.TrimSpace(toString(m["remote_subscription_id"])),
		PollInterval:         strings.TrimSpace(toString(m["poll_interval"])),
		RemoteToken:          strings.TrimSpace(toString(m["remote_token"])),
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return trimFloatJSON(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func trimFloatJSON(f float64) string {
	s := fmt.Sprintf("%.12f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

func (c Config) normalizedDeliveryMode() string {
	switch strings.ToLower(strings.TrimSpace(c.DeliveryMode)) {
	case DeliveryRemotePull:
		return DeliveryRemotePull
	case DeliveryBoth:
		return DeliveryBoth
	default:
		return DeliveryWebhook
	}
}

// AllowsWebhook reports whether inbound HTTP webhook should be accepted for this agent.
func (c Config) AllowsWebhook() bool {
	switch c.normalizedDeliveryMode() {
	case DeliveryWebhook, DeliveryBoth:
		return strings.TrimSpace(c.WebhookToken) != ""
	default:
		return false
	}
}

// AllowsPull reports whether background relay polling should run (requires remote_url).
func (c Config) AllowsPull() bool {
	if strings.TrimSpace(c.RemoteURL) == "" {
		return false
	}
	switch c.normalizedDeliveryMode() {
	case DeliveryRemotePull, DeliveryBoth:
		return true
	default:
		return false
	}
}

// PollIntervalDuration defaults to 30s when unset or invalid.
func (c Config) PollIntervalDuration() time.Duration {
	s := strings.TrimSpace(c.PollInterval)
	if s == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 5*time.Second {
		return 30 * time.Second
	}
	return d
}
