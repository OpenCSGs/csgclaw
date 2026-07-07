package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultOpenCSGBaseURL   = "https://opencsg.com"
	DefaultCSGHubBaseURL    = "https://hub.opencsg.com"
	StageOpenCSGBaseURL     = "https://opencsg-stg.com"
	StageCSGHubBaseURL      = "https://opencsg-stg.com"
	StageAIGatewayBaseURL   = "https://aigateway.opencsg-stg.com/v1"
	callbackAuthStateParam  = "auth_state"
	customAIGatewayPathName = "/aigateway"
)

type LoginResponse struct {
	LoginURL string `json:"login_url"`
}

type LoginOptions struct {
	ReturnURL        string
	CallbackURL      string
	OpenCSGBaseURL   string
	CSGHubBaseURL    string
	AIGatewayBaseURL string
}

type Service struct {
	Store          Store
	HTTPClient     *http.Client
	OpenCSGBaseURL string
	CSGHubBaseURL  string
	Now            func() time.Time
}

type authEnvironment struct {
	OpenCSGBaseURL   string
	CSGHubBaseURL    string
	AIGatewayBaseURL string
}

var defaultService = &Service{}

func Default() *Service {
	return defaultService
}

func (s *Service) Status(context.Context) (Status, error) {
	store, err := s.store()
	if err != nil {
		return Status{}, err
	}
	return store.Status()
}

func (s *Service) Login(_ context.Context, opts ...LoginOptions) (LoginResponse, error) {
	returnURL := ""
	callbackURL := ""
	env := authEnvironment{
		OpenCSGBaseURL: s.opencsgBaseURL(),
		CSGHubBaseURL:  s.csghubBaseURL(),
	}
	if len(opts) > 0 {
		returnURL = sanitizeReturnURL(opts[0].ReturnURL)
		callbackURL = sanitizeCallbackURL(opts[0].CallbackURL)
		var err error
		env, err = s.loginEnvironment(opts[0])
		if err != nil {
			return LoginResponse{}, err
		}
	}
	if callbackURL == "" {
		return LoginResponse{}, fmt.Errorf("auth callback url is required")
	}
	callbackURL = callbackURLWithAuthState(callbackURL, env, returnURL)
	return LoginResponse{LoginURL: s.buildLoginURL(callbackURL, env.OpenCSGBaseURL)}, nil
}

func (s *Service) Logout(context.Context) (Status, error) {
	store, err := s.store()
	if err != nil {
		return Status{}, err
	}
	if err := store.Delete(); err != nil {
		return Status{}, err
	}
	return Status{}, nil
}

func (s *Service) CompleteCallback(ctx context.Context, values url.Values) (string, error) {
	return s.completeCallback(ctx, values)
}

