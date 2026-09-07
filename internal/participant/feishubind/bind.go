package feishubind

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/participant"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrBotAppIDConflict = errors.New("feishu bot app_id is already used by another worker")
	bindBotMu           sync.Mutex
)

type Result struct {
	Status           string   `json:"status"`
	Channel          string   `json:"channel"`
	ParticipantType  string   `json:"participant_type"`
	ParticipantID    string   `json:"participant_id"`
	AgentID          string   `json:"agent_id,omitempty"`
	ConfigSaved      bool     `json:"config_saved"`
	RestartStatus    string   `json:"restart_status,omitempty"`
	RestartError     string   `json:"restart_error,omitempty"`
	ActivationStatus string   `json:"activation_status,omitempty"`
	ActivationError  string   `json:"activation_error,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
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

func BindBot(ctx context.Context, engine agentengine.Interface, participantSvc *participant.Service, agentRef, appID, appSecret string) (Result, error) {
	if engine == nil {
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
	target, err := ResolveAgent(engine, agentRef)
	if err != nil {
		return Result{}, fmt.Errorf("resolve agent %q: %w", agentRef, err)
	}
	participantID := agent.ParticipantIDForAgent(target.Name, target.ID)
	var item participant.Participant
	var warnings []string
	err = WithExclusiveBotAppID(participantSvc, target.ID, appID, func() error {
		var err error
		item, warnings, err = upsertBotParticipant(ctx, participantSvc, participantID, target, appID, appSecret)
		return err
	})
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
	result.RestartStatus = "restart_skipped"
	return result, nil
}

// WithExclusiveBotAppID serializes the cross-Agent uniqueness check with the
// Participant write. Runtime reconciliation is outside this short global lock.
func WithExclusiveBotAppID(svc *participant.Service, agentID, appID string, write func() error) error {
	bindBotMu.Lock()
	defer bindBotMu.Unlock()
	if err := ValidateBotAppIDExclusive(svc, agentID, appID); err != nil {
		return err
	}
	return write()
}

func ValidateBotAppIDExclusive(participantSvc *participant.Service, agentID, appID string) error {
	if participantSvc == nil {
		return fmt.Errorf("participant service is required")
	}
	agentID = agent.CanonicalID(agentID)
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil
	}
	for _, item := range participantSvc.List(participant.ListOptions{
		Channel: participant.ChannelFeishu,
		Type:    participant.TypeAgent,
	}) {
		otherAgentID := agent.CanonicalID(item.AgentID)
		if otherAgentID == "" || otherAgentID == agentID {
			continue
		}
		if channelAppConfigString(item.ChannelAppConfig, "app_id") == appID {
			return fmt.Errorf("%w: app_id %q is already connected to worker %q; disconnect Feishu from that worker or use another Bot app", ErrBotAppIDConflict, appID, otherAgentID)
		}
	}
	return nil
}

func ResolveAgent(engine agentengine.Interface, ref string) (agent.Agent, error) {
	if engine == nil {
		return agent.Agent{}, fmt.Errorf("agent service is required")
	}
	agents := engine.Agents()
	ref = strings.TrimSpace(ref)
	for _, candidate := range agentIDCandidates(ref) {
		if got, err := agents.Get(context.Background(), candidate, agentengine.AgentGetOptions{}); err == nil {
			return bindingAgentFromEngine(got), nil
		}
	}
	var matches []agent.Agent
	items, err := agents.List(context.Background(), agentengine.AgentListOptions{})
	if err != nil {
		return agent.Agent{}, err
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Spec.Name), ref) {
			matches = append(matches, bindingAgentFromEngine(item))
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

func bindingAgentFromEngine(item agentengine.Agent) agent.Agent {
	return agent.Agent{
		ID:           item.ID,
		Name:         item.Spec.Name,
		Description:  item.Spec.Description,
		Instructions: item.Spec.Instructions,
		Role:         string(item.Spec.Role),
		RuntimeKind:  item.Status.RuntimeKind,
		Status:       string(item.Status.State),
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func CanonicalParticipantID(target agent.Agent) string {
	return agent.ParticipantIDForAgent(target.Name, target.ID)
}

func agentIDCandidates(ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	candidates := []string{ref, agent.CanonicalID(ref)}
	suffix := strings.TrimPrefix(agent.CanonicalID(ref), agent.AgentIDPrefix)
	if suffix != "" {
		candidates = append(candidates, "u-"+suffix, suffix)
	}
	return compactAgentIDCandidates(candidates)
}

func compactAgentIDCandidates(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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
	item, ok := participantSvc.Get(channel, id)
	return item, ok, nil
}

func channelAppConfigString(values map[string]any, key string) string {
	for candidate, value := range values {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}
