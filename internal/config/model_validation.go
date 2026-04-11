package config

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ProviderLLMAPI = "llm-api"
)

type ModelValidationError struct {
	MissingFields []string
	Message       string
}

func (e *ModelValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if len(e.MissingFields) == 0 {
		return "invalid model config"
	}
	return fmt.Sprintf("missing required model fields: %s", strings.Join(e.MissingFields, ", "))
}

func NormalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", ProviderLLMAPI:
		return ProviderLLMAPI
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func (c ModelConfig) EffectiveProvider() string {
	return NormalizeProvider(c.Provider)
}

func (c ModelConfig) Resolved() ModelConfig {
	out := c
	out.Provider = out.EffectiveProvider()
	out.BaseURL = strings.TrimRight(strings.TrimSpace(out.BaseURL), "/")
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.ModelID = strings.TrimSpace(out.ModelID)
	out.ReasoningEffort = strings.ToLower(strings.TrimSpace(out.ReasoningEffort))
	return out
}

func (c ModelConfig) MissingFields() []string {
	cfg := c.Resolved()
	var missing []string
	if cfg.BaseURL == "" {
		missing = append(missing, "base_url")
	}
	if cfg.APIKey == "" {
		missing = append(missing, "api_key")
	}
	if cfg.ModelID == "" {
		missing = append(missing, "model_id")
	}
	return missing
}

func (c ModelConfig) Validate() error {
	cfg := c.Resolved()
	if err := cfg.validateProvider(); err != nil {
		return err
	}
	if missing := cfg.MissingFields(); len(missing) > 0 {
		return &ModelValidationError{
			MissingFields: missing,
			Message:       fmt.Sprintf("provider %q is missing required fields: %s", cfg.Provider, strings.Join(missing, ", ")),
		}
	}
	return nil
}

func (c LLMConfig) Normalized() LLMConfig {
	out := LLMConfig{
		DefaultProfile: strings.TrimSpace(c.DefaultProfile),
		Profiles:       make(map[string]ModelConfig, len(c.Profiles)),
	}
	for name, profile := range c.Profiles {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out.Profiles[name] = profile.Resolved()
	}
	if out.DefaultProfile == "" {
		out.DefaultProfile = out.EffectiveDefaultProfile()
	}
	return out
}

func (c LLMConfig) EffectiveDefaultProfile() string {
	defaultProfile := strings.TrimSpace(c.DefaultProfile)
	if defaultProfile != "" {
		return defaultProfile
	}
	if len(c.Profiles) == 1 {
		for name := range c.Profiles {
			return strings.TrimSpace(name)
		}
	}
	if _, ok := c.Profiles[DefaultLLMProfile]; ok {
		return DefaultLLMProfile
	}
	return ""
}

func (c LLMConfig) Resolve(profile string) (string, ModelConfig, error) {
	cfg := c.Normalized()
	name := strings.TrimSpace(profile)
	if name == "" {
		name = cfg.EffectiveDefaultProfile()
	}
	if name == "" {
		return "", ModelConfig{}, &ModelValidationError{Message: "llm default_profile is not configured"}
	}
	model, ok := cfg.Profiles[name]
	if !ok {
		return "", ModelConfig{}, &ModelValidationError{Message: fmt.Sprintf("llm profile %q was not found", name)}
	}
	return name, model.Resolved(), nil
}

func (c LLMConfig) MatchProfile(candidate ModelConfig) (string, ModelConfig, bool) {
	cfg := c.Normalized()
	candidate = candidate.Resolved()
	for _, name := range sortedProfileNames(cfg.Profiles) {
		profile := cfg.Profiles[name].Resolved()
		if !strings.EqualFold(profile.EffectiveProvider(), candidate.EffectiveProvider()) {
			continue
		}
		if strings.TrimSpace(profile.ModelID) != strings.TrimSpace(candidate.ModelID) {
			continue
		}
		if candidate.ReasoningEffort != "" && strings.TrimSpace(profile.ReasoningEffort) != strings.TrimSpace(candidate.ReasoningEffort) {
			continue
		}
		return name, profile, true
	}
	return "", ModelConfig{}, false
}

func (c LLMConfig) MissingFields() []string {
	_, model, err := c.Resolve("")
	if err != nil {
		var validationErr *ModelValidationError
		if errors.As(err, &validationErr) {
			return append([]string(nil), validationErr.MissingFields...)
		}
		return nil
	}
	return model.MissingFields()
}

func (c LLMConfig) Validate() error {
	cfg := c.Normalized()
	if len(cfg.Profiles) == 0 {
		return SingleProfileLLM(ModelConfig{}).Validate()
	}
	defaultProfile := cfg.EffectiveDefaultProfile()
	if defaultProfile == "" {
		return &ModelValidationError{
			MissingFields: []string{"default_profile"},
			Message:       "llm default_profile is required",
		}
	}
	if _, ok := cfg.Profiles[defaultProfile]; !ok {
		return &ModelValidationError{
			MissingFields: []string{"default_profile"},
			Message:       fmt.Sprintf("llm default_profile %q does not match any llm.profiles entry", defaultProfile),
		}
	}
	for _, name := range sortedProfileNames(cfg.Profiles) {
		profile := cfg.Profiles[name]
		if err := profile.Validate(); err != nil {
			if name == defaultProfile {
				return err
			}
			return fmt.Errorf("llm profile %q is invalid: %w", name, err)
		}
	}
	return nil
}

func (c ModelConfig) validateProvider() error {
	if c.EffectiveProvider() == ProviderLLMAPI {
		return nil
	}
	return &ModelValidationError{
		Message: fmt.Sprintf(
			"unsupported model provider %q; only %q is supported now",
			strings.TrimSpace(c.Provider),
			ProviderLLMAPI,
		),
	}
}
