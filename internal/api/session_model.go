package api

import (
	"errors"
	"regexp"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/runtime"
)

var autoProfilePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type sessionModelService interface {
	ResolvedSessionProfile(string, string, string) (agent.AgentProfile, error)
}

func (h *Handler) sessionTurnModel(selected agentengine.Agent, model, reasoningEffort, autoProfile string) (*agentengine.TurnModel, error) {
	model, reasoningEffort = strings.TrimSpace(model), strings.TrimSpace(reasoningEffort)
	autoProfile = strings.TrimSpace(autoProfile)
	if model == "" && reasoningEffort == "" && autoProfile == "" {
		return nil, nil
	}
	param := "model"
	if model == "" {
		param = "reasoning_effort"
	}
	invalid := func(message string) (*agentengine.TurnModel, error) {
		return nil, &agentSessionRequestValidationError{param: param, message: message}
	}
	if autoProfile != "" && (model != "auto" || !autoProfilePattern.MatchString(autoProfile)) {
		param = "auto_profile"
		return invalid("auto_profile requires model auto and a valid routing profile ID")
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
	if autoProfile != "" && !profile.UsesOpenCSGAIGateway() {
		param = "auto_profile"
		return invalid("auto_profile requires the OpenCSG model provider")
	}
	return &agentengine.TurnModel{
		ModelID: profile.ModelID, ReasoningEffort: profile.ReasoningEffort, AutoProfile: autoProfile, Metadata: profile.ModelMetadata,
	}, nil
}
