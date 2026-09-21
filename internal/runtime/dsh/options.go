package dsh

import (
	"fmt"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const (
	executablePathOption           = "executable_path"
	PermissionModeOptionKey        = "permission_mode"
	PermissionModeReadOnly         = "read-only"
	PermissionModeWorkspaceWrite   = "workspace-write"
	PermissionModeDangerFullAccess = "danger-full-access"
	defaultPermissionMode          = PermissionModeWorkspaceWrite
)

type RuntimeOptions struct {
	ExecutablePath string
	PermissionMode string
}

func DecodeRuntimeOptions(raw map[string]any) (RuntimeOptions, error) {
	out := RuntimeOptions{PermissionMode: defaultPermissionMode}
	if raw == nil {
		return out, nil
	}
	if value, ok := raw[executablePathOption]; ok && value != nil {
		text, ok := value.(string)
		if !ok {
			return RuntimeOptions{}, fmt.Errorf("%s must be a string", executablePathOption)
		}
		out.ExecutablePath = strings.TrimSpace(text)
	}
	if value, ok := raw[PermissionModeOptionKey]; ok && value != nil {
		text, ok := value.(string)
		if !ok {
			return RuntimeOptions{}, fmt.Errorf("%s must be a string", PermissionModeOptionKey)
		}
		mode := strings.ToLower(strings.TrimSpace(text))
		if mode != "" {
			switch mode {
			case PermissionModeReadOnly, PermissionModeWorkspaceWrite, PermissionModeDangerFullAccess:
				out.PermissionMode = mode
			default:
				return RuntimeOptions{}, fmt.Errorf("%s must be %q, %q, or %q", PermissionModeOptionKey, PermissionModeReadOnly, PermissionModeWorkspaceWrite, PermissionModeDangerFullAccess)
			}
		}
	}
	return out, nil
}

func IsReadOnlyPermissionMode(raw map[string]any) bool {
	opts, err := DecodeRuntimeOptions(raw)
	return err == nil && opts.PermissionMode == PermissionModeReadOnly
}

func (r *Runtime) RuntimeOptionsSchema() []agentruntime.RuntimeOptionSchema {
	return []agentruntime.RuntimeOptionSchema{{
		Key:           PermissionModeOptionKey,
		Path:          PermissionModeOptionKey,
		Label:         "Permission Mode",
		LabelZh:       "运行模式",
		LabelEn:       "Permission Mode",
		Description:   "Controls which local files DSH may modify.",
		DescriptionZh: "控制 DSH 可以修改的本地文件范围。",
		DescriptionEn: "Controls which local files DSH may modify.",
		Type:          "select",
		Options:       []string{PermissionModeWorkspaceWrite, PermissionModeReadOnly, PermissionModeDangerFullAccess},
		Choices: []agentruntime.RuntimeOptionChoice{
			{
				Value:         PermissionModeWorkspaceWrite,
				Label:         "Workspace mode",
				LabelZh:       "工作区模式",
				LabelEn:       "Workspace mode",
				Description:   "Can modify this Agent's automatically managed workspace and temporary files; broader access requires approval.",
				DescriptionZh: "可修改系统自动管理的当前 Agent 工作区和临时文件；更大范围的访问需要确认，无需额外设置工作区。",
				DescriptionEn: "Can modify this Agent's automatically managed workspace and temporary files; broader access requires approval.",
			},
			{
				Value:         PermissionModeReadOnly,
				Label:         "Read-only mode",
				LabelZh:       "只读模式",
				LabelEn:       "Read-only mode",
				Description:   "Can read local files but cannot modify them; operations requiring broader access ask for approval.",
				DescriptionZh: "可读取本地文件，但不能修改；需要更高权限的操作会请求确认。",
				DescriptionEn: "Can read local files but cannot modify them; operations requiring broader access ask for approval.",
			},
			{
				Value:         PermissionModeDangerFullAccess,
				Label:         "Full access",
				LabelZh:       "完全访问",
				LabelEn:       "Full access",
				Description:   "Can modify files outside the Agent workspace without approval. Use only for trusted tasks.",
				DescriptionZh: "可直接修改 Agent 工作区之外的本机文件，且不会请求确认；仅用于可信任务。",
				DescriptionEn: "Can modify files outside the Agent workspace without approval. Use only for trusted tasks.",
			},
		},
		DefaultValue: defaultPermissionMode,
		Presentation: "dsh_permission_mode",
	}}
}
