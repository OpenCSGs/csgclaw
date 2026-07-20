package connectors

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultGitHubAuthorizeURL = "https://github.com/login/oauth/authorize"
	defaultGitHubTokenURL     = "https://github.com/login/oauth/access_token"
	defaultGitHubAPIBaseURL   = "https://api.github.com"
	defaultGitHubAppSlug      = "csgclaw"

	envGitHubOAuthClientID     = "CSGCLAW_GITHUB_OAUTH_CLIENT_ID"
	envGitHubOAuthClientSecret = "CSGCLAW_GITHUB_OAUTH_CLIENT_SECRET"
	envGitHubOAuthScopes       = "CSGCLAW_GITHUB_OAUTH_SCOPES"
	envGitHubAppSlug           = "CSGCLAW_GITHUB_APP_SLUG"
)

type Endpoints struct {
	AuthorizeURL string
	TokenURL     string
	APIBaseURL   string
}

type Service struct {
	Store              Store
	HTTPClient         *http.Client
	Endpoints          Endpoints
	GitHubOAuthApp     Config
	GitHubAppSlug      string
	Now                func() time.Time
	GenerateOAuthState func() (OAuthState, error)
}

type callbackValidationError struct {
	msg string
}

func (e callbackValidationError) Error() string {
	return e.msg
}

func IsCallbackValidationError(err error) bool {
	var target callbackValidationError
	return errors.As(err, &target)
}

func NewService(store Store) *Service {
	return &Service{Store: store, GitHubOAuthApp: DefaultGitHubOAuthApp(), GitHubAppSlug: DefaultGitHubAppSlug()}
}

func (s *Service) List(ctx context.Context, callbackURL string) ([]Status, error) {
	status, err := s.Status(ctx, ProviderGitHub, callbackURL)
	if err != nil {
		return nil, err
	}
	gitlab, err := s.Status(ctx, ProviderGitLab, "")
	if err != nil {
		return nil, err
	}
	return []Status{status, gitlab}, nil
}

func (s *Service) Status(_ context.Context, provider, callbackURL string) (Status, error) {
	if err := validateProvider(provider); err != nil {
		return Status{}, err
	}
	if strings.EqualFold(strings.TrimSpace(provider), ProviderGitLab) {
		state, _, err := s.store().LoadGitLab()
		if err != nil {
			return Status{}, err
		}
		return state.GitLabStatus(), nil
	}
	state, _, err := s.store().LoadGitHub()
	if err != nil {
		return Status{}, err
	}
	status := state.Status(callbackURL)
	if !status.Configured {
		app := s.githubOAuthApp()
		if app.ClientID != "" && app.ClientSecret != "" {
			status.Configured = true
			status.ClientSecretSet = true
			status.Scopes = cloneStrings(app.Scopes)
		}
	}
	status.AppManageable = s.githubAppSlug() != ""
	return status, nil
}

func (s *Service) SaveGitLabConfig(ctx context.Context, config Config) (Status, error) {
	store := s.store()
	state, _, err := store.LoadGitLab()
	if err != nil {
		return Status{}, err
	}
	next := NormalizeGitLabConfig(config)
	existing := NormalizeGitLabConfig(state.Config)
	if next.AccessToken == "" {
		next.AccessToken = existing.AccessToken
	}
	if err := validateGitLabBaseURL(next.BaseURL); err != nil {
		return Status{}, err
	}
	if next.AccessToken == "" {
		return Status{}, fmt.Errorf("gitlab access_token is required")
	}
	account, err := s.fetchGitLabAccount(ctx, next)
	if err != nil {
		return Status{}, fmt.Errorf("validate gitlab connector: %w", err)
	}
	now := s.now()
	state.Config = next
	state.Account = &account
	state.ConnectedAt = now
	state.UpdatedAt = now
	if err := store.SaveGitLab(state); err != nil {
		return Status{}, err
	}
	return state.GitLabStatus(), nil
}

