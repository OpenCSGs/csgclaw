package feishubind

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
)

type Client interface {
	ListParticipants(ctx context.Context, channel, typ, agentID string) ([]apitypes.Participant, error)
	ListAgents(ctx context.Context) ([]apitypes.Agent, error)
	GetAgent(ctx context.Context, id string) (apitypes.Agent, error)
	CreateParticipant(ctx context.Context, req participant.CreateRequest) (apitypes.Participant, error)
	UpdateParticipant(ctx context.Context, channel, id string, req participant.UpdateRequest) (apitypes.Participant, error)
	RecreateAgent(ctx context.Context, id string) (apitypes.Agent, error)
}

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

func BindAdminHuman(ctx context.Context, client Client, openID, name string) (Result, error) {
	if client == nil {
		return Result{}, fmt.Errorf("feishu bind client is required")
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
	item, err := upsertAdminParticipant(ctx, client, participantID, name, openID)
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

func BindBot(ctx context.Context, client Client, agentRef, appID, appSecret string, restart bool) (Result, error) {
	if client == nil {
		return Result{}, fmt.Errorf("feishu bind client is required")
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
	target, err := ResolveAgent(ctx, client, agentRef)
	if err != nil {
		return Result{}, fmt.Errorf("resolve agent %q: %w", agentRef, err)
	}
	participantID := agent.ParticipantIDForAgent(target.Name, target.ID)
	item, warnings, err := upsertBotParticipant(ctx, client, participantID, target, appID, appSecret)
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
		if strings.EqualFold(target.ID, agent.ManagerUserID) || strings.EqualFold(target.Role, agent.RoleManager) {
			result.RestartStatus = "manager_restart_required"
		} else if _, err := client.RecreateAgent(ctx, target.ID); err != nil {
			result.Status = "partial"
			result.RestartStatus = "recreate_failed"
			result.RestartError = err.Error()
		} else {
			result.RestartStatus = "worker_recreated"
		}
	} else {
		result.RestartStatus = "restart_skipped"
	}
	return result, nil
}

func ResolveAgent(ctx context.Context, client Client, ref string) (apitypes.Agent, error) {
	ref = strings.TrimSpace(ref)
	for _, candidate := range agentIDCandidates(ref) {
		if got, err := client.GetAgent(ctx, candidate); err == nil {
			return got, nil
		}
	}
	agents, err := client.ListAgents(ctx)
	if err != nil {
		return apitypes.Agent{}, err
	}
	var matches []apitypes.Agent
	for _, item := range agents {
		if strings.EqualFold(strings.TrimSpace(item.Name), ref) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return apitypes.Agent{}, fmt.Errorf("agent name %q matched multiple agents", ref)
	}
	return apitypes.Agent{}, fmt.Errorf("agent %q not found", ref)
}

func CanonicalParticipantID(target apitypes.Agent) string {
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

func upsertAdminParticipant(ctx context.Context, client Client, participantID, name, openID string) (apitypes.Participant, error) {
	existing, ok, err := findParticipantByID(ctx, client, participant.ChannelFeishu, participantID)
	if err != nil {
		return apitypes.Participant{}, err
	}
	if ok {
		if existing.Type != participant.TypeHuman {
			return apitypes.Participant{}, fmt.Errorf("existing participant type is %q, want %q", existing.Type, participant.TypeHuman)
		}
		kind := participant.ChannelUserKindOpenID
		return client.UpdateParticipant(ctx, participant.ChannelFeishu, participantID, participant.UpdateRequest{
			Name:            &name,
			ChannelUserRef:  &openID,
			ChannelUserKind: &kind,
		})
	}
	return client.CreateParticipant(ctx, participant.CreateRequest{
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

func upsertBotParticipant(ctx context.Context, client Client, participantID string, target apitypes.Agent, appID, appSecret string) (apitypes.Participant, []string, error) {
	all, err := client.ListParticipants(ctx, participant.ChannelFeishu, "", "")
	if err != nil {
		return apitypes.Participant{}, nil, err
	}
	var existing apitypes.Participant
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
			return apitypes.Participant{}, warnings, fmt.Errorf("existing participant type is %q, want %q", existing.Type, participant.TypeAgent)
		}
		if strings.TrimSpace(existing.AgentID) != "" && strings.TrimSpace(existing.AgentID) != strings.TrimSpace(target.ID) {
			return apitypes.Participant{}, warnings, fmt.Errorf("existing participant is bound to agent %q", existing.AgentID)
		}
		name := displayName
		agentID := target.ID
		channelUserRef := ""
		updated, err := client.UpdateParticipant(ctx, participant.ChannelFeishu, participantID, participant.UpdateRequest{
			Name:             &name,
			ChannelUserRef:   &channelUserRef,
			ChannelUserKind:  &kind,
			ChannelAppConfig: cfg,
			AgentID:          &agentID,
		})
		return updated, warnings, err
	}
	created, err := client.CreateParticipant(ctx, participant.CreateRequest{
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

func findParticipantByID(ctx context.Context, client Client, channel, id string) (apitypes.Participant, bool, error) {
	items, err := client.ListParticipants(ctx, channel, "", "")
	if err != nil {
		return apitypes.Participant{}, false, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return apitypes.Participant{}, false, nil
}
