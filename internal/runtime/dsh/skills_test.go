package dsh

import (
	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillProjectionPreservesFilesAndPermissions(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "home", "skills")
	for _, name := range []string{"reviewer", "writer"} {
		dir := filepath.Join(source, name, "scripts")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, name, "SKILL.md"), []byte("说明"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, enabled := range []bool{false, true, false} {
		if err := writeRuntimePatch(filepath.Join(root, patchFileName), agentruntime.Profile{}); err != nil {
			t.Fatal(err)
		}
		if err := projectSkills(root, source, map[string]skill.State{"reviewer": {Enabled: enabled}}); err != nil {
			t.Fatal(err)
		}
		_, err := os.Stat(filepath.Join(root, "skill-view", "skills", "reviewer"))
		if enabled && err != nil || !enabled && !os.IsNotExist(err) {
			t.Fatalf("启用状态 %v：%v", enabled, err)
		}
		for _, base := range []string{source, filepath.Join(root, "skill-view", "skills")} {
			info, err := os.Stat(filepath.Join(base, "writer", "scripts", "run.sh"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0755 {
				t.Fatalf("执行权限改变：%v", info.Mode())
			}
		}
		if _, err := os.Stat(filepath.Join(source, "reviewer", "scripts", "run.sh")); err != nil {
			t.Fatal(err)
		}
		patch, err := os.ReadFile(filepath.Join(root, contextPatchFileName))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(patch), "id: skill-filesystem") != 1 || !strings.Contains(string(patch), filepath.Join(root, "skill-view")) {
			t.Fatalf("Skill 目录配置错误：%s", patch)
		}
	}
}

func TestDisabledMCPExcludedFromACPSessions(t *testing.T) {
	servers, err := buildACPMCPServers(map[string]any{
		"disabled": map[string]any{"command": "relative-command", "enabled": false},
		"active":   map[string]any{"url": "https://example.com/mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Name != "active" {
		t.Fatalf("ACP 使用了禁用的 MCP：%v", servers)
	}
}
