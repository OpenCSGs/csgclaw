package cliproxy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	sdkauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/auth"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

const (
	autoLoginEnv       = "CSGCLAW_CLIPROXY_AUTO_LOGIN"
	noBrowserEnv       = "CSGCLAW_CLIPROXY_NO_BROWSER"
	disableKeychainEnv = "CSGCLAW_CLIPROXY_DISABLE_KEYCHAIN"

	authProviderClaude = "claude"
)

type AuthStatus struct {
	Provider         string `json:"provider"`
	Authenticated    bool   `json:"authenticated"`
	LoginRequired    bool   `json:"login_required"`
	Source           string `json:"source,omitempty"`
	Message          string `json:"message,omitempty"`
	SupportsLogin    bool   `json:"supports_login"`
	SupportsKeychain bool   `json:"supports_keychain,omitempty"`
}

type LoginOptions struct {
	NoBrowser bool
	Prompt    func(prompt string) (string, error)
}

type authImportResult struct {
	imported bool
	source   string
	message  string
}

type keychainCandidate struct {
	service string
	account string
}

var (
	errAuthNotFound      = errors.New("cliproxy auth not found")
	errAuthNotImportable = errors.New("cliproxy auth not importable")

	currentGOOS              = runtime.GOOS
	claudeKeychainReader     = securityFindGenericPassword
	claudeKeychainCandidates = []keychainCandidate{
		{service: "com.anthropic.claude-code"},
		{service: "Claude Code"},
		{service: "Claude"},
		{service: "Anthropic"},
	}
)

func (s *Service) AuthStatus(ctx context.Context, provider string) (AuthStatus, error) {
	return s.authStatus(ctx, provider, true)
}

func (s *Service) Login(ctx context.Context, provider string, opts LoginOptions) (AuthStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	authProvider, statusProvider, err := normalizeAuthProvider(provider)
	if err != nil {
		return AuthStatus{}, err
	}
	cfg, err := authConfig()
	if err != nil {
		return AuthStatus{}, err
	}

	if existing, err := findAuth(ctx, cfg.AuthDir, authProvider); err == nil && existing != nil {
		return authenticatedStatus(statusProvider, "cli-proxy", authProvider), nil
	}

	if authProvider == authProviderClaude && keychainImportEnabled() {
		if imported, _ := importExternalAuth(ctx, cfg.AuthDir, authProvider); imported.imported {
			_ = s.Shutdown(context.Background())
			return authenticatedStatus(statusProvider, imported.source, authProvider), nil
		}
	}

	noBrowser := opts.NoBrowser || envBool(noBrowserEnv)
	manager := sdkauth.NewManager(
		sdkauth.NewFileTokenStore(),
		sdkauth.NewCodexAuthenticator(),
		sdkauth.NewClaudeAuthenticator(),
	)
	if _, _, err = manager.Login(ctx, authProvider, cfg, &sdkauth.LoginOptions{
		NoBrowser: noBrowser,
		Prompt:    opts.Prompt,
	}); err != nil {
		return AuthStatus{}, err
	}
	_ = s.Shutdown(context.Background())
	return authenticatedStatus(statusProvider, "oauth", authProvider), nil
}

func (s *Service) HasAuth(ctx context.Context, provider string) (bool, error) {
	status, err := s.AuthStatus(ctx, provider)
	if err != nil {
		return false, err
	}
	return status.Authenticated, nil
}

func (s *Service) authStatus(ctx context.Context, provider string, allowImport bool) (AuthStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	authProvider, statusProvider, err := normalizeAuthProvider(provider)
	if err != nil {
		return AuthStatus{}, err
	}
	cfg, err := authConfig()
	if err != nil {
		return AuthStatus{}, err
	}

	if existing, err := findAuth(ctx, cfg.AuthDir, authProvider); err == nil && existing != nil {
		return authenticatedStatus(statusProvider, "cli-proxy", authProvider), nil
	}

	var importMessage string
	if allowImport && autoAuthImportEnabled() {
		imported, _ := importExternalAuth(ctx, cfg.AuthDir, authProvider)
		importMessage = imported.message
		if imported.imported {
			_ = s.Shutdown(context.Background())
			return authenticatedStatus(statusProvider, imported.source, authProvider), nil
		}
		if existing, err := findAuth(ctx, cfg.AuthDir, authProvider); err == nil && existing != nil {
			return authenticatedStatus(statusProvider, "cli-proxy", authProvider), nil
		}
	}

	status := unauthenticatedStatus(statusProvider, authProvider)
	if importMessage != "" {
		status.Message = importMessage
	}
	return status, nil
}

func importExistingAuth(ctx context.Context, authDir string) {
	if !autoAuthImportEnabled() {
		return
	}
	for _, provider := range []string{ProviderCodex, authProviderClaude} {
		if existing, err := findAuth(ctx, authDir, provider); err == nil && existing != nil {
			continue
		}
		_, _ = importExternalAuth(ctx, authDir, provider)
	}
}

