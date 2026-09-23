package dsh

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	templateembed "csgclaw/internal/template/embed"
)

// The embedded AGENTS.md is an Agent-level default. Return it for DSH_HOME,
// and remove its workspace copy only when that copy is still the exact seed.
func embeddedWorkspaceInstructions(root string) (string, bool, error) {
	seed, err := fs.ReadFile(templateembed.FS(), path.Join(templateembed.DSHWorkerRoot, templateembed.InstructionsDirName, "AGENTS.md"))
	if err != nil {
		return "", false, fmt.Errorf("read embedded DSH instructions: %w", err)
	}
	workspacePath := filepath.Join(root, workspaceDirName, "AGENTS.md")
	info, err := os.Lstat(workspacePath)
	if errors.Is(err, os.ErrNotExist) {
		return string(seed), false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect DSH workspace instructions: %w", err)
	}
	if !info.Mode().IsRegular() {
		return string(seed), false, nil
	}
	current, err := os.ReadFile(workspacePath)
	if err != nil {
		return "", false, fmt.Errorf("read DSH workspace instructions: %w", err)
	}
	return string(seed), string(current) == string(seed), nil
}
