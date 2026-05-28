package skill

import "errors"

var (
	ErrSkillNotFound       = errors.New("skill not found")
	ErrRegistryUnavailable = errors.New("skill registry API not found or not deployed")
	ErrSkillBlocked        = errors.New("skill is blocked or not installable")
	ErrSkillDirExists      = errors.New("skill directory already exists")
	ErrWorkspacePathUnsafe = errors.New("skill archive path is unsafe")
	ErrSkillArchiveEmpty   = errors.New("skill archive is empty")
	ErrSKILLMDMissing      = errors.New("skill archive must contain SKILL.md")
)
