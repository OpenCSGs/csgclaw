package codex

import (
	"fmt"
	"strings"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/codexmodel"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func turnModelFromProfile(profile agentruntime.Profile) contract.TurnModel {
	return contract.TurnModel{
		ModelID:         profile.ModelID,
		ReasoningEffort: config.NormalizeReasoningEffort(profile.ReasoningEffort),
		Metadata:        profile.ModelMetadata.Normalized(),
	}
}

func (r *Runtime) sessionModelCatalog(agentID string, profile agentruntime.Profile) []codexmodel.Profile {
	var catalog []codexmodel.Profile
	if r.deps.ModelCatalog != nil {
		catalog = r.deps.ModelCatalog(agentID)
	}
	return mergedModelCatalog(profile, catalog)
}

func mergedModelCatalog(profile agentruntime.Profile, catalog []codexmodel.Profile) []codexmodel.Profile {
	profile = profile.Normalized()
	models := []codexmodel.Profile{{
		Provider: profile.Provider, ModelID: profile.ModelID,
		Metadata: profile.ModelMetadata.Normalized(),
	}}
	seen := map[string]bool{profile.ModelID: true}
	for _, model := range catalog {
		model.ModelID = strings.TrimSpace(model.ModelID)
		if model.ModelID == "" || seen[model.ModelID] {
			continue
		}
		seen[model.ModelID] = true
		// Profile effort is sent on each turn. Keeping it out of model metadata
		// lets an explicit auto selection clear that default for one request.
		model.ReasoningEffort = ""
		model.Metadata = model.Metadata.Normalized()
		models = append(models, model)
	}
	return models
}

func (s *liveSession) resolveTurnModel(override *contract.TurnModel) (contract.TurnModel, error) {
	if override == nil {
		return turnModelFromProfile(s.spec.Profile), nil
	}
	selected := *override
	selected.ModelID = strings.TrimSpace(selected.ModelID)
	selected.ReasoningEffort = config.NormalizeReasoningEffort(selected.ReasoningEffort)
	selected.Metadata = selected.Metadata.Normalized()
	for _, model := range mergedModelCatalog(s.spec.Profile, s.spec.ModelCatalog) {
		if model.ModelID != selected.ModelID {
			continue
		}
		if model.Metadata.Normalized().ContextWindow != selected.Metadata.ContextWindow {
			return contract.TurnModel{}, fmt.Errorf("model %q context metadata differs from the running Codex catalog", selected.ModelID)
		}
		return selected, nil
	}
	return contract.TurnModel{}, fmt.Errorf("model %q is unavailable in the running Codex catalog", selected.ModelID)
}

// TurnModel returns only a request override for the active native conversation.
// Before turn/start acknowledges its ID, the thread already owns its immutable
// selection. Only native start notifications and responses establish the ID.
func (r *Runtime) TurnModel(agentID, threadID, turnID string) (contract.TurnModel, bool) {
	threadID, turnID = strings.TrimSpace(threadID), strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" || len(threadID) > 256 || len(turnID) > 256 {
		return contract.TurnModel{}, false
	}
	_, live, err := r.activeAgentSession(agentID)
	if err != nil {
		return contract.TurnModel{}, false
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	waiter := live.turnWaiters[threadID]
	if waiter == nil || !waiter.modelOverride {
		return contract.TurnModel{}, false
	}
	if expected := waiter.currentTurnID(); expected != "" && expected != turnID {
		return contract.TurnModel{}, false
	}
	return waiter.model, true
}

func applyTurnModelParams(params map[string]any, selected contract.TurnModel) {
	if selected.ModelID != "" {
		params["model"] = selected.ModelID
	}
	effort := config.NormalizeReasoningEffort(selected.ReasoningEffort)
	if !config.UsesModelReasoningDefault(effort) {
		params["effort"] = effort
		delete(params, "collaborationMode")
		return
	}
	delete(params, "effort")
	if selected.ModelID == "" {
		delete(params, "collaborationMode")
		return
	}
	// A null turn effort inherits the previous turn. Collaboration settings can
	// explicitly clear it so the selected model's default applies again.
	params["collaborationMode"] = map[string]any{
		"mode": "default",
		"settings": map[string]any{
			"model": selected.ModelID, "reasoning_effort": nil, "developer_instructions": nil,
		},
	}
}
