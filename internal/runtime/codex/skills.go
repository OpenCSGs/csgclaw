package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
)

const skillConfigBegin = "# BEGIN CSGCLAW SKILL STATES"
const skillConfigEnd = "# END CSGCLAW SKILL STATES"

func (r *Runtime) ReconcileSkills(_ context.Context, h agentruntime.Handle, states map[string]skill.State) error {
	ref, err := r.resolveAgent(h)
	if err != nil {
		return err
	}
	home, err := r.resolveCodexHomeDir(ref.ID)
	if err != nil {
		return err
	}
	return r.writeSkillStates(home, states)
}

func (r *Runtime) writeSkillStates(home string, states map[string]skill.State) error {
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
	return r.writeFile(path, []byte(renderSkillStates(string(data), home, states)), 0o600)
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
