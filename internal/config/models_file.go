package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const ModelsFileName = "models.json"

type modelProvidersFile struct {
	Version   int                       `json:"version"`
	Default   string                    `json:"default,omitempty"`
	Providers map[string]ProviderConfig `json:"providers"`
}

func DefaultModelsPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ModelsFileName), nil
}

func ModelsPathForConfigPath(configPath string) (string, error) {
	if configPath == "" {
		return DefaultModelsPath()
	}
	return filepath.Join(filepath.Dir(configPath), ModelsFileName), nil
}

func LoadModels(path string) (LLMConfig, bool, error) {
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create models config dir: %w", err)
	}
	llm = llm.Normalized()
	providers := make(map[string]ProviderConfig, len(llm.Providers))
	for name, provider := range llm.Providers {
		provider = provider.Resolved()
		provider.ReasoningEffort = ""
		providers[name] = provider
	}
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