func importExternalAuth(ctx context.Context, authDir, provider string) (authImportResult, error) {
	switch provider {
	case ProviderCodex:
		return importCodexHomeAuth(ctx, authDir)
	case authProviderClaude:
		return importClaudeKeychainAuth(ctx, authDir)
	default:
		return authImportResult{}, fmt.Errorf("unsupported cliproxy auth provider %q", provider)
	}
}

func importCodexHomeAuth(ctx context.Context, authDir string) (authImportResult, error) {
	path, err := codexHomeAuthPath()
	if err != nil {
		return authImportResult{message: "Codex auth was not found. Run csgclaw model auth login codex."}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return authImportResult{message: "Codex auth was not found. Run csgclaw model auth login codex."}, nil
		}
		return authImportResult{message: "Codex auth could not be read. Run csgclaw model auth login codex."}, nil
	}
	metadata, err := codexMetadataFromAuthJSON(raw)
	if err != nil {
		return authImportResult{message: "Codex auth is not importable. Run csgclaw model auth login codex."}, nil
	}
	fileName := authFileName(ProviderCodex, metadata, "codex-imported")
	if _, err = saveMetadataAuth(ctx, authDir, ProviderCodex, fileName, metadata); err != nil {
		return authImportResult{}, err
	}
	return authImportResult{imported: true, source: "codex-home"}, nil
}

func importClaudeKeychainAuth(ctx context.Context, authDir string) (authImportResult, error) {
	if currentGOOS != "darwin" {
		return authImportResult{message: "Claude Code auth is required. Run csgclaw model auth login claude-code."}, nil
	}
	if !keychainImportEnabled() {
		return authImportResult{message: "Claude Keychain probing is disabled. Run csgclaw model auth login claude-code."}, nil
	}
	for _, candidate := range claudeKeychainCandidates {
		raw, err := claudeKeychainReader(ctx, candidate.service, candidate.account)
		if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}
		metadata, err := claudeMetadataFromKeychain(raw)
		if err != nil {
			continue
		}
		fileName := authFileName(authProviderClaude, metadata, "claude-keychain-"+shortHash(candidate.service+":"+candidate.account))
		if _, err = saveMetadataAuth(ctx, authDir, authProviderClaude, fileName, metadata); err != nil {
			return authImportResult{}, err
		}
		return authImportResult{imported: true, source: "macos-keychain"}, nil
	}
	return authImportResult{message: "Claude Code auth was not found in macOS Keychain. Run csgclaw model auth login claude-code."}, nil
}

func securityFindGenericPassword(ctx context.Context, service, account string) ([]byte, error) {
	args := []string{"find-generic-password", "-s", service, "-w"}
	if strings.TrimSpace(account) != "" {
		args = []string{"find-generic-password", "-s", service, "-a", account, "-w"}
	}
	cmd := exec.CommandContext(ctx, "security", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, errAuthNotFound
	}
	return out, nil
}

func codexHomeAuthPath() (string, error) {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Join(home, "auth.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

func codexMetadataFromAuthJSON(raw []byte) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	tokenMap := root
	if nested, ok := mapValue(root, "tokens"); ok {
		tokenMap = nested
	}
	accessToken := stringFromMap(tokenMap, "access_token", "accessToken")
	refreshToken := stringFromMap(tokenMap, "refresh_token", "refreshToken")
	if accessToken == "" || refreshToken == "" {
		return nil, errAuthNotImportable
	}
	metadata := map[string]any{
		"type":          ProviderCodex,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"disabled":      false,
	}
	copyString(metadata, tokenMap, "id_token", "idToken")
	copyString(metadata, tokenMap, "account_id", "accountID", "accountId")
	copyString(metadata, root, "last_refresh", "lastRefresh")
	copyString(metadata, tokenMap, "last_refresh", "lastRefresh")
	if email := findFirstString(root, "email", "emailAddress"); email != "" {
		metadata["email"] = email
	} else if email = emailFromJWT(stringFromMap(tokenMap, "id_token", "idToken")); email != "" {
		metadata["email"] = email
	}
	if expired := expiryFromValue(firstValue(tokenMap, "expired", "expires_at", "expiresAt", "expiry", "expiration")); expired != "" {
		metadata["expired"] = expired
	}
	return metadata, nil
}

func claudeMetadataFromKeychain(raw []byte) (map[string]any, error) {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	for _, candidate := range collectMaps(root) {
		accessToken := stringFromMap(candidate, "access_token", "accessToken")
		refreshToken := stringFromMap(candidate, "refresh_token", "refreshToken")
		if accessToken == "" || refreshToken == "" {
			continue
		}
		metadata := map[string]any{
			"type":          authProviderClaude,
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"disabled":      false,
		}
		copyString(metadata, candidate, "id_token", "idToken")
		if email := findFirstString(root, "email", "emailAddress", "accountEmail"); email != "" {
			metadata["email"] = email
		}
		if expired := expiryFromValue(firstValue(candidate, "expired", "expires_at", "expiresAt", "expiry", "expiration")); expired != "" {
			metadata["expired"] = expired
		}
		if lastRefresh := stringFromMap(candidate, "last_refresh", "lastRefresh"); lastRefresh != "" {
			metadata["last_refresh"] = lastRefresh
		}
		return metadata, nil
	}
	return nil, errAuthNotImportable
}

func saveMetadataAuth(ctx context.Context, authDir, provider, fileName string, metadata map[string]any) (string, error) {
	store := sdkauth.NewFileTokenStore()
	store.SetBaseDir(authDir)
	return store.Save(ctx, &cliproxyauth.Auth{
		ID:       fileName,
		Provider: provider,
		FileName: fileName,
		Metadata: metadata,
	})
}

func findAuth(ctx context.Context, authDir, provider string) (*cliproxyauth.Auth, error) {
	store := sdkauth.NewFileTokenStore()
	store.SetBaseDir(authDir)
	records, err := store.List(ctx)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, record := range records {
		if record == nil || record.Disabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(record.Provider), provider) {
			return record, nil
		}
	}
	return nil, nil
}

