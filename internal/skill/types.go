package skill

import (
	"encoding/json"
	"time"
)

type SearchResult struct {
	Registry    RegistryID `json:"registry"`
	Score       float64    `json:"score"`
	Slug        string     `json:"slug"`
	DisplayName string     `json:"displayName"`
	Summary     string     `json:"summary"`
	Version     string     `json:"version"`
	UpdatedAt   int64      `json:"updatedAt"`
	OwnerHandle string     `json:"ownerHandle"`
}

type SkillDetail struct {
	Registry RegistryID `json:"registry"`
	SkillGetResponse
}

type SkillVersionDetail struct {
	Registry RegistryID `json:"registry"`
	SkillVersionGetResponse
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

type SkillVersion struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	Changelog string `json:"changelog"`
	CreatedAt int64  `json:"createdAt"`
}

type SkillSummary struct {
	Slug          string          `json:"slug"`
	DisplayName   string          `json:"displayName"`
	Summary       string          `json:"summary"`
	Tags          json.RawMessage `json:"tags,omitempty"`
	UpdatedAt     int64           `json:"updatedAt"`
	LatestVersion *SkillVersion   `json:"latestVersion"`
}

type SkillModeration struct {
	IsSuspicious     bool   `json:"isSuspicious"`
	IsMalwareBlocked bool   `json:"isMalwareBlocked"`
	Verdict          string `json:"verdict"`
}

type SkillGetResponse struct {
	Skill         SkillSummary     `json:"skill"`
	LatestVersion *SkillVersion    `json:"latestVersion"`
	Versions      []SkillVersion   `json:"versions"`
	Moderation    *SkillModeration `json:"moderation"`
}

type SkillVersionGetResponse struct {
	Skill   SkillSummary `json:"skill"`
	Version SkillVersion `json:"version"`
}

type InstallRecord struct {
	Registry    RegistryID `json:"registry,omitempty"`
	Slug        string     `json:"slug"`
	Version     string     `json:"version"`
	InstalledAt time.Time  `json:"installed_at"`
	SHA256      string     `json:"sha256,omitempty"`
}

type InstallResult struct {
	Registry  RegistryID
	Slug      string
	Version   string
	SkillsDir string
}
