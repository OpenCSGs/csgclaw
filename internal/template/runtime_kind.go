package template

import (
	"strings"

	"csgclaw/internal/runtime"
)

func normalizeTemplateRuntimeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case runtime.NamePicoClaw:
		return runtime.NamePicoClaw
	case runtime.NameOpenClaw:
		return runtime.NameOpenClaw
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
	case runtime.KindCodex:
		return runtime.KindCodex
	default:
		return ""
	}
}
