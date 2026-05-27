package cliproxy

import (
	"context"
	"encoding/base64"
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

func TestAuthStatusRefreshesCodexHomeAuthWhenCLIProxyAuthExists(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, authDir)

	stale := map[string]any{
		"type":          "codex",
		"access_token":  "old-access",
		"refresh_token": "old-refresh",
		"account_id":    "acct_123",
		"disabled":      false,
	}
	fileName := authFileName(ProviderCodex, stale, "codex-imported")
	staleRaw, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, fileName), staleRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	accessToken := testJWT(t, `{"exp":1893456000}`)
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"last_refresh": "2029-12-31T23:00:00Z",
		"tokens": {
			"access_token": "`+accessToken+`",
			"refresh_token": "new-refresh",
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
		t.Fatalf("status = %+v, want refreshed codex-home auth", status)
	}

	raw, err := os.ReadFile(filepath.Join(authDir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["access_token"] != accessToken || metadata["refresh_token"] != "new-refresh" {
		t.Fatalf("metadata = %#v, want refreshed tokens from codex home", metadata)
	}
	if metadata["expired"] != "2030-01-01T00:00:00Z" {
		t.Fatalf("metadata expired = %#v, want JWT expiry", metadata["expired"])
	}
}

func TestAuthStatusKeepsEquivalentCodexHomeAuth(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, authDir)

	accessToken := testJWT(t, `{"exp":1893456000,"email":"dev@example.test"}`)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"last_refresh": "2029-12-31T23:00:00Z",
		"tokens": {
			"access_token": "`+accessToken+`",
			"refresh_token": "refresh",
			"account_id": "acct_123"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := (&Service{}).AuthStatus(context.Background(), ProviderCodex)
	if err != nil {
		t.Fatalf("first AuthStatus() error = %v", err)
	}
	if !first.Authenticated || first.Source != "codex-home" {
		t.Fatalf("first status = %+v, want imported codex-home", first)
	}

	second, err := (&Service{}).AuthStatus(context.Background(), ProviderCodex)
	if err != nil {
		t.Fatalf("second AuthStatus() error = %v", err)
	}
	if !second.Authenticated || second.Source != "cli-proxy" {
		t.Fatalf("second status = %+v, want existing cli-proxy auth", second)
	}
}

func TestAuthStatusDoesNotReimportDisabledCodexAuth(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, authDir)

	existing := map[string]any{
		"type":          "codex",
		"access_token":  "old-access",
		"refresh_token": "old-refresh",
		"email":         "dev@example.test",
		"disabled":      true,
	}
	fileName := authFileName(ProviderCodex, existing, "codex-imported")
	raw, err := json.Marshal(existing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, fileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"tokens": {
			"access_token": "new-access",
			"refresh_token": "new-refresh",
			"email": "dev@example.test"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := (&Service{}).AuthStatus(context.Background(), ProviderCodex)
	if err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	if status.Authenticated || !status.LoginRequired {
		t.Fatalf("status = %+v, want login required for disabled auth", status)
	}

	raw, err = os.ReadFile(filepath.Join(authDir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["access_token"] != "old-access" || metadata["refresh_token"] != "old-refresh" {
		t.Fatalf("metadata = %#v, want disabled auth preserved", metadata)
	}
	if disabled, _ := metadata["disabled"].(bool); !disabled {
		t.Fatalf("metadata disabled = %#v, want true", metadata["disabled"])
	}
}

func TestAuthStatusImportDoesNotRestartRunningService(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, authDir)

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"tokens": {
			"access_token": "`+testJWT(t, `{"exp":1893456000}`)+`",
			"refresh_token": "refresh",
			"account_id": "acct_123"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := &Service{
		started: true,
		baseURL: "http://127.0.0.1:1",
		cancel:  func() {},
		errCh:   make(chan error, 1),
	}
	svc.errCh <- nil

	status, err := svc.AuthStatus(context.Background(), ProviderCodex)
	if err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	if !status.Authenticated || status.Source != "codex-home" {
		t.Fatalf("status = %+v, want imported codex-home", status)
	}
	svc.mu.Lock()
	started := svc.started
	baseURL := svc.baseURL
	svc.mu.Unlock()
	if !started || baseURL == "" {
		t.Fatalf("AuthStatus restarted service: started=%v baseURL=%q", started, baseURL)
	}
}

func TestCodexMetadataFromAuthJSONExtractsAccessTokenExpiry(t *testing.T) {
	metadata, err := codexMetadataFromAuthJSON([]byte(`{
		"tokens": {
			"access_token": "` + testJWT(t, `{"exp":1893456000}`) + `",
			"refresh_token": "refresh"
		}
	}`))
	if err != nil {
		t.Fatalf("codexMetadataFromAuthJSON() error = %v", err)
	}
	if metadata["expired"] != "2030-01-01T00:00:00Z" {
		t.Fatalf("metadata expired = %#v, want JWT expiry", metadata["expired"])
	}
}

func TestAuthStatusKeepsFresherCLIProxyCodexAuth(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, authDir)

	existing := map[string]any{
		"type":          "codex",
		"access_token":  "fresh-access",
		"refresh_token": "fresh-refresh",
		"account_id":    "acct_123",
		"expired":       "2031-01-01T00:00:00Z",
		"disabled":      false,
	}
	fileName := authFileName(ProviderCodex, existing, "codex-imported")
	existingRaw, err := json.Marshal(existing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, fileName), existingRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	staleHomeAccess := testJWT(t, `{"exp":1893456000}`)
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"tokens": {
			"access_token": "`+staleHomeAccess+`",
			"refresh_token": "home-refresh",
			"account_id": "acct_123"
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	status, err := (&Service{}).AuthStatus(context.Background(), ProviderCodex)
	if err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	if !status.Authenticated || status.Source != "cli-proxy" {
		t.Fatalf("status = %+v, want existing cli-proxy auth", status)
	}

	raw, err := os.ReadFile(filepath.Join(authDir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["access_token"] != "fresh-access" {
		t.Fatalf("metadata = %#v, want existing auth preserved", metadata)
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

func TestImportExistingAuthRefreshesClaudeKeychainWhenExisting(t *testing.T) {
	home := t.TempDir()
	authDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(authDirEnv, authDir)

	existing := map[string]any{
		"type":          authProviderClaude,
		"email":         "dev@example.test",
		"access_token":  "old-access",
		"refresh_token": "old-refresh",
		"disabled":      false,
	}
	fileName := authFileName(authProviderClaude, existing, "claude-keychain")
	raw, err := json.Marshal(existing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, fileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	restore := stubKeychain(t, func(context.Context, string, string) ([]byte, error) {
		return []byte(`{
			"oauthAccount": {"emailAddress": "dev@example.test"},
			"claudeAiOauth": {
				"accessToken": "new-access",
				"refreshToken": "new-refresh",
				"expiresAt": 1893456000000
			}
		}`), nil
	})
	defer restore()

	importExistingAuth(context.Background(), authDir)

	raw, err = os.ReadFile(filepath.Join(authDir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["access_token"] != "new-access" || metadata["refresh_token"] != "new-refresh" {
		t.Fatalf("metadata = %#v, want refreshed keychain auth", metadata)
	}
}

func testJWT(t *testing.T, claims string) string {
	t.Helper()
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".sig"
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
