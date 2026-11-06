package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"csgclaw/internal/auth"
	"csgclaw/internal/knowledgebase"
)

// Community owns discovery and durable instance metadata; relay owns delivery.
// relay.agent_id is the registration identity, not a WebSocket connection ID.
type Community interface {
	Register(context.Context, string, PublishRequest, *CommunityTarget) (CommunityInstance, error)
	Delete(context.Context, int64) error
	DeleteExternalRegistration(context.Context, string) error
	List(context.Context) ([]CommunityInstance, error)
}
type CommunityTarget struct {
	Type      string `json:"type"`
	ContentID string `json:"content_id"`
}

func (t CommunityTarget) validate() error {
	if (t.Type != "csgclaw" && t.Type != "csgclaw-workspace") || strings.TrimSpace(t.ContentID) == "" || len(t.ContentID) > 1024 || strings.ContainsAny(t.ContentID, "\x00\r\n") {
		return fmt.Errorf("invalid platform instance target")
	}
	return nil
}

// PlatformTargetFromEnvironment is used only by the automatic startup request.
// CSGCLAW_NAME is the existing row's content_id; never guess an instance ID.
func PlatformTargetFromEnvironment() (*CommunityTarget, error) {
	contentID := strings.TrimSpace(os.Getenv("CSGCLAW_NAME"))
	typ := strings.TrimSpace(os.Getenv("OPENCSG_SANDBOX_TYPE"))
	if contentID == "" {
		if os.Getenv("CSGCLAW_AGENT_AUTO_PUBLISH") == "true" && (typ == "csgclaw" || typ == "csgclaw-workspace") {
			return nil, fmt.Errorf("CSGCLAW_NAME is required to update a platform instance")
		}
		return nil, nil
	}
	if typ == "" {
		typ = "csgclaw"
	}
	target := &CommunityTarget{Type: typ, ContentID: contentID}
	return target, target.validate()
}

var relayAgentIDPattern = regexp.MustCompile(`^agt_[0-9a-f]{24}$`)

type RelayRegistration struct {
	AgentID string `json:"agent_id"`
}
type CommunityInstance struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Public      bool               `json:"public"`
	Type        string             `json:"type"`
	Kind        string             `json:"kind"`
	Source      string             `json:"source"`
	ContentID   string             `json:"content_id"`
	Relay       *RelayRegistration `json:"relay,omitempty"`
	Metadata    struct {
		Avatar string `json:"avatar"`
	} `json:"metadata"`
}

func (i CommunityInstance) relayID() string {
	if i.Relay == nil {
		return ""
	}
	return i.Relay.AgentID
}

type SiteCredentials struct{ Site, Token, User string }
type CredentialsSource func() (SiteCredentials, error)

var ErrSiteLoginRequired = errors.New("sign in to OpenCSG before publishing or using community agents")

func CurrentSiteCredentials() (SiteCredentials, error) {
	store, err := auth.DefaultStore()
	if err != nil {
		return SiteCredentials{}, err
	}
	record, found, err := store.Load()
	if err != nil {
		return SiteCredentials{}, err
	}
	if !found || !record.Status().Authenticated {
		return SiteCredentials{}, ErrSiteLoginRequired
	}
	return SiteCredentials{Site: strings.TrimRight(record.Account.BaseURL, "/"), Token: record.Tokens.AccessToken, User: record.Account.UserUUID}, nil
}

// LoadSiteCredentials prefers the interactive login as a complete identity.
// Only an absent/expired login falls back to the existing managed-runner env.
func LoadSiteCredentials() (SiteCredentials, error) {
	return loadSiteCredentials(CurrentSiteCredentials)
}
func loadSiteCredentials(interactive CredentialsSource) (SiteCredentials, error) {
	cred, err := interactive()
	if err == nil {
		return cred, nil
	}
	if !errors.Is(err, ErrSiteLoginRequired) {
		return SiteCredentials{}, err
	}
	connection, ok := knowledgebase.ManagedConnection()
	if !ok {
		return SiteCredentials{}, ErrSiteLoginRequired
	}
	// This is a local credential binding, never an upstream user identity header.
	// Without a login UUID, scope stored publications to this injected credential.
	user := fmt.Sprintf("managed-%x", sha256.Sum256([]byte(connection.CSGHubAccessToken)))
	return SiteCredentials{Site: connection.CSGHubBaseURL, Token: connection.CSGHubAccessToken, User: user}, nil
}

// ResolveRelayProxy targets the authenticated StarHub business relay gateway.
func ResolveRelayProxy(site string) string { return strings.TrimRight(site, "/") + "/api/v1/relay" }

type HubCommunity struct{ Credentials CredentialsSource }

func (h HubCommunity) client() (*Client, error) {
	cred, err := h.Credentials()
	if err != nil {
		return nil, err
	}
	return NewClient(cred.Site, cred.Token, nil)
}

func (h HubCommunity) request(ctx context.Context, method, path string, body, out any) error {
	c, err := h.client()
	if err != nil {
		return err
	}
	return c.Request(ctx, method, "/api/v1/agent/instances"+path, "", body, out)
}

