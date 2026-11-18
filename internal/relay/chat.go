package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"
)

var ErrChatReference = errors.New("published agent or relay connection is no longer available")
var ErrInvalidChatResponse = errors.New("invalid relay chat response")

var ErrExecutorFilesUnsupported = errors.New("upgrade the publishing CSGClaw and Relay to support attachments")

func (m *Manager) CheckAttachmentSupport(ctx context.Context, id, connectionID string) error {
	client, err := m.invocationClient(ctx, id, connectionID)
	if err != nil {
		return err
	}
	return client.checkAttachmentSupport(ctx, id)
}

func (c *Client) checkAttachmentSupport(ctx context.Context, id string) error {
	var agent Agent
	if err := c.Request(ctx, http.MethodGet, "/v1/agents/"+url.PathEscape(id), "", nil, &agent); err != nil {
		return err
	}
	if agent.ID != id || !slices.Contains(agent.Capabilities, CapabilityInputFiles) {
		return ErrExecutorFilesUnsupported
	}
	return nil
}

// invocationClient resolves only a server-owned reference. Browsers cannot
// supply a URL or a token and cannot silently switch to another identity.
func (m *Manager) invocationClient(ctx context.Context, id, connectionID string) (*Client, error) {
	if err := m.lockInvocation(ctx); err != nil {
		return nil, err
	}
	if m.community != nil {
		expected, refErr := m.connectionReference()
		if refErr != nil {
			m.mu.Unlock()
			return nil, refErr
		}
		if connectionID == "" || connectionID != expected || !safeID(id) {
			m.mu.Unlock()
			return nil, ErrChatReference
		}
		client, err := m.client(m.manifest.Endpoint, m.manifest.Token)
		community := m.community
		m.mu.Unlock()
		if err != nil {
			return nil, err
		}
		// Catalog reads use the caller's deadline without holding the manager
		// mutex, so a slow invocation cannot block another room's preflight.
		instances, err := community.List(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, err
		}
		for _, item := range instances {
			if item.relayID() == id {
				return client, nil
			}
		}
		return nil, ErrChatReference
	}
	defer m.mu.Unlock()
	req, exists := m.manifest.Agents[id]
	if !exists || req.Visibility != "public" || connectionID == "" || connectionID != m.manifest.ConnectionID || !safeID(id) {
		return nil, ErrChatReference
	}
	return m.client(m.manifest.Endpoint, m.manifest.Token)
}

// Publication can hold mu across network calls. Invocation callers must be able
// to abandon that wait without leaving a goroutine queued to acquire the lock.
func (m *Manager) lockInvocation(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.mu.TryLock() {
		return nil
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			if m.mu.TryLock() {
				return nil
			}
		}
	}
}
func (m *Manager) Chat(ctx context.Context, id, connectionID, key string, input string, previous string, attachments ...File) (Response, error) {
	c, err := m.invocationClient(ctx, id, connectionID)
	if err != nil {
		return Response{}, err
	}
	var wireInput any = input
	if len(attachments) > 0 {
		parts := make([]map[string]string, 0, len(attachments)+1)
		if input != "" {
			parts = append(parts, map[string]string{"type": "input_text", "text": input})
		}
		for _, attachment := range attachments {
			parts = append(parts, map[string]string{"type": "input_file", "file_id": attachment.ID})
		}
		wireInput = []map[string]any{{"role": "user", "content": parts}}
	}
	raw, err := json.Marshal(wireInput)
	if err != nil {
		return Response{}, err
	}
	result, err := c.Call(ctx, key, CallRequest{Model: id, Input: raw, PreviousResponseID: previous})
	if err == nil && result.Model != id {
		// A successful submission can still be confirmed using its response ID.
		return Response{}, responseBodyUnavailable(result.ID)
	}
	return result, err
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
		var syntax *json.SyntaxError
		var fieldType *json.UnmarshalTypeError
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &syntax) || errors.As(err, &fieldType) {
			return Response{}, ErrInvalidChatResponse
		}
		return Response{}, err
	}
	if result.ID == "" || result.Model == "" || result.Status == "" {
		return Response{}, ErrInvalidChatResponse
	}
	if result.ID != responseID || result.Model != id {
		return Response{}, ErrChatReference
	}
	return result, nil
}
