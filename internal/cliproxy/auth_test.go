package cliproxy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthStatusImportsCodexHomeAuth(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, authDir)

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"last_refresh": "2026-04-29T00:00:00Z",
		"tokens": {
			"access_token": "access",
			"refresh_token": "refresh",
			"id_token": "id",
			"account_id": "acct_123"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := (&Service{}).AuthStatus(context.Background(), ProviderCodex)
	if err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	if !status.Authenticated || status.Source != "codex-home" {
		t.Fatalf("status = %+v, want authenticated codex-home", status)
	}

	files, err := filepath.Glob(filepath.Join(authDir, "codex-*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("imported files = %v, %v; want one codex file", files, err)
	}
	var metadata map[string]any
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["type"] != "codex" || metadata["access_token"] != "access" || metadata["refresh_token"] != "refresh" {
		t.Fatalf("metadata = %#v, want codex tokens", metadata)
	}
}

func TestAuthStatusImportsClaudeKeychainToken(t *testing.T) {
	authDir := t.TempDir()
	t.Setenv(authDirEnv, authDir)
	restore := stubKeychain(t, func(context.Context, string, string) ([]byte, error) {
		return []byte(`{
			"oauthAccount": {"emailAddress": "dev@example.test"},
			"claudeAiOauth": {
				"accessToken": "access",
				"refreshToken": "refresh",
				"expiresAt": 1777392000000
			}
		}`), nil
	})
	defer restore()

	status, err := (&Service{}).AuthStatus(context.Background(), "claude_code")
	if err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	if !status.Authenticated || status.Source != "macos-keychain" {
		t.Fatalf("status = %+v, want authenticated macos-keychain", status)
	}

	files, err := filepath.Glob(filepath.Join(authDir, "claude-*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("imported files = %v, %v; want one claude file", files, err)
	}
	var metadata map[string]any
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["type"] != "claude" || metadata["email"] != "dev@example.test" || metadata["access_token"] != "access" || metadata["refresh_token"] != "refresh" {
		t.Fatalf("metadata = %#v, want claude keychain tokens", metadata)
	}
	if metadata["expired"] == "" {
		t.Fatalf("metadata missing normalized expiry: %#v", metadata)
	}
}

func TestAuthStatusClaudeKeychainMissingDoesNotFail(t *testing.T) {
	authDir := t.TempDir()
	t.Setenv(authDirEnv, authDir)
	restore := stubKeychain(t, func(context.Context, string, string) ([]byte, error) {
		return nil, errors.New("not found")
	})
	defer restore()

	status, err := (&Service{}).AuthStatus(context.Background(), "claude_code")
	if err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	if status.Authenticated || !status.LoginRequired {
		t.Fatalf("status = %+v, want login required", status)
	}
	files, err := filepath.Glob(filepath.Join(authDir, "*.json"))
	if err != nil || len(files) != 0 {
		t.Fatalf("files = %v, %v; want no auth files", files, err)
	}
}

func TestAuthStatusClaudeMalformedKeychainDoesNotWrite(t *testing.T) {
	authDir := t.TempDir()
	t.Setenv(authDirEnv, authDir)
	restore := stubKeychain(t, func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"claudeAiOauth":{"accessToken":"access-only"}}`), nil
	})
	defer restore()

	status, err := (&Service{}).AuthStatus(context.Background(), "claude_code")
	if err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	if status.Authenticated || !status.LoginRequired {
		t.Fatalf("status = %+v, want login required", status)
	}
	files, err := filepath.Glob(filepath.Join(authDir, "*.json"))
	if err != nil || len(files) != 0 {
		t.Fatalf("files = %v, %v; want no auth files", files, err)
	}
}

func stubKeychain(t *testing.T, reader func(context.Context, string, string) ([]byte, error)) func() {
	t.Helper()
	previousGOOS := currentGOOS
	previousReader := claudeKeychainReader
	currentGOOS = "darwin"
	claudeKeychainReader = reader
	return func() {
		currentGOOS = previousGOOS
		claudeKeychainReader = previousReader
	}
}
