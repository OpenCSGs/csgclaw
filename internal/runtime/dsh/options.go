package dsh

import (
	"fmt"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const (
	PermissionModeOptionKey        = "permission_mode"
	localWorkspaceDirOptionKey     = "local_workspace_dir"
	PermissionModeReadOnly         = "read-only"
	PermissionModeWorkspaceWrite   = "workspace-write"
	PermissionModeDangerFullAccess = "danger-full-access"
	defaultPermissionMode          = PermissionModeWorkspaceWrite
)

type RuntimeOptions struct {
	AutoCompact       bool
	PermissionMode    string
	LocalWorkspaceDir string
}

func DecodeRuntimeOptions(raw map[string]any) (RuntimeOptions, error) {
	out := RuntimeOptions{AutoCompact: true, PermissionMode: defaultPermissionMode}
	if raw == nil {
		return out, nil
	}
	if value, ok := raw["auto_compact"]; ok && value != nil {
		if value != "enabled" && value != "disabled" {
			return out, fmt.Errorf("auto_compact must be enabled or disabled")
		}
		out.AutoCompact = value == "enabled"
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
	if value, ok := raw[localWorkspaceDirOptionKey]; ok && value != nil {
		path, ok := value.(string)
		if !ok {
			return RuntimeOptions{}, fmt.Errorf("%s must be a string", localWorkspaceDirOptionKey)
		}
		path = strings.TrimSpace(path)
		if path != "" && !filepath.IsAbs(path) {
			return RuntimeOptions{}, fmt.Errorf("%s must be an absolute path", localWorkspaceDirOptionKey)
		}
		out.LocalWorkspaceDir = path
	}
	return out, nil
}

func ResolveWorkspaceDir(agentHome string, raw map[string]any) (string, error) {
	if strings.TrimSpace(agentHome) == "" {
		return "", fmt.Errorf("agent home is required")
	}
	opts, err := DecodeRuntimeOptions(raw)
	if err != nil {
		return "", err
	}
	if opts.LocalWorkspaceDir != "" {
		return filepath.Clean(opts.LocalWorkspaceDir), nil
	}
	return filepath.Join(agentHome, hostStateDirName, workspaceDirName), nil
}

func IsReadOnlyPermissionMode(raw map[string]any) bool {
	opts, err := DecodeRuntimeOptions(raw)
	return err == nil && opts.PermissionMode == PermissionModeReadOnly
}

func (r *Runtime) RuntimeOptionsSchema() []agentruntime.RuntimeOptionSchema {
	return []agentruntime.RuntimeOptionSchema{
		{
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
					Label:         "Workspace write",
					LabelZh:       "工作区内修改",
					LabelEn:       "Workspace write",
					Description:   "Can modify the selected workspace and temporary files; broader access requires approval.",
					DescriptionZh: "可修改选定的工作目录和临时文件；更大范围的访问需要确认。",
					DescriptionEn: "Can modify the selected workspace and temporary files; broader access requires approval.",
				},
				{
					Value:         PermissionModeReadOnly,
					Label:         "View only",
					LabelZh:       "仅可查看",
					LabelEn:       "View only",
					Description:   "Can read local files but cannot modify them; operations requiring broader access ask for approval.",
					DescriptionZh: "可读取本地文件，但不能修改；需要更高权限的操作会请求确认。",
					DescriptionEn: "Can read local files but cannot modify them; operations requiring broader access ask for approval.",
				},
				{
					Value:         PermissionModeDangerFullAccess,
					Label:         "Full access",
					LabelZh:       "完全权限",
					LabelEn:       "Full access",
					Description:   "Can modify any local path visible to the DSH process without requesting file access approval.",
					DescriptionZh: "可修改 DSH 进程能访问的任意本地路径，无需逐次申请文件访问权限。",
					DescriptionEn: "Can modify any local path visible to the DSH process without requesting file access approval.",
				},
			},
			DefaultValue: defaultPermissionMode,
			Presentation: "dsh_permission_mode",
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
