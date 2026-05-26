package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/picoclawsandbox"
)

// ResolveSkillsRoot returns the local workspace skills directory for the current
// runtime environment (the sandbox where csgclaw-cli is executed).
func ResolveSkillsRoot(override string) (string, error) {
	if dir := strings.TrimSpace(override); dir != "" {
		return filepath.Clean(dir), nil
	}
	if dir := strings.TrimSpace(os.Getenv("CSGCLAW_SKILLS_DIR")); dir != "" {
		return filepath.Clean(dir), nil
	}
	for _, candidate := range localSkillsRootCandidates() {
		if workspaceSkillsRootUsable(candidate) {
			return candidate, nil
		}
	}
	root := filepath.Join(picoclawsandbox.BoxWorkspaceDir, "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create skills directory %q: %w", root, err)
	}
	return root, nil
}

func localSkillsRootCandidates() []string {
	candidates := []string{
		filepath.Join(picoclawsandbox.BoxWorkspaceDir, "skills"),
		filepath.Join(openclawsandbox.BoxWorkspaceDir, "skills"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".picoclaw", "workspace", "skills"),
			filepath.Join(home, ".openclaw", "workspace", "skills"),
		)
	}
	return candidates
}

func workspaceSkillsRootUsable(skillsRoot string) bool {
	workspace := filepath.Dir(skillsRoot)
	info, err := os.Stat(workspace)
	return err == nil && info.IsDir()
}
