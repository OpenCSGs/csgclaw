package notification

import (
	"crypto/subtle"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DeliveryWebhook    = "webhook"
	DeliveryRemotePull = "remote_pull"
	DeliveryBoth       = "both"
)

// Config is parsed from flat delivery settings stored on participant metadata.
type Config struct {
	DeliveryMode         string
	WebhookToken         string
	RemoteURL            string
	RemoteMessagesURL    string
	RemoteAckURL         string
	RemoteSubscriptionID string
	PollInterval         string
	RemoteToken          string
}

// ParseNotifierDetails parses flat notifier configuration (delivery_mode, webhook_token, …).
func ParseNotifierDetails(m map[string]any) Config {
	if m == nil {
		return Config{}
	}
	return Config{
		DeliveryMode:         strings.TrimSpace(toString(m["delivery_mode"])),
		WebhookToken:         strings.TrimSpace(toString(m["webhook_token"])),
		RemoteURL:            strings.TrimSpace(toString(m["remote_url"])),
		RemoteMessagesURL:    strings.TrimSpace(toString(m["remote_messages_url"])),
		RemoteAckURL:         strings.TrimSpace(toString(m["remote_ack_url"])),
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

// WebhookDeliveryComplete reports whether inbound webhook delivery is fully configured.
func (c Config) WebhookDeliveryComplete() bool {
	return c.AllowsWebhook()
}

// PullDeliveryComplete reports whether relay pull is fully configured (URL + token).
func (c Config) PullDeliveryComplete() bool {
	if !c.AllowsPull() {
		return false
	}
	return strings.TrimSpace(c.RemoteToken) != ""
}

// PollIntervalDuration defaults to 5s when unset or invalid.
// Accepts Go duration strings (e.g. "2s", "45s") or a bare positive number meaning seconds (UI often sends "2").
func (c Config) PollIntervalDuration() time.Duration {
	const (
		defaultPoll = 5 * time.Second
		minPoll     = time.Second
		maxPoll     = 24 * time.Hour
	)
	s := strings.TrimSpace(c.PollInterval)
	if s == "" {
		return defaultPoll
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d < minPoll || d > maxPoll {
			return defaultPoll
		}
		return d
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil || sec <= 0 {
		return defaultPoll
	}
	d := time.Duration(sec * float64(time.Second))
	if d < minPoll || d > maxPoll {
		return defaultPoll
	}
	return d
}

// SecretMatch compares secrets in constant time when lengths match (recommended: fixed-length random tokens).
func SecretMatch(expected, got string) bool {
	if len(expected) == 0 || len(got) == 0 {
		return false
	}
	if len(expected) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}
