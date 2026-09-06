package agents

import "csgclaw/internal/config"

// ModelConfiguration resolves desired model profiles and provider credentials.
// It reads Repository snapshots but never starts or controls a Runtime.
type ModelConfiguration struct {
	*Repository
	model config.ModelConfig
	llm   config.LLMConfig
}

func (s *Controller) Models() *ModelConfiguration {
	if s == nil {
		return nil
	}
	s.bindResources()
	return &s.ModelConfiguration
}
