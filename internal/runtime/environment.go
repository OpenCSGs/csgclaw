package runtime

import (
	"net/url"
	stdruntime "runtime"
	"slices"
	"strings"
)

// IsManagedAgentEnvironmentKey identifies connection settings projected by the
// Controller. They must not be inherited from the server process or shadowed by
// differently cased profile keys on Windows.
func IsManagedAgentEnvironmentKey(key string) bool {
	switch CanonicalEnvironmentKey(key) {
	case "CSGCLAW_BASE_URL", "CSGCLAW_ACCESS_TOKEN", "CSGCLAW_CONNECTOR_CAPABILITY", "CSGCLAW_CALLER_AGENT_ID", "CSGCLAW_CLI":
		return true
	default:
		return false
	}
}

// WithLocalProxyBypass keeps same-host callbacks out of outbound proxies used
// by the runtime process. Preserve configured exclusions when HTTP clients
// give different precedence to NO_PROXY and no_proxy.
func WithLocalProxyBypass(environment []string, callbackURL string) []string {
	hosts := []string{"127.0.0.1", "localhost", "::1", "[::1]"}
	if callback, err := url.Parse(callbackURL); err == nil && callback.Scheme == "http" {
		if host := strings.ToLower(callback.Hostname()); host != "" && !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
			if strings.Contains(host, ":") {
				// Node's fetch matches bracketed IPv6 hostnames.
				hosts = append(hosts, "["+host+"]")
			}
		}
	}

	var fallback string
	var hasLowercaseOverride bool
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !isProxyBypassEnvironmentKey(key) {
			continue
		}
		if fallback == "" {
			fallback = value
		}
		if key == "no_proxy" {
			hasLowercaseOverride = value != "" && strings.TrimSpace(value) != "*"
		}
	}

	hasBypass := false
	for i, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !isProxyBypassEnvironmentKey(key) {
			continue
		}
		hasBypass = true
		// A whitespace-only value still takes precedence in some HTTP clients.
		if value == "" {
			value = fallback
		}
		// Codex's HTTP client needs explicit IP entries even alongside an
		// uppercase wildcard. curl still uses the configured lowercase value.
		expandUppercaseWildcard := stdruntime.GOOS != "windows" && key == "NO_PROXY" && hasLowercaseOverride
		if strings.TrimSpace(value) != "*" || expandUppercaseWildcard {
			rules := strings.FieldsFunc(value, func(r rune) bool { return r == ',' })
			for _, host := range hosts {
				if !slices.ContainsFunc(rules, func(rule string) bool {
					return strings.EqualFold(strings.TrimSpace(rule), host)
				}) {
					rules = append(rules, host)
				}
			}
			value = strings.Join(rules, ",")
		}
		environment[i] = key + "=" + value
	}
	if !hasBypass {
		environment = append(environment, "NO_PROXY="+strings.Join(hosts, ","))
	}
	return environment
}

func isProxyBypassEnvironmentKey(key string) bool {
	return key == "NO_PROXY" || key == "no_proxy" ||
		(stdruntime.GOOS == "windows" && strings.EqualFold(key, "NO_PROXY"))
}
