package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
const communityInstanceTemplatePendingCode = "AGENT-ERR-22"
const communityInstanceSensitiveCheckCode = "AGENT-ERR-23"
const communityInstanceResourceUnavailableCode = "RESOURCE-ERR-1"
const communityInstanceDeployRetries = 3
const communityInstanceDeployRetryDelay = 3 * time.Second

var errCommunityInstanceSensitiveCheck = errors.New("community template has not passed the sensitive-content check")
var errCommunityInstanceTemplatePending = errors.New("community template is not ready for deployment")
var errCommunityInstanceResourceUnavailable = errors.New("community deployment resource is temporarily unavailable")

type communityAgentInstanceRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        string         `json:"type"`
	Public      bool           `json:"public"`
	ContentID   string         `json:"content_id"`
	Metadata    map[string]any `json:"metadata"`
}

type communityAgentInstanceResponse struct {
	Code    string `json:"code"`
	Msg     string `json:"msg"`
	Context struct {
		RepoPath             string `json:"repo_path"`
		SensitiveCheckStatus string `json:"sensitive_check_status"`
	} `json:"context"`
	Data json.RawMessage `json:"data"`
}

var createCommunityAgentInstance = createCommunityAgentInstanceRequest
var communityInstanceHTTPClient = &http.Client{Timeout: 30 * time.Second}
var waitForCommunityInstanceRetry = func(ctx context.Context) error {
	timer := time.NewTimer(communityInstanceDeployRetryDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
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

	for retry := 0; ; retry++ {
		retryable, err := createCommunityAgentInstanceOnce(req)
		if err == nil {
			return nil
		}
		if !retryable || retry >= communityInstanceDeployRetries {
			return err
		}
		if err := waitForCommunityInstanceRetry(ctx); err != nil {
			return fmt.Errorf("wait to retry community instance deployment: %w", err)
		}
	}
}

func createCommunityAgentInstanceOnce(req *http.Request) (bool, error) {
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return false, fmt.Errorf("reset community instance request body: %w", err)
		}
		req.Body = body
	}
	resp, err := communityInstanceHTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("create community instance: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, fmt.Errorf("read community instance response: %w", err)
	}
	var result communityAgentInstanceResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		if resp.StatusCode != http.StatusOK {
			return false, fmt.Errorf("create community instance: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
		}
		return false, fmt.Errorf("decode community instance response: %w", err)
	}
	if result.Code == communityInstanceTemplatePendingCode {
		message := strings.TrimSpace(result.Msg)
		if message == "" {
			message = communityInstanceTemplatePendingCode
		}
		return true, fmt.Errorf("%w: %s", errCommunityInstanceTemplatePending, message)
	}
	if result.Code == communityInstanceSensitiveCheckCode {
		message := strings.TrimSpace(result.Msg)
		if message == "" {
			message = communityInstanceSensitiveCheckCode
		}
		if strings.EqualFold(strings.TrimSpace(result.Context.SensitiveCheckStatus), "pending") {
			return true, fmt.Errorf("%w: %s", errCommunityInstanceTemplatePending, message)
		}
		return false, fmt.Errorf("%w: %s", errCommunityInstanceSensitiveCheck, message)
	}
	if result.Code == communityInstanceResourceUnavailableCode {
		message := strings.TrimSpace(result.Msg)
		if message == "" {
			message = communityInstanceResourceUnavailableCode
		}
		return false, fmt.Errorf("%w: %s", errCommunityInstanceResourceUnavailable, message)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("create community instance: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if len(result.Data) == 0 || string(result.Data) == "null" {
		return false, fmt.Errorf("create community instance: response data is missing")
	}
	return false, nil
}

func communityInstanceUpstreamMessage(err error) string {
	message := err.Error()
	for _, sentinel := range []error{
		errCommunityInstanceTemplatePending,
		errCommunityInstanceSensitiveCheck,
		errCommunityInstanceResourceUnavailable,
	} {
		if errors.Is(err, sentinel) {
			return strings.TrimPrefix(message, sentinel.Error()+": ")
		}
	}
	return message
}