func (s *Service) fetchGitLabAccount(ctx context.Context, config Config) (Account, error) {
	endpoint := strings.TrimRight(config.BaseURL, "/") + "/api/v4/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Account{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", config.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return Account{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Account{}, fmt.Errorf("gitlab user api returned status %d", resp.StatusCode)
	}
	var value struct {
		Username  string `json:"username"`
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
		WebURL    string `json:"web_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&value); err != nil {
		return Account{}, fmt.Errorf("decode gitlab user: %w", err)
	}
	if strings.TrimSpace(value.Username) == "" {
		return Account{}, fmt.Errorf("gitlab user api returned empty username")
	}
	return Account{Login: value.Username, ID: value.ID, Name: value.Name, Email: value.Email, AvatarURL: value.AvatarURL, HTMLURL: value.WebURL}, nil
}

func validateGitLabBaseURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("gitlab base_url is required")
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("gitlab base_url must be an absolute http(s) URL without credentials, query, or fragment")
	}
	return nil
}

func (s *Service) SaveConfig(ctx context.Context, provider string, config Config) (Status, error) {
	if err := validateProvider(provider); err != nil {
		return Status{}, err
	}
	store := s.store()
	state, _, err := store.LoadGitHub()
	if err != nil {
		return Status{}, err
	}
	next := NormalizeConfig(config)
	existing := NormalizeConfig(state.Config)
	if next.ClientID == "" {
		next.ClientID = existing.ClientID
	}
	if next.ClientSecret == "" {
		next.ClientSecret = existing.ClientSecret
	}
	if next.ClientID == "" {
		return Status{}, fmt.Errorf("github client_id is required")
	}
	if next.ClientSecret == "" {
		return Status{}, fmt.Errorf("github client_secret is required")
	}
	state.Config = next
	state.UpdatedAt = s.now()
	if err := store.SaveGitHub(state); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, provider, "")
}

func (s *Service) StartOAuth(_ context.Context, provider string, opts OAuthStartOptions) (OAuthStartResponse, error) {
	if err := validateProvider(provider); err != nil {
		return OAuthStartResponse{}, err
	}
	callbackURL := strings.TrimSpace(opts.CallbackURL)
	if callbackURL == "" {
		return OAuthStartResponse{}, fmt.Errorf("callback_url is required")
	}
	store := s.store()
	state, _, err := store.LoadGitHub()
	if err != nil {
		return OAuthStartResponse{}, err
	}
	config := s.githubOAuthConfig(state.Config)
	if config.ClientID == "" || config.ClientSecret == "" {
		return OAuthStartResponse{}, fmt.Errorf("github oauth app is not configured")
	}
	oauthState, err := s.oauthState()
	if err != nil {
		return OAuthStartResponse{}, err
	}
	if strings.TrimSpace(oauthState.State) == "" || strings.TrimSpace(oauthState.CodeVerifier) == "" {
		return OAuthStartResponse{}, fmt.Errorf("oauth state generator returned empty values")
	}
	state.Pending = &PendingAuth{
		State:        oauthState.State,
		CodeVerifier: oauthState.CodeVerifier,
		CallbackURL:  callbackURL,
		ReturnURL:    strings.TrimSpace(opts.ReturnURL),
		CreatedAt:    s.now(),
	}
	state.UpdatedAt = s.now()
	if err := store.SaveGitHub(state); err != nil {
		return OAuthStartResponse{}, err
	}
	authURL, err := s.authorizationURL(config, *state.Pending)
	if err != nil {
		return OAuthStartResponse{}, err
	}
	return OAuthStartResponse{Provider: ProviderGitHub, AuthorizationURL: authURL}, nil
}

func (s *Service) CompleteOAuth(ctx context.Context, provider string, values url.Values) (Status, error) {
	if err := validateProvider(provider); err != nil {
		return Status{}, err
	}
	code := strings.TrimSpace(values.Get("code"))
	if code == "" {
		return Status{}, callbackValidationError{"oauth code is required"}
	}
	gotState := strings.TrimSpace(values.Get("state"))
	if gotState == "" {
		return Status{}, callbackValidationError{"oauth state is required"}
	}
	store := s.store()
	state, ok, err := store.LoadGitHub()
	if err != nil {
		return Status{}, err
	}
	if !ok || state.Pending == nil {
		return Status{}, callbackValidationError{"oauth login was not started"}
	}
	pending := *state.Pending
	if gotState != pending.State {
		return Status{}, callbackValidationError{"oauth state mismatch"}
	}
	config := s.githubOAuthConfig(state.Config)
	if config.ClientID == "" || config.ClientSecret == "" {
		return Status{}, fmt.Errorf("github oauth app is not configured")
	}
	token, err := s.exchangeCode(ctx, config, pending, code)
	if err != nil {
		return Status{}, err
	}
	account, err := s.fetchGitHubAccount(ctx, token)
	if err != nil {
		return Status{}, err
	}
	now := s.now()
	state.Pending = nil
	state.Token = &token
	state.Account = &account
	state.ConnectedAt = now
	state.UpdatedAt = now
	if err := store.SaveGitHub(state); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, provider, "")
}

func (s *Service) Disconnect(ctx context.Context, provider string) (Status, error) {
	if err := validateProvider(provider); err != nil {
		return Status{}, err
	}
	store := s.store()
	if strings.EqualFold(strings.TrimSpace(provider), ProviderGitLab) {
		state, _, err := store.LoadGitLab()
		if err != nil {
			return Status{}, err
		}
		state.Config.AccessToken = ""
		state.Account = nil
		state.ConnectedAt = time.Time{}
		state.UpdatedAt = s.now()
		if err := store.SaveGitLab(state); err != nil {
			return Status{}, err
		}
		return state.GitLabStatus(), nil
	}
	state, _, err := store.LoadGitHub()
	if err != nil {
		return Status{}, err
	}
	state.Pending = nil
	state.Token = nil
	state.Account = nil
	state.ConnectedAt = time.Time{}
	state.UpdatedAt = s.now()
	if err := store.SaveGitHub(state); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, provider, "")
}

func (s *Service) Credential(ctx context.Context, provider string) (Credential, error) {
	if err := validateProvider(provider); err != nil {
		return Credential{}, err
	}
	store := s.store()
	if strings.EqualFold(strings.TrimSpace(provider), ProviderGitLab) {
		state, ok, err := store.LoadGitLab()
		if err != nil {
			return Credential{}, err
		}
		config := NormalizeGitLabConfig(state.Config)
		if !ok || config.AccessToken == "" {
			return Credential{}, fmt.Errorf("gitlab connector is not connected")
		}
		account, err := s.fetchGitLabAccount(ctx, config)
		if err != nil {
			return Credential{}, fmt.Errorf("gitlab connector credential is invalid; reconnect GitLab: %w", err)
		}
		state.Account = &account
		state.UpdatedAt = s.now()
		if err := store.SaveGitLab(state); err != nil {
			return Credential{}, err
		}
		return Credential{Provider: ProviderGitLab, AccessToken: config.AccessToken, TokenType: "private-token", BaseURL: config.BaseURL}, nil
	}
	state, ok, err := store.LoadGitHub()
	if err != nil {
		return Credential{}, err
	}
	if !ok || state.Token == nil || strings.TrimSpace(state.Token.AccessToken) == "" {
		return Credential{}, fmt.Errorf("github connector is not connected")
	}
	token := normalizeToken(state.Token)
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return Credential{}, fmt.Errorf("github connector is not connected")
	}
	validToken := *token
	if s.tokenExpired(validToken) {
		refreshed, err := s.refreshStoredGitHubToken(ctx, store, state, validToken)
		if err != nil {
			return Credential{}, fmt.Errorf("github connector credential is invalid or expired; reconnect GitHub: %w", err)
		}
		validToken = refreshed
	} else if _, err := s.fetchGitHubAccount(ctx, validToken); err != nil {
		refreshed, refreshErr := s.refreshStoredGitHubToken(ctx, store, state, validToken)
		if refreshErr != nil {
			return Credential{}, fmt.Errorf("github connector credential is invalid or expired; reconnect GitHub: %w", err)
		}
		validToken = refreshed
	}
	return Credential{
		Provider:    ProviderGitHub,
		AccessToken: validToken.AccessToken,
		TokenType:   validToken.TokenType,
		Scopes:      cloneStrings(validToken.Scopes),
	}, nil
}

func (s *Service) refreshStoredGitHubToken(ctx context.Context, store Store, state State, token Token) (Token, error) {
	refreshToken := strings.TrimSpace(token.RefreshToken)
	if refreshToken == "" {
		return Token{}, fmt.Errorf("refresh token is not available")
	}
	if !token.RefreshTokenExpiresAt.IsZero() && !s.now().Before(token.RefreshTokenExpiresAt) {
		return Token{}, fmt.Errorf("refresh token is expired")
	}
	config := s.githubOAuthConfig(state.Config)
	if config.ClientID == "" || config.ClientSecret == "" {
		return Token{}, fmt.Errorf("github oauth app is not configured")
	}
	refreshed, err := s.refreshGitHubToken(ctx, config, refreshToken)
	if err != nil {
		return Token{}, err
	}
	account, err := s.fetchGitHubAccount(ctx, refreshed)
	if err != nil {
		return Token{}, err
	}
	state.Token = &refreshed
	state.Account = &account
	state.UpdatedAt = s.now()
	if err := store.SaveGitHub(state); err != nil {
		return Token{}, err
	}
	return refreshed, nil
}

func (s *Service) StartGitHubAppInstall(_ context.Context, provider string) (AppInstallStartResponse, error) {
	if err := validateProvider(provider); err != nil {
		return AppInstallStartResponse{}, err
	}
	slug := s.githubAppSlug()
	if slug == "" {
		return AppInstallStartResponse{}, fmt.Errorf("github app is not configured")
	}
	oauthState, err := s.oauthState()
	if err != nil {
		return AppInstallStartResponse{}, err
	}
	state := strings.TrimSpace(oauthState.State)
	if state == "" {
		return AppInstallStartResponse{}, fmt.Errorf("app install state generator returned empty value")
	}
	u := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/apps/" + url.PathEscape(slug) + "/installations/select_target",
	}
	q := u.Query()
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return AppInstallStartResponse{Provider: ProviderGitHub, InstallURL: u.String()}, nil
}

func (s *Service) authorizationURL(config Config, pending PendingAuth) (string, error) {
	baseURL := strings.TrimSpace(s.endpoints().AuthorizeURL)
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", config.ClientID)
	q.Set("redirect_uri", pending.CallbackURL)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(config.Scopes, " "))
	q.Set("state", pending.State)
	q.Set("code_challenge", codeChallengeS256(pending.CodeVerifier))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Service) exchangeCode(ctx context.Context, config Config, pending PendingAuth, code string) (Token, error) {
	form := url.Values{}
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", config.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", pending.CallbackURL)
	form.Set("code_verifier", pending.CodeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoints().TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var resp githubTokenResponse
	if err := s.doJSON(req, &resp); err != nil {
		return Token{}, fmt.Errorf("exchange github oauth code: %w", err)
	}
	token, err := s.tokenFromResponse(resp)
	if err != nil {
		return Token{}, fmt.Errorf("exchange github oauth code: %w", err)
	}
	return token, nil
}

func (s *Service) refreshGitHubToken(ctx context.Context, config Config, refreshToken string) (Token, error) {
	form := url.Values{}
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", config.ClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoints().TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var resp githubTokenResponse
	if err := s.doJSON(req, &resp); err != nil {
		return Token{}, fmt.Errorf("refresh github oauth token: %w", err)
	}
	token, err := s.tokenFromResponse(resp)
	if err != nil {
		return Token{}, fmt.Errorf("refresh github oauth token: %w", err)
	}
	return token, nil
}

type githubTokenResponse struct {
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	Description           string `json:"error_description"`
}

func (s *Service) tokenFromResponse(resp githubTokenResponse) (Token, error) {
	if resp.Error != "" {
		msg := strings.TrimSpace(resp.Description)
		if msg == "" {
			msg = resp.Error
		}
		return Token{}, fmt.Errorf("%s", msg)
	}
	now := s.now()
	token := Token{
		AccessToken:  resp.AccessToken,
		TokenType:    resp.TokenType,
		Scopes:       splitScopes(resp.Scope),
		RefreshToken: resp.RefreshToken,
	}
	if resp.ExpiresIn > 0 {
		token.ExpiresAt = now.Add(time.Duration(resp.ExpiresIn) * time.Second).UTC()
	}
	if resp.RefreshTokenExpiresIn > 0 {
		token.RefreshTokenExpiresAt = now.Add(time.Duration(resp.RefreshTokenExpiresIn) * time.Second).UTC()
	}
	token = *normalizeToken(&token)
	if token.AccessToken == "" {
		return Token{}, fmt.Errorf("access token not found")
	}
	return token, nil
}

func (s *Service) tokenExpired(token Token) bool {
	if token.ExpiresAt.IsZero() {
		return false
	}
	return !s.now().Before(token.ExpiresAt)
}

func (s *Service) fetchGitHubAccount(ctx context.Context, token Token) (Account, error) {
	endpoint := strings.TrimRight(s.endpoints().APIBaseURL, "/") + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Account{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	var account Account
	if err := s.doJSON(req, &account); err != nil {
		return Account{}, fmt.Errorf("fetch github account: %w", err)
	}
	account = normalizeAccount(account)
	if account.Login == "" {
		return Account{}, fmt.Errorf("fetch github account: login not found")
	}
	return account, nil
}

func (s *Service) doJSON(req *http.Request, out any) error {
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (s *Service) store() Store {
	if s != nil && strings.TrimSpace(s.Store.path) != "" {
		return s.Store
	}
	store, err := DefaultStore()
	if err != nil {
		return Store{}
	}
	return store
}

func (s *Service) httpClient() *http.Client {
	if s != nil && s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (s *Service) endpoints() Endpoints {
	endpoints := Endpoints{
		AuthorizeURL: defaultGitHubAuthorizeURL,
		TokenURL:     defaultGitHubTokenURL,
		APIBaseURL:   defaultGitHubAPIBaseURL,
	}
	if s == nil {
		return endpoints
	}
	if value := strings.TrimSpace(s.Endpoints.AuthorizeURL); value != "" {
		endpoints.AuthorizeURL = value
	}
	if value := strings.TrimSpace(s.Endpoints.TokenURL); value != "" {
		endpoints.TokenURL = value
	}
	if value := strings.TrimRight(strings.TrimSpace(s.Endpoints.APIBaseURL), "/"); value != "" {
		endpoints.APIBaseURL = value
	}
	return endpoints
}

func DefaultGitHubOAuthApp() Config {
	scopes := DefaultGitHubScopes
	if value := strings.TrimSpace(os.Getenv(envGitHubOAuthScopes)); value != "" {
		scopes = splitScopes(value)
	}
	return NormalizeConfig(Config{
		ClientID:     os.Getenv(envGitHubOAuthClientID),
		ClientSecret: os.Getenv(envGitHubOAuthClientSecret),
		Scopes:       scopes,
	})
}

func DefaultGitHubAppSlug() string {
	if value := strings.TrimSpace(os.Getenv(envGitHubAppSlug)); value != "" {
		return value
	}
	return defaultGitHubAppSlug
}

func (s *Service) githubOAuthConfig(saved Config) Config {
	saved = NormalizeConfig(saved)
	if saved.ClientID != "" && saved.ClientSecret != "" {
		return saved
	}
	return s.githubOAuthApp()
}

func (s *Service) githubOAuthApp() Config {
	if s == nil {
		return DefaultGitHubOAuthApp()
	}
	return NormalizeConfig(s.GitHubOAuthApp)
}

func (s *Service) githubAppSlug() string {
	if s == nil {
		return DefaultGitHubAppSlug()
	}
	return strings.Trim(strings.TrimSpace(s.GitHubAppSlug), "/")
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) oauthState() (OAuthState, error) {
	if s != nil && s.GenerateOAuthState != nil {
		return s.GenerateOAuthState()
	}
	state, err := randomURLSafe(32)
	if err != nil {
		return OAuthState{}, err
	}
	verifier, err := randomURLSafe(32)
	if err != nil {
		return OAuthState{}, err
	}
	return OAuthState{State: state, CodeVerifier: verifier}, nil
}

func validateProvider(provider string) error {
	if strings.EqualFold(strings.TrimSpace(provider), ProviderGitHub) || strings.EqualFold(strings.TrimSpace(provider), ProviderGitLab) {
		return nil
	}
	return fmt.Errorf("unsupported connector provider %q", strings.TrimSpace(provider))
}

func codeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLSafe(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func splitScopes(scope string) []string {
	scope = strings.ReplaceAll(scope, ",", " ")
	return normalizeScopes(strings.Fields(scope))
}
