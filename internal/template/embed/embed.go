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
	DSHWorkerRoot       = "worker/dsh"
)

//go:embed manager/codex worker/codex worker/dsh
var runtimeTemplateFS embed.FS
