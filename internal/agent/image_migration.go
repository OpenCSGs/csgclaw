package agent

import (
	"context"
	"strconv"
	"strings"

	"csgclaw/internal/hub"
)

func (s *Service) withRuntimeImageMigrationStatus(ctx context.Context, a Agent) Agent {
	a = *cloneAgent(&a)
	if _, ok := s.imageUpgradeCandidate(ctx, a); !ok {
		return a
	}
	a.AgentProfile.ImageUpgradeRequired = true
	return a
}

type defaultAgentImage struct {
	image   string
	version string
}

func (s *Service) imageForRecreate(ctx context.Context, a Agent) string {
	current := strings.TrimSpace(a.Image)
	if latestImage, ok := s.imageUpgradeCandidate(ctx, a); ok {
		return latestImage
	}
	if isGatewayRuntimeKind(strings.TrimSpace(a.RuntimeKind)) && current == "" {
		s.mu.RLock()
		managerImage := strings.TrimSpace(s.managerImage)
		s.mu.RUnlock()
		return managerImage
	}
	return current
}

func (s *Service) imageForUpgrade(ctx context.Context, a Agent) (string, bool) {
	candidate, ok := s.currentDefaultImageForAgent(ctx, a)
	image := strings.TrimSpace(candidate.image)
	if !ok || image == "" || isDevImageTag(dockerImageTag(image)) {
		return "", false
	}
	return image, true
}

func (s *Service) imageUpgradeCandidate(ctx context.Context, a Agent) (string, bool) {
	candidate, ok := s.currentDefaultImageForAgent(ctx, a)
	if !ok {
		return "", false
	}
	if imageNeedsTemplateVersionUpgrade(a.Image, candidate) {
		return strings.TrimSpace(candidate.image), true
	}
	return "", false
}

func (s *Service) localImageCandidates(ctx context.Context) []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	provider := s.sandbox
	s.mu.RUnlock()
	if provider == nil {
		return nil
	}
	homeDir, err := SandboxRuntimeHome(ManagerName)
	if err != nil {
		return nil
	}
	images, err := provider.ListImages(ctx, homeDir)
	if err != nil {
		return nil
	}
	return images
}

func (s *Service) currentDefaultImageForAgent(ctx context.Context, a Agent) (defaultAgentImage, bool) {
	if s == nil || !isGatewayRuntimeKind(strings.TrimSpace(a.RuntimeKind)) {
		return defaultAgentImage{}, false
	}
	role := normalizeRole(a.Role)
	if isManagerAgent(a) {
		role = RoleManager
	}
	if role != RoleManager && role != RoleWorker {
		return defaultAgentImage{}, false
	}

	if role == RoleManager {
		return s.defaultManagerImageForRuntime(ctx, a.RuntimeKind)
	}

	s.mu.RLock()
	hubSvc := s.hub
	workerTemplate := strings.TrimSpace(s.defaultWorkerTemplate)
	s.mu.RUnlock()

	if workerTemplate != "" && hubSvc != nil {
		item, err := hubSvc.Get(ctx, workerTemplate)
		if err == nil && defaultTemplateMatchesAgent(item.Role, item.RuntimeKind, role, a.RuntimeKind) {
			if image := strings.TrimSpace(item.Image); image != "" {
				return defaultAgentImage{
					image:   image,
					version: strings.TrimSpace(item.Version),
				}, true
			}
		}
	}

	return defaultAgentImage{}, false
}

func (s *Service) defaultManagerImageForRuntime(ctx context.Context, runtimeKind string) (defaultAgentImage, bool) {
	if s == nil {
		return defaultAgentImage{}, false
	}
	runtimeKind = strings.TrimSpace(runtimeKind)

	s.mu.RLock()
	hubSvc := s.hub
	managerTemplate := strings.TrimSpace(s.defaultManagerTemplate)
	managerImage := strings.TrimSpace(s.managerImage)
	gatewayRuntime := s.gatewayRuntimeKind()
	s.mu.RUnlock()

	if runtimeKind == "" {
		runtimeKind = gatewayRuntime
	}
	if managerTemplate != "" && hubSvc != nil {
		item, err := hubSvc.Get(ctx, managerTemplate)
		if err == nil {
			if candidate, ok := managerTemplateImageForRuntime(item, runtimeKind); ok {
				return candidate, true
			}
		}
	}
	if hubSvc != nil {
		if candidate, ok := managerTemplateImageFromList(ctx, hubSvc, runtimeKind, true); ok {
			return candidate, true
		}
		if candidate, ok := managerTemplateImageFromList(ctx, hubSvc, runtimeKind, false); ok {
			return candidate, true
		}
	}
	if managerImage != "" && (runtimeKind == "" || runtimeKind == gatewayRuntime) {
		return defaultAgentImage{image: managerImage}, true
	}
	return defaultAgentImage{}, false
}

