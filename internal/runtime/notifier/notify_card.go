package notifier

import (
	"bytes"
	"encoding/json"
	"strings"
)

// NotifyCardType is the JSON "type" for Web UI structured notifier messages.
const NotifyCardType = "csgclaw.notify_card"

const (
	notifyCardSchemaVersion = 1
	maxNotifyCardJSONRunes  = 12000
	maxNotifySummaryRunes   = 4000
	maxNotifyMetaValueRunes = 2000
	maxNotifyRawRunes       = 10000
)

// NotifyCard is the canonical JSON shape stored as IM message content for notifier deliveries.
type NotifyCard struct {
	Type          string       `json:"type"`
	SchemaVersion int          `json:"schema_version"`
	Provider      string       `json:"provider"`
	Event         string       `json:"event,omitempty"`
	Title         string       `json:"title"`
	Subtitle      string       `json:"subtitle,omitempty"`
	Badge         string       `json:"badge,omitempty"`
	Summary       string       `json:"summary,omitempty"`
	Link          string       `json:"link,omitempty"`
	Meta          []NotifyMeta `json:"meta,omitempty"`
	Raw           string       `json:"raw,omitempty"`
}

// NotifyMeta is one label/value row for the Web UI card body.
type NotifyMeta struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// FormatPayloadAsChatContent turns webhook (or other) bytes into a single-line JSON notify card
// for the CSGClaw Web IM. Known GitLab/GitHub webhook shapes get structured fields; others use
// provider "generic" with optional Raw.
func FormatPayloadAsChatContent(payload []byte, contentType string) string {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return marshalNotifyCard(emptyBodyCard())
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	jsonLike := strings.Contains(ct, "json") || json.Valid(payload)
	if !jsonLike {
		return marshalNotifyCard(nonJSONBodyCard(string(payload)))
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return marshalNotifyCard(invalidJSONBodyCard(string(payload)))
	}
	if card, ok := cardFromKnownWebhooks(root); ok {
		return marshalNotifyCard(card)
	}
	return marshalNotifyCard(genericJSONObjectCard(root))
}

func marshalNotifyCard(c NotifyCard) string {
	c.Type = NotifyCardType
	c.SchemaVersion = notifyCardSchemaVersion
	truncateNotifyCardFields(&c)
	b, err := json.Marshal(c)
	if err != nil {
		fallback, _ := json.Marshal(NotifyCard{
			Type:          NotifyCardType,
			SchemaVersion: notifyCardSchemaVersion,
			Provider:      "generic",
			Title:         "Notification",
			Summary:       "Could not encode notify card.",
		})
		return string(fallback)
	}
	out := string(b)
	if len([]rune(out)) <= maxNotifyCardJSONRunes {
		return out
	}
	c.Raw = truncateRunes(c.Raw, maxNotifyRawRunes/2)
	c.Summary = truncateRunes(c.Summary, 800) + "…"
	for i := range c.Meta {
		c.Meta[i].Value = truncateRunes(c.Meta[i].Value, 400)
	}
	b, _ = json.Marshal(c)
	return string(b)
}

func truncateNotifyCardFields(c *NotifyCard) {
	c.Title = truncateRunes(strings.TrimSpace(c.Title), 500)
	c.Subtitle = truncateRunes(strings.TrimSpace(c.Subtitle), 500)
	c.Badge = truncateRunes(strings.TrimSpace(c.Badge), 200)
	c.Summary = truncateRunes(strings.TrimSpace(c.Summary), maxNotifySummaryRunes)
	c.Link = strings.TrimSpace(c.Link)
	c.Raw = truncateRunes(strings.TrimSpace(c.Raw), maxNotifyRawRunes)
	for i := range c.Meta {
		c.Meta[i].Label = truncateRunes(strings.TrimSpace(c.Meta[i].Label), 120)
		c.Meta[i].Value = truncateRunes(strings.TrimSpace(c.Meta[i].Value), maxNotifyMetaValueRunes)
	}
}

func emptyBodyCard() NotifyCard {
	return NotifyCard{
		Provider: "generic",
		Event:    "empty",
		Title:    "Notification",
		Summary:  "Empty notification body",
	}
}

func nonJSONBodyCard(body string) NotifyCard {
	return NotifyCard{
		Provider: "generic",
		Event:    "text",
		Title:    "Notification",
		Summary:  truncateRunes(strings.TrimSpace(body), maxNotifySummaryRunes),
	}
}

func invalidJSONBodyCard(raw string) NotifyCard {
	return NotifyCard{
		Provider: "generic",
		Event:    "invalid_json",
		Title:    "Notification",
		Summary:  "Body is not valid JSON; raw payload in details.",
		Raw:      truncateRunes(raw, maxNotifyRawRunes),
	}
}

func genericJSONObjectCard(root map[string]any) NotifyCard {
	raw, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return NotifyCard{
			Provider: "generic",
			Event:    "json",
			Title:    "Notification",
			Summary:  "Could not format JSON payload.",
		}
	}
	return NotifyCard{
		Provider: "generic",
		Event:    "json",
		Title:    "Notification",
		Subtitle: "Unknown JSON",
		Summary:  "No GitLab/GitHub webhook matcher; see raw payload below.",
		Raw:      truncateRunes(string(raw), maxNotifyRawRunes),
	}
}
