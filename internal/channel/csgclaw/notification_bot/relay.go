package notification_bot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	relayRouteInboxMessages    = "/inbox/messages"
	relayRouteInboxAck       = "/inbox/ack"
	relayRouteWebhooksIngress = "/webhooks/ingress"
)

// Inbox JSON types for relay GET list / POST ack (see types below).

type inboxMessagesResponse struct {
	Messages   []InboxMessage `json:"messages"`
	NextCursor *string        `json:"next_cursor"`
}

// InboxMessage is one row from the relay inbox messages list response.
type InboxMessage struct {
	ID                 string `json:"id"`
	PayloadContentType string `json:"payload_content_type"`
	PayloadBase64      string `json:"payload_base64"`
	ReceivedAt         string `json:"received_at"`
}

type inboxAckRequest struct {
	SubscriptionID string   `json:"subscription_id"`
	MessageIDs     []string `json:"message_ids"`
}

// RelayClient pulls from a compatible relay; remote_url is the relay service base URL.
type RelayClient struct {
	HTTP *http.Client
}

func (c *RelayClient) client() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 45 * time.Second}
}

var relayBasePathSuffixes = []string{
	relayRouteWebhooksIngress,
	"/webhook/ingress",
	relayRouteInboxMessages,
	relayRouteInboxAck,
}

func relayURLPathBase(path string) string {
	p := strings.TrimSuffix(strings.TrimSpace(path), "/")
	if p == "" || p == "/" {
		return ""
	}
	lower := strings.ToLower(p)
	for {
		trimmed := false
		for _, suf := range relayBasePathSuffixes {
			if strings.HasSuffix(lower, strings.ToLower(suf)) {
				p = strings.TrimSuffix(p, suf)
				p = strings.TrimSuffix(p, "/")
				lower = strings.ToLower(p)
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return p
}

// normalizeRelayBaseURL parses remote_url as the relay service base (no route suffix).
func normalizeRelayBaseURL(remoteURL string) (*url.URL, error) {
	base := strings.TrimSpace(remoteURL)
	if base == "" {
		return nil, fmt.Errorf("remote_url is required")
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid remote_url")
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = relayURLPathBase(u.Path)
	return u, nil
}

// NormalizeRemoteURLForStorage returns canonical relay base URL for persistence.
func NormalizeRemoteURLForStorage(remoteURL string) string {
	u, err := normalizeRelayBaseURL(remoteURL)
	if err != nil {
		return strings.TrimSpace(remoteURL)
	}
	return u.String()
}

func relayURLWithSuffix(base *url.URL, route string) string {
	u := *base
	root := strings.TrimSuffix(strings.TrimSpace(u.Path), "/")
	suffix := strings.TrimPrefix(strings.TrimSpace(route), "/")
	switch {
	case root == "" || root == "/":
		u.Path = "/" + suffix
	default:
		u.Path = root + "/" + suffix
	}
	return u.String()
}

// ResolveRelayRoutes appends /inbox/messages, /inbox/ack, and /webhooks/ingress under remote_url.
func ResolveRelayRoutes(remoteURL string) (messages, ack, ingress string, err error) {
	base, err := normalizeRelayBaseURL(remoteURL)
	if err != nil {
		return "", "", "", err
	}
	messages = relayURLWithSuffix(base, relayRouteInboxMessages)
	ack = relayURLWithSuffix(base, relayRouteInboxAck)
	ingress = relayURLWithSuffix(base, relayRouteWebhooksIngress)
	return messages, ack, ingress, nil
}

func resolveRelayEndpoints(remoteURL string) (messages string, ack string, err error) {
	messages, ack, _, err = ResolveRelayRoutes(remoteURL)
	return messages, ack, err
}

func bearerAuthHeaderValue(token string) string {
	tok := strings.TrimSpace(token)
	if tok == "" {
		return ""
	}
	if len(tok) >= 7 && strings.EqualFold(tok[:7], "bearer ") {
		tok = strings.TrimSpace(tok[7:])
	}
	if tok == "" {
		return ""
	}
	return "Bearer " + tok
}

// ResolvePullEndpoints returns the GET inbox list URL and POST ack URL for pull mode.
// Non-empty remote_messages_url / remote_ack_url on cfg override the corresponding default derived from remote_url.
func ResolvePullEndpoints(cfg Config) (messagesURL, ackURL string, err error) {
	defMsg, defAck, err := resolveRelayEndpoints(strings.TrimSpace(cfg.RemoteURL))
	if err != nil {
		return "", "", err
	}
	if s := strings.TrimSpace(cfg.RemoteMessagesURL); s != "" {
		defMsg = s
	}
	if s := strings.TrimSpace(cfg.RemoteAckURL); s != "" {
		defAck = s
	}
	return defMsg, defAck, nil
}

// FetchInbox performs GET on the configured relay messages list URL (see ResolvePullEndpoints).
func (c *RelayClient) FetchInbox(ctx context.Context, cfg Config, limit int, cursor string) ([]InboxMessage, string, error) {
	ep, _, err := ResolvePullEndpoints(cfg)
	if err != nil {
		return nil, "", err
	}
	u, err := url.Parse(ep)
	if err != nil {
		return nil, "", err
	}
	q := u.Query()
	if cfg.RemoteSubscriptionID != "" {
		q.Set("subscription_id", cfg.RemoteSubscriptionID)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if strings.TrimSpace(cursor) != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if auth := bearerAuthHeaderValue(cfg.RemoteToken); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("relay inbox: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed inboxMessagesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", fmt.Errorf("relay inbox json: %w", err)
	}
	next := ""
	if parsed.NextCursor != nil {
		next = strings.TrimSpace(*parsed.NextCursor)
	}
	return parsed.Messages, next, nil
}

// Ack posts to the relay ack URL (see ResolvePullEndpoints).
func (c *RelayClient) Ack(ctx context.Context, cfg Config, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	_, ep, err := ResolvePullEndpoints(cfg)
	if err != nil {
		return err
	}
	payload := inboxAckRequest{
		SubscriptionID: cfg.RemoteSubscriptionID,
		MessageIDs:     append([]string(nil), messageIDs...),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth := bearerAuthHeaderValue(cfg.RemoteToken); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay ack: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// DecodePayload returns raw bytes from inbox payload_base64.
func DecodePayload(m InboxMessage) ([]byte, string, error) {
	ct := strings.TrimSpace(m.PayloadContentType)
	if strings.TrimSpace(m.PayloadBase64) == "" {
		return nil, ct, fmt.Errorf("empty payload_base64")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(m.PayloadBase64))
	if err != nil {
		return nil, ct, err
	}
	return raw, ct, nil
}
