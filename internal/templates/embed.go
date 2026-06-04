package templates

import "embed"

const (
	Root                = "embed"
	ManifestFileName    = "agent.toml"
	WorkspaceDirName    = "workspace"
	PicoClawDistDirName = "dist"
	PicoClawManagerRoot = Root + "/picoclaw-manager/" + PicoClawDistDirName
	PicoClawWorkerRoot  = Root + "/picoclaw-worker/" + PicoClawDistDirName
	OpenClawManagerRoot = Root + "/openclaw-manager"
	OpenClawWorkerRoot  = Root + "/openclaw-worker"
)

//go:embed embed/picoclaw-manager/dist embed/picoclaw-worker/dist embed/openclaw-manager embed/openclaw-worker
var runtimeTemplateFS embed.FS
