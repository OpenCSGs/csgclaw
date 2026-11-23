package dsh

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	agentruntime "csgclaw/internal/runtime"
	runtimeworkspace "csgclaw/internal/runtime/workspace"
	"csgclaw/internal/skill/installstate"
	skill "csgclaw/internal/skill/state"
)

func (r *Runtime) ReconcileSkills(_ context.Context, h agentruntime.Handle, states map[string]skill.State) error {
	ref, err := r.deps.ResolveAgent(h)
	if err != nil {
		return err
	}
	home, err := r.deps.AgentHome(ref.ID)
	if err != nil {
		return err
	}
	layout, err := r.ensureRuntimeDirs(home)
	if err != nil {
		return err
	}
	runtimeRoot := filepath.Dir(filepath.Dir(layout.SkillsRoot))
	// A CLI may be replacing files while the worker is running. Project a
	// single committed installation snapshot, not a mix of two versions.
	return installstate.ReadSnapshot(layout.SkillsRoot, func(_ map[string]installstate.Record) error {
		return projectSkills(runtimeRoot, layout.SkillsRoot, states)
	})
}

// projectSkills 在完整文件目录之外生成 DSH 使用的 Skill 目录。
// 调用者与其他配置写入互斥；更新磁盘不要求中断当前会话。
func projectSkills(root, source string, states map[string]skill.State) error {
	entries, err := os.ReadDir(source)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.MkdirTemp(root, ".skill-view-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	targetSkills := filepath.Join(stage, "skills")
	if err := os.MkdirAll(targetSkills, 0o700); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !skill.Enabled(states, entry.Name()) {
			continue
		}
		if _, err := os.Stat(filepath.Join(source, entry.Name(), "SKILL.md")); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := runtimeworkspace.OverlayWorkspaceTree(filepath.Join(source, entry.Name()), filepath.Join(targetSkills, entry.Name())); err != nil {
			return err
		}
	}
	target := filepath.Join(root, "skill-view")
	backup := filepath.Join(root, ".skill-view-backup")
	// 上次进程终止在目录替换中间时，恢复可读取的目录。
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(backup); err == nil {
			if err := os.Rename(backup, target); err != nil {
				return err
			}
		}
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	previous := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		previous = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		if previous {
			return errors.Join(err, os.Rename(backup, target))
		}
		return err
	}
	return os.RemoveAll(backup)
}
