package notifier

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Fanouter delivers notifier chat content (JSON notify card) from notifier payloads to IM rooms.
type Fanouter interface {
	DeliverNotifierFanout(agentID string, content string) error
}

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

// RelayClient pulls from a compatible relay; remote_url resolution is in resolveRelayEndpoints.
type RelayClient struct {
	HTTP *http.Client
}

func (c *RelayClient) client() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func joinAPIPath(base, suffix string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", fmt.Errorf("remote_url is required")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid remote_url")
	}
	rel, err := url.Parse(strings.TrimPrefix(suffix, "/"))
	if err != nil {
		return "", err
	}
	return u.ResolveReference(rel).String(), nil
}

// deriveInboxURLsFromThirdPartyIngress treats remote_url values that point at a relay **POST webhook
// ingress** (what GitLab etc. paste) and maps them to the sibling **GET list** and **POST ack**
// paths under the same path prefix: …/webhooks/ingress → …/inbox/messages and …/inbox/ack.
func deriveInboxURLsFromThirdPartyIngress(u *url.URL) (messages *url.URL, ack *url.URL, ok bool) {
	if u == nil {
		return nil, nil, false
	}
	p := path.Clean(strings.TrimSpace(u.Path))
	if p == "." || p == "/" {
		return nil, nil, false
	}
	lower := strings.ToLower(p)
	suffixes := []string{"/webhooks/ingress", "/webhook/ingress"}
	for _, suf := range suffixes {
		idx := strings.LastIndex(lower, suf)
		if idx < 0 || idx+len(suf) != len(lower) {
			continue
		}
		parent := strings.TrimSuffix(strings.TrimSuffix(p[:idx], "/"), "/")
		if parent == "" {
			parent = "/"
		}
		msgPath := path.Join(parent, "inbox", "messages")
		ackPath := path.Join(parent, "inbox", "ack")
		mu := *u
		mu.Path = msgPath
		mu.Fragment = ""
		au := *u
		au.Path = ackPath
		au.RawQuery = ""
		au.Fragment = ""
		return &mu, &au, true
	}
	return nil, nil, false
}

// resolveRelayEndpoints interprets remote_url for pull mode.
// If the URL has no path (or only "/"), legacy mode appends /api/v1/inbox/messages and /api/v1/inbox/ack.
// If the path ends with …/webhooks/ingress (third-party paste URL), it is rewritten to …/inbox/messages
// and …/inbox/ack under the same prefix.
// Otherwise remote_url is the full URL of the GET messages list endpoint; ack is the same path
// with the last path segment replaced by "ack" (sibling resource).
func resolveRelayEndpoints(remoteURL string) (messages string, ack string, err error) {
	base := strings.TrimSpace(remoteURL)
	if base == "" {
		return "", "", fmt.Errorf("remote_url is required")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid remote_url")
	}
	if mu, au, ok := deriveInboxURLsFromThirdPartyIngress(u); ok {
		return mu.String(), au.String(), nil
	}
	p := strings.TrimSuffix(strings.TrimSpace(u.Path), "/")
	if p == "" {
		msgs, err := joinAPIPath(base, "/api/v1/inbox/messages")
		if err != nil {
			return "", "", err
		}
		ackEp, err := joinAPIPath(base, "/api/v1/inbox/ack")
		if err != nil {
			return "", "", err
		}
		return msgs, ackEp, nil
	}
	ackURL := *u
	ackURL.RawQuery = ""
	p = path.Clean(u.Path)
	parent := path.Dir(p)
	ackURL.Path = path.Join(parent, "ack")
	u.Fragment = ""
	ackURL.Fragment = ""
	return u.String(), ackURL.String(), nil
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
	if tok := strings.TrimSpace(cfg.RemoteToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
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
	if tok := strings.TrimSpace(cfg.RemoteToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
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
