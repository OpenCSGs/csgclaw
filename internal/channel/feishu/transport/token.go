package transport

import (
	"context"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type tenantTokenSource interface {
	Token(context.Context) (string, error)
}

type invalidatableTenantTokenSource interface {
	tenantTokenSource
	Invalidate(string)
}

type tenantTokenFetchFunc func(context.Context) (string, time.Duration, error)

func loadTenantToken(ctx context.Context, source tenantTokenSource) (string, error) {
	if ctx == nil {
		return "", errNilContext
	}
	if source == nil {
		return "", ErrInvalidConfig
	}
	token, err := source.Token(ctx)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", missingAPIResult("get tenant access token", "tenant_access_token", nil)
	}
	return token, nil
}

type cachedTenantTokenSource struct {
	mu         sync.Mutex
	fetch      tenantTokenFetchFunc
	now        func() time.Time
	token      string
	validUntil time.Time
}

func newCachedTenantTokenSource(client *lark.Client, appID, appSecret string) tenantTokenSource {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	return &cachedTenantTokenSource{
		now: time.Now,
		fetch: func(ctx context.Context) (string, time.Duration, error) {
			if client == nil || appID == "" || appSecret == "" {
				return "", 0, ErrInvalidConfig
			}
			resp, err := client.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
				AppID: appID, AppSecret: appSecret,
			})
			if err != nil {
				return "", 0, requestAPIError("get tenant access token", err)
			}
			if resp == nil {
				return "", 0, missingAPIResponse("get tenant access token")
			}
			if apiResponseFailed(resp.Success(), resp.ApiResp) {
				return "", 0, responseAPIError("get tenant access token", resp.Code, resp.Msg, resp.ApiResp)
			}
			token := strings.TrimSpace(resp.TenantAccessToken)
			if token == "" {
				return "", 0, missingAPIResult("get tenant access token", "tenant_access_token", resp.ApiResp)
			}
			return token, time.Duration(resp.Expire) * time.Second, nil
		},
	}
}

func (s *cachedTenantTokenSource) Token(ctx context.Context) (string, error) {
	if s == nil || s.fetch == nil {
		return "", ErrInvalidConfig
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	if s.token != "" && now.Before(s.validUntil) {
		return s.token, nil
	}
	token, ttl, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", missingAPIResult("get tenant access token", "tenant_access_token", nil)
	}
	refreshMargin := min(time.Minute, ttl/10)
	s.token = token
	s.validUntil = now.Add(ttl - refreshMargin)
	return token, nil
}

// Invalidate forgets token only when it still matches the rejected value. A
// stale request cannot erase a newer token refreshed by another goroutine.
func (s *cachedTenantTokenSource) Invalidate(token string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == strings.TrimSpace(token) {
		s.token = ""
		s.validUntil = time.Time{}
	}
}

var _ invalidatableTenantTokenSource = (*cachedTenantTokenSource)(nil)

// TenantTokenSource is a private, refreshable application credential source.
// Each caller owns its cache; it must not be shared across App installations.
type TenantTokenSource interface {
	Token(context.Context) (string, error)
	Invalidate(string)
}

// NewTenantTokenSource obtains application tokens from Feishu, never from the
// configured MCP endpoint. It uses the same cache and expiry handling as IM.
func NewTenantTokenSource(appID, appSecret string) TenantTokenSource {
	client := lark.NewClient(appID, appSecret,
		lark.WithSource(larkTransportSource), lark.WithEnableTokenCache(false),
		lark.WithHttpClient(newSingleAttemptHTTPClient()), lark.WithLogger(quietTenantTokenLogger{}))
	return newCachedTenantTokenSource(client, appID, appSecret).(TenantTokenSource)
}

// Token exchange payloads must never be emitted by the SDK logger.
type quietTenantTokenLogger struct{}

func (quietTenantTokenLogger) Debug(context.Context, ...interface{}) {}
func (quietTenantTokenLogger) Info(context.Context, ...interface{})  {}
func (quietTenantTokenLogger) Warn(context.Context, ...interface{})  {}
func (quietTenantTokenLogger) Error(context.Context, ...interface{}) {}
