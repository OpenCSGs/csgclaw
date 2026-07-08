package templateembed

import "embed"

const (
	Root               = ""
	ManifestFileName   = "agent.toml"
	WorkspaceDirName   = "workspace"
	CodexManagerRoot   = "manager/codex"
	PicoClawWorkerRoot = "worker/picoclaw"
	OpenClawWorkerRoot = "worker/openclaw"
)

//go:embed manager/codex worker/picoclaw worker/openclaw
var runtimeTemplateFS embed.FS
