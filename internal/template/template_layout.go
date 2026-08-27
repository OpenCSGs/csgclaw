package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const requiredInstructionsFile = "AGENTS.md"
const codexSystemSkillsDirName = ".system"
const codexTemplateMemoryStagingDir = ".csgclaw-template-memory"

func materializeTemplateDir(templateRoot, runtimeKind string) (WorkspaceRef, error) {
	if !templateLayoutExists(templateRoot) && legacyTemplateWorkspaceExists(templateRoot) {
		return materializeLegacyTemplateWorkspace(templateRoot)
	}
	if strings.TrimSpace(runtimeKind) == "" {
		manifestPath := templateManifestFSPath(templateRoot)
		_, manifest, err := loadManifestFS(os.DirFS(filepath.Dir(templateRoot)), manifestPath, "template")
		if err != nil {
			return WorkspaceRef{}, err
		}
		runtimeKind = manifest.RuntimeKind
	}
	return materializeTemplateFS(os.DirFS(templateRoot), ".", runtimeKind)
}

func templateManifestFSPath(templateRoot string) string {
	return path.Join(filepath.ToSlash(filepath.Base(templateRoot)), localManifestFileName)
}

func materializeLegacyTemplateWorkspace(templateRoot string) (WorkspaceRef, error) {
	dstRoot, err := mkdirHubWorkspaceTemp("csgclaw-template-workspace-*")
	if err != nil {
		return WorkspaceRef{}, err
	}
	if err := copyWorkspaceTree(filepath.Join(templateRoot, "workspace"), dstRoot); err != nil {
		_ = os.RemoveAll(dstRoot)
		return WorkspaceRef{}, fmt.Errorf("materialize legacy template workspace: %w", err)
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: dstRoot, Temporary: true}, nil
}

func materializeTemplateFS(srcFS fs.FS, templateRoot, runtimeKind string) (WorkspaceRef, error) {
	instructionsRoot := filepath.ToSlash(filepath.Join(templateRoot, localInstructionsDirName))
	if _, err := fs.Stat(srcFS, instructionsRoot+"/"+requiredInstructionsFile); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return WorkspaceRef{}, fmt.Errorf("template instructions/%s is required", requiredInstructionsFile)
		}
		return WorkspaceRef{}, err
	}
	dstRoot, err := mkdirHubWorkspaceTemp("csgclaw-template-workspace-*")
	if err != nil {
		return WorkspaceRef{}, err
	}
	cleanup := func(err error) (WorkspaceRef, error) {
		_ = os.RemoveAll(dstRoot)
		return WorkspaceRef{}, err
	}
	if err := copyWorkspaceTreeFS(srcFS, instructionsRoot, dstRoot, "template instructions"); err != nil {
		return cleanup(err)
	}
	for _, part := range []struct{ source, target string }{
		{localSkillsDirName, localSkillsDirName},
		{localMemoriesDirName, ""},
	} {
		if part.source == localMemoriesDirName && !templateUsesWorkspaceMemory(runtimeKind) {
			continue
		}
		source := filepath.ToSlash(filepath.Join(templateRoot, part.source))
		if info, statErr := fs.Stat(srcFS, source); statErr == nil && info.IsDir() {
			target := dstRoot
			if part.target != "" {
				target = filepath.Join(dstRoot, part.target)
			}
			if err := copyWorkspaceTreeFS(srcFS, source, target, "template "+part.source); err != nil {
				return cleanup(err)
			}
		}
	}
	var memoryPath string
	if normalizeTemplateRuntimeKind(runtimeKind) == agentruntime.KindCodex {
		source := filepath.ToSlash(filepath.Join(templateRoot, localMemoriesDirName, "memory_summary.md"))
		if _, statErr := fs.Stat(srcFS, source); statErr == nil {
			memoryPath = filepath.Join(dstRoot, codexTemplateMemoryStagingDir, "memory_summary.md")
			if err := copySingleTemplateFileFS(srcFS, source, memoryPath); err != nil {
				return cleanup(err)
			}
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return cleanup(statErr)
		}
	}
	servers, err := readTemplateMCPServers(srcFS, filepath.ToSlash(filepath.Join(templateRoot, localMCPsDirName, localMCPFileName)))
	if err != nil {
		return cleanup(err)
	}
	encodedServers, err := json.Marshal(servers)
	if err != nil {
		return cleanup(fmt.Errorf("encode materialized template mcp servers: %w", err))
	}
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: dstRoot, MemoryPath: memoryPath, MCPServersJSON: string(encodedServers), Temporary: true}, nil
}

