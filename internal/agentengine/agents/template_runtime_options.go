package agents

import (
	"fmt"
	"strings"

	runtimedsh "csgclaw/internal/runtime/dsh"
)

const (
	templateExecutionModeKey      = "execution_mode"
	templateExecutionModeStandard = "standard"
	templateExecutionModeReadOnly = "read_only"
	templateMemoryModeKey         = "memory_mode"
	templateMemoryModeEnabled     = "enabled"
	templateMemoryModeDisabled    = "disabled"
)

func templateSafeRuntimeOptions(item Agent) (map[string]any, error) {
	if item.ID == ManagerUserID || item.Role == RoleManager {
		return nil, nil
	}
	if strings.TrimSpace(item.RuntimeKind) == RuntimeKindDSH {
		opts, err := runtimedsh.DecodeRuntimeOptions(item.RuntimeOptions)
		if err != nil {
			return nil, fmt.Errorf("agent %q runtime_options.permission_mode is invalid: %w", item.ID, err)
		}
		return map[string]any{runtimedsh.PermissionModeOptionKey: opts.PermissionMode}, nil
	}
	if strings.TrimSpace(item.RuntimeKind) != RuntimeKindCodex {
		return nil, nil
	}
	mode := templateExecutionModeStandard
	if raw, ok := item.RuntimeOptions[templateExecutionModeKey]; ok && raw != nil {
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("agent %q runtime_options.execution_mode must be a string", item.ID)
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			mode = value
		}
	}
	if mode != templateExecutionModeStandard && mode != templateExecutionModeReadOnly {
		return nil, fmt.Errorf("agent %q runtime_options.execution_mode must be %q or %q", item.ID, templateExecutionModeStandard, templateExecutionModeReadOnly)
	}
	memoryMode := templateMemoryModeEnabled
	if raw, ok := item.RuntimeOptions[templateMemoryModeKey]; ok && raw != nil {
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("agent %q runtime_options.memory_mode must be a string", item.ID)
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			memoryMode = value
		}
	}
	if memoryMode != templateMemoryModeEnabled && memoryMode != templateMemoryModeDisabled {
		return nil, fmt.Errorf("agent %q runtime_options.memory_mode must be %q or %q", item.ID, templateMemoryModeEnabled, templateMemoryModeDisabled)
	}
	return map[string]any{
		templateExecutionModeKey: mode,
		templateMemoryModeKey:    memoryMode,
	}, nil
}
