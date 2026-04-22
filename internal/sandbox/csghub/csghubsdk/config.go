package csghubsdk

import (
	"strings"
)

// Config pairs the two endpoint bases used by CSGHub Sandbox APIs.
//
// BaseURL points at the Hub/Starhub origin that hosts the lifecycle API
// (/api/v1/sandboxes). AIGatewayURL points at the AI Gateway that proxies the
// sandbox-runtime routes (/v1/sandboxes/...). When AIGatewayURL is empty, the
// client routes runtime calls to BaseURL (single-host deployments).
type Config struct {
	// BaseURL is the Hub / Starhub origin for /api/v1/sandboxes.
	BaseURL string
	// AIGatewayURL is the optional AI Gateway base for /v1/sandboxes/...
	// proxy routes. Empty string means reuse BaseURL.
	AIGatewayURL string
	// Token is the bearer token sent on every request (Authorization header).
	Token string
}

// apiSandboxesRoot returns the collection URL for lifecycle APIs.
func (c Config) apiSandboxesRoot() string {
	return strings.TrimRight(c.BaseURL, "/") + "/api/v1/sandboxes"
}

// aigatewayBase returns the base for sandbox-runtime routes, falling back to
// BaseURL when AIGatewayURL is empty.
func (c Config) aigatewayBase() string {
	if gw := strings.TrimSpace(c.AIGatewayURL); gw != "" {
		return strings.TrimRight(gw, "/")
	}
	return strings.TrimRight(c.BaseURL, "/")
}
