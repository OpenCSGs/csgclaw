package templateembed

import "embed"

const (
	Root               = ""
	ManifestFileName   = "agent.toml"
	WorkspaceDirName   = "workspace"
	CodexManagerRoot   = "manager/codex"
	CodexWorkerRoot    = "worker/codex"
	PicoClawWorkerRoot = "worker/picoclaw"
	OpenClawWorkerRoot = "worker/openclaw"
)

//go:embed manager/codex worker/codex worker/picoclaw worker/openclaw
var runtimeTemplateFS embed.FS
