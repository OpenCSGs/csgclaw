package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"csgclaw/internal/auth"
)

func TestConcurrentConnectorAndAccountSavesPreserveAllCredentials(t *testing.T) {
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" || r.Header.Get("PRIVATE-TOKEN") != "test-gitlab-token" {
			http.Error(w, "unexpected GitLab request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"test-user","id":1}`))
	}))
	defer gitlab.Close()

	for round := range 32 {
		path := filepath.Join(t.TempDir(), "state.json")
		connectorStore := NewStore(path)
		service := NewService(connectorStore)
		service.HTTPClient = gitlab.Client()
		authStore := auth.NewStore(path)
		runConcurrentStoreChanges(t,
			func() error {
				_, err := service.SaveConfig(context.Background(), ProviderGitHub, Config{ClientID: "test-github-id", ClientSecret: "test-github-secret"})
				return err
			},
			func() error {
				_, err := service.SaveGitLabConfig(context.Background(), Config{BaseURL: gitlab.URL, AccessToken: "test-gitlab-token"})
				return err
			},
			func() error {
				return authStore.Save(auth.Record{Tokens: auth.Tokens{AccessToken: "test-account-token"}, Account: auth.Account{BaseURL: "https://account.example"}})
			},
			func() error {
				return authStore.SaveCSGHubProviderCredentials(auth.CSGHubProviderCredentials{AIGatewayBaseURL: "https://gateway.example/v1", AIGatewayBuiltinAPIKey: "test-gateway-key"})
			},
		)

		github, found, err := connectorStore.LoadGitHub()
		if err != nil || !found || github.Config.ClientID != "test-github-id" {
			t.Fatalf("round %d: GitHub config lost after successful saves, found=%v, err=%v", round, found, err)
		}
		gitlabState, found, err := connectorStore.LoadGitLab()
		if err != nil || !found || gitlabState.Config.AccessToken != "test-gitlab-token" || gitlabState.Account == nil || gitlabState.Account.Login != "test-user" {
			t.Fatalf("round %d: GitLab credentials lost after successful saves, found=%v, err=%v", round, found, err)
		}
		account, found, err := authStore.Load()
		if err != nil || !found || account.Tokens.AccessToken != "test-account-token" {
			t.Fatalf("round %d: account credentials lost after successful saves, found=%v, err=%v", round, found, err)
		}
		gateway, found, err := authStore.LoadCSGHubProviderCredentials()
		if err != nil || !found || gateway.AIGatewayBuiltinAPIKey != "test-gateway-key" {
			t.Fatalf("round %d: gateway credentials lost after successful saves, found=%v, err=%v", round, found, err)
		}
	}
}
