package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/auth"
	hub "csgclaw/internal/template"
)

func TestCreateCommunityAgentInstanceUsesCSGClawType(t *testing.T) {
	var got communityAgentInstanceRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != communityAgentInstancePath {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if gotUser, want := r.URL.Query().Get("current_user"), "alice"; gotUser != want {
			t.Fatalf("current_user = %q, want %q", gotUser, want)
		}
		if gotUUID, want := r.URL.Query().Get("current_user_uuid"), "uuid-alice"; gotUUID != want {
			t.Fatalf("current_user_uuid = %q, want %q", gotUUID, want)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer hub-token" {
			t.Fatalf("Authorization = %q", authorization)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":"instance-1"}}`))
	}))
	defer server.Close()

	previousCredentials := loadCommunityInstanceCredentials
	loadCommunityInstanceCredentials = func() (string, string, bool, error) {
		return server.URL, "hub-token", true, nil
	}
	t.Cleanup(func() { loadCommunityInstanceCredentials = previousCredentials })

	err := createCommunityAgentInstanceRequest(context.Background(), hub.Template{
		Namespace:   "alice",
		Name:        "ReviewBot",
		Description: "Reviews changes",
	}, auth.Status{UserID: "alice", UserUUID: "uuid-alice", Name: "Alice Zhang"})
	if err != nil {
		t.Fatalf("createCommunityAgentInstanceRequest() error = %v", err)
	}
	if got.Type != "csgclaw" {
		t.Fatalf("type = %q, want csgclaw", got.Type)
	}
	if got.ContentID != "alice/ReviewBot" || got.Name != "ReviewBot" || got.Public {
		t.Fatalf("payload = %+v", got)
	}
	if latest, ok := got.Metadata["latest_version"].(bool); !ok || !latest {
		t.Fatalf("metadata = %+v", got.Metadata)
	}
	provisionRequest, ok := got.Metadata["provision_request"].(map[string]any)
	if !ok || provisionRequest["repo_path"] != "alice/ReviewBot" {
		t.Fatalf("provision_request = %+v", provisionRequest)
	}
	if _, hasEnv := provisionRequest["env"]; hasEnv {
		t.Fatalf("provision_request.env must not contain platform-managed TEMPLATE_ID: %+v", provisionRequest)
	}
}

func TestCreateCommunityAgentInstanceRetriesTemplatePendingCode(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts <= communityInstanceDeployRetries {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error.AGENT-ERR-22":{"other":"csgclaw template not found for repository path alice/ReviewBot"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":"instance-1"}}`))
	}))
	defer server.Close()

	previousCredentials := loadCommunityInstanceCredentials
	previousWait := waitForCommunityInstanceRetry
	loadCommunityInstanceCredentials = func() (string, string, bool, error) {
		return server.URL, "hub-token", true, nil
	}
	waitForCommunityInstanceRetry = func(context.Context) error { return nil }
	t.Cleanup(func() {
		loadCommunityInstanceCredentials = previousCredentials
		waitForCommunityInstanceRetry = previousWait
	})

	err := createCommunityAgentInstanceRequest(context.Background(), hub.Template{
		Namespace: "alice",
		Name:      "ReviewBot",
	}, auth.Status{UserID: "alice", UserUUID: "uuid-alice"})
	if err != nil {
		t.Fatalf("createCommunityAgentInstanceRequest() error = %v", err)
	}
	if got, want := attempts, communityInstanceDeployRetries+1; got != want {
		t.Fatalf("attempts = %d, want %d", got, want)
	}
}

func TestCreateCommunityAgentInstanceReturnsLastTemplatePendingError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error.AGENT-ERR-22":{"other":"csgclaw template not found for repository path alice/ReviewBot"}}`))
	}))
	defer server.Close()

	previousCredentials := loadCommunityInstanceCredentials
	previousWait := waitForCommunityInstanceRetry
	loadCommunityInstanceCredentials = func() (string, string, bool, error) {
		return server.URL, "hub-token", true, nil
	}
	waitForCommunityInstanceRetry = func(context.Context) error { return nil }
	t.Cleanup(func() {
		loadCommunityInstanceCredentials = previousCredentials
		waitForCommunityInstanceRetry = previousWait
	})

	err := createCommunityAgentInstanceRequest(context.Background(), hub.Template{
		Namespace: "alice",
		Name:      "ReviewBot",
	}, auth.Status{UserID: "alice", UserUUID: "uuid-alice"})
	if err == nil || !strings.Contains(err.Error(), "csgclaw template not found for repository path alice/ReviewBot") {
		t.Fatalf("createCommunityAgentInstanceRequest() error = %v, want final deployment reason", err)
	}
	if got, want := attempts, communityInstanceDeployRetries+1; got != want {
		t.Fatalf("attempts = %d, want %d", got, want)
	}
}

func TestCreateCommunityAgentInstanceDoesNotRetryOtherCodes(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error.AGENT-ERR-21":{"other":"invalid request"}}`))
	}))
	defer server.Close()

	previousCredentials := loadCommunityInstanceCredentials
	loadCommunityInstanceCredentials = func() (string, string, bool, error) {
		return server.URL, "hub-token", true, nil
	}
	t.Cleanup(func() { loadCommunityInstanceCredentials = previousCredentials })

	err := createCommunityAgentInstanceRequest(context.Background(), hub.Template{
		Namespace: "alice",
		Name:      "ReviewBot",
	}, auth.Status{UserID: "alice", UserUUID: "uuid-alice"})
	if err == nil || !strings.Contains(err.Error(), "invalid request") {
		t.Fatalf("createCommunityAgentInstanceRequest() error = %v, want invalid request", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
