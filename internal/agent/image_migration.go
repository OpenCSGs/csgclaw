package agent

import (
	"context"
	"strings"
)

func (s *Service) withRuntimeImageMigrationStatus(ctx context.Context, a Agent) Agent {
	a = *cloneAgent(&a)
	latestImage, ok := s.currentDefaultImageForAgent(ctx, a)
	if !ok || !imageNeedsDefaultRecreate(a.Image, latestImage) {
		return a
	}
	a.AgentProfile.ImageUpgradeRequired = true
	return a
}

func (s *Service) imageForRecreate(ctx context.Context, a Agent) string {
	current := strings.TrimSpace(a.Image)
	latestImage, ok := s.currentDefaultImageForAgent(ctx, a)
	if ok && imageNeedsDefaultRecreate(current, latestImage) {
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
	return currentRepo != "" && latestRepo != "" && strings.EqualFold(currentRepo, latestRepo)
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
