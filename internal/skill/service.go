package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/config"
)

type Service struct {
	registries []registryEntry
}

func NewService(cfg config.SkillConfig, httpClient apiclient.HTTPClient) *Service {
	return &Service{registries: buildRegistries(cfg, httpClient)}
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if len(s.registries) == 0 {
		return nil, fmt.Errorf("no skill registries configured")
	}
	perRegistryLimit := limit
	if perRegistryLimit <= 0 {
		perRegistryLimit = 20
	}

	var errs []error
	for _, reg := range s.registries {
		items, err := reg.client.Search(ctx, query, perRegistryLimit)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", reg.id, err))
			continue
		}
		for i := range items {
			items[i].Registry = reg.id
		}
		if len(items) == 0 {
			continue
		}
		if limit > 0 && len(items) > limit {
			items = items[:limit]
		}
		return items, nil
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return []SearchResult{}, nil
}

func (s *Service) Get(ctx context.Context, slug string, registry RegistryID) (SkillDetail, error) {
	reg, detail, err := s.resolveSkill(ctx, slug, registry)
	if err != nil {
		return SkillDetail{}, err
	}
	return SkillDetail{Registry: reg.id, SkillGetResponse: detail}, nil
}

func (s *Service) ListVersions(ctx context.Context, slug string, registry RegistryID, limit int) (SkillVersionsList, error) {
	reg, detail, err := s.resolveSkill(ctx, slug, registry)
	if err != nil {
		return SkillVersionsList{}, err
	}
	versions, err := reg.client.ListVersions(ctx, slug, limit)
	if err != nil {
		return SkillVersionsList{}, err
	}
	return SkillVersionsList{
		Registry:    reg.id,
		Slug:        detail.Skill.Slug,
		DisplayName: detail.Skill.DisplayName,
		Versions:    versions,
	}, nil
}

func (s *Service) GetVersion(ctx context.Context, slug, version string, registry RegistryID) (SkillVersionDetail, error) {
	reg, err := s.resolveRegistry(ctx, slug, registry)
	if err != nil {
		return SkillVersionDetail{}, err
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return SkillVersionDetail{}, fmt.Errorf("skill version is required")
	}
	detail, err := reg.client.GetVersion(ctx, slug, version)
	if err != nil {
		return SkillVersionDetail{}, err
	}
	return SkillVersionDetail{Registry: reg.id, SkillVersionGetResponse: detail}, nil
}

func (s *Service) Install(ctx context.Context, slug, version string, registry RegistryID, skillsRoot string, force bool) (InstallResult, error) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return InstallResult{}, fmt.Errorf("skill slug is required")
	}
	if err := validateSkillSlug(slug); err != nil {
		return InstallResult{}, err
	}
	skillsRoot = strings.TrimSpace(skillsRoot)
	if skillsRoot == "" {
		return InstallResult{}, fmt.Errorf("skills directory is required")
	}
	skillsRoot = filepath.Clean(skillsRoot)

	reg, detail, err := s.resolveSkill(ctx, slug, registry)
	if err != nil {
		return InstallResult{}, err
	}
	installSlug, err := installSlugFromDetail(detail, slug)
	if err != nil {
		return InstallResult{}, err
	}
	if err := checkInstallable(detail.Moderation, reg.client.nonSuspiciousOnly); err != nil {
		return InstallResult{}, err
	}

	requestedVersion := strings.TrimSpace(version)
	var resolvedVersion string
	switch {
	case requestedVersion != "":
		if _, err := reg.client.GetVersion(ctx, installSlug, requestedVersion); err != nil {
			return InstallResult{}, fmt.Errorf("skill %q version %q (%s): %w", installSlug, requestedVersion, reg.id, err)
		}
		resolvedVersion = requestedVersion
	default:
		resolvedVersion = resolveInstallVersion(detail)
	}

	destDir := filepath.Join(skillsRoot, installSlug)
	if err := ensurePathInsideRoot(skillsRoot, destDir); err != nil {
		return InstallResult{}, err
	}
	if info, err := os.Stat(destDir); err == nil {
		if info.IsDir() && !force {
			return InstallResult{}, fmt.Errorf("%w: %s", ErrSkillDirExists, destDir)
		}
	} else if !os.IsNotExist(err) {
		return InstallResult{}, fmt.Errorf("stat skill dir %q: %w", destDir, err)
	}
	if force {
		if err := os.RemoveAll(destDir); err != nil {
			return InstallResult{}, fmt.Errorf("remove existing skill dir %q: %w", destDir, err)
		}
	}

	archive, err := reg.client.Download(ctx, installSlug, resolvedVersion, "")
	if err != nil && resolvedVersion == "" {
		return InstallResult{}, fmt.Errorf("skill %q has no installable version (%s)", installSlug, reg.id)
	}
	if err != nil {
		return InstallResult{}, err
	}
	sha256, err := extractSkillZip(archive, destDir, reg.client.maxArchive)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return InstallResult{}, err
	}
	if err := writeLockRecord(skillsRoot, newInstallRecord(reg.id, installSlug, resolvedVersion, sha256)); err != nil {
		_ = os.RemoveAll(destDir)
		return InstallResult{}, err
	}
	return InstallResult{
		Registry:  reg.id,
		Slug:      installSlug,
		Version:   resolvedVersion,
		SkillsDir: destDir,
	}, nil
}

