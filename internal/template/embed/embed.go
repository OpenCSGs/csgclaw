package templateembed

import "embed"

const (
	Root                = ""
	ManifestFileName    = "agent.toml"
	WorkspaceDirName    = "workspace"
	PicoClawManagerRoot = "manager/picoclaw"
	PicoClawWorkerRoot  = "worker/picoclaw"
	OpenClawManagerRoot = "manager/openclaw"
	OpenClawWorkerRoot  = "worker/openclaw"
)

//go:embed manager/picoclaw worker/picoclaw manager/openclaw worker/openclaw
var runtimeTemplateFS embed.FS
