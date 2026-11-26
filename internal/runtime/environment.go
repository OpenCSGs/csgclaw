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
// by the runtime process. Preserve existing exclusions and their casing, as
// HTTP clients can give different precedence to NO_PROXY and no_proxy.
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

	hasBypass := false
	for i, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || (key != "NO_PROXY" && key != "no_proxy" &&
			(stdruntime.GOOS != "windows" || !strings.EqualFold(key, "NO_PROXY"))) {
			continue
		}
		hasBypass = true
		if strings.TrimSpace(value) == "*" {
			continue
		}
		rules := strings.FieldsFunc(value, func(r rune) bool { return r == ',' })
		for _, host := range hosts {
			if !slices.ContainsFunc(rules, func(rule string) bool {
				return strings.EqualFold(strings.TrimSpace(rule), host)
			}) {
				rules = append(rules, host)
			}
		}
		environment[i] = key + "=" + strings.Join(rules, ",")
	}
	if !hasBypass {
		environment = append(environment, "NO_PROXY="+strings.Join(hosts, ","))
	}
	return environment
}
