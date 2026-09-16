package openclawsandbox

import (
	runtimeinstructions "csgclaw/internal/runtime/instructions"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	workspaceInstructionsBlockStart = "<!-- BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit) -->"
	workspaceInstructionsBlockEnd   = "<!-- END CSGCLAW-INSTRUCTIONS -->"
)

func refreshWorkspaceAgentsFile(path, instructions string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("workspace AGENTS.md path is required")
	}
	instructions = strings.TrimSpace(instructions)
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read openclaw workspace AGENTS.md %s: %w", path, err)
	}

	block := renderWorkspaceInstructionsBlock(instructions)
	merged := mergeWorkspaceInstructionsBlock(string(current), block)
	if err == nil && string(current) == merged {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create openclaw workspace AGENTS.md dir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(merged), 0o644); err != nil {
		return fmt.Errorf("write openclaw workspace AGENTS.md %s: %w", path, err)
	}
	return nil
}

func renderWorkspaceInstructionsBlock(instructions string) string {
	sections := []string{workspaceInstructionsBlockStart}
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		sections = append(sections, "# Agent Instructions", instructions)
	}
	sections = append(sections, runtimeinstructions.RoomAttachmentInstructions("csgclaw-cli"), workspaceInstructionsBlockEnd)
	return strings.Join(sections, "\n\n") + "\n"
}

func mergeWorkspaceInstructionsBlock(current, block string) string {
	current = strings.ReplaceAll(current, "\r\n", "\n")
	block = strings.TrimRight(strings.ReplaceAll(block, "\r\n", "\n"), "\n")
	if block == "" {
		return current
	}
	if replaced, ok := replaceWorkspaceInstructionsBlock(current, block); ok {
		return replaced
	}
	if strings.TrimSpace(current) == "" {
		return block + "\n"
	}
	return joinWorkspaceInstructionsSections(current, block, "")
}

func replaceWorkspaceInstructionsBlock(current, block string) (string, bool) {
	startIdx := strings.Index(current, workspaceInstructionsBlockStart)
	if startIdx < 0 {
		return "", false
	}
	endIdx := strings.Index(current[startIdx:], workspaceInstructionsBlockEnd)
	if endIdx < 0 {
		return joinWorkspaceInstructionsSections(current[:startIdx], block, ""), true
	}
	endPos := startIdx + endIdx + len(workspaceInstructionsBlockEnd)
	return joinWorkspaceInstructionsSections(current[:startIdx], block, current[endPos:]), true
}

func joinWorkspaceInstructionsSections(parts ...string) string {
	sections := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ReplaceAll(part, "\r\n", "\n"))
		if part == "" {
			continue
		}
		sections = append(sections, part)
	}
	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n\n") + "\n"
}