func (s *Service) resolveSkill(ctx context.Context, slug string, registry RegistryID) (*registryEntry, SkillGetResponse, error) {
	if registry != "" {
		reg, err := s.registryByID(registry)
		if err != nil {
			return nil, SkillGetResponse{}, err
		}
		detail, err := reg.client.Get(ctx, slug)
		if err != nil {
			return nil, SkillGetResponse{}, err
		}
		return reg, detail, nil
	}
	var lastNotFound error
	for i := range s.registries {
		reg := &s.registries[i]
		detail, err := reg.client.Get(ctx, slug)
		if err == nil {
			return reg, detail, nil
		}
		if IsNotFound(err) {
			lastNotFound = err
			continue
		}
		return nil, SkillGetResponse{}, fmt.Errorf("%s: %w", reg.id, err)
	}
	if lastNotFound != nil {
		return nil, SkillGetResponse{}, lastNotFound
	}
	return nil, SkillGetResponse{}, ErrSkillNotFound
}

func (s *Service) resolveRegistry(ctx context.Context, slug string, registry RegistryID) (*registryEntry, error) {
	if registry != "" {
		return s.registryByID(registry)
	}
	reg, _, err := s.resolveSkill(ctx, slug, "")
	if err != nil {
		return nil, err
	}
	return reg, nil
}

func resolveInstallVersion(detail SkillGetResponse) string {
	if v := latestVersion(detail.LatestVersion); v != "" {
		return v
	}
	if v := latestVersion(detail.Skill.LatestVersion); v != "" {
		return v
	}
	if v := newestListedVersion(detail.Versions); v != "" {
		return v
	}
	if v := latestTagVersion(detail.Skill.Tags); v != "" {
		return v
	}
	return ""
}

func newestListedVersion(versions []SkillVersion) string {
	var best SkillVersion
	var found bool
	for _, item := range versions {
		ver := strings.TrimSpace(item.Version)
		if ver == "" {
			continue
		}
		if !found || item.CreatedAt >= best.CreatedAt {
			best = item
			found = true
		}
	}
	if !found {
		return ""
	}
	return strings.TrimSpace(best.Version)
}

func latestTagVersion(tags json.RawMessage) string {
	if len(tags) == 0 {
		return ""
	}
	var tagMap map[string]string
	if err := json.Unmarshal(tags, &tagMap); err == nil {
		if v := strings.TrimSpace(tagMap["latest"]); v != "" {
			return v
		}
	}
	return ""
}

func checkInstallable(moderation *SkillModeration, nonSuspiciousOnly bool) error {
	if moderation == nil {
		return nil
	}
	if moderation.IsMalwareBlocked {
		return fmt.Errorf("%w: malware blocked", ErrSkillBlocked)
	}
	if nonSuspiciousOnly && moderation.IsSuspicious {
		return fmt.Errorf("%w: flagged suspicious", ErrSkillBlocked)
	}
	return nil
}

func latestVersion(version *SkillVersion) string {
	if version == nil {
		return ""
	}
	return strings.TrimSpace(version.Version)
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrSkillNotFound)
}

func installSlugFromDetail(detail SkillGetResponse, requested string) (string, error) {
	canonical := normalizeSlug(detail.Skill.Slug)
	if canonical == "" {
		canonical = normalizeSlug(requested)
	}
	if err := validateSkillSlug(canonical); err != nil {
		return "", err
	}
	return canonical, nil
}

func validateSkillSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("skill slug is required")
	}
	if filepath.IsAbs(slug) {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, slug)
	}
	if strings.Contains(slug, "..") {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, slug)
	}
	if strings.ContainsAny(slug, `/\`) {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, slug)
	}
	return nil
}