func managerTemplateImageFromList(ctx context.Context, hubSvc templateService, runtimeKind string, builtinOnly bool) (defaultAgentImage, bool) {
	if hubSvc == nil {
		return defaultAgentImage{}, false
	}
	items, err := hubSvc.List(ctx)
	if err != nil {
		return defaultAgentImage{}, false
	}
	for _, item := range items {
		if builtinOnly && strings.TrimSpace(item.Source.Kind) != hub.RegistryKindBuiltin {
			continue
		}
		if candidate, ok := managerTemplateImageForRuntime(item, runtimeKind); ok {
			return candidate, true
		}
	}
	return defaultAgentImage{}, false
}

func managerTemplateImageForRuntime(item hub.Template, runtimeKind string) (defaultAgentImage, bool) {
	if normalizeRole(item.Role) != RoleManager {
		return defaultAgentImage{}, false
	}
	templateRuntimeKind := strings.TrimSpace(item.RuntimeKind)
	if runtimeKind != "" && templateRuntimeKind != runtimeKind {
		return defaultAgentImage{}, false
	}
	image := strings.TrimSpace(item.Image)
	if image == "" {
		return defaultAgentImage{}, false
	}
	return defaultAgentImage{
		image:   image,
		version: strings.TrimSpace(item.Version),
	}, true
}

func defaultTemplateMatchesAgent(templateRole, templateRuntimeKind, agentRole, agentRuntimeKind string) bool {
	if normalizeRole(templateRole) != normalizeRole(agentRole) {
		return false
	}
	templateRuntimeKind = strings.TrimSpace(templateRuntimeKind)
	if templateRuntimeKind == "" {
		return true
	}
	return templateRuntimeKind == strings.TrimSpace(agentRuntimeKind)
}

func imageNeedsTemplateVersionUpgrade(current string, latest defaultAgentImage) bool {
	current = strings.TrimSpace(current)
	latestImage := strings.TrimSpace(latest.image)
	latestVersion := strings.TrimSpace(latest.version)
	if latestImage == "" || current == latestImage {
		return false
	}
	if current == "" {
		return true
	}
	currentRepo := dockerImageRepository(current)
	latestRepo := dockerImageRepository(latestImage)
	if currentRepo == "" || latestRepo == "" {
		return false
	}
	currentTag := dockerImageTag(current)
	if isDevImageTag(currentTag) {
		return false
	}
	if !strings.EqualFold(currentRepo, latestRepo) {
		if !isLegacyPicoClawRepositoryUpgrade(currentRepo, latestRepo) {
			return false
		}
		_, ok := parseSemanticVersion(latestVersion)
		return ok
	}
	if latestVersion == "" {
		return false
	}
	if cmp, ok := compareSemanticVersions(currentTag, latestVersion); ok {
		return cmp < 0
	}
	return true
}

func isLegacyPicoClawRepositoryUpgrade(currentRepo, latestRepo string) bool {
	if !strings.EqualFold(dockerImageRepositoryName(currentRepo), "picoclaw") {
		return false
	}
	latestName := dockerImageRepositoryName(latestRepo)
	if !strings.EqualFold(latestName, "picoclaw-manager") && !strings.EqualFold(latestName, "picoclaw-worker") {
		return false
	}
	return strings.EqualFold(dockerImageRepositoryParent(currentRepo), dockerImageRepositoryParent(latestRepo))
}

func dockerImageRepositoryName(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		return strings.TrimSpace(repo[idx+1:])
	}
	return repo
}

