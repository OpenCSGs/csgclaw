package agent

import (
	"context"
	"strconv"
	"strings"
)

func (s *Service) withRuntimeImageMigrationStatus(ctx context.Context, a Agent) Agent {
	return s.withRuntimeImageMigrationStatusFromCandidates(ctx, a, s.localImageCandidates(ctx))
}

func (s *Service) withRuntimeImageMigrationStatusFromCandidates(ctx context.Context, a Agent, localImages []string) Agent {
	a = *cloneAgent(&a)
	if _, ok := s.imageUpgradeCandidateFromCandidates(ctx, a, localImages); !ok {
		return a
	}
	a.AgentProfile.ImageUpgradeRequired = true
	return a
}

func (s *Service) imageForRecreate(ctx context.Context, a Agent) string {
	current := strings.TrimSpace(a.Image)
	if latestImage, ok := s.imageUpgradeCandidateFromCandidates(ctx, a, s.localImageCandidates(ctx)); ok {
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
	if image, ok := s.imageUpgradeCandidateFromCandidates(ctx, a, s.localImageCandidates(ctx)); ok {
		return image, true
	}
	image, ok := s.currentDefaultImageForAgent(ctx, a)
	if isDevImageTag(dockerImageTag(image)) {
		return "", false
	}
	return strings.TrimSpace(image), ok && strings.TrimSpace(image) != ""
}

func (s *Service) imageUpgradeCandidateFromCandidates(ctx context.Context, a Agent, localImages []string) (string, bool) {
	if image, ok := latestNewerLocalImageForReference(a.Image, localImages); ok {
		return image, true
	}
	latestImage, ok := s.currentDefaultImageForAgent(ctx, a)
	if ok && imageNeedsDefaultRecreate(a.Image, latestImage) {
		return latestImage, true
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

func (s *Service) currentDefaultImageForAgent(ctx context.Context, a Agent) (string, bool) {
	if s == nil || !isGatewayRuntimeKind(strings.TrimSpace(a.RuntimeKind)) {
		return "", false
	}
	role := normalizeRole(a.Role)
	if isManagerAgent(a) {
		role = RoleManager
	}
	if role != RoleManager && role != RoleWorker {
		return "", false
	}

	s.mu.RLock()
	hubSvc := s.hub
	managerTemplate := strings.TrimSpace(s.defaultManagerTemplate)
	workerTemplate := strings.TrimSpace(s.defaultWorkerTemplate)
	managerImage := strings.TrimSpace(s.managerImage)
	gatewayRuntime := s.gatewayRuntimeKind()
	s.mu.RUnlock()

	templateRef := workerTemplate
	if role == RoleManager {
		templateRef = managerTemplate
	}
	if templateRef != "" && hubSvc != nil {
		item, err := hubSvc.Get(ctx, templateRef)
		if err == nil && defaultTemplateMatchesAgent(item.Role, item.RuntimeKind, role, a.RuntimeKind) {
			if image := strings.TrimSpace(item.Image); image != "" {
				return image, true
			}
		}
	}

	if role == RoleManager && managerImage != "" && (strings.TrimSpace(a.RuntimeKind) == gatewayRuntime || imageNeedsDefaultRecreate(a.Image, managerImage)) {
		return managerImage, true
	}
	return "", false
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

func latestNewerLocalImageForReference(current string, candidates []string) (string, bool) {
	current = strings.TrimSpace(current)
	currentRepo := dockerImageRepository(current)
	currentTag := dockerImageTag(current)
	if current == "" || currentRepo == "" || currentTag == "" || isDevImageTag(currentTag) {
		return "", false
	}
	best := ""
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == current {
			continue
		}
		if !strings.EqualFold(currentRepo, dockerImageRepository(candidate)) {
			continue
		}
		if cmp, ok := compareImageTags(currentTag, dockerImageTag(candidate)); !ok || cmp >= 0 {
			continue
		}
		if best == "" {
			best = candidate
			continue
		}
		if cmp, ok := compareImageTags(dockerImageTag(best), dockerImageTag(candidate)); ok && cmp < 0 {
			best = candidate
		}
	}
	return best, best != ""
}

func imageNeedsDefaultRecreate(current, latest string) bool {
	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)
	if latest == "" || current == latest {
		return false
	}
	if current == "" {
		return true
	}
	currentRepo := dockerImageRepository(current)
	latestRepo := dockerImageRepository(latest)
	if currentRepo == "" || latestRepo == "" || !strings.EqualFold(currentRepo, latestRepo) {
		return false
	}
	currentTag := dockerImageTag(current)
	latestTag := dockerImageTag(latest)
	if currentTag == latestTag {
		return false
	}
	if isDevImageTag(currentTag) || isDevImageTag(latestTag) {
		return false
	}
	if cmp, ok := compareImageTags(currentTag, latestTag); ok {
		return cmp < 0
	}
	return true
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

func compareImageTags(current, latest string) (int, bool) {
	currentParts, ok := parseNumericImageTag(current)
	if !ok {
		return 0, false
	}
	latestParts, ok := parseNumericImageTag(latest)
	if !ok {
		return 0, false
	}
	maxLen := len(currentParts)
	if len(latestParts) > maxLen {
		maxLen = len(latestParts)
	}
	for i := 0; i < maxLen; i++ {
		currentPart := 0
		if i < len(currentParts) {
			currentPart = currentParts[i]
		}
		latestPart := 0
		if i < len(latestParts) {
			latestPart = latestParts[i]
		}
		if currentPart < latestPart {
			return -1, true
		}
		if currentPart > latestPart {
			return 1, true
		}
	}
	return 0, true
}

func parseNumericImageTag(tag string) ([]int, bool) {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(strings.ToLower(tag), "v")
	if tag == "" {
		return nil, false
	}
	fields := strings.FieldsFunc(tag, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	if len(fields) == 0 {
		return nil, false
	}
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			return nil, false
		}
		part, err := strconv.Atoi(field)
		if err != nil {
			return nil, false
		}
		parts = append(parts, part)
	}
	return parts, true
}