func (s *Service) completeCallback(ctx context.Context, values url.Values) (string, error) {
	values = callbackValuesWithAuthState(values)
	jwtToken := strings.TrimSpace(values.Get("jwt_token"))
	if jwtToken == "" {
		jwtToken = strings.TrimSpace(values.Get("jwt"))
	}
	portalURL := strings.TrimSpace(values.Get("portal_url"))
	if jwtToken == "" {
		return "", callbackValidationError("jwt is required")
	}

	claims, err := JWTClaims(jwtToken)
	if err != nil {
		return "", callbackValidationError(err.Error())
	}
	userID := strings.TrimSpace(stringClaim(claims, "current_user"))
	if userID == "" {
		return "", callbackValidationError("jwt current_user is required")
	}
	userUUID := strings.TrimSpace(stringClaim(claims, "uuid"))
	if userUUID == "" {
		return "", callbackValidationError("jwt uuid is required")
	}

	env, err := s.callbackEnvironment(values)
	if err != nil {
		return "", callbackValidationError(err.Error())
	}
	csgHubBaseURL := env.CSGHubBaseURL
	if portalURL != "" {
		portalURL = sanitizePortalURL(portalURL, csgHubBaseURL)
		if portalURL == "" {
			return "", callbackValidationError("portal_url must match csghub base url")
		}
	}
	accessToken, builtinAPIKey, profile, err := s.fetchAuthDetails(ctx, csgHubBaseURL, jwtToken, userID, userUUID)
	if err != nil {
		return "", err
	}

	store, err := s.store()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	record := Record{
		Tokens: Tokens{
			AccessToken: accessToken,
		},
		Account: Account{
			UserID:         userID,
			UserUUID:       userUUID,
			Name:           profile.Name,
			Avatar:         profile.Avatar,
			OpenCSGBaseURL: env.OpenCSGBaseURL,
			BaseURL:        csgHubBaseURL,
			PortalURL:      portalURL,
			LoggedInAt:     now,
		},
		LastRefresh: now,
	}
	if err := store.Save(record); err != nil {
		return "", err
	}
	if builtinAPIKey != "" || env.AIGatewayBaseURL != "" {
		if err := store.SaveCSGHubProviderCredentials(CSGHubProviderCredentials{
			AIGatewayBaseURL:       env.AIGatewayBaseURL,
			AIGatewayBuiltinAPIKey: builtinAPIKey,
		}); err != nil {
			return "", err
		}
	}

	if returnURL := callbackReturnURL(values); returnURL != "" {
		return returnURL, nil
	}
	if portalURL == "" {
		return csgHubBaseURL, nil
	}
	return portalRedirectURL(portalURL, jwtToken), nil
}

type userProfile struct {
	Username string
	Name     string
	Avatar   string
}

func (s *Service) fetchAuthDetails(ctx context.Context, baseURL, jwtToken, userID, userUUID string) (string, string, userProfile, error) {
	token, err := s.fetchUserAccessToken(ctx, baseURL, jwtToken, userID)
	if err != nil {
		return "", "", userProfile{}, err
	}
	profile, err := s.fetchUserProfile(ctx, baseURL, userID)
	if err != nil {
		profile = userProfile{}
	}
	apiKey, err := s.fetchBuiltinAPIKey(ctx, baseURL, jwtToken, userID, userUUID)
	if err != nil {
		apiKey = ""
	}
	return token, apiKey, profile, nil
}

func (s *Service) fetchUserAccessToken(ctx context.Context, baseURL, jwtToken, userID string) (string, error) {
	endpoint, err := joinAPIPath(baseURL, "/api/v1/user/"+url.PathEscape(userID)+"/tokens")
	if err != nil {
		return "", err
	}
	q := endpoint.Query()
	q.Set("app", "git")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	var resp struct {
		Msg  string `json:"msg"`
		Data []struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := s.doJSON(req, &resp); err != nil {
		return "", fmt.Errorf("get csghub user access token: %w", err)
	}
	for _, item := range resp.Data {
		if token := strings.TrimSpace(item.Token); token != "" {
			return token, nil
		}
	}
	return "", fmt.Errorf("get csghub user access token: token not found")
}

func (s *Service) fetchUserProfile(ctx context.Context, baseURL, userID string) (userProfile, error) {
	endpoint, err := joinAPIPath(baseURL, "/api/v1/user/"+url.PathEscape(userID))
	if err != nil {
		return userProfile{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return userProfile{}, err
	}
	req.Header.Set("Accept", "application/json")

	var resp struct {
		Msg  string `json:"msg"`
		Data struct {
			Username    string `json:"username"`
			Name        string `json:"name"`
			Nickname    string `json:"nickname"`
			DisplayName string `json:"display_name"`
			Avatar      string `json:"avatar"`
		} `json:"data"`
	}
	if err := s.doJSON(req, &resp); err != nil {
		return userProfile{}, err
	}
	return userProfile{
		Username: strings.TrimSpace(resp.Data.Username),
		Name:     firstNonEmpty(resp.Data.Name, resp.Data.Nickname, resp.Data.DisplayName),
		Avatar:   strings.TrimSpace(resp.Data.Avatar),
	}, nil
}

func (s *Service) fetchBuiltinAPIKey(ctx context.Context, baseURL, authToken, userID, namespaceUUID string) (string, error) {
	return fetchBuiltinAPIKey(ctx, s.httpClient(), baseURL, authToken, userID, namespaceUUID)
}

func (s *Service) doJSON(req *http.Request, out any) error {
	return doJSONWithClient(s.httpClient(), req, out)
}

func (s *Service) buildLoginURL(callbackURL string, baseURLs ...string) string {
	callbackURL = strings.TrimSpace(callbackURL)
	base := s.opencsgBaseURL()
	if len(baseURLs) > 0 && strings.TrimSpace(baseURLs[0]) != "" {
		base = strings.TrimRight(strings.TrimSpace(baseURLs[0]), "/")
	}
	loginURL, err := url.Parse(base + "/sso/login")
	if err != nil {
		return DefaultOpenCSGBaseURL + "/sso/login"
	}
	q := loginURL.Query()
	q.Set("redirect_url", callbackURL)
	loginURL.RawQuery = q.Encode()
	return loginURL.String()
}

func (s *Service) store() (Store, error) {
	if strings.TrimSpace(s.Store.path) != "" {
		return s.Store, nil
	}
	return DefaultStore()
}

func (s *Service) httpClient() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (s *Service) csghubBaseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(s.CSGHubBaseURL), "/")
	if baseURL == "" {
		return DefaultCSGHubBaseURL
	}
	return baseURL
}

