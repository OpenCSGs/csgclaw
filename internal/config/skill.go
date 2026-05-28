package config

import (
	"os"
	"strings"
)

// DefaultSkillBaseURL is the primary OpenCSG skill registry.
// Override via [skill].base_url or SKILL_BASE_URL (CLAWHUB_BASE_URL is legacy).
const DefaultSkillBaseURL = "https://claw.opencsg.com"

// DefaultSkillOfficialBaseURL is the public clawhub.ai registry.
// Override via [skill].official_base_url or SKILL_OFFICIAL_BASE_URL (CLAWHUB_OFFICIAL_BASE_URL is legacy).
const DefaultSkillOfficialBaseURL = "https://clawhub.ai"

type SkillConfig struct {
	BaseURL            string
	OfficialBaseURL    string
	Token              string
	NonSuspiciousOnly  bool
	OfficialBaseURLSet bool
}

type rawSkillConfig struct {
	BaseURL              string
	OfficialBaseURL      string
	Token                string
	NonSuspiciousOnly    bool
	NonSuspiciousOnlySet bool
	OfficialBaseURLSet   bool
}

func (c SkillConfig) Resolved() SkillConfig {
	out := c
	if u := strings.TrimSpace(out.BaseURL); u != "" {
		out.BaseURL = strings.TrimRight(u, "/")
	} else if u := skillEnvFirst("SKILL_BASE_URL", "CLAWHUB_BASE_URL"); u != "" {
		out.BaseURL = strings.TrimRight(u, "/")
	} else {
		out.BaseURL = DefaultSkillBaseURL
	}
	if strings.TrimSpace(out.Token) == "" {
		out.Token = skillEnvFirst("SKILL_TOKEN", "CLAWHUB_TOKEN")
	}

	if c.OfficialBaseURLSet {
		if u := strings.TrimSpace(c.OfficialBaseURL); u != "" {
			out.OfficialBaseURL = strings.TrimRight(u, "/")
		} else {
			out.OfficialBaseURL = ""
		}
	} else if u := skillEnvFirst("SKILL_OFFICIAL_BASE_URL", "CLAWHUB_OFFICIAL_BASE_URL"); u != "" {
		out.OfficialBaseURL = strings.TrimRight(u, "/")
	} else {
		out.OfficialBaseURL = DefaultSkillOfficialBaseURL
	}
	if out.OfficialBaseURL != "" && skillRegistryURLsEqual(out.OfficialBaseURL, out.BaseURL) {
		out.OfficialBaseURL = ""
	}
	return out
}

func skillEnvFirst(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func skillRegistryURLsEqual(a, b string) bool {
	return strings.TrimRight(strings.TrimSpace(a), "/") == strings.TrimRight(strings.TrimSpace(b), "/")
}
