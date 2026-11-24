package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
	"csgclaw/internal/utils/filebrowse"
	toml "github.com/pelletier/go-toml/v2"
)

const skillConfigBegin = "# BEGIN CSGCLAW SKILL STATES"
const skillConfigEnd = "# END CSGCLAW SKILL STATES"

func (r *Runtime) SkillDiskSync(layout agentruntime.Layout, name string, enabled bool, options map[string]any) (string, error) {
	opts, err := conversationRuntimeOptions(options)
	if err != nil {
		return "unknown", err
	}
	if opts.ExecutionMode == ExecutionModeReadOnly && enabled {
		return "restricted", nil
	}
	var cfg struct {
		Skills struct {
			Config []struct {
				Path    string
				Enabled *bool
			}
		}
	}
	r.configMu.Lock()
	defer r.configMu.Unlock()
	f, err := filebrowse.OpenFile(filepath.Dir(layout.SkillsRoot), configFileName)
	if err == nil {
		defer f.Close()
		const maxSize = 4 << 20
		data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
		if err != nil {
			return "unknown", err
		}
		if len(data) > maxSize {
			return "unknown", fmt.Errorf("Codex configuration too large")
		}
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return "unknown", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "unknown", err
	}
	actual := true
	for _, s := range cfg.Skills.Config {
		if filepath.Clean(s.Path) == filepath.Join(layout.SkillsRoot, name, "SKILL.md") && s.Enabled != nil && !*s.Enabled {
			actual = false
		}
	}
	if actual != enabled {
		return "pending", nil
	}
	return "in_sync", nil
}

func (r *Runtime) ReconcileSkills(_ context.Context, h agentruntime.Handle, states map[string]skill.State) error {
	return r.reconcileSkills(h, states, "")
}

func (r *Runtime) EnableInstalledSkill(_ context.Context, h agentruntime.Handle, name string, states map[string]skill.State) error {
	return r.reconcileSkills(h, states, name)
}

func (r *Runtime) reconcileSkills(h agentruntime.Handle, states map[string]skill.State, name string) error {
	ref, err := r.resolveAgent(h)
	if err != nil {
		return err
	}
	home, err := r.resolveCodexHomeDir(ref.ID)
	if err != nil {
		return err
	}
	opts, err := conversationRuntimeOptions(ref.RuntimeOptions)
	if err != nil {
		return err
	}
	states, err = r.conversationSkillStates(home, opts.ExecutionMode, states)
	if err != nil {
		return err
	}
	if name != "" && !skill.Enabled(states, name) {
		return fmt.Errorf("runtime policy prevents enabling skill %q", name)
	}
	return r.updateSkillConfig(home, states, name)
}

func (r *Runtime) writeSkillStates(home string, states map[string]skill.State) error {
	return r.updateSkillConfig(home, states, "")
}

func (r *Runtime) updateSkillConfig(home string, states map[string]skill.State, enableName string) error {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	path := filepath.Join(home, configFileName)
	data, err := r.readFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := r.mkdirAll(home, 0o700); err != nil {
		return err
	}
	content := renderSkillStates(string(data), home, states)
	if enableName != "" {
		content, err = enableSkillConfigEntries(content, filepath.Join(home, "skills", enableName, "SKILL.md"))
		if err != nil {
			return err
		}
	}
	return r.writeFile(path, []byte(content), 0o600)
}

func renderSkillStates(content, home string, states map[string]skill.State) string {
	content = stripManagedBlock(content, skillConfigBegin, skillConfigEnd)
	names := make([]string, 0, len(states))
	for name, state := range states {
		if !state.Enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return content
	}
	var block strings.Builder
	block.WriteString(strings.TrimRight(content, "\n") + "\n" + skillConfigBegin + "\n")
	for _, name := range names {
		fmt.Fprintf(&block, "[[skills.config]]\npath = %s\nenabled = false\n", strconv.Quote(filepath.Join(home, "skills", name, "SKILL.md")))
	}
	block.WriteString(skillConfigEnd + "\n")
	return block.String()
}
