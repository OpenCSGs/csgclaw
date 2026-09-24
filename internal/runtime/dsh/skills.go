package dsh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/sandboxgateway"
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
	root := filepath.Dir(layout.SkillsRoot)
	runtimeRoot := filepath.Dir(root)
	if err := writeRuntimePatch(filepath.Join(runtimeRoot, patchFileName), ref.Profile); err != nil {
		return err
	}
	return projectSkills(runtimeRoot, layout.SkillsRoot, states)
}

// projectSkills 在完整文件目录之外生成 DSH 使用的 Skill 目录。
// 调用者持有生命周期控制权，启动前完成目录替换。
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
		if err := sandboxgateway.OverlayWorkspaceTree(filepath.Join(source, entry.Name()), filepath.Join(targetSkills, entry.Name())); err != nil {
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
	patch := fmt.Sprintf("\n- id: skill-filesystem\n  config:\n    dshHome: %q\n    includeDefaultRoots: true\n", target)
	file, err := os.OpenFile(filepath.Join(root, contextPatchFileName), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(patch)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.RemoveAll(backup)
}
