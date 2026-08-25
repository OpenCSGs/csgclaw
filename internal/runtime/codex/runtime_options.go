package codex

import (
	"fmt"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const (
	localWorkspaceDirOptionKey = "local_workspace_dir"
	executionModeOptionKey     = "execution_mode"
	memoryModeOptionKey        = "memory_mode"
	ExecutionModeStandard      = "standard"
	ExecutionModeReadOnly      = "read_only"
	MemoryModeEnabled          = "enabled"
	MemoryModeDisabled         = "disabled"
)

type RuntimeOptions struct {
	LocalWorkspaceDir string `json:"local_workspace_dir"`
	ExecutionMode     string `json:"execution_mode"`
	MemoryMode        string `json:"memory_mode"`
}

func DecodeRuntimeOptions(raw map[string]any) (RuntimeOptions, error) {
	if len(raw) == 0 {
		return defaultRuntimeOptions(), nil
	}
	opts := defaultRuntimeOptions()
	if value, ok := raw[localWorkspaceDirOptionKey]; ok && value != nil {
		text, ok := value.(string)
		if !ok {
			return RuntimeOptions{}, fmt.Errorf("%s must be a string", localWorkspaceDirOptionKey)
		}
		opts.LocalWorkspaceDir = strings.TrimSpace(text)
	}
	if value, ok := raw[executionModeOptionKey]; ok && value != nil {
		text, ok := value.(string)
		if !ok {
			return RuntimeOptions{}, fmt.Errorf("%s must be a string", executionModeOptionKey)
		}
		mode := strings.ToLower(strings.TrimSpace(text))
		if mode != "" {
			switch mode {
			case ExecutionModeStandard, ExecutionModeReadOnly:
				opts.ExecutionMode = mode
			default:
				return RuntimeOptions{}, fmt.Errorf("%s must be %q or %q", executionModeOptionKey, ExecutionModeStandard, ExecutionModeReadOnly)
			}
		}
	}
	if value, ok := raw[memoryModeOptionKey]; ok && value != nil {
		text, ok := value.(string)
		if !ok {
			return RuntimeOptions{}, fmt.Errorf("%s must be a string", memoryModeOptionKey)
		}
		mode := strings.ToLower(strings.TrimSpace(text))
		if mode != "" {
			switch mode {
			case MemoryModeEnabled, MemoryModeDisabled:
				opts.MemoryMode = mode
			default:
				return RuntimeOptions{}, fmt.Errorf("%s must be %q or %q", memoryModeOptionKey, MemoryModeEnabled, MemoryModeDisabled)
			}
		}
	}
	return opts, nil
}

func defaultRuntimeOptions() RuntimeOptions {
	return RuntimeOptions{
		ExecutionMode: ExecutionModeStandard,
		MemoryMode:    MemoryModeEnabled,
	}
}

func IsReadOnlyExecutionMode(raw map[string]any) bool {
	opts, err := DecodeRuntimeOptions(raw)
	return err == nil && opts.ExecutionMode == ExecutionModeReadOnly
}

func ResolveWorkspaceDir(agentHome string, raw map[string]any) (string, error) {
	agentHome = strings.TrimSpace(agentHome)
	if agentHome == "" {
		return "", fmt.Errorf("agent home is required")
	}
	opts, err := DecodeRuntimeOptions(raw)
	if err != nil {
		return "", err
	}
	if opts.LocalWorkspaceDir != "" {
		return opts.LocalWorkspaceDir, nil
	}
	return filepath.Join(agentHome, filepath.FromSlash(hostStateDirName), workspaceDirName), nil
}

func (r *Runtime) RuntimeOptionsSchema() []agentruntime.RuntimeOptionSchema {
	return []agentruntime.RuntimeOptionSchema{
		{
			Key:           executionModeOptionKey,
			Path:          executionModeOptionKey,
			Label:         "Execution Mode",
			LabelZh:       "运行模式",
			LabelEn:       "Execution Mode",
			Description:   "Standard mode can act on data; read-only mode can only analyze provided content and approved read-only data sources.",
			DescriptionZh: "标准模式可操作数据；只读模式仅可分析已提供内容并查询允许的只读数据源。",
			DescriptionEn: "Standard mode can act on data; read-only mode can only analyze provided content and approved read-only data sources.",
			Type:          "select",
			Options:       []string{ExecutionModeStandard, ExecutionModeReadOnly},
			DefaultValue:  ExecutionModeStandard,
		},
		{
			Key:           localWorkspaceDirOptionKey,
			Path:          localWorkspaceDirOptionKey,
			Label:         "Local Workspace Dir",
			LabelZh:       "本地工作目录",
			LabelEn:       "Local Workspace Dir",
			Description:   "Leave empty to use the default agent workspace.",
			DescriptionZh: "留空时使用默认 Agent 工作目录。",
			DescriptionEn: "Leave empty to use the default agent workspace.",
			Type:          "directory",
			Picker:        "optional",
		},
	}
}
