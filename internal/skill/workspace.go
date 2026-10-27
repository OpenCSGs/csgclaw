package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSkillsRoot returns the local workspace skills directory for the current
// runtime environment in which csgclaw-cli is executed.
func ResolveSkillsRoot(override string) (string, error) {
	if dir := strings.TrimSpace(override); dir != "" {
		return filepath.Clean(dir), nil
	}
	if dir := strings.TrimSpace(os.Getenv("CSGCLAW_SKILLS_DIR")); dir != "" {
		return filepath.Clean(dir), nil
	}
	for _, key := range []string{"DSH_HOME", "CODEX_HOME"} {
		if home := strings.TrimSpace(os.Getenv(key)); home != "" {
			return filepath.Join(home, "skills"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	root := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create skills directory %q: %w", root, err)
	}
	return root, nil
}
