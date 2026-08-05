package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"csgclaw/internal/auth"
	hub "csgclaw/internal/template"
)

const communityAgentInstancePath = "/api/v1/agent/instances"

type communityAgentInstanceRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        string         `json:"type"`
	Public      bool           `json:"public"`
	ContentID   string         `json:"content_id"`
	Metadata    map[string]any `json:"metadata"`
}

type communityAgentInstanceResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

var createCommunityAgentInstance = createCommunityAgentInstanceRequest
var communityInstanceHTTPClient = &http.Client{Timeout: 30 * time.Second}
var loadCommunityInstanceCredentials = func() (string, string, bool, error) {
	store, err := auth.DefaultStore()
	if err != nil {
		return "", "", false, err
	}
	return store.Credentials()
}

func createCommunityAgentInstanceRequest(ctx context.Context, item hub.Template, status auth.Status) error {
	baseURL, token, ok, err := loadCommunityInstanceCredentials()
	if err != nil {
		return fmt.Errorf("load OpenCSG credentials: %w", err)
	}
	if !ok {
		return fmt.Errorf("OpenCSG sign-in is required to deploy templates")
	}
	currentUser := strings.TrimSpace(status.UserID)
	if currentUser == "" || strings.TrimSpace(status.UserUUID) == "" {
		return fmt.Errorf("OpenCSG account information is incomplete")
	}

	namespace := currentUser
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return fmt.Errorf("published template name is missing")
	}
	if publishedNamespace := strings.TrimSpace(item.Namespace); publishedNamespace != "" && publishedNamespace != namespace {
		return fmt.Errorf("published template namespace %q does not match current user %q", publishedNamespace, namespace)
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + communityAgentInstancePath)
	if err != nil {
		return fmt.Errorf("build community instance URL: %w", err)
	}
	query := u.Query()
	query.Set("current_user", currentUser)
	query.Set("current_user_uuid", status.UserUUID)
	u.RawQuery = query.Encode()

	payload := communityAgentInstanceRequest{
		Name:        name,
		Description: strings.TrimSpace(item.Description),
		Type:        "csgclaw",
		Public:      false,
		ContentID:   namespace + "/" + name,
		Metadata: map[string]any{
			"latest_version": true,
			"provision_request": map[string]any{
				"repo_path": namespace + "/" + name,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode community instance request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create community instance request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := communityInstanceHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("create community instance: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read community instance response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create community instance: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result communityAgentInstanceResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("decode community instance response: %w", err)
	}
	if result.Code != 0 {
		message := strings.TrimSpace(result.Msg)
		if message == "" {
			message = "unknown error"
		}
		return fmt.Errorf("create community instance: %s", message)
	}
	if len(result.Data) == 0 || string(result.Data) == "null" {
		return fmt.Errorf("create community instance: response data is missing")
	}
	return nil
}
