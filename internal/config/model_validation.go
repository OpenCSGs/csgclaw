package config

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ProviderLLMAPI     = "llm-api"
	DefaultLLMProfile  = "default"
	DefaultModelFormat = "%s.%s"
)

type ProviderConfig struct {
	BaseURL         string
	APIKey          string
	Models          []string
	ReasoningEffort string
}

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

func ModelSelector(providerName, modelID string) string {
	providerName = strings.TrimSpace(providerName)
	modelID = strings.TrimSpace(modelID)
	if providerName == "" || modelID == "" {
		return ""
	}
	return fmt.Sprintf(DefaultModelFormat, providerName, modelID)
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

func (c ProviderConfig) Resolved() ProviderConfig {
	out := c
	out.BaseURL = strings.TrimRight(strings.TrimSpace(out.BaseURL), "/")
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.ReasoningEffort = strings.ToLower(strings.TrimSpace(out.ReasoningEffort))
	out.Models = normalizeModelIDs(out.Models)
	return out
}

func (c ProviderConfig) MissingFields() []string {
	cfg := c.Resolved()
	var missing []string
	if cfg.BaseURL == "" {
		missing = append(missing, "base_url")
	}
	if cfg.APIKey == "" {
		missing = append(missing, "api_key")
	}
	if len(cfg.Models) == 0 {
		missing = append(missing, "model_id")
	}
	return missing
}

func (c ProviderConfig) Validate() error {
	cfg := c.Resolved()
	if missing := cfg.MissingFields(); len(missing) > 0 {
		return &ModelValidationError{
			MissingFields: missing,
			Message:       fmt.Sprintf("provider %q is missing required fields: %s", ProviderLLMAPI, strings.Join(missing, ", ")),
		}
	}
	return nil
}

func (c ProviderConfig) modelConfig(modelID string) ModelConfig {
	cfg := c.Resolved()
	return ModelConfig{
		Provider:        ProviderLLMAPI,
		BaseURL:         cfg.BaseURL,
		APIKey:          cfg.APIKey,
		ModelID:         strings.TrimSpace(modelID),
		ReasoningEffort: cfg.ReasoningEffort,
	}.Resolved()
}

func (c LLMConfig) IsZero() bool {
	return len(c.Providers) == 0 && len(c.Profiles) == 0 && strings.TrimSpace(c.Default) == "" && strings.TrimSpace(c.DefaultProfile) == ""
}

func (c LLMConfig) Normalized() LLMConfig {
	out := LLMConfig{
		Default:        strings.TrimSpace(c.Default),
		Providers:      make(map[string]ProviderConfig),
		DefaultProfile: strings.TrimSpace(c.DefaultProfile),
		Profiles:       make(map[string]ModelConfig),
	}

	for name, provider := range c.Providers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out.Providers[name] = provider.Resolved()
	}
	for name, profile := range c.Profiles {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		provider := out.Providers[name]
		if provider.BaseURL == "" {
			provider.BaseURL = strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
		}
		if provider.APIKey == "" {
			provider.APIKey = strings.TrimSpace(profile.APIKey)
		}
		if provider.ReasoningEffort == "" {
			provider.ReasoningEffort = strings.ToLower(strings.TrimSpace(profile.ReasoningEffort))
		}
		if modelID := strings.TrimSpace(profile.ModelID); modelID != "" {
			provider.Models = append(provider.Models, modelID)
		}
		out.Providers[name] = provider.Resolved()
		out.Profiles[name] = profile.Resolved()
	}

	if out.Default == "" {
		out.Default = out.DefaultProfile
	}
	if out.DefaultProfile == "" {
		out.DefaultProfile = out.Default
	}

	for name, provider := range out.Providers {
		if _, ok := out.Profiles[name]; ok {
			continue
		}
		if len(provider.Models) == 1 {
			out.Profiles[name] = provider.modelConfig(provider.Models[0])
		}
	}

	if out.Default == "" && out.DefaultProfile != "" {
		out.Default = out.DefaultProfile
	}
	if out.DefaultProfile == "" && out.Default != "" {
		out.DefaultProfile = out.Default
	}
	return out
}

func (c LLMConfig) DefaultSelector() string {
	name, _, err := c.Resolve("")
	if err != nil {
		return ""
	}
	return name
}

func (c LLMConfig) EffectiveDefaultProvider() string {
	cfg := c.Normalized()
	if selector := cfg.DefaultSelector(); selector != "" {
		providerName, _, ok := splitModelSelector(selector)
		if ok {
			return providerName
		}
	}
	defaultValue := strings.TrimSpace(cfg.Default)
	if defaultValue == "" {
		defaultValue = strings.TrimSpace(cfg.DefaultProfile)
	}
	if defaultValue != "" {
		if providerName, _, ok := splitModelSelector(defaultValue); ok {
			return providerName
		}
		if _, ok := cfg.Providers[defaultValue]; ok {
			return defaultValue
		}
	}
	if len(cfg.Providers) == 1 {
		for name := range cfg.Providers {
			return name
		}
	}
	if _, ok := cfg.Providers[DefaultLLMProfile]; ok {
		return DefaultLLMProfile
	}
	return ""
}

