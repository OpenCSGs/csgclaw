package auth

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/localstore"
)

const (
	authFileName               = "auth.json"
	providerAuthDirName        = "auth"
	csgHubProviderAuthFileName = "csghub.json"
	rootAuthSectionName        = "auth"
	openCSGAuthKey             = "opencsg"
	DefaultAIGatewayBaseURL    = "https://ai.space.opencsg.com/v1"
)

type Record struct {
	Tokens      Tokens    `json:"tokens"`
	Account     Account   `json:"account"`
	LastRefresh time.Time `json:"last_refresh"`
}

type Tokens struct {
	AccessToken string `json:"access_token"`
}

type Account struct {
	UserID         string    `json:"user_id"`
	UserUUID       string    `json:"user_uuid"`
	Name           string    `json:"name,omitempty"`
	Avatar         string    `json:"avatar"`
	OpenCSGBaseURL string    `json:"opencsg_base_url,omitempty"`
	BaseURL        string    `json:"base_url"`
	PortalURL      string    `json:"portal_url"`
	LoggedInAt     time.Time `json:"logged_in_at"`
}

type CSGHubProviderCredentials struct {
	AIGatewayBaseURL       string `json:"ai_gateway_base_url,omitempty"`
	AIGatewayBuiltinAPIKey string `json:"ai_gateway_builtin_api_key"`
}

type Status struct {
	Authenticated    bool       `json:"authenticated"`
	UserID           string     `json:"user_id,omitempty"`
	UserUUID         string     `json:"user_uuid,omitempty"`
	Name             string     `json:"name,omitempty"`
	Avatar           string     `json:"avatar,omitempty"`
	OpenCSGBaseURL   string     `json:"opencsg_base_url,omitempty"`
	BaseURL          string     `json:"base_url,omitempty"`
	AIGatewayBaseURL string     `json:"ai_gateway_base_url,omitempty"`
	PortalURL        string     `json:"portal_url,omitempty"`
	LoggedInAt       *time.Time `json:"logged_in_at,omitempty"`
}

type Store struct {
	path                   string
	csgHubProviderAuthPath string
}

type openCSGAuthRecord struct {
	Tokens                 Tokens    `json:"tokens,omitempty"`
	Account                Account   `json:"account,omitempty"`
	LastRefresh            time.Time `json:"last_refresh,omitempty"`
	AIGatewayBaseURL       string    `json:"ai_gateway_base_url,omitempty"`
	AIGatewayBuiltinAPIKey string    `json:"ai_gateway_builtin_api_key,omitempty"`
}

func NewStore(path string) Store {
	return Store{path: strings.TrimSpace(path)}
}

func NewStoreWithProviderPath(path, csgHubProviderAuthPath string) Store {
	return Store{
		path:                   strings.TrimSpace(path),
		csgHubProviderAuthPath: strings.TrimSpace(csgHubProviderAuthPath),
	}
}

func DefaultPath() (string, error) {
	path, err := config.DefaultStatePath()
	if err != nil {
		return "", fmt.Errorf("resolve auth state path: %w", err)
	}
	return path, nil
}

func DefaultCSGHubProviderPath() (string, error) {
	return DefaultPath()
}

func DefaultStore() (Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return Store{}, err
	}
	return NewStore(path), nil
}

func (s Store) Path() (string, error) {
	path := strings.TrimSpace(s.path)
	if path != "" {
		return path, nil
	}
	return DefaultPath()
}

func (s Store) CSGHubProviderPath() (string, error) {
	path := strings.TrimSpace(s.csgHubProviderAuthPath)
	if path != "" {
		return path, nil
	}
	authPath := strings.TrimSpace(s.path)
	if authPath != "" {
		if localstore.IsRootStatePath(authPath) {
			return authPath, nil
		}
		return filepath.Join(filepath.Dir(authPath), providerAuthDirName, csgHubProviderAuthFileName), nil
	}
	return DefaultCSGHubProviderPath()
}

