package relay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// TemplateProfile links a cloud instance to its authorized Hub template.
type TemplateProfile struct {
	RepoPath string `json:"repo_path"`
}

func (m *Manager) AgentTemplate(ctx context.Context, id, connectionID string) (*TemplateProfile, error) {
	if m.credentials == nil || !relayAgentIDPattern.MatchString(id) {
		return nil, ErrChatReference
	}
	cred, err := m.boundCredentials()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	expected := m.connectionReferenceFor(cred)
	m.mu.Unlock()
	if connectionID != expected {
		return nil, ErrChatReference
	}
	client, err := NewClient(cred.Site, cred.Token, nil)
	if err != nil {
		return nil, err
	}
	instances, err := (HubCommunity{}).list(ctx, client)
	if err != nil {
		return nil, err
	}
	for _, item := range instances {
		if item.relayID() == id && item.Source == "platform" && item.TemplateID > 0 {
			return fetchTemplateProfile(ctx, client, item.TemplateID)
		}
	}
	return nil, nil
}

func fetchTemplateProfile(ctx context.Context, client *Client, id int64) (*TemplateProfile, error) {
	var result struct {
		Data struct {
			Metadata struct {
				RepoPath string `json:"repo_path"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := client.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v1/agent/templates/%d", id), "", nil, &result); err != nil {
		var remote *HTTPError
		if errors.As(err, &remote) && (remote.Status == http.StatusForbidden || remote.Status == http.StatusNotFound) {
			return nil, nil
		}
		return nil, err
	}
	repo := validatedTemplateRepoPath(result.Data.Metadata.RepoPath)
	if repo == "" {
		return nil, nil
	}
	return &TemplateProfile{RepoPath: repo}, nil
}

func validatedTemplateRepoPath(repo string) string {
	repo = strings.TrimSpace(repo)
	namespace, name, ok := strings.Cut(repo, "/")
	if !ok || namespace == "" || name == "" || strings.Contains(name, "/") {
		return ""
	}
	return repo
}