func (s *Service) opencsgBaseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(s.OpenCSGBaseURL), "/")
	if baseURL == "" {
		return DefaultOpenCSGBaseURL
	}
	return baseURL
}

func (s *Service) loginEnvironment(opts LoginOptions) (authEnvironment, error) {
	env := authEnvironment{
		OpenCSGBaseURL: s.opencsgBaseURL(),
		CSGHubBaseURL:  s.csghubBaseURL(),
	}
	hasOpenCSGBaseURL := false
	hasCSGHubBaseURL := false
	hasAIGatewayBaseURL := false
	if raw := strings.TrimSpace(opts.OpenCSGBaseURL); raw != "" {
		baseURL, err := normalizeAuthBaseURL(raw, "opencsg base url")
		if err != nil {
			return authEnvironment{}, err
		}
		env.OpenCSGBaseURL = baseURL
		hasOpenCSGBaseURL = true
	}
	if raw := strings.TrimSpace(opts.CSGHubBaseURL); raw != "" {
		baseURL, err := normalizeAuthBaseURL(raw, "csghub base url")
		if err != nil {
			return authEnvironment{}, err
		}
		env.CSGHubBaseURL = baseURL
		hasCSGHubBaseURL = true
	}
	if raw := strings.TrimSpace(opts.AIGatewayBaseURL); raw != "" {
		baseURL, err := normalizeAuthAIGatewayBaseURL(raw)
		if err != nil {
			return authEnvironment{}, err
		}
		env.AIGatewayBaseURL = baseURL
		hasAIGatewayBaseURL = true
	}
	if hasOpenCSGBaseURL {
		if !hasCSGHubBaseURL {
			env.CSGHubBaseURL = authCSGHubBaseURLForOpenCSGBaseURL(env.OpenCSGBaseURL)
		}
		if !hasAIGatewayBaseURL {
			env.AIGatewayBaseURL = authAIGatewayBaseURLForOpenCSGBaseURL(env.OpenCSGBaseURL)
		}
	}
	return env, nil
}

