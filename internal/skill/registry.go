package skill

import (
	"fmt"
	"strings"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/config"
)

// Registry identifiers used in CLI output, lockfiles, and --registry flags.
const (
	RegistryOpenCSG RegistryID = "opencsg"
	RegistryClawHub RegistryID = "clawhub"
)

type RegistryID string

type registryEntry struct {
	id     RegistryID
	client *Client
}

func ParseRegistry(value string) (RegistryID, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case string(RegistryOpenCSG), "open-csg":
		return RegistryOpenCSG, nil
	case string(RegistryClawHub), "official":
		return RegistryClawHub, nil
	default:
		return "", fmt.Errorf("unknown registry %q (use opencsg or clawhub)", value)
	}
}

func (id RegistryID) String() string {
	return string(id)
}

func buildRegistries(cfg config.SkillConfig, httpClient apiclient.HTTPClient) []registryEntry {
	resolved := cfg.Resolved()
	var entries []registryEntry

	primaryURL := strings.TrimSpace(resolved.BaseURL)
	if primaryURL != "" {
		entries = append(entries, registryEntry{
			id: RegistryOpenCSG,
			client: NewClient(config.SkillConfig{
				BaseURL:           primaryURL,
				Token:             resolved.Token,
				NonSuspiciousOnly: resolved.NonSuspiciousOnly,
			}, httpClient),
		})
	}

	officialURL := strings.TrimSpace(resolved.OfficialBaseURL)
	if officialURL != "" && !registryURLsEqual(officialURL, primaryURL) {
		entries = append(entries, registryEntry{
			id: RegistryClawHub,
			client: NewClient(config.SkillConfig{
				BaseURL:           officialURL,
				Token:             resolved.Token,
				NonSuspiciousOnly: resolved.NonSuspiciousOnly,
			}, httpClient),
		})
	}
	return entries
}

func registryURLsEqual(a, b string) bool {
	return strings.TrimRight(strings.TrimSpace(a), "/") == strings.TrimRight(strings.TrimSpace(b), "/")
}

func (s *Service) registryByID(id RegistryID) (*registryEntry, error) {
	for i := range s.registries {
		if s.registries[i].id == id {
			return &s.registries[i], nil
		}
	}
	return nil, fmt.Errorf("registry %q is not configured", id)
}

func mergeSearchResults(batches [][]SearchResult) []SearchResult {
	seen := make(map[string]struct{})
	var merged []SearchResult
	for _, batch := range batches {
		for _, item := range batch {
			slug := strings.TrimSpace(item.Slug)
			if slug == "" {
				continue
			}
			if _, ok := seen[slug]; ok {
				continue
			}
			seen[slug] = struct{}{}
			merged = append(merged, item)
		}
	}
	return merged
}
