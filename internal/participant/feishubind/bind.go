package feishubind

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/participant"
)

type Result struct {
	Status          string   `json:"status"`
	Channel         string   `json:"channel"`
	ParticipantType string   `json:"participant_type"`
	ParticipantID   string   `json:"participant_id"`
	AgentID         string   `json:"agent_id,omitempty"`
	ConfigSaved     bool     `json:"config_saved"`
	RestartStatus   string   `json:"restart_status,omitempty"`
	RestartError    string   `json:"restart_error,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

func BindAdminHuman(ctx context.Context, participantSvc *participant.Service, openID, name string) (Result, error) {
	if participantSvc == nil {
		return Result{}, fmt.Errorf("participant service is required")
	}
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return Result{}, fmt.Errorf("open_id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "admin"
	}
	participantID := "admin"
	item, err := upsertAdminParticipant(ctx, participantSvc, participantID, name, openID)
	if err != nil {
		return Result{}, fmt.Errorf("bind feishu admin human participant_id=%q: %w", participantID, err)
	}
	return Result{
		Status:          "configured",
		Channel:         participant.ChannelFeishu,
		ParticipantType: participant.TypeHuman,
		ParticipantID:   item.ID,
		ConfigSaved:     true,
	}, nil
}

func BindBot(ctx context.Context, agentSvc *agent.Service, participantSvc *participant.Service, agentRef, appID, appSecret string, restart bool) (Result, error) {
	if agentSvc == nil {
		return Result{}, fmt.Errorf("agent service is required")
	}
	if participantSvc == nil {
		return Result{}, fmt.Errorf("participant service is required")
	}
	agentRef = strings.TrimSpace(agentRef)
	if agentRef == "" {
		return Result{}, fmt.Errorf("agent is required")
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return Result{}, fmt.Errorf("app_id is required")
	}
	appSecret = strings.TrimSpace(appSecret)
	if appSecret == "" {
		return Result{}, fmt.Errorf("app_secret is required")
	}
	target, err := ResolveAgent(agentSvc, agentRef)
	if err != nil {
		return Result{}, fmt.Errorf("resolve agent %q: %w", agentRef, err)
	}
	participantID := agent.ParticipantIDForAgent(target.Name, target.ID)
	item, warnings, err := upsertBotParticipant(ctx, participantSvc, participantID, target, appID, appSecret)
	if err != nil {
		return Result{}, fmt.Errorf("bind feishu bot participant_id=%q agent_id=%q: %w", participantID, target.ID, err)
	}

	result := Result{
		Status:          "configured",
		Channel:         participant.ChannelFeishu,
		ParticipantType: participant.TypeAgent,
		ParticipantID:   item.ID,
		AgentID:         target.ID,
		ConfigSaved:     true,
		Warnings:        warnings,
	}
	if restart {
		restartStatus := "worker_recreated"
		if strings.EqualFold(target.ID, agent.ManagerUserID) || strings.EqualFold(target.Role, agent.RoleManager) {
			restartStatus = "manager_recreated"
		}
		if _, err := agentSvc.Recreate(ctx, target.ID); err != nil {
			result.Status = "partial"
			result.RestartStatus = "recreate_failed"
			result.RestartError = err.Error()
		} else {
			result.RestartStatus = restartStatus
		}
	} else {
		result.RestartStatus = "restart_skipped"
	}
	return result, nil
}

func ResolveAgent(agentSvc *agent.Service, ref string) (agent.Agent, error) {
	if agentSvc == nil {
		return agent.Agent{}, fmt.Errorf("agent service is required")
	}
	ref = strings.TrimSpace(ref)
	for _, candidate := range agentIDCandidates(ref) {
		if got, ok := agentSvc.Agent(candidate); ok {
			return got, nil
		}
	}
	var matches []agent.Agent
	for _, item := range agentSvc.List() {
		if strings.EqualFold(strings.TrimSpace(item.Name), ref) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return agent.Agent{}, fmt.Errorf("agent name %q matched multiple agents", ref)
	}
	return agent.Agent{}, fmt.Errorf("agent %q not found", ref)
}

func CanonicalParticipantID(target agent.Agent) string {
	return agent.ParticipantIDForAgent(target.Name, target.ID)
}

func agentIDCandidates(ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	candidates := []string{ref}
	if !strings.HasPrefix(ref, "u-") {
		candidates = append(candidates, "u-"+ref)
	}
	return candidates
}

func upsertAdminParticipant(ctx context.Context, participantSvc *participant.Service, participantID, name, openID string) (participant.Participant, error) {
	existing, ok, err := findParticipantByID(participantSvc, participant.ChannelFeishu, participantID)
	if err != nil {
		return participant.Participant{}, err
	}
	if ok {
		if existing.Type != participant.TypeHuman {
			return participant.Participant{}, fmt.Errorf("existing participant type is %q, want %q", existing.Type, participant.TypeHuman)
		}
		kind := participant.ChannelUserKindOpenID
		updated, ok, err := participantSvc.Update(ctx, participant.ChannelFeishu, participantID, participant.UpdateRequest{
			Name:            &name,
			ChannelUserRef:  &openID,
			ChannelUserKind: &kind,
		})
		if err != nil {
			return participant.Participant{}, err
		}
		if !ok {
			return participant.Participant{}, fmt.Errorf("participant %s:%s not found", participant.ChannelFeishu, participantID)
		}
		return updated, nil
	}
	return participantSvc.Create(ctx, participant.CreateRequest{
		ID:      participantID,
		Channel: participant.ChannelFeishu,
		Type:    participant.TypeHuman,
		Name:    name,
		ChannelUser: participant.ChannelUserSpec{
			Ref:  openID,
			Kind: participant.ChannelUserKindOpenID,
		},
	})
}

func upsertBotParticipant(ctx context.Context, participantSvc *participant.Service, participantID string, target agent.Agent, appID, appSecret string) (participant.Participant, []string, error) {
	all := participantSvc.List(participant.ListOptions{Channel: participant.ChannelFeishu})
	var existing participant.Participant
	hasExisting := false
	var warnings []string
	for _, item := range all {
		if item.ID == participantID {
			existing = item
			hasExisting = true
			continue
		}
		if item.Type == participant.TypeAgent && strings.TrimSpace(item.AgentID) == strings.TrimSpace(target.ID) {
			warnings = append(warnings, fmt.Sprintf("found noncanonical feishu participant %q for agent %q; keeping it and writing canonical participant %q", item.ID, target.ID, participantID))
		}
	}
	cfg := map[string]any{
		"app_id":     appID,
		"app_secret": appSecret,
	}
	kind := participant.ChannelUserKindAppID
	displayName := strings.TrimSpace(target.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(target.ID)
	}
	if hasExisting {
		if existing.Type != participant.TypeAgent {
			return participant.Participant{}, warnings, fmt.Errorf("existing participant type is %q, want %q", existing.Type, participant.TypeAgent)
		}
		if strings.TrimSpace(existing.AgentID) != "" && strings.TrimSpace(existing.AgentID) != strings.TrimSpace(target.ID) {
			return participant.Participant{}, warnings, fmt.Errorf("existing participant is bound to agent %q", existing.AgentID)
		}
		name := displayName
		agentID := target.ID
		channelUserRef := ""
		updated, ok, err := participantSvc.Update(ctx, participant.ChannelFeishu, participantID, participant.UpdateRequest{
			Name:             &name,
			ChannelUserRef:   &channelUserRef,
			ChannelUserKind:  &kind,
			ChannelAppConfig: cfg,
			AgentID:          &agentID,
		})
		if err != nil {
			return participant.Participant{}, warnings, err
		}
		if !ok {
			return participant.Participant{}, warnings, fmt.Errorf("participant %s:%s not found", participant.ChannelFeishu, participantID)
		}
		return updated, warnings, err
	}
	created, err := participantSvc.Create(ctx, participant.CreateRequest{
		ID:               participantID,
		Channel:          participant.ChannelFeishu,
		Type:             participant.TypeAgent,
		Name:             displayName,
		ChannelAppConfig: cfg,
		ChannelUser: participant.ChannelUserSpec{
			Kind: participant.ChannelUserKindAppID,
		},
		AgentBinding: participant.AgentBindingSpec{
			Mode:    participant.BindingModeReuse,
			AgentID: target.ID,
		},
	})
	return created, warnings, err
}

func findParticipantByID(participantSvc *participant.Service, channel, id string) (participant.Participant, bool, error) {
	if participantSvc == nil {
		return participant.Participant{}, false, fmt.Errorf("participant service is required")
	}
	items := participantSvc.List(participant.ListOptions{Channel: channel})
	for _, item := range items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return participant.Participant{}, false, nil
}
