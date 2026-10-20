package apps

import (
	"context"
	"slices"
)

type CallableTool struct {
	Name        string
	Description string
}

// CallableTools 读取当前 Agent 已注册的目录，沿用 gateway 的权限和名称。
func (s *Service) CallableTools(ctx context.Context, agentID, installationID string) ([]CallableTool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.refreshReadOnly(agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked(agentID, installationID)
	if err != nil {
		return nil, err
	}
	result := []CallableTool{}
	if !e.record.active() || e.record.Disconnected || e.connection == nil {
		return result, nil
	}
	for _, upstream := range e.connection.tools {
		name := toolName(installationID, upstream.Name)
		if slices.Contains(e.connection.toolNames, name) {
			result = append(result, CallableTool{Name: name, Description: upstream.Description})
		}
	}
	return result, nil
}
