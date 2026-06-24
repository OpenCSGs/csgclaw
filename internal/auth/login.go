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
	DefaultOpenCSGBaseURL = "https://opencsg.com"
	DefaultCSGHubBaseURL  = "https://hub.opencsg.com"
)

type LoginResponse struct {
	LoginURL string `json:"login_url"`
}

type LoginOptions struct {
	ReturnURL   string
	CallbackURL string
}

type Service struct {
	Store          Store
	HTTPClient     *http.Client
	OpenCSGBaseURL string
	CSGHubBaseURL  string
	Now            func() time.Time
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
	if len(opts) > 0 {
		returnURL = sanitizeReturnURL(opts[0].ReturnURL)
		callbackURL = callbackURLWithReturnURL(sanitizeCallbackURL(opts[0].CallbackURL), returnURL)
	}
	if callbackURL == "" {
		return LoginResponse{}, fmt.Errorf("auth callback url is required")
	}
	return LoginResponse{LoginURL: s.buildLoginURL(callbackURL)}, nil
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

	csgHubBaseURL := s.csghubBaseURL()
	if portalURL != "" {
		portalURL = sanitizePortalURL(portalURL, csgHubBaseURL)
		if portalURL == "" {
			return "", callbackValidationError("portal_url must match csghub base url")
		}
	}
	accessToken, builtinAPIKey, avatar, err := s.fetchAuthDetails(ctx, csgHubBaseURL, jwtToken, userID, userUUID)
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
			UserID:     userID,
			UserUUID:   userUUID,
			Avatar:     avatar,
			BaseURL:    csgHubBaseURL,
			PortalURL:  portalURL,
			LoggedInAt: now,
		},
		LastRefresh: now,
	}
	if err := store.Save(record); err != nil {
		return "", err
	}
	if builtinAPIKey != "" {
		if err := store.SaveCSGHubProviderCredentials(CSGHubProviderCredentials{
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

func (s *Service) fetchAuthDetails(ctx context.Context, baseURL, jwtToken, userID, userUUID string) (string, string, string, error) {
	token, err := s.fetchUserAccessToken(ctx, baseURL, jwtToken, userID)
	if err != nil {
		return "", "", "", err
	}
	avatar, err := s.fetchUserAvatar(ctx, baseURL, userID)
	if err != nil {
		avatar = ""
	}
	apiKey, err := s.fetchBuiltinAPIKey(ctx, baseURL, jwtToken, userID, userUUID)
	if err != nil {
		apiKey = ""
	}
	return token, apiKey, avatar, nil
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

func (s *Service) fetchUserAvatar(ctx context.Context, baseURL, userID string) (string, error) {
	endpoint, err := joinAPIPath(baseURL, "/api/v1/user/"+url.PathEscape(userID))
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	var resp struct {
		Msg  string `json:"msg"`
		Data struct {
			Avatar string `json:"avatar"`
		} `json:"data"`
	}
	if err := s.doJSON(req, &resp); err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Data.Avatar), nil
}

func (s *Service) fetchBuiltinAPIKey(ctx context.Context, baseURL, authToken, userID, namespaceUUID string) (string, error) {
	return fetchBuiltinAPIKey(ctx, s.httpClient(), baseURL, authToken, userID, namespaceUUID)
}

func (s *Service) doJSON(req *http.Request, out any) error {
	return doJSONWithClient(s.httpClient(), req, out)
}

func (s *Service) buildLoginURL(callbackURL string) string {
	callbackURL = strings.TrimSpace(callbackURL)
	base := s.opencsgBaseURL()
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

func callbackURLWithReturnURL(callbackURL, returnURL string) string {
	if callbackURL == "" || returnURL == "" {
		return callbackURL
	}
	u, err := url.Parse(callbackURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	q := u.Query()
	q.Set("return_url", returnURL)
	u.RawQuery = q.Encode()
	return u.String()
}

func callbackReturnURL(values url.Values) string {
	for _, key := range []string{"return_url", "url"} {
		if returnURL := sanitizeReturnURL(values.Get(key)); returnURL != "" {
			return returnURL
		}
	}
	return ""
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