func dockerImageRepositoryParent(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		return strings.TrimSpace(repo[:idx])
	}
	return ""
}

func isDevImageTag(tag string) bool {
	return strings.EqualFold(strings.TrimSpace(tag), "dev")
}

func dockerImageRepository(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if beforeDigest, _, ok := strings.Cut(ref, "@"); ok {
		ref = beforeDigest
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		ref = ref[:lastColon]
	}
	return strings.TrimSpace(ref)
}

func dockerImageTag(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if beforeDigest, _, ok := strings.Cut(ref, "@"); ok {
		ref = beforeDigest
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		return strings.TrimSpace(ref[lastColon+1:])
	}
	return ""
}

type semanticVersion struct {
	major int
	minor int
	patch int
	pre   []string
}

func compareSemanticVersions(current, latest string) (int, bool) {
	currentVersion, ok := parseSemanticVersion(current)
	if !ok {
		return 0, false
	}
	latestVersion, ok := parseSemanticVersion(latest)
	if !ok {
		return 0, false
	}
	if currentVersion.major != latestVersion.major {
		return compareInts(currentVersion.major, latestVersion.major), true
	}
	if currentVersion.minor != latestVersion.minor {
		return compareInts(currentVersion.minor, latestVersion.minor), true
	}
	if currentVersion.patch != latestVersion.patch {
		return compareInts(currentVersion.patch, latestVersion.patch), true
	}
	return comparePrerelease(currentVersion.pre, latestVersion.pre), true
}

func parseSemanticVersion(value string) (semanticVersion, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if value == "" {
		return semanticVersion{}, false
	}
	if beforeBuild, _, ok := strings.Cut(value, "+"); ok {
		value = beforeBuild
	}
	core := value
	var pre []string
	if beforePre, afterPre, ok := strings.Cut(value, "-"); ok {
		core = beforePre
		if afterPre == "" {
			return semanticVersion{}, false
		}
		pre = strings.Split(afterPre, ".")
		for _, item := range pre {
			if item == "" {
				return semanticVersion{}, false
			}
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	major, ok := parseSemanticVersionNumber(parts[0])
	if !ok {
		return semanticVersion{}, false
	}
	minor, ok := parseSemanticVersionNumber(parts[1])
	if !ok {
		return semanticVersion{}, false
	}
	patch, ok := parseSemanticVersionNumber(parts[2])
	if !ok {
		return semanticVersion{}, false
	}
	return semanticVersion{major: major, minor: minor, patch: patch, pre: pre}, true
}

func parseSemanticVersionNumber(value string) (int, bool) {
	if value == "" || (len(value) > 1 && strings.HasPrefix(value, "0")) {
		return 0, false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}
	part, err := strconv.Atoi(value)
	return part, err == nil
}

func compareInts(left, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func comparePrerelease(current, latest []string) int {
	if len(current) == 0 && len(latest) == 0 {
		return 0
	}
	if len(current) == 0 {
		return 1
	}
	if len(latest) == 0 {
		return -1
	}
	maxLen := len(current)
	if len(latest) > maxLen {
		maxLen = len(latest)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(current) {
			return -1
		}
		if i >= len(latest) {
			return 1
		}
		if cmp := comparePrereleaseIdentifier(current[i], latest[i]); cmp != 0 {
			return cmp
		}
	}
	return 0
}

func comparePrereleaseIdentifier(current, latest string) int {
	currentNumeric, currentNumber, currentOK := parsePrereleaseNumber(current)
	latestNumeric, latestNumber, latestOK := parsePrereleaseNumber(latest)
	if currentOK && latestOK {
		return compareInts(currentNumber, latestNumber)
	}
	if currentNumeric != latestNumeric {
		if currentNumeric {
			return -1
		}
		return 1
	}
	return strings.Compare(current, latest)
}

func parsePrereleaseNumber(value string) (bool, int, bool) {
	if value == "" {
		return false, 0, false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false, 0, false
		}
	}
	if len(value) > 1 && strings.HasPrefix(value, "0") {
		return true, 0, false
	}
	part, err := strconv.Atoi(value)
	return true, part, err == nil
}