// Register upserts external agents after relay registration. Platform workers
// patch only the relay field of their existing instance, preserving its lifecycle.
func (h HubCommunity) Register(ctx context.Context, relayID string, req PublishRequest, target *CommunityTarget) (CommunityInstance, error) {
	if !relayAgentIDPattern.MatchString(relayID) {
		return CommunityInstance{}, fmt.Errorf("invalid relay agent ID")
	}
	if target != nil {
		return h.updatePlatformRegistration(ctx, relayID, target)
	}
	body := map[string]any{"type": "csgclaw", "kind": "runtime", "source": "external", "name": req.Name, "description": req.Description, "public": req.Visibility == "public", "content_id": relayID, "relay": RelayRegistration{AgentID: relayID}}
	var result struct {
		Data CommunityInstance `json:"data"`
	}
	if err := h.request(ctx, http.MethodPost, "", body, &result); err != nil {
		return CommunityInstance{}, err
	}
	i := result.Data
	if i.ID <= 0 || i.Kind != "runtime" || i.relayID() != relayID {
		return i, fmt.Errorf("community returned an invalid relay registration")
	}
	if i.Source != "external" || i.ContentID != relayID || i.Type != "csgclaw" {
		return i, fmt.Errorf("community returned a different external instance")
	}
	return i, nil
}

func (h HubCommunity) updatePlatformRegistration(ctx context.Context, relayID string, target *CommunityTarget) (CommunityInstance, error) {
	if err := target.validate(); err != nil {
		return CommunityInstance{}, err
	}
	c, err := h.client()
	if err != nil {
		return CommunityInstance{}, err
	}
	path := "/api/v1/agent/instances/by-content-id/" + url.PathEscape(target.Type) + "/" + url.PathEscape(target.ContentID)
	body := map[string]RelayRegistration{"relay": {AgentID: relayID}}
	resp, err := c.request(ctx, http.MethodPut, path, "", body)
	if err != nil {
		return CommunityInstance{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CommunityInstance{}, &HTTPError{Status: resp.StatusCode}
	}

	const maxResponse = 8 << 20
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return CommunityInstance{}, fmt.Errorf("read platform registration response: %w", err)
	}
	if len(payload) > maxResponse {
		return CommunityInstance{}, fmt.Errorf("platform registration response exceeds 8 MiB")
	}
	// A successful writeback may return the instance or just acknowledge the update.
	payload = bytes.TrimSpace(payload)
	if len(payload) > 0 && !bytes.Equal(payload, []byte("null")) {
		var result struct {
			Msg  string          `json:"msg"`
			Code string          `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(payload, &result); err != nil || result.Code != "" {
			return CommunityInstance{}, fmt.Errorf("invalid platform registration response")
		}
		switch {
		case len(result.Data) == 0:
			if result.Msg != "OK" {
				return CommunityInstance{}, fmt.Errorf("invalid platform registration response")
			}
		case !bytes.Equal(result.Data, []byte("null")):
			var instance CommunityInstance
			if err := json.Unmarshal(result.Data, &instance); err != nil || !matchesPlatformRegistration(instance, relayID, target) {
				return CommunityInstance{}, fmt.Errorf("community returned an invalid platform instance")
			}
		}
	}

	instances, err := h.list(ctx, c)
	if err != nil {
		return CommunityInstance{}, err
	}
	for _, instance := range instances {
		if matchesPlatformRegistration(instance, relayID, target) {
			return instance, nil
		}
	}
	return CommunityInstance{}, fmt.Errorf("platform registration not found in community catalog")
}

func matchesPlatformRegistration(instance CommunityInstance, relayID string, target *CommunityTarget) bool {
	return instance.ID > 0 && instance.Kind == "runtime" && instance.relayID() == relayID &&
		instance.Source == "platform" && instance.ContentID == target.ContentID && instance.Type == target.Type
}

func (h HubCommunity) Delete(ctx context.Context, id int64) error {
	path := "/" + strconv.FormatInt(id, 10)
	var record struct {
		Data CommunityInstance `json:"data"`
	}
	if err := h.request(ctx, http.MethodGet, path, nil, &record); err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if record.Data.Source != "external" || record.Data.Type != "csgclaw" {
		return fmt.Errorf("refusing to delete a non-external community instance")
	}
	err := h.request(ctx, http.MethodDelete, path, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// DeleteExternalRegistration recovers an instance whose successful creation
// response was lost. Do not filter public: private external rows also need cleanup.
func (h HubCommunity) DeleteExternalRegistration(ctx context.Context, relayID string) error {
	if !relayAgentIDPattern.MatchString(relayID) {
		return fmt.Errorf("invalid relay agent ID")
	}
	for page := 1; ; page++ {
		var result struct {
			Data  []CommunityInstance `json:"data"`
			Total int                 `json:"total"`
		}
		if err := h.request(ctx, http.MethodGet, fmt.Sprintf("?source=external&kind=runtime&per=100&page=%d", page), nil, &result); err != nil {
			return err
		}
		for _, item := range result.Data {
			if item.Source == "external" && item.Type == "csgclaw" && item.ContentID == relayID && item.relayID() == relayID {
				return h.Delete(ctx, item.ID)
			}
		}
		if len(result.Data) < 100 || (result.Total > 0 && page*100 >= result.Total) {
			return nil
		}
	}
}

func (h HubCommunity) List(ctx context.Context) ([]CommunityInstance, error) {
	c, err := h.client()
	if err != nil {
		return nil, err
	}
	return h.list(ctx, c)
}

// StarHub returns public instances and private instances owned by this login.
func (h HubCommunity) list(ctx context.Context, c *Client) ([]CommunityInstance, error) {
	items := []CommunityInstance{}
	for page := 1; ; page++ {
		var result struct {
			Data  []CommunityInstance `json:"data"`
			Total int                 `json:"total"`
		}
		if err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v1/agent/instances?registered=true&kind=runtime&per=100&page=%d", page), "", nil, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Data {
			if (item.Type == "csgclaw" || item.Type == "csgclaw-workspace") && item.Kind == "runtime" && relayAgentIDPattern.MatchString(item.relayID()) {
				items = append(items, item)
			}
		}
		if len(result.Data) < 100 || (result.Total > 0 && page*100 >= result.Total) {
			break
		}
	}
	return items, nil
}
