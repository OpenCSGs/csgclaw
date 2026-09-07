package agents

import agentruntime "csgclaw/internal/runtime"

// RuntimeOptionsSchema exposes metadata, never the mutable Runtime object.
func (s *Controller) RuntimeOptionsSchema(kind string) []agentruntime.RuntimeOptionSchema {
	rt, err := s.runtimeForKind(kind)
	if err != nil {
		return nil
	}
	provider, ok := rt.(agentruntime.RuntimeOptionSchemaProvider)
	if !ok {
		return nil
	}
	return append([]agentruntime.RuntimeOptionSchema(nil), provider.RuntimeOptionsSchema()...)
}
