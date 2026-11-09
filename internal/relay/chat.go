package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

var ErrChatReference = errors.New("published agent or relay connection is no longer available")

// invocationClient resolves only a server-owned reference. Browsers cannot
// supply a URL or a token and cannot silently switch to another identity.
func (m *Manager) invocationClient(ctx context.Context, id, connectionID string) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.community != nil {
		expected, refErr := m.connectionReference()
		if refErr != nil {
			return nil, refErr
		}
		if connectionID == "" || connectionID != expected || !safeID(id) {
			return nil, ErrChatReference
		}
		instances, err := m.community.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range instances {
			if item.relayID() == id {
				return m.client(m.manifest.Endpoint, m.manifest.Token)
			}
		}
		return nil, ErrChatReference
	}
	req, exists := m.manifest.Agents[id]
	if !exists || req.Visibility != "public" || connectionID == "" || connectionID != m.manifest.ConnectionID || !safeID(id) {
		return nil, ErrChatReference
	}
	return m.client(m.manifest.Endpoint, m.manifest.Token)
}
func (m *Manager) Chat(ctx context.Context, id, connectionID, key string, input string, previous string) (Response, error) {
	c, err := m.invocationClient(ctx, id, connectionID)
	if err != nil {
		return Response{}, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return Response{}, err
	}
	return c.Call(ctx, key, CallRequest{Model: id, Input: raw, PreviousResponseID: previous})
}
func (m *Manager) ChatResponse(ctx context.Context, id, connectionID, responseID string) (Response, error) {
	c, err := m.invocationClient(ctx, id, connectionID)
	if err != nil {
		return Response{}, err
	}
	if !safeID(responseID) {
		return Response{}, ErrChatReference
	}
	var result Response
	err = c.Request(ctx, http.MethodGet, "/v1/responses/"+url.PathEscape(responseID), "", nil, &result)
	if err != nil {
		return Response{}, err
	}
	if result.ID == "" || result.Model == "" || result.Status == "" {
		return Response{}, errors.New("incomplete relay response")
	}
	if result.ID != responseID || result.Model != id {
		return Response{}, ErrChatReference
	}
	return result, nil
}