func (s *Service) callbackEnvironment(values url.Values) (authEnvironment, error) {
	env := authEnvironment{
		OpenCSGBaseURL: s.opencsgBaseURL(),
		CSGHubBaseURL:  s.csghubBaseURL(),
	}
	hasOpenCSGBaseURL := false
	hasCSGHubBaseURL := false
	if raw := strings.TrimSpace(values.Get("opencsg_base_url")); raw != "" {
		baseURL, err := normalizeAuthBaseURL(raw, "opencsg base url")
		if err != nil {
			return authEnvironment{}, err
		}
		env.OpenCSGBaseURL = baseURL
		hasOpenCSGBaseURL = true
	}
	if raw := strings.TrimSpace(values.Get("csghub_base_url")); raw != "" {
		baseURL, err := normalizeAuthBaseURL(raw, "csghub base url")
		if err != nil {
			return authEnvironment{}, err
		}
		env.CSGHubBaseURL = baseURL
		hasCSGHubBaseURL = true
	}
	rawAIGatewayBaseURL := strings.TrimSpace(values.Get("ai_gateway_base_url"))
	if rawAIGatewayBaseURL == "" {
		rawAIGatewayBaseURL = strings.TrimSpace(values.Get("aigateway_base_url"))
	}
	if raw := rawAIGatewayBaseURL; raw != "" {
		baseURL, err := normalizeAuthAIGatewayBaseURL(raw)
		if err != nil {
			return authEnvironment{}, err
		}
		env.AIGatewayBaseURL = baseURL
		if derivedBaseURL := authBaseURLFromAIGatewayBaseURL(baseURL); derivedBaseURL != "" {
			if !hasCSGHubBaseURL {
				env.CSGHubBaseURL = derivedBaseURL
			}
			if !hasOpenCSGBaseURL {
				env.OpenCSGBaseURL = derivedBaseURL
			}
		}
	}
	if hasOpenCSGBaseURL {
		if !hasCSGHubBaseURL {
			env.CSGHubBaseURL = authCSGHubBaseURLForOpenCSGBaseURL(env.OpenCSGBaseURL)
		}
		if env.AIGatewayBaseURL == "" {
			env.AIGatewayBaseURL = authAIGatewayBaseURLForOpenCSGBaseURL(env.OpenCSGBaseURL)
		}
	}
	return env, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func JWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("jwt token is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("jwt payload is invalid: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("jwt payload is invalid: %w", err)
	}
	if claims == nil {
		claims = map[string]any{}
	}
	return claims, nil
}

