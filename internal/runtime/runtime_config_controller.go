package runtime

import "context"

type RuntimeProfileConfig struct {
	Provider        string
	BaseURL         string
	APIKey          string
	ModelID         string
	ReasoningEffort string
	Headers         map[string]string
	RequestOptions  map[string]any
}

type RuntimeConfigSnapshot struct {
	Profile RuntimeProfileConfig
	Options map[string]any
}

type RuntimeConfigChange struct {
	Previous RuntimeConfigSnapshot
	Current  RuntimeConfigSnapshot
}

type RuntimeConfigController interface {
	ValidateConfig(ctx context.Context, current RuntimeConfigSnapshot) error
	RestartRequired(change RuntimeConfigChange) (bool, error)
	ReconcileConfig(ctx context.Context, h Handle, change RuntimeConfigChange) error
}

type RuntimeStartConfigValidator interface {
	ValidateStartConfig(ctx context.Context, current RuntimeConfigSnapshot) error
}
