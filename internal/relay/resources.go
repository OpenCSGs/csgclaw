package relay

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/conversationresource"
	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/resourcequery"
)

var ErrResourcesUnsupported = errors.New("update the cloud agent to support resource selection")
var ErrCloudEndpointUnavailable = errors.New("cloud agent read endpoint unavailable")
var ErrCloudAgentOffline = errors.New("cloud agent is offline; passive reads cannot resume it")

func (m *Manager) cloudReadConnection(ctx context.Context) (knowledgebase.Connection, error) {
	if m.credentials == nil {
		return knowledgebase.Connection{}, ErrChatReference
	}
	cred, err := m.credentials()
	if err != nil {
		return knowledgebase.Connection{}, err
	}
	loadConnection := m.cloudConnection
	if loadConnection == nil {
		loadConnection = knowledgebase.LoadConnection
	}
	connection, err := loadConnection(ctx)
	if err != nil || strings.TrimRight(connection.CSGHubBaseURL, "/") != strings.TrimRight(cred.Site, "/") || connection.CSGHubAccessToken != cred.Token {
		return knowledgebase.Connection{}, ErrChatReference
	}
	return connection, nil
}

// CloudProfile deliberately excludes environment, provider credentials and MCP configuration.
// A runtime instance, rather than its template, is authoritative for these fields.
type CloudProfile struct {
	Provider     string `json:"provider,omitempty"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	Runtime      string `json:"runtime"`
	Model        string `json:"model"`
}
type ResourcePage struct {
	Items []apitypes.AgentResourceSummary `json:"items"`
	resourcequery.Metadata
}

func (m *Manager) CheckResourceSupport(ctx context.Context, id, connectionID string) error {
	c, err := m.invocationClient(ctx, id, connectionID)
	if err != nil {
		return err
	}
	var agent Agent
	if err := c.Request(ctx, http.MethodGet, "/v1/agents/"+url.PathEscape(id), "", nil, &agent); err != nil {
		return err
	}
	for _, capability := range agent.Capabilities {
		if capability == CapabilityResourceRefs && agent.ID == id {
			return nil
		}
	}
	return ErrResourcesUnsupported
}

// Only a current, authorized platform record can select an endpoint. The browser
// never supplies a URL or credential, and credentials never follow redirects.
func (m *Manager) cloudReader(ctx context.Context, id, connectionID string) (*Client, string, error) {
	entry, err := m.DiscoverAgent(ctx, id, connectionID)
	if err != nil {
		return nil, "", err
	}
	if m.credentials == nil || m.community == nil || entry.Source != "platform" || (entry.Type != "csgclaw" && entry.Type != "csgclaw-workspace") {
		return nil, "", ErrCloudEndpointUnavailable
	}
	if entry.Agent.Status != "online" {
		return nil, "", ErrCloudAgentOffline
	}
	if !safeID(entry.Agent.SourceAgentID) {
		return nil, "", ErrCloudEndpointUnavailable
	}
	cred, err := m.credentials()
	if err != nil {
		return nil, "", err
	}
	connection, err := m.cloudReadConnection(ctx)
	if err != nil {
		return nil, "", err
	}
	m.mu.Lock()
	_, community, reference, err := m.consumerConnection()
	m.mu.Unlock()
	if err != nil {
		return nil, "", err
	}
	if reference != connectionID {
		return nil, "", ErrChatReference
	}
	instances, err := community.List(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, instance := range instances {
		if instance.relayID() != id || instance.Source != "platform" {
			continue
		}
		expectedSandbox := instance.ContentID
		if instance.Kind == "logical" {
			expectedSandbox = ""
		}
		endpoint, err := validatedCloudEndpoint(instance.Endpoint, connection.AIGatewayBaseURL, expectedSandbox)
		if err != nil {
			return nil, "", err
		}
		c, err := NewClient(endpoint, cred.Token, nil)
		if err == nil {
			c.credentials = func() (SiteCredentials, error) {
				current, err := m.credentials()
				if err != nil {
					return SiteCredentials{}, err
				}
				if m.connectionReferenceFor(current) != connectionID {
					return SiteCredentials{}, ErrChatReference
				}
				return current, nil
			}
		}
		return c, entry.Agent.SourceAgentID, err
	}
	return nil, "", ErrChatReference
}

func validatedCloudEndpoint(raw, gateway, expectedSandbox string) (string, error) {
	endpoint, err := url.Parse(raw)
	trusted, trustedErr := url.Parse(strings.TrimSuffix(strings.TrimRight(gateway, "/"), "/v1"))
	if err != nil || trustedErr != nil || endpoint == nil || trusted == nil || endpoint.Host == "" || endpoint.Host != trusted.Host || endpoint.Scheme != trusted.Scheme || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "", ErrCloudEndpointUnavailable
	}
	sandbox, ok := strings.CutPrefix(endpoint.Path, strings.TrimRight(trusted.Path, "/")+"/v1/sandboxes/")
	if !ok || !safeID(sandbox) || sandbox == "." || sandbox == ".." || (expectedSandbox != "" && sandbox != expectedSandbox) || endpoint.RawPath != "" {
		return "", ErrCloudEndpointUnavailable
	}
	return endpoint.String(), nil
}

func (m *Manager) CloudProfile(ctx context.Context, id, connectionID string) (CloudProfile, error) {
	c, source, err := m.cloudReader(ctx, id, connectionID)
	if err != nil {
		return CloudProfile{}, err
	}
	var agent apitypes.Agent
	if err := c.Request(ctx, http.MethodGet, "/api/v1/agents/"+url.PathEscape(source), "", nil, &agent); err != nil {
		return CloudProfile{}, err
	}
	if agent.ID != source {
		return CloudProfile{}, ErrChatReference
	}
	return CloudProfile{Provider: agent.ProfileConfig.ModelProviderID, ID: source, Name: agent.Name, Description: agent.Description, Instructions: agent.Instructions, Runtime: agent.Runtime.Name, Model: agent.ProfileConfig.ModelID}, nil
}

func (m *Manager) CloudResources(ctx context.Context, id, connectionID string, query url.Values) (ResourcePage, error) {
	c, source, err := m.cloudReader(ctx, id, connectionID)
	if err != nil {
		return ResourcePage{}, err
	}
	return readCloudResources(ctx, c, source, query)
}

func readCloudResources(ctx context.Context, c *Client, source string, query url.Values) (ResourcePage, error) {
	var page ResourcePage
	err := c.Request(ctx, http.MethodGet, "/api/v1/agents/"+url.PathEscape(source)+"/resources?"+query.Encode(), "", nil, &page)
	return page, err
}

// Rebuild display snapshots from the remote catalog; user-provided labels are never trusted.
// The executor resolves these IDs again immediately before the runtime turn.
func (m *Manager) ResolveCloudResources(ctx context.Context, id, connectionID, content string, refs []apitypes.ResourceReference) (string, *apitypes.ResourceSelection, error) {
	input, err := conversationresource.Normalize(content, refs)
	if err != nil {
		return "", nil, err
	}
	if err := m.CheckResourceSupport(ctx, id, connectionID); err != nil {
		return "", nil, err
	}
	c, source, err := m.cloudReader(ctx, id, connectionID)
	if err != nil {
		return "", nil, err
	}
	selection := &apitypes.ResourceSelection{Version: 1, AgentID: id, Refs: []apitypes.ResourceSnapshot{}}
	for _, ref := range input.Refs {
		kinds := []apitypes.ResourceType{apitypes.ResourceTypeSkill}
		if ref.Source != apitypes.ResourceSourceSkill {
			kinds = []apitypes.ResourceType{apitypes.ResourceTypeConnector, apitypes.ResourceTypeKnowledgeBase}
		}
		var found *apitypes.AgentResourceSummary
		for _, kind := range kinds {
			query := url.Values{"pagination": {"cursor"}, "limit": {"100"}, "type": {string(kind)}, "search": {ref.ID}}
			for {
				page, err := readCloudResources(ctx, c, source, query)
				if err != nil {
					return "", nil, err
				}
				for _, item := range page.Items {
					if item.ResourceReference == ref {
						copy := item
						found = &copy
						break
					}
				}
				if found != nil || !page.HasMore || page.NextCursor == "" {
					break
				}
				query.Set("cursor", page.NextCursor)
			}
			if found != nil {
				break
			}
		}
		if found == nil {
			return "", nil, &conversationresource.Error{Code: conversationresource.ResourceNotAttached, Reference: ref}
		}
		if !found.Enabled {
			return "", nil, &conversationresource.Error{Code: conversationresource.ResourceDisabled, Reference: ref}
		}
		if !found.Available {
			return "", nil, &conversationresource.Error{Code: conversationresource.ResourceUnavailable, Reference: ref}
		}
		selection.Refs = append(selection.Refs, found.ResourceSnapshot)
	}
	return input.Content, selection, nil
}
