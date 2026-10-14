package taskconfig

// Config contains session choices only; credentials always come from the catalog.
type Config struct {
	ModelProviderID string `json:"model_provider_id,omitempty"`
	ModelID         string `json:"model_id,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	WorkspaceDir    string `json:"workspace_dir,omitempty"`
	ExecutionMode   string `json:"execution_mode,omitempty"`
}

func Clone(value *Config) *Config {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