func authConfig() (*sdkconfig.Config, error) {
	authDir, err := configuredAuthDir()
	if err != nil {
		return nil, err
	}
	return &sdkconfig.Config{AuthDir: authDir}, nil
}

func normalizeAuthProvider(provider string) (authProvider, statusProvider string, err error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderCodex:
		return ProviderCodex, ProviderCodex, nil
	case "claude_code", "claude-code", "claude", ProviderAnthropic:
		return authProviderClaude, "claude_code", nil
	default:
		return "", "", fmt.Errorf("unsupported cliproxy auth provider %q", provider)
	}
}

func authenticatedStatus(statusProvider, source, authProvider string) AuthStatus {
	status := AuthStatus{
		Provider:      statusProvider,
		Authenticated: true,
		Source:        source,
		SupportsLogin: true,
	}
	if authProvider == authProviderClaude && currentGOOS == "darwin" {
		status.SupportsKeychain = true
	}
	return status
}

func unauthenticatedStatus(statusProvider, authProvider string) AuthStatus {
	command := "csgclaw model auth login " + statusProvider
	if statusProvider == "claude_code" {
		command = "csgclaw model auth login claude-code"
	}
	status := AuthStatus{
		Provider:      statusProvider,
		LoginRequired: true,
		SupportsLogin: true,
		Message:       "Auth required. Run " + command + " or connect this provider in the CSGClaw UI.",
	}
	if authProvider == authProviderClaude && currentGOOS == "darwin" {
		status.SupportsKeychain = true
	}
	return status
}

func autoAuthImportEnabled() bool {
	return !envIsFalse(autoLoginEnv)
}

func keychainImportEnabled() bool {
	return !envBool(disableKeychainEnv)
}

func envBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func envIsFalse(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "0" || value == "false" || value == "no" || value == "off"
}

func authFileName(provider string, metadata map[string]any, fallback string) string {
	base := fallback
	if email, _ := metadata["email"].(string); strings.TrimSpace(email) != "" {
		base = provider + "-" + sanitizeFileSegment(email)
	} else if accountID, _ := metadata["account_id"].(string); strings.TrimSpace(accountID) != "" {
		base = provider + "-" + shortHash(accountID)
	}
	if !strings.HasSuffix(strings.ToLower(base), ".json") {
		base += ".json"
	}
	return base
}

func sanitizeFileSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "account"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '_', r == '-', r == '@':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" {
		return "account"
	}
	return out
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func mapValue(values map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if v, ok := values[key].(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

func stringFromMap(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := values[key].(string); ok {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func copyString(dst map[string]any, src map[string]any, canonical string, aliases ...string) {
	keys := append([]string{canonical}, aliases...)
	if value := stringFromMap(src, keys...); value != "" {
		dst[canonical] = value
	}
}

func firstValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func collectMaps(root any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			out = append(out, typed)
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(root)
	return out
}

func findFirstString(root any, keys ...string) string {
	for _, candidate := range collectMaps(root) {
		if value := stringFromMap(candidate, keys...); value != "" {
			return value
		}
	}
	return ""
}

func expiryFromValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return unixExpiry(typed)
	case int64:
		return unixExpiry(float64(typed))
	case json.Number:
		n, err := strconv.ParseFloat(string(typed), 64)
		if err != nil {
			return ""
		}
		return unixExpiry(n)
	default:
		return ""
	}
}

func unixExpiry(value float64) string {
	if value <= 0 {
		return ""
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(int64(value)).UTC().Format(time.RFC3339)
	}
	return time.Unix(int64(value), 0).UTC().Format(time.RFC3339)
}

func emailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err = json.Unmarshal(raw, &claims); err != nil {
		return ""
	}
	if email, _ := claims["email"].(string); strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	return ""
}
