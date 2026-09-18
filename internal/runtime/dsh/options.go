package dsh

import (
	"fmt"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const executablePathOption = "executable_path"

type RuntimeOptions struct {
	ExecutablePath string
}

func DecodeRuntimeOptions(raw map[string]any) (RuntimeOptions, error) {
	var out RuntimeOptions
	if raw == nil {
		return out, nil
	}
	value, ok := raw[executablePathOption]
	if !ok || value == nil {
		return out, nil
	}
	text, ok := value.(string)
	if !ok {
		return out, fmt.Errorf("%s must be a string", executablePathOption)
	}
	out.ExecutablePath = strings.TrimSpace(text)
	return out, nil
}

func (r *Runtime) RuntimeOptionsSchema() []agentruntime.RuntimeOptionSchema {
	return []agentruntime.RuntimeOptionSchema{{
		Key:           executablePathOption,
		Path:          executablePathOption,
		Label:         "DSH Executable",
		LabelZh:       "DSH 可执行文件",
		LabelEn:       "DSH Executable",
		Description:   "Leave empty to use CSGCLAW_DSH_PATH or dsh from PATH.",
		DescriptionZh: "留空时依次使用 CSGCLAW_DSH_PATH 或 PATH 中的 dsh。",
		DescriptionEn: "Leave empty to use CSGCLAW_DSH_PATH or dsh from PATH.",
		Type:          "string",
	}}
}