func (s Store) Load() (Record, bool, error) {
	path, err := s.Path()
	if err != nil {
		return Record{}, false, err
	}
	if localstore.IsRootStatePath(path) {
		state, ok, err := s.loadRootOpenCSGAuth(path)
		if err != nil || !ok {
			return Record{}, false, err
		}
		if !hasOpenCSGAccountAuth(state) {
			return Record{}, false, nil
		}
		return normalizeRecord(state.Record()), true, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, fmt.Errorf("read auth store: %w", err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, false, fmt.Errorf("decode auth store: %w", err)
	}
	return normalizeRecord(record), true, nil
}

func (s Store) Save(record Record) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	record = normalizeRecord(record)
	if record.Tokens.AccessToken == "" {
		return fmt.Errorf("auth access token is required")
	}
	if record.Account.BaseURL == "" {
		return fmt.Errorf("auth base url is required")
	}
	if localstore.IsRootStatePath(path) {
		authState, _, err := readRootAuthState(path)
		if err != nil {
			return err
		}
		openCSG := openCSGAuthRecord{}
		if existing, ok := authState[openCSGAuthKey]; ok && len(existing) > 0 {
			if err := json.Unmarshal(existing, &openCSG); err != nil {
				return fmt.Errorf("decode root opencsg auth: %w", err)
			}
		}
		openCSG.Tokens = record.Tokens
		openCSG.Account = record.Account
		openCSG.LastRefresh = record.LastRefresh
		if err := setRootOpenCSGAuth(path, authState, openCSG); err != nil {
			return fmt.Errorf("write auth store: %w", err)
		}
		return nil
	}
	if err := writeJSONFile(path, record); err != nil {
		return fmt.Errorf("write auth store: %w", err)
	}
	return nil
}

func (s Store) Delete() error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	if localstore.IsRootStatePath(path) {
		authState, found, err := readRootAuthState(path)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		delete(authState, openCSGAuthKey)
		if err := localstore.WriteSection(path, rootAuthSectionName, authState); err != nil {
			return fmt.Errorf("delete auth store: %w", err)
		}
		return nil
	}
	if err := removeFile(path); err != nil {
		return fmt.Errorf("delete auth store: %w", err)
	}
	providerPath, err := s.CSGHubProviderPath()
	if err != nil {
		return err
	}
	if err := removeFile(providerPath); err != nil {
		return fmt.Errorf("delete csghub provider auth store: %w", err)
	}
	return nil
}

func (s Store) Status() (Status, error) {
	record, ok, err := s.Load()
	if err != nil {
		return Status{}, err
	}
	if !ok {
		return Status{}, nil
	}
	status := record.Status()
	credentials, found, err := s.LoadCSGHubProviderCredentials()
	if err != nil {
		return Status{}, err
	}
	if found && credentials.AIGatewayBaseURL != "" {
		status.AIGatewayBaseURL = credentials.AIGatewayBaseURL
	} else if status.Authenticated {
		status.AIGatewayBaseURL = AIGatewayBaseURL("")
	}
	return status, nil
}

func (s Store) Credentials() (baseURL, token string, ok bool, err error) {
	record, found, err := s.Load()
	if err != nil || !found {
		return "", "", false, err
	}
	baseURL = strings.TrimRight(strings.TrimSpace(record.Account.BaseURL), "/")
	token = strings.TrimSpace(record.Tokens.AccessToken)
	return baseURL, token, baseURL != "" && token != "", nil
}

