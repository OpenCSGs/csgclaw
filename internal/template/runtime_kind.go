package template

import (
	"strings"

	"csgclaw/internal/runtime"
)

func normalizeTemplateRuntimeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case runtime.NamePicoClaw, runtime.KindPicoClawSandbox:
		return runtime.NamePicoClaw
	case runtime.NameOpenClaw, runtime.KindOpenClawSandbox:
		return runtime.NameOpenClaw
	case "codex-sandbox", runtime.KindCodexSandbox:
		return runtime.KindCodexSandbox
	case runtime.KindCodex:
		return runtime.KindCodex
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func templateLegacyRuntimeKind(kind string) string {
	switch normalizeTemplateRuntimeKind(kind) {
	case runtime.NamePicoClaw:
		return runtime.KindPicoClawSandbox
	case runtime.NameOpenClaw:
		return runtime.KindOpenClawSandbox
	case runtime.KindCodexSandbox:
		return runtime.KindCodexSandbox
	case runtime.KindCodex:
		return runtime.KindCodex
	default:
		return ""
	}
}
