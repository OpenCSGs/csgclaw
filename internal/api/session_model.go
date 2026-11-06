package api

import (
	"errors"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/runtime"
)

type sessionModelService interface {
	ResolvedSessionProfile(string, string, string) (agent.AgentProfile, error)
}

func (h *Handler) sessionTurnModel(selected agentengine.Agent, model, reasoningEffort string) (*agentengine.TurnModel, error) {
	model, reasoningEffort = strings.TrimSpace(model), strings.TrimSpace(reasoningEffort)
	if model == "" && reasoningEffort == "" {
		return nil, nil
	}
	param := "model"
	if model == "" {
		param = "reasoning_effort"
	}
	invalid := func(message string) (*agentengine.TurnModel, error) {
		return nil, &agentSessionRequestValidationError{param: param, message: message}
	}
	if selected.Spec.Runtime.Adapter != runtime.KindCodex {
		return invalid("model and reasoning_effort overrides require a Codex Agent")
	}
	models, ok := h.agentModels.(sessionModelService)
	if !ok {
		return invalid("session model configuration is unavailable")
	}
	profile, err := models.ResolvedSessionProfile(selected.ID, model, reasoningEffort)
	if err != nil {
		var optionError *agent.SessionModelError
		if errors.As(err, &optionError) {
			param = optionError.Field
		}
		return invalid(err.Error())
	}
	return &agentengine.TurnModel{
		ModelID: profile.ModelID, ReasoningEffort: profile.ReasoningEffort, Metadata: profile.ModelMetadata,
	}, nil
}