func (s Store) LoadCSGHubProviderCredentials() (CSGHubProviderCredentials, bool, error) {
	path, err := s.CSGHubProviderPath()
	if err != nil {
		return CSGHubProviderCredentials{}, false, err
	}
	if localstore.IsRootStatePath(path) {
		state, ok, err := s.loadRootOpenCSGAuth(path)
		if err != nil || !ok {
			return CSGHubProviderCredentials{}, false, err
		}
		credentials := normalizeCSGHubProviderCredentials(CSGHubProviderCredentials{
			AIGatewayBaseURL:       state.AIGatewayBaseURL,
			AIGatewayBuiltinAPIKey: state.AIGatewayBuiltinAPIKey,
		})
		return credentials, credentials.AIGatewayBaseURL != "" || credentials.AIGatewayBuiltinAPIKey != "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CSGHubProviderCredentials{}, false, nil
		}
		return CSGHubProviderCredentials{}, false, fmt.Errorf("read csghub provider auth store: %w", err)
	}
	var credentials CSGHubProviderCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return CSGHubProviderCredentials{}, false, fmt.Errorf("decode csghub provider auth store: %w", err)
	}
	return normalizeCSGHubProviderCredentials(credentials), true, nil
}

func (s Store) SaveCSGHubProviderCredentials(credentials CSGHubProviderCredentials) error {
	path, err := s.CSGHubProviderPath()
	if err != nil {
		return err
	}
	credentials = normalizeCSGHubProviderCredentials(credentials)
	if credentials.AIGatewayBuiltinAPIKey == "" {
		if credentials.AIGatewayBaseURL == "" {
			return fmt.Errorf("csghub ai gateway credentials are required")
		}
	}
	if localstore.IsRootStatePath(path) {
		authState, _, err := readRootAuthState(path)
		if err != nil {
			return err
		}
		openCSG := openCSGAuthRecord{}
		if existing, ok := authState[openCSGAuthKey]; ok && len(existing) > 0 {
			if err := json.Unmarshal(existing, &openCSG); err != nil {
				return fmt.Errorf("decode root opencsg auth: %w", err)
			}
		}
		openCSG.AIGatewayBaseURL = credentials.AIGatewayBaseURL
		openCSG.AIGatewayBuiltinAPIKey = credentials.AIGatewayBuiltinAPIKey
		if err := setRootOpenCSGAuth(path, authState, openCSG); err != nil {
			return fmt.Errorf("write csghub provider auth store: %w", err)
		}
		return nil
	}
	if err := writeJSONFile(path, credentials); err != nil {
		return fmt.Errorf("write csghub provider auth store: %w", err)
	}
	return nil
}

func (s Store) AIGatewayCredentials() (baseURL, apiKey string, ok bool, err error) {
	baseURL = AIGatewayBaseURL("")
	credentials, found, err := s.LoadCSGHubProviderCredentials()
	if err != nil {
		return "", "", false, err
	}
	if !found {
		return baseURL, "", false, nil
	}
	if credentials.AIGatewayBaseURL != "" {
		baseURL = credentials.AIGatewayBaseURL
	}
	apiKey = strings.TrimSpace(credentials.AIGatewayBuiltinAPIKey)
	if !isBuiltinAIGatewayAPIKey(apiKey) {
		apiKey = ""
	}
	return baseURL, apiKey, baseURL != "" && apiKey != "", nil
}

func (r Record) Status() Status {
	r = normalizeRecord(r)
	if r.Tokens.AccessToken == "" {
		return Status{}
	}
	status := Status{
		Authenticated:  true,
		UserID:         r.Account.UserID,
		UserUUID:       r.Account.UserUUID,
		Name:           r.Account.Name,
		Avatar:         r.Account.Avatar,
		OpenCSGBaseURL: r.Account.OpenCSGBaseURL,
		BaseURL:        r.Account.BaseURL,
		PortalURL:      r.Account.PortalURL,
	}
	if !r.Account.LoggedInAt.IsZero() {
		loggedInAt := r.Account.LoggedInAt
		status.LoggedInAt = &loggedInAt
	}
	return status
}

