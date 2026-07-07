package connectors

import (
	"strings"
	"time"
)

const ProviderGitHub = "github"

var DefaultGitHubScopes = []string{"repo", "read:user", "user:email"}

type Config struct {
	ClientID     string   `json:"client_id,omitempty"`
	ClientSecret string   `json:"client_secret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

type PendingAuth struct {
	State        string    `json:"state,omitempty"`
	CodeVerifier string    `json:"code_verifier,omitempty"`
	CallbackURL  string    `json:"callback_url,omitempty"`
	ReturnURL    string    `json:"return_url,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}

type Token struct {
	AccessToken string   `json:"access_token,omitempty"`
	TokenType   string   `json:"token_type,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

type Account struct {
	Login     string `json:"login,omitempty"`
	ID        int64  `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
}

type State struct {
	Config      Config       `json:"config,omitempty"`
	Pending     *PendingAuth `json:"pending,omitempty"`
	Token       *Token       `json:"token,omitempty"`
	Account     *Account     `json:"account,omitempty"`
	ConnectedAt time.Time    `json:"connected_at,omitempty"`
	UpdatedAt   time.Time    `json:"updated_at,omitempty"`
}

type Status struct {
	Provider        string     `json:"provider"`
	Name            string     `json:"name"`
	Configured      bool       `json:"configured"`
	Connected       bool       `json:"connected"`
	AppManageable   bool       `json:"app_manageable"`
	OAuthPending    bool       `json:"oauth_pending"`
	ClientID        string     `json:"client_id,omitempty"`
	ClientSecretSet bool       `json:"client_secret_set"`
	Scopes          []string   `json:"scopes,omitempty"`
	Account         *Account   `json:"account,omitempty"`
	CallbackURL     string     `json:"callback_url,omitempty"`
	ConnectedAt     *time.Time `json:"connected_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type OAuthStartOptions struct {
	CallbackURL string
	ReturnURL   string
}

type OAuthStartResponse struct {
	Provider         string `json:"provider"`
	AuthorizationURL string `json:"authorization_url"`
}

type AppInstallStartResponse struct {
	Provider   string `json:"provider"`
	InstallURL string `json:"install_url"`
}

type OAuthState struct {
	State        string
	CodeVerifier string
}

type Credential struct {
	Provider    string   `json:"provider"`
	AccessToken string   `json:"access_token"`
	TokenType   string   `json:"token_type,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

func (s State) Status(callbackURL string) Status {
	config := NormalizeConfig(s.Config)
	token := normalizeToken(s.Token)
	connected := token != nil && token.AccessToken != ""
	scopes := config.Scopes
	if connected && len(token.Scopes) > 0 {
		scopes = cloneStrings(token.Scopes)
	}
	status := Status{
		Provider:        ProviderGitHub,
		Name:            "GitHub",
		Configured:      config.ClientID != "" && config.ClientSecret != "",
		Connected:       connected,
		OAuthPending:    s.Pending != nil,
		ClientID:        config.ClientID,
		ClientSecretSet: config.ClientSecret != "",
		Scopes:          scopes,
		CallbackURL:     strings.TrimSpace(callbackURL),
	}
	if connected && s.Account != nil {
		account := normalizeAccount(*s.Account)
		status.Account = &account
	}
	if !s.ConnectedAt.IsZero() {
		connectedAt := s.ConnectedAt.UTC()
		status.ConnectedAt = &connectedAt
	}
	if !s.UpdatedAt.IsZero() {
		updatedAt := s.UpdatedAt.UTC()
		status.UpdatedAt = &updatedAt
	}
	return status
}

func NormalizeConfig(config Config) Config {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.Scopes = normalizeScopes(config.Scopes)
	if len(config.Scopes) == 0 {
		config.Scopes = cloneStrings(DefaultGitHubScopes)
	}
	return config
}

func normalizeState(state State) State {
	state.Config = NormalizeConfig(state.Config)
	if state.Pending != nil {
		pending := *state.Pending
		pending.State = strings.TrimSpace(pending.State)
		pending.CodeVerifier = strings.TrimSpace(pending.CodeVerifier)
		pending.CallbackURL = strings.TrimSpace(pending.CallbackURL)
		pending.ReturnURL = strings.TrimSpace(pending.ReturnURL)
		pending.CreatedAt = pending.CreatedAt.UTC()
		state.Pending = &pending
	}
	state.Token = normalizeToken(state.Token)
	if state.Account != nil {
		account := normalizeAccount(*state.Account)
		state.Account = &account
	}
	state.ConnectedAt = state.ConnectedAt.UTC()
	state.UpdatedAt = state.UpdatedAt.UTC()
	return state
}

func normalizeToken(token *Token) *Token {
	if token == nil {
		return nil
	}
	out := *token
	out.AccessToken = strings.TrimSpace(out.AccessToken)
	out.TokenType = strings.TrimSpace(out.TokenType)
	if out.TokenType == "" && out.AccessToken != "" {
		out.TokenType = "bearer"
	}
	out.Scopes = normalizeScopes(out.Scopes)
	if out.AccessToken == "" && out.TokenType == "" && len(out.Scopes) == 0 {
		return nil
	}
	return &out
}

func normalizeAccount(account Account) Account {
	account.Login = strings.TrimSpace(account.Login)
	account.Name = strings.TrimSpace(account.Name)
	account.Email = strings.TrimSpace(account.Email)
	account.AvatarURL = strings.TrimSpace(account.AvatarURL)
	account.HTMLURL = strings.TrimSpace(account.HTMLURL)
	return account
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, item := range scopes {
		scope := strings.TrimSpace(item)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	return append([]string(nil), items...)
}
