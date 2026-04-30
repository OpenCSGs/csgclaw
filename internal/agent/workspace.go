package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	workspaceTemplateManager = "embed/runtimes/picoclaw/manager/workspace"
	workspaceTemplateWorker  = "embed/runtimes/picoclaw/worker/workspace"
)

func workspaceTemplateForAgent(name, botID string) string {
	if strings.EqualFold(strings.TrimSpace(name), ManagerName) || strings.TrimSpace(botID) == ManagerUserID {
		return workspaceTemplateManager
	}
	return workspaceTemplateWorker
}

func ensureAgentWorkspace(agentName, template string) (string, error) {
	hostRoot, err := agentWorkspaceRoot(agentName)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(template) == "" {
		return "", fmt.Errorf("workspace template is required")
	}
	if err := os.MkdirAll(hostRoot, 0o755); err != nil {
		return "", fmt.Errorf("create agent workspace dir: %w", err)
	}
	if err := copyEmbeddedWorkspace(template, hostRoot); err != nil {
		return "", err
	}
	return hostRoot, nil
}

func agentWorkspaceRoot(agentName string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, hostWorkspaceDir), nil
}

func copyEmbeddedWorkspace(template, dstRoot string) error {
	template = strings.Trim(strings.TrimSpace(template), "/")
	return fs.WalkDir(workspaceTemplateFS, template, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk embedded workspace %q: %w", template, walkErr)
		}
		rel := strings.TrimPrefix(path, template)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		dst := filepath.Join(dstRoot, filepath.FromSlash(rel))
		if d.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create workspace dir %q: %w", dst, err)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("read embedded workspace file info %q: %w", path, err)
		}
		data, err := fs.ReadFile(workspaceTemplateFS, path)
		if err != nil {
			return fmt.Errorf("read embedded workspace file %q: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create workspace parent %q: %w", filepath.Dir(dst), err)
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		mode |= 0o200
		if err := os.WriteFile(dst, data, mode); err != nil {
			return fmt.Errorf("write workspace file %q: %w", dst, err)
		}
		return nil
	})
}
