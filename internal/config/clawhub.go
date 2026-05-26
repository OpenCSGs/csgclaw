package config

import (
	"os"
	"strings"
)

// DefaultClawHubBaseURL is the primary OpenCSG skill registry.
// Override via [clawhub].base_url or CLAWHUB_BASE_URL.
const DefaultClawHubBaseURL = "https://claw.opencsg.com"

// DefaultClawHubOfficialBaseURL is the public OpenClaw ClawHub registry (clawhub.ai).
// Override via [clawhub].official_base_url or CLAWHUB_OFFICIAL_BASE_URL.
const DefaultClawHubOfficialBaseURL = "https://clawhub.ai"

type ClawHubConfig struct {
	BaseURL            string
	OfficialBaseURL    string
	Token              string
	NonSuspiciousOnly  bool
	OfficialBaseURLSet bool
}

type rawClawHubConfig struct {
	BaseURL              string
	OfficialBaseURL      string
	Token                string
	NonSuspiciousOnly    bool
	NonSuspiciousOnlySet bool
	OfficialBaseURLSet   bool
}

func (c ClawHubConfig) Resolved() ClawHubConfig {
	out := c
	if u := strings.TrimSpace(out.BaseURL); u != "" {
		out.BaseURL = strings.TrimRight(u, "/")
	} else if u := strings.TrimSpace(os.Getenv("CLAWHUB_BASE_URL")); u != "" {
		out.BaseURL = strings.TrimRight(u, "/")
	} else {
		out.BaseURL = DefaultClawHubBaseURL
	}
	if strings.TrimSpace(out.Token) == "" {
		out.Token = strings.TrimSpace(os.Getenv("CLAWHUB_TOKEN"))
	}

	if c.OfficialBaseURLSet {
		if u := strings.TrimSpace(c.OfficialBaseURL); u != "" {
			out.OfficialBaseURL = strings.TrimRight(u, "/")
		} else {
			out.OfficialBaseURL = ""
		}
	} else if u := strings.TrimSpace(os.Getenv("CLAWHUB_OFFICIAL_BASE_URL")); u != "" {
		out.OfficialBaseURL = strings.TrimRight(u, "/")
	} else {
		out.OfficialBaseURL = DefaultClawHubOfficialBaseURL
	}
	if out.OfficialBaseURL != "" && registryURLsEqual(out.OfficialBaseURL, out.BaseURL) {
		out.OfficialBaseURL = ""
	}
	return out
}

func registryURLsEqual(a, b string) bool {
	return strings.TrimRight(strings.TrimSpace(a), "/") == strings.TrimRight(strings.TrimSpace(b), "/")
}
