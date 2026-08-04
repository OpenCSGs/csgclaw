package modelprovider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	CSGHubLiteProviderName      = "csghub-lite"
	CSGHubLiteDefaultBaseURL    = "http://127.0.0.1:11435/v1"
	CSGHubLiteDesktopAPIBaseURL = "http://127.0.0.1:11436/v1"
	CSGHubLiteDefaultAPIKey     = "local"
)

// ModelDiscoveryResult keeps the discovered models together with the endpoint
// that served them. Callers can persist ResolvedBaseURL so later inference uses
// the same reachable endpoint as model discovery.
type ModelDiscoveryResult struct {
	ResolvedBaseURL string
	Models          []string
}

// ListCSGHubLiteModels lists models using the default discovery client.
func ListCSGHubLiteModels(
	ctx context.Context,
	baseURL string,
	apiKey string,
	headers map[string]string,
) (ModelDiscoveryResult, error) {
	return ListCSGHubLiteModelsWithClient(
		ctx,
		&http.Client{Timeout: 3 * time.Second},
		baseURL,
		apiKey,
		headers,
	)
}

// ListCSGHubLiteModelsWithClient tries the configured endpoint first. The
// two built-in local endpoints form an ordered fallback chain because CSGHub Lite
// CLI and Desktop expose the same API on different ports. Explicit custom
// endpoints are never redirected to a local port.
func ListCSGHubLiteModelsWithClient(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	apiKey string,
	headers map[string]string,
) (ModelDiscoveryResult, error) {
	baseURL = normalizeBaseURL(baseURL)
	if baseURL == "" {
		baseURL = CSGHubLiteDefaultBaseURL
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		apiKey = CSGHubLiteDefaultAPIKey
	}

	candidates := csgHubLiteEndpointCandidates(baseURL)
	var lastErr error
	for _, candidate := range candidates {
		models, err := ListOpenAIModelsWithClient(ctx, client, candidate, apiKey, headers)
		if err == nil {
			return ModelDiscoveryResult{
				ResolvedBaseURL: candidate,
				Models:          models,
			}, nil
		}
		lastErr = err
	}
	if len(candidates) > 1 {
		return ModelDiscoveryResult{}, fmt.Errorf(
			"CSGHub Lite is unavailable at %s: %w",
			strings.Join(candidates, " or "),
			lastErr,
		)
	}
	return ModelDiscoveryResult{}, lastErr
}

func csgHubLiteEndpointCandidates(baseURL string) []string {
	baseURL = normalizeBaseURL(baseURL)
	defaultBaseURL := normalizeBaseURL(CSGHubLiteDefaultBaseURL)
	desktopBaseURL := normalizeBaseURL(CSGHubLiteDesktopAPIBaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	switch baseURL {
	case defaultBaseURL:
		return []string{defaultBaseURL, desktopBaseURL}
	case desktopBaseURL:
		return []string{desktopBaseURL, defaultBaseURL}
	default:
		return []string{baseURL}
	}
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}