func (c LLMConfig) Resolve(profile string) (string, ModelConfig, error) {
	cfg := c.Normalized()
	requested := strings.TrimSpace(profile)
	if requested == "" {
		requested = strings.TrimSpace(cfg.Default)
		if requested == "" {
			requested = strings.TrimSpace(cfg.DefaultProfile)
		}
		if requested == "" && len(cfg.Providers) == 1 {
			for name := range cfg.Providers {
				requested = name
			}
		}
	}
	if requested == "" {
		return "", ModelConfig{}, &ModelValidationError{
			MissingFields: []string{"default"},
			Message:       "models default is not configured",
		}
	}
	return cfg.resolveSelector(requested)
}

func (c LLMConfig) resolveSelector(selector string) (string, ModelConfig, error) {
	cfg := c.Normalized()
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", ModelConfig{}, &ModelValidationError{
			MissingFields: []string{"default"},
			Message:       "models default is not configured",
		}
	}

	providerName, modelID, hasModelID := splitModelSelector(selector)
	if !hasModelID {
		providerName = selector
	}
	provider, ok := cfg.Providers[providerName]
	if !ok {
		return "", ModelConfig{}, &ModelValidationError{
			MissingFields: []string{"default"},
			Message:       fmt.Sprintf("models provider %q was not found", providerName),
		}
	}
	provider = provider.Resolved()
	if !hasModelID {
		switch len(provider.Models) {
		case 0:
			modelID = ""
		case 1:
			modelID = provider.Models[0]
		default:
			return "", ModelConfig{}, &ModelValidationError{
				MissingFields: []string{"default"},
				Message:       fmt.Sprintf("models provider %q has multiple models; set models.default to %q", providerName, ModelSelector(providerName, provider.Models[0])),
			}
		}
	} else if !containsString(provider.Models, modelID) {
		return "", ModelConfig{}, &ModelValidationError{
			MissingFields: []string{"default"},
			Message:       fmt.Sprintf("models default %q does not match any models.providers entry", selector),
		}
	}

	model := provider.modelConfig(modelID)
	name := providerName
	if modelID != "" {
		name = ModelSelector(providerName, modelID)
	}
	return name, model, nil
}

func (c LLMConfig) MatchProfile(candidate ModelConfig) (string, ModelConfig, bool) {
	cfg := c.Normalized()
	candidate = candidate.Resolved()
	for _, name := range sortedProviderNames(cfg.Providers) {
		provider := cfg.Providers[name].Resolved()
		if !strings.EqualFold(ProviderLLMAPI, candidate.EffectiveProvider()) {
			continue
		}
		if candidate.ReasoningEffort != "" && strings.TrimSpace(provider.ReasoningEffort) != strings.TrimSpace(candidate.ReasoningEffort) {
			continue
		}
		for _, modelID := range provider.Models {
			if strings.TrimSpace(modelID) != strings.TrimSpace(candidate.ModelID) {
				continue
			}
			model := provider.modelConfig(modelID)
			return ModelSelector(name, modelID), model, true
		}
	}
	return "", ModelConfig{}, false
}

func (c LLMConfig) MissingFields() []string {
	cfg := c.Normalized()
	if len(cfg.Providers) == 0 {
		return ProviderConfig{}.MissingFields()
	}
	_, model, err := cfg.Resolve("")
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
	if len(cfg.Providers) == 0 {
		return SingleProfileLLM(ModelConfig{}).Validate()
	}
	_, model, err := cfg.Resolve("")
	if err != nil {
		return err
	}
	if missing := model.MissingFields(); len(missing) > 0 {
		return &ModelValidationError{
			MissingFields: missing,
			Message:       fmt.Sprintf("models default is missing required fields: %s", strings.Join(missing, ", ")),
		}
	}
	for _, name := range sortedProviderNames(cfg.Providers) {
		provider := cfg.Providers[name]
		if err := provider.Validate(); err != nil {
			return fmt.Errorf("models provider %q is invalid: %w", name, err)
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

func splitModelSelector(selector string) (string, string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", "", false
	}
	dot := strings.Index(selector, ".")
	colon := strings.Index(selector, ":")
	switch {
	case dot == -1 && colon == -1:
		return "", "", false
	case dot == -1:
		providerName, modelID, ok := strings.Cut(selector, ":")
		return strings.TrimSpace(providerName), strings.TrimSpace(modelID), ok && strings.TrimSpace(providerName) != "" && strings.TrimSpace(modelID) != ""
	case colon == -1 || dot < colon:
		providerName, modelID, ok := strings.Cut(selector, ".")
		return strings.TrimSpace(providerName), strings.TrimSpace(modelID), ok && strings.TrimSpace(providerName) != "" && strings.TrimSpace(modelID) != ""
	default:
		providerName, modelID, ok := strings.Cut(selector, ":")
		return strings.TrimSpace(providerName), strings.TrimSpace(modelID), ok && strings.TrimSpace(providerName) != "" && strings.TrimSpace(modelID) != ""
	}
}

func normalizeModelIDs(models []string) []string {
	if len(models) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, modelID := range models {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if _, ok := seen[modelID]; ok {
			continue
		}
		seen[modelID] = struct{}{}
		out = append(out, modelID)
	}
	return out
}

func containsString(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
