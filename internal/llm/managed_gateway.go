package llm

import (
	"context"
	"csgclaw/internal/auth"
	"csgclaw/internal/config"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CSGHubGatewayConfig is supplied by the platform for the image's remote provider.
type CSGHubGatewayConfig struct {
	GatewayURL string
	HubURL     string
	UserToken  string
	UserName   string
	UserUUID   string
}

type managedGateway struct {
	config CSGHubGatewayConfig
	mu     sync.Mutex
	apiKey string
}

func NewCSGHubService(defaults config.ModelConfig, agents AgentProfileReader, cfg CSGHubGatewayConfig) *Service {
	service := NewService(defaults, agents)
	cfg.GatewayURL = strings.TrimRight(strings.TrimSpace(cfg.GatewayURL), "/")
	cfg.HubURL = strings.TrimRight(strings.TrimSpace(cfg.HubURL), "/")
	cfg.UserToken = strings.TrimSpace(cfg.UserToken)
	cfg.UserName = strings.TrimSpace(cfg.UserName)
	cfg.UserUUID = strings.TrimSpace(cfg.UserUUID)
	// Older images and deployments can still supply a static provider alone.
	if validPlatformURL(cfg.GatewayURL) && validPlatformURL(cfg.HubURL) &&
		cfg.UserToken != "" && cfg.UserName != "" && cfg.UserUUID != "" {
		service.managedGateway = &managedGateway{config: cfg}
	}
	return service
}

func validPlatformURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" &&
		u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func (g *managedGateway) matches(req *http.Request) bool {
	if g == nil {
		return false
	}
	meta, _ := req.Context().Value(modelRequestContextKey{}).(modelRequestContext)
	if meta.provider != "remote" {
		return false
	}
	switch req.URL.String() {
	case g.config.GatewayURL + "/responses", g.config.GatewayURL + "/chat/completions":
		return true
	default:
		return false
	}
}

func (g *managedGateway) key(ctx context.Context, client *http.Client, rejected string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.apiKey != "" && g.apiKey != rejected {
		return g.apiKey, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cfg := g.config
	key, err := auth.FetchBuiltinAPIKey(ctx, client, cfg.HubURL, cfg.UserToken, cfg.UserName, cfg.UserUUID)
	if err != nil {
		// The Hub's error may contain credential material.
		return "", errors.New("could not retrieve the AI Gateway key using the sandbox's CSGHub user token")
	}
	g.apiKey = key
	return key, nil
}

func (s *Service) managedGatewayRequest(req *http.Request, stream bool) (*http.Response, error) {
	ctx := req.Context()
	client := *s.client
	// Both credentials are scoped to the platform endpoints above.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	key, err := s.managedGateway.key(ctx, &client, "")
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	exchange, _ := ctx.Value(modelExchangeKey{}).(*modelExchange)
	if exchange != nil {
		exchange.recorder.AddSecret(key)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := s.modelRequestWithRecovery(&client, req, stream)
	if err != nil || response.StatusCode != http.StatusUnauthorized || ctx.Err() != nil ||
		(req.Body != nil && req.GetBody == nil) {
		return response, err
	}

	refreshed, refreshErr := s.managedGateway.key(ctx, &client, key)
	if refreshErr != nil {
		slog.WarnContext(ctx, "refresh managed AI Gateway key after unauthorized response", "error", refreshErr)
		return response, nil
	}
	if refreshed == key {
		return response, nil
	}
	retry := req.Clone(ctx)
	if req.GetBody != nil {
		retry.Body, err = req.GetBody()
		if err != nil {
			_ = response.Body.Close()
			return nil, err
		}
	}
	_ = response.Body.Close()
	if exchange != nil {
		exchange.recorder.AddSecret(refreshed)
	}
	retry.Header.Set("Authorization", "Bearer "+refreshed)
	return s.modelRequestWithRecovery(&client, retry, stream)
}
