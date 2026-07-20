package agentmanager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/connectors"
)

func TestDefaultConnectorGrantPolicyGrantsGitHubOnlyToManager(t *testing.T) {
	policy := DefaultConnectorGrantPolicy{}
	manager := AgentConnectorRef{AgentID: agent.ManagerUserID, AgentRole: agent.RoleManager}
	worker := AgentConnectorRef{AgentID: "agent-worker", AgentRole: agent.RoleWorker}

	if !policy.AllowsConnectorCredential(context.Background(), manager, connectors.ProviderGitHub) {
		t.Fatal("manager github credential access denied, want allowed")
	}
	if !policy.AllowsConnectorCredential(context.Background(), manager, connectors.ProviderGitLab) {
		t.Fatal("manager gitlab credential access denied, want allowed")
	}
	if policy.AllowsConnectorCredential(context.Background(), worker, connectors.ProviderGitHub) {
		t.Fatal("worker github credential access allowed, want denied in v1")
	}
	if policy.AllowsConnectorCredential(context.Background(), manager, "slack") {
		t.Fatal("manager slack credential access allowed, want denied in v1")
	}
}

func TestConnectorServiceProviderReturnsManagerGitHubLeaseOnly(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			if got := r.Header.Get("Authorization"); got != "Bearer gho_manager_secret" {
				t.Fatalf("user Authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"login":"octocat","id":42}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(github.Close)

	store := connectors.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.SaveGitHub(connectors.State{
		Token: &connectors.Token{
			AccessToken: "gho_manager_secret",
			TokenType:   "bearer",
			Scopes:      []string{"repo"},
		},
		Account: &connectors.Account{
			Login: "octocat",
			ID:    42,
		},
		ConnectedAt: time.Date(2026, 7, 7, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveGitHub() error = %v", err)
	}

	service := connectors.NewService(store)
	service.HTTPClient = github.Client()
	service.Endpoints = connectors.Endpoints{APIBaseURL: github.URL}
	provider := NewConnectorServiceCredentialProvider(service, DefaultConnectorGrantPolicy{})
	manager := AgentConnectorRef{AgentID: agent.ManagerUserID, AgentRole: agent.RoleManager}
	worker := AgentConnectorRef{AgentID: "agent-worker", AgentRole: agent.RoleWorker}

	status, err := provider.ConnectorStatus(context.Background(), manager, connectors.ProviderGitHub, "http://127.0.0.1/callback")
	if err != nil {
		t.Fatalf("ConnectorStatus() error = %v", err)
	}
	if !status.Connected || status.Account == nil || status.Account.Login != "octocat" {
		t.Fatalf("ConnectorStatus() = %+v, want connected octocat account", status)
	}

	lease, err := provider.ManagedCredentialLease(context.Background(), manager, connectors.ProviderGitHub)
	if err != nil {
		t.Fatalf("ManagedCredentialLease() error = %v", err)
	}
	if lease.Provider != connectors.ProviderGitHub || lease.AccessToken != "gho_manager_secret" || lease.TokenType != "bearer" {
		t.Fatalf("lease = %+v, want github bearer access token", lease)
	}
	if lease.Account == nil || lease.Account.Login != "octocat" {
		t.Fatalf("lease account = %+v, want octocat", lease.Account)
	}

	if _, err := provider.ManagedCredentialLease(context.Background(), worker, connectors.ProviderGitHub); err == nil {
		t.Fatal("worker ManagedCredentialLease() error = nil, want access denied")
	}
}

func TestManagedCredentialLeaseRedactsSecrets(t *testing.T) {
	lease := ManagedCredentialLease{
		Provider:    connectors.ProviderGitHub,
		AccessToken: "gho_manager_secret",
	}

	got := lease.Redact("token=gho_manager_secret\nAuthorization: Bearer gho_manager_secret")
	if strings.Contains(got, "gho_manager_secret") {
		t.Fatalf("Redact() leaked token: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("Redact() = %q, want redaction marker", got)
	}
}