func stringClaim(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func portalRedirectURL(portalURL, jwtToken string) string {
	u, err := url.Parse(strings.TrimSpace(portalURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return portalURL
	}
	q := u.Query()
	q.Set("type", "ide")
	q.Set("jwt", jwtToken)
	u.RawQuery = q.Encode()
	return u.String()
}

func sanitizePortalURL(raw, baseURL string) string {
	portal, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(portal.Scheme, "https") || portal.Host == "" {
		return ""
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || !strings.EqualFold(base.Scheme, "https") || base.Host == "" {
		return ""
	}

	if !strings.EqualFold(portal.Hostname(), base.Hostname()) {
		return ""
	}
	return portal.String()
}

func joinAPIPath(baseURL, apiPath string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/")
	if err != nil {
		return nil, fmt.Errorf("parse csghub base url: %w", err)
	}
	ref, err := url.Parse(strings.TrimPrefix(apiPath, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse csghub api path: %w", err)
	}
	return base.ResolveReference(ref), nil
}

func sanitizeReturnURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	if isLocalHostname(u.Hostname()) {
		return u.String()
	}
	return ""
}

func sanitizeCallbackURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	if !isLocalHostname(u.Hostname()) {
		return ""
	}
	return u.String()
}

func callbackURLWithAuthState(callbackURL string, env authEnvironment, returnURL string) string {
	u, err := url.Parse(callbackURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	state := url.Values{}
	if returnURL != "" {
		state.Set("return_url", returnURL)
	}
	if env.OpenCSGBaseURL != "" {
		state.Set("opencsg_base_url", env.OpenCSGBaseURL)
	}
	if env.CSGHubBaseURL != "" {
		state.Set("csghub_base_url", env.CSGHubBaseURL)
	}
	if env.AIGatewayBaseURL != "" {
		state.Set("ai_gateway_base_url", env.AIGatewayBaseURL)
	}
	if encoded := state.Encode(); encoded != "" {
		q := u.Query()
		q.Set(callbackAuthStateParam, base64.RawURLEncoding.EncodeToString([]byte(encoded)))
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func callbackValuesWithAuthState(values url.Values) url.Values {
	raw := strings.TrimSpace(values.Get(callbackAuthStateParam))
	if raw == "" {
		return values
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return values
	}
	state, err := url.ParseQuery(string(decoded))
	if err != nil {
		return values
	}
	merged := cloneURLValues(state)
	for key, list := range values {
		merged[key] = append([]string(nil), list...)
	}
	return merged
}

func cloneURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, list := range values {
		cloned[key] = append([]string(nil), list...)
	}
	return cloned
}

func callbackReturnURL(values url.Values) string {
	for _, key := range []string{"return_url", "url"} {
		if returnURL := sanitizeReturnURL(values.Get(key)); returnURL != "" {
			return returnURL
		}
	}
	return ""
}

func normalizeAuthBaseURL(raw, name string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("%s must be an absolute http(s) URL", name)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s must use http or https", name)
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func normalizeAuthAIGatewayBaseURL(raw string) (string, error) {
	baseURL, err := normalizeAuthBaseURL(raw, "aigateway base url")
	if err != nil {
		return "", err
	}
	return normalizeAIGatewayBaseURL(baseURL), nil
}

func authBaseURLFromAIGatewayBaseURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		path = strings.TrimRight(strings.TrimSuffix(path, "/v1"), "/")
	}
	if path == "" && strings.HasPrefix(strings.ToLower(u.Host), "aigateway.") {
		u.Host = u.Host[len("aigateway."):]
		u.Path = ""
		u.RawPath = ""
		u.RawQuery = ""
		u.Fragment = ""
		return strings.TrimRight(u.String(), "/")
	}
	if !strings.HasSuffix(path, "/aigateway") {
		return ""
	}
	u.Path = strings.TrimRight(strings.TrimSuffix(path, "/aigateway"), "/")
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

func authCSGHubBaseURLForOpenCSGBaseURL(openCSGBaseURL string) string {
	switch normalizeKnownAuthBaseURL(openCSGBaseURL) {
	case DefaultOpenCSGBaseURL:
		return DefaultCSGHubBaseURL
	case StageOpenCSGBaseURL:
		return StageCSGHubBaseURL
	default:
		return strings.TrimRight(strings.TrimSpace(openCSGBaseURL), "/")
	}
}

func authAIGatewayBaseURLForOpenCSGBaseURL(openCSGBaseURL string) string {
	switch normalizeKnownAuthBaseURL(openCSGBaseURL) {
	case DefaultOpenCSGBaseURL:
		return DefaultAIGatewayBaseURL
	case StageOpenCSGBaseURL:
		return StageAIGatewayBaseURL
	default:
		baseURL := strings.TrimRight(strings.TrimSpace(openCSGBaseURL), "/")
		if baseURL == "" {
			return ""
		}
		return normalizeAIGatewayBaseURL(baseURL + customAIGatewayPathName)
	}
}

func normalizeKnownAuthBaseURL(raw string) string {
	baseURL, err := normalizeAuthBaseURL(raw, "auth base url")
	if err != nil {
		return strings.TrimRight(strings.TrimSpace(raw), "/")
	}
	return baseURL
}

func isLocalHostname(hostname string) bool {
	switch strings.ToLower(strings.Trim(hostname, "[]")) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

type callbackValidationError string

func (e callbackValidationError) Error() string {
	return string(e)
}

func isCallbackValidationError(err error) bool {
	_, ok := err.(callbackValidationError)
	return ok
}

func IsCallbackValidationError(err error) bool {
	return isCallbackValidationError(err)
}