func normalizeRecord(record Record) Record {
	record.Tokens.AccessToken = strings.TrimSpace(record.Tokens.AccessToken)
	record.Account.UserID = strings.TrimSpace(record.Account.UserID)
	record.Account.UserUUID = strings.TrimSpace(record.Account.UserUUID)
	record.Account.Name = strings.TrimSpace(record.Account.Name)
	record.Account.Avatar = strings.TrimSpace(record.Account.Avatar)
	record.Account.OpenCSGBaseURL = strings.TrimRight(strings.TrimSpace(record.Account.OpenCSGBaseURL), "/")
	record.Account.BaseURL = strings.TrimRight(strings.TrimSpace(record.Account.BaseURL), "/")
	record.Account.PortalURL = strings.TrimSpace(record.Account.PortalURL)
	if record.LastRefresh.IsZero() {
		record.LastRefresh = record.Account.LoggedInAt
	}
	if record.LastRefresh.IsZero() {
		record.LastRefresh = time.Now().UTC()
	}
	return record
}

func normalizeCSGHubProviderCredentials(credentials CSGHubProviderCredentials) CSGHubProviderCredentials {
	credentials.AIGatewayBaseURL = normalizeAIGatewayBaseURL(credentials.AIGatewayBaseURL)
	credentials.AIGatewayBuiltinAPIKey = strings.TrimSpace(credentials.AIGatewayBuiltinAPIKey)
	return credentials
}

func (s Store) loadRootOpenCSGAuth(path string) (openCSGAuthRecord, bool, error) {
	authState, found, err := readRootAuthState(path)
	if err != nil || !found {
		return openCSGAuthRecord{}, false, err
	}
	raw, ok := authState[openCSGAuthKey]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return openCSGAuthRecord{}, false, nil
	}
	var state openCSGAuthRecord
	if err := json.Unmarshal(raw, &state); err != nil {
		return openCSGAuthRecord{}, false, fmt.Errorf("decode root opencsg auth: %w", err)
	}
	return state, hasOpenCSGAuth(state), nil
}

func readRootAuthState(path string) (map[string]json.RawMessage, bool, error) {
	authState := make(map[string]json.RawMessage)
	found, err := localstore.ReadSection(path, rootAuthSectionName, &authState)
	if err != nil {
		return nil, false, err
	}
	if authState == nil {
		authState = make(map[string]json.RawMessage)
	}
	return authState, found, nil
}

func setRootOpenCSGAuth(path string, authState map[string]json.RawMessage, state openCSGAuthRecord) error {
	if authState == nil {
		authState = make(map[string]json.RawMessage)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode root opencsg auth: %w", err)
	}
	authState[openCSGAuthKey] = raw
	return localstore.WriteSection(path, rootAuthSectionName, authState)
}

func (r openCSGAuthRecord) Record() Record {
	return Record{
		Tokens:      r.Tokens,
		Account:     r.Account,
		LastRefresh: r.LastRefresh,
	}
}

func hasOpenCSGAuth(state openCSGAuthRecord) bool {
	return strings.TrimSpace(state.Tokens.AccessToken) != "" ||
		strings.TrimSpace(state.Account.BaseURL) != "" ||
		strings.TrimSpace(state.Account.UserID) != "" ||
		strings.TrimSpace(state.Account.UserUUID) != "" ||
		strings.TrimSpace(state.AIGatewayBaseURL) != "" ||
		strings.TrimSpace(state.AIGatewayBuiltinAPIKey) != ""
}

func hasOpenCSGAccountAuth(state openCSGAuthRecord) bool {
	return strings.TrimSpace(state.Tokens.AccessToken) != "" ||
		strings.TrimSpace(state.Account.BaseURL) != "" ||
		strings.TrimSpace(state.Account.UserID) != "" ||
		strings.TrimSpace(state.Account.UserUUID) != ""
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure file: %w", err)
	}
	return nil
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func AIGatewayBaseURL(_ string) string {
	if baseURL := normalizeAIGatewayBaseURL(os.Getenv("CSGHUB_AIGATEWAY_BASE_URL")); baseURL != "" {
		return baseURL
	}
	if baseURL := normalizeAIGatewayBaseURL(os.Getenv("CSGHUB_AIGATEWAY_URL")); baseURL != "" {
		return baseURL
	}
	return DefaultAIGatewayBaseURL
}

func normalizeAIGatewayBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/v1") {
		u.Path = strings.TrimRight(u.Path, "/") + "/v1"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}
