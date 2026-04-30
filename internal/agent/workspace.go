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
	if err := migrateLegacyAgentWorkspace(agentName, hostRoot); err != nil {
		return "", err
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
	picoClawRoot, err := agentPicoClawRoot(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(picoClawRoot, hostWorkspaceDir), nil
}

func legacyAgentWorkspaceRoot(agentName string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, hostWorkspaceDir), nil
}

func migrateLegacyAgentWorkspace(agentName, dstRoot string) error {
	srcRoot, err := legacyAgentWorkspaceRoot(agentName)
	if err != nil {
		return err
	}
	if srcRoot == dstRoot {
		return nil
	}
	if _, err := os.Lstat(srcRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect legacy agent workspace: %w", err)
	}
	if _, err := os.Lstat(dstRoot); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect agent workspace: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(dstRoot), 0o755); err != nil {
			return fmt.Errorf("create agent workspace parent: %w", err)
		}
		if err := os.Rename(srcRoot, dstRoot); err != nil {
			return fmt.Errorf("move legacy agent workspace: %w", err)
		}
		return nil
	}
	if err := mergeLegacyWorkspace(srcRoot, dstRoot); err != nil {
		return err
	}
	return nil
}

func mergeLegacyWorkspace(srcRoot, dstRoot string) error {
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return fmt.Errorf("read legacy workspace: %w", err)
	}
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		return fmt.Errorf("create merged workspace: %w", err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcRoot, entry.Name())
		dstPath := filepath.Join(dstRoot, entry.Name())
		dstInfo, err := os.Lstat(dstPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("inspect merged workspace path: %w", err)
			}
			if err := os.Rename(srcPath, dstPath); err != nil {
				return fmt.Errorf("move legacy workspace path: %w", err)
			}
			continue
		}
		if entry.IsDir() && dstInfo.IsDir() {
			if err := mergeLegacyWorkspace(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		legacyPath := filepath.Join(dstRoot, ".legacy-workspace", entry.Name())
		if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
			return fmt.Errorf("create legacy workspace archive: %w", err)
		}
		if err := os.Rename(srcPath, legacyPath); err != nil {
			return fmt.Errorf("archive legacy workspace path: %w", err)
		}
	}
	if err := os.Remove(srcRoot); err != nil {
		return fmt.Errorf("remove legacy workspace dir: %w", err)
	}
	return nil
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
