package templates

import "embed"

const (
	Root                = "embed"
	ManifestFileName    = "agent.toml"
	WorkspaceDirName    = "workspace"
	PicoClawManagerRoot = Root + "/picoclaw-manager"
	PicoClawWorkerRoot  = Root + "/picoclaw-worker"
	OpenClawManagerRoot = Root + "/openclaw-manager"
	OpenClawWorkerRoot  = Root + "/openclaw-worker"
)

//go:embed embed/picoclaw-manager embed/picoclaw-worker embed/openclaw-manager embed/openclaw-worker
var runtimeTemplateFS embed.FS
