package connectors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorePersistsGitHubUnderRootAuthAndPreservesOtherSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	writeJSON(t, path, map[string]any{
		"version":         1,
		"model_providers": map[string]any{"items": map[string]any{"openai": map[string]any{}}},
		"auth": map[string]any{
			"opencsg": map[string]any{
				"tokens": map[string]any{"access_token": "opencsg-token"},
			},
		},
	})

	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	store := NewStore(path)
	err := store.SaveGitHub(State{
		Config: Config{
			ClientID:     " client-id ",
			ClientSecret: " client-secret ",
			Scopes:       []string{" repo ", "read:user", "repo"},
		},
		Pending: &PendingAuth{
			State:        "pending-state",
			CodeVerifier: "verifier",
			CallbackURL:  "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback",
			ReturnURL:    "http://127.0.0.1:18080/#/workspace",
			CreatedAt:    now,
		},
		Token: &Token{
			AccessToken: " gh-token ",
			TokenType:   " bearer ",
			Scopes:      []string{"repo", "read:user"},
		},
		Account: &Account{
			Login:     "octocat",
			ID:        1,
			AvatarURL: "https://avatars.example/octocat.png",
			HTMLURL:   "https://github.com/octocat",
		},
		ConnectedAt: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("SaveGitHub() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("state mode = %v, want %v", got, want)
	}

	var root map[string]any
	readJSON(t, path, &root)
	if _, ok := root["model_providers"]; !ok {
		t.Fatalf("model_providers was not preserved: %#v", root)
	}
	authState := root["auth"].(map[string]any)
	if _, ok := authState["opencsg"]; !ok {
		t.Fatalf("auth.opencsg was not preserved: %#v", authState)
	}
	github := authState["github"].(map[string]any)
	config := github["config"].(map[string]any)
	if config["client_id"] != "client-id" || config["client_secret"] != "client-secret" {
		t.Fatalf("github config = %#v", config)
	}

	got, ok, err := store.LoadGitHub()
	if err != nil {
		t.Fatalf("LoadGitHub() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadGitHub() ok = false, want true")
	}
	if got.Config.ClientID != "client-id" || got.Config.ClientSecret != "client-secret" {
		t.Fatalf("loaded config = %+v", got.Config)
	}
	if len(got.Config.Scopes) != 2 || got.Config.Scopes[0] != "repo" || got.Config.Scopes[1] != "read:user" {
		t.Fatalf("loaded scopes = %#v", got.Config.Scopes)
	}
	if got.Token == nil || got.Token.AccessToken != "gh-token" || got.Token.TokenType != "bearer" {
		t.Fatalf("loaded token = %+v", got.Token)
	}

	status := got.Status("callback")
	rawStatus, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if strings.Contains(string(rawStatus), "client-secret") || strings.Contains(string(rawStatus), "gh-token") {
		t.Fatalf("status leaks secret material: %s", rawStatus)
	}
	if !status.Configured || !status.Connected || !status.ClientSecretSet || status.Account == nil || status.Account.Login != "octocat" {
		t.Fatalf("status = %+v, want configured connected octocat", status)
	}
}

func TestStoreDeleteGitHubPreservesOtherAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(path)
	if err := store.SaveGitHub(State{Config: Config{ClientID: "id", ClientSecret: "secret"}}); err != nil {
		t.Fatalf("SaveGitHub() error = %v", err)
	}

	var root map[string]any
	readJSON(t, path, &root)
	authState := root["auth"].(map[string]any)
	authState["opencsg"] = map[string]any{"tokens": map[string]any{"access_token": "opencsg-token"}}
	writeJSON(t, path, root)

	if err := store.DeleteGitHub(); err != nil {
		t.Fatalf("DeleteGitHub() error = %v", err)
	}

	readJSON(t, path, &root)
	authState = root["auth"].(map[string]any)
	if _, ok := authState["github"]; ok {
		t.Fatalf("auth.github still exists: %#v", authState)
	}
	if _, ok := authState["opencsg"]; !ok {
		t.Fatalf("auth.opencsg was not preserved: %#v", authState)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
}