func readTemplateMCPServers(srcFS fs.FS, filePath string) (map[string]any, error) {
	data, err := fs.ReadFile(srcFS, filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("template %s/%s is required", localMCPsDirName, localMCPFileName)
		}
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode template %s: %w", filePath, err)
	}
	if wrapped, ok := raw["mcpServers"]; ok && len(raw) == 1 {
		servers, ok := wrapped.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("template %s mcpServers must be an object", filePath)
		}
		return servers, nil
	}
	return raw, nil
}

func writeTemplateLayout(workspace WorkspaceRef, templateRoot, runtimeKind string, mcpServers map[string]any) error {
	workspaceRoot := workspace.Path
	instructionsRoot := filepath.Join(templateRoot, localInstructionsDirName)
	if err := os.MkdirAll(instructionsRoot, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		source := filepath.Join(workspaceRoot, name)
		switch name {
		case requiredInstructionsFile:
			if strings.TrimSpace(workspace.InstructionsPath) != "" {
				continue
			}
			if err := copySingleTemplateFile(source, filepath.Join(instructionsRoot, name)); err != nil {
				return err
			}
		case localSkillsDirName:
			if strings.TrimSpace(workspace.SkillsPath) != "" {
				continue
			}
			if err := copyTemplateSkills(source, filepath.Join(templateRoot, localSkillsDirName), runtimeKind); err != nil {
				return err
			}
		case "memory":
			if !templateUsesWorkspaceMemory(runtimeKind) {
				continue
			}
			if err := copyWorkspaceTree(source, filepath.Join(templateRoot, localMemoriesDirName)); err != nil {
				return err
			}
		case "MEMORY.md":
			if !templateUsesWorkspaceMemory(runtimeKind) {
				continue
			}
			if err := copySingleTemplateFile(source, filepath.Join(templateRoot, localMemoriesDirName, name)); err != nil {
				return err
			}
		default:
			if entry.IsDir() {
				if err := copyWorkspaceTree(source, filepath.Join(instructionsRoot, name)); err != nil {
					return err
				}
			} else if err := copySingleTemplateFile(source, filepath.Join(instructionsRoot, name)); err != nil {
				return err
			}
		}
	}
	if source := strings.TrimSpace(workspace.InstructionsPath); source != "" {
		if err := copySingleTemplateFile(source, filepath.Join(instructionsRoot, requiredInstructionsFile)); err != nil {
			return err
		}
	}
	if source := strings.TrimSpace(workspace.SkillsPath); source != "" {
		if info, err := os.Stat(source); err == nil && info.IsDir() {
			if err := copyTemplateSkills(source, filepath.Join(templateRoot, localSkillsDirName), runtimeKind); err != nil {
				return err
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if source := strings.TrimSpace(workspace.MemoryPath); source != "" && normalizeTemplateRuntimeKind(runtimeKind) == agentruntime.KindCodex {
		if err := copySingleTemplateFile(source, filepath.Join(templateRoot, localMemoriesDirName, "memory_summary.md")); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(instructionsRoot, requiredInstructionsFile)); err != nil {
		legacy := filepath.Join(instructionsRoot, "AGENT.md")
		if _, legacyErr := os.Stat(legacy); legacyErr == nil {
			if err := os.Rename(legacy, filepath.Join(instructionsRoot, requiredInstructionsFile)); err != nil {
				return err
			}
		} else if err := os.WriteFile(filepath.Join(instructionsRoot, requiredInstructionsFile), []byte("# Agent Instructions\n"), 0o644); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(templateRoot, localMCPsDirName), 0o755); err != nil {
		return err
	}
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}
	data, err := json.MarshalIndent(mcpServers, "", "  ")
	if err != nil {
		return fmt.Errorf("encode template mcp servers: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(templateRoot, localMCPsDirName, localMCPFileName), data, 0o644)
}

func copyTemplateSkills(source, target, runtimeKind string) error {
	if normalizeTemplateRuntimeKind(runtimeKind) != agentruntime.KindCodex {
		return copyWorkspaceTree(source, target)
	}
	return copyWorkspaceTreeWithFilter(source, target, func(path string, _ fs.DirEntry) bool {
		return filepath.ToSlash(filepath.Clean(path)) == codexSystemSkillsDirName
	})
}

func templateUsesWorkspaceMemory(runtimeKind string) bool {
	return normalizeTemplateRuntimeKind(runtimeKind) != agentruntime.KindCodex
}

func copySingleTemplateFile(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrWorkspaceSymlinkDenied
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	mode := info.Mode().Perm() | 0o200
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(target, data, mode); err != nil {
		return err
	}
	return nil
}

func copySingleTemplateFileFS(sourceFS fs.FS, source, target string) error {
	data, err := fs.ReadFile(sourceFS, source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}
