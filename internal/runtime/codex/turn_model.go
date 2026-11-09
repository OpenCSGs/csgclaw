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

// TurnModel resolves a model request belonging to the active native conversation.
// Before turn/start acknowledges its ID, the thread already owns its immutable
// selection. Only native start notifications and responses establish the ID.
func (r *Runtime) TurnModel(agentID, threadID, turnID, requestedModel string) (contract.TurnModel, bool, error) {
	threadID, turnID = strings.TrimSpace(threadID), strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" || len(threadID) > 256 || len(turnID) > 256 {
		return contract.TurnModel{}, false, nil
	}
	_, live := r.nativeTurnOwner(agentID, threadID, turnID)
	if live == nil {
		return contract.TurnModel{}, false, nil
	}
	live.mu.Lock()
	waiter := live.turnWaiters[threadID]
	if waiter == nil || !waiter.matchesTurn(turnID) {
		live.mu.Unlock()
		return contract.TurnModel{}, false, nil
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel != "" && requestedModel != waiter.model.ModelID && !waiter.compacting {
		active := live.turnContexts[threadID]
		live.mu.Unlock()
		if active.waiter != waiter || active.ctx == nil {
			return contract.TurnModel{}, false, fmt.Errorf("native model selection has no active turn context")
		}
		// Notifications and inference use separate transports. A read response
		// passes through the same stdout reader after preceding compaction events.
		if _, err := live.appClient.request(active.ctx, "thread/read", map[string]any{"threadId": threadID, "includeTurns": false}); err != nil {
			return contract.TurnModel{}, false, fmt.Errorf("synchronize native model selection: %w", err)
		}
		live.mu.Lock()
		if live.turnWaiters[threadID] != waiter || waiter.currentTurnID() != turnID {
			live.mu.Unlock()
			return contract.TurnModel{}, false, nil
		}
	}
	defer live.mu.Unlock()
	if waiter.compacting && requestedModel != "" && requestedModel != waiter.model.ModelID {
		loaded, available := live.loadedTurnModel(requestedModel)
		if !available {
			return contract.TurnModel{}, false, fmt.Errorf("compaction model %q is unavailable in the running Codex catalog", requestedModel)
		}
		// Native compaction can use the last accepted model before starting
		// inference with the current turn's selection.
		if previous, ok := live.threadModels[threadID]; ok && previous.ModelID == loaded.ModelID {
			loaded.ReasoningEffort = previous.ReasoningEffort
		}
		return loaded, true, nil
	}
	return waiter.model, waiter.modelOverride, nil
}

func (s *liveSession) loadedTurnModel(modelID string) (contract.TurnModel, bool) {
	selected := contract.TurnModel{ReasoningEffort: config.ReasoningEffortAuto}
	for _, model := range s.spec.ModelCatalog {
		if model.ModelID == modelID && model.Provider == s.spec.Profile.Provider {
			selected.ModelID = model.ModelID
			selected.Metadata = model.Metadata.Normalized()
			return selected, true
		}
	}
	if modelID == s.spec.Profile.ModelID {
		selected.ModelID = modelID
		selected.Metadata = s.spec.Profile.ModelMetadata.Normalized()
		return selected, true
	}
	return contract.TurnModel{}, false
}

func (s *liveSession) setTurnCompacting(threadID, turnID, method string) {
	if method != "item/started" && method != "item/completed" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if waiter := s.turnWaiters[threadID]; waiter != nil && turnID != "" && waiter.currentTurnID() == turnID {
		waiter.compacting = method == "item/started"
	}
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
