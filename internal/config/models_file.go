package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"csgclaw/internal/localstore"
)

const ModelsFileName = "models.json"

type modelProvidersFile struct {
	Version   int                       `json:"version"`
	Default   string                    `json:"default,omitempty"`
	Providers map[string]ProviderConfig `json:"providers"`
}

type modelProvidersState struct {
	DefaultModel *modelProviderDefaultRef  `json:"default_model,omitempty"`
	Items        map[string]ProviderConfig `json:"items"`
	Providers    map[string]ProviderConfig `json:"providers,omitempty"`
	Default      string                    `json:"default,omitempty"`
}

type modelProviderDefaultRef struct {
	ModelProviderID string `json:"model_provider_id"`
	ModelID         string `json:"model_id"`
}

func DefaultModelsPath() (string, error) {
	return DefaultStatePath()
}

func ModelsPathForConfigPath(configPath string) (string, error) {
	if configPath == "" {
		return DefaultModelsPath()
	}
	return filepath.Join(filepath.Dir(configPath), StateFileName), nil
}

func LoadModels(path string) (LLMConfig, bool, error) {
	if localstore.IsRootStatePath(path) {
		var state modelProvidersState
		ok, err := localstore.ReadSection(path, "model_providers", &state)
		if err != nil {
			return LLMConfig{}, ok, err
		}
		if ok {
			cfg := llmConfigFromModelProviderState(state)
			return cfg.Normalized(), true, nil
		}
		legacyPath := filepath.Join(filepath.Dir(path), ModelsFileName)
		if legacyPath != path {
			return LoadModels(legacyPath)
		}
		return LLMConfig{}, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LLMConfig{}, false, nil
		}
		return LLMConfig{}, false, fmt.Errorf("read models config: %w", err)
	}
	var file modelProvidersFile
	if err := json.Unmarshal(data, &file); err != nil {
		return LLMConfig{}, true, fmt.Errorf("parse models config: %w", err)
	}
	cfg := LLMConfig{
		Default:        file.Default,
		DefaultProfile: file.Default,
		Providers:      file.Providers,
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
	cfg.Profiles = make(map[string]ModelConfig)
	return cfg.Normalized(), true, nil
}

func SaveModels(path string, llm LLMConfig) error {
	if localstore.IsRootStatePath(path) {
		llm = llm.Normalized()
		providers := providersForStorage(llm)
		state := modelProvidersState{
			Items: providers,
		}
		if state.Items == nil {
			state.Items = make(map[string]ProviderConfig)
		}
		return localstore.WriteSection(path, "model_providers", state)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create models config dir: %w", err)
	}
	llm = llm.Normalized()
	providers := providersForStorage(llm)
	file := modelProvidersFile{
		Version:   1,
		Default:   llm.DefaultSelector(),
		Providers: providers,
	}
	if file.Default == "" {
		file.Default = llm.Default
	}
	if file.Providers == nil {
		file.Providers = make(map[string]ProviderConfig)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode models config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write models config: %w", err)
	}
	return nil
}

func providersForStorage(llm LLMConfig) map[string]ProviderConfig {
	providers := make(map[string]ProviderConfig, len(llm.Providers))
	for name, provider := range llm.Providers {
		provider = provider.Resolved()
		provider.ReasoningEffort = ""
		providers[name] = provider
	}
	return providers
}

func llmConfigFromModelProviderState(state modelProvidersState) LLMConfig {
	providers := state.Items
	if len(providers) == 0 {
		providers = state.Providers
	}
	if providers == nil {
		providers = make(map[string]ProviderConfig)
	}
	defaultSelector := ""
	if state.DefaultModel != nil {
		defaultSelector = ModelSelector(state.DefaultModel.ModelProviderID, state.DefaultModel.ModelID)
	}
	if defaultSelector == "" {
		defaultSelector = state.Default
	}
	return LLMConfig{
		Default:        defaultSelector,
		DefaultProfile: defaultSelector,
		Providers:      providers,
		Profiles:       make(map[string]ModelConfig),
	}
}
