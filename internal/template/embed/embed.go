package templateembed

import "embed"

const (
	Root                = ""
	ManifestFileName    = "agent.toml"
	InstructionsDirName = "instructions"
	SkillsDirName       = "skills"
	MCPsDirName         = "mcps"
	MemoriesDirName     = "memories"
	MCPFileName         = "mcp.json"
	CodexManagerRoot    = "manager/codex"
	CodexWorkerRoot     = "worker/codex"
	OpenClawWorkerRoot  = "worker/openclaw"
)

//go:embed manager/codex worker/codex worker/openclaw
var runtimeTemplateFS embed.FS
