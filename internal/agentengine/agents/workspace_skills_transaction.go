package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"csgclaw/internal/config"
)

const (
	workspaceSkillsTransactionVersion = 1
	workspaceSkillsStateFileName      = "state.json"
	workspaceSkillsStatePreparing     = "preparing"
	workspaceSkillsStatePrepared      = "prepared"
	workspaceSkillsStateApplying      = "applying"
	workspaceSkillsStateCommitted     = "committed"
	workspaceSkillsTransactionSuffix  = "-preserve-skills"
)

type workspaceSkillsPreservation struct {
	agentID       string
	sourceRoot    string
	targetRoot    string
	tempRoot      string
	preservedRoot string
	displacedRoot string
	rollbackRoot  string
	statePath     string
	sourceMode    os.FileMode
	entries       []workspaceSkillsPreservedEntry
	appliedRoot   string
	phase         string
	modeChanges   []workspaceSkillsModeChange
}

type workspaceSkillsPreservedEntry struct {
	Name      string                  `json:"name"`
	Restored  bool                    `json:"restored,omitempty"`
	Displaced bool                    `json:"displaced,omitempty"`
	Merged    []workspaceSkillsRename `json:"merged,omitempty"`
}

type workspaceSkillsRename struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type workspaceSkillsModeChange struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
}

type workspaceSkillsTransactionState struct {
	Version     int                             `json:"version"`
	Phase       string                          `json:"phase"`
	AgentID     string                          `json:"agent_id"`
	SourceRoot  string                          `json:"source_root"`
	TargetRoot  string                          `json:"target_root"`
	SourceMode  uint32                          `json:"source_mode"`
	AppliedRoot string                          `json:"applied_root,omitempty"`
	Entries     []workspaceSkillsPreservedEntry `json:"entries,omitempty"`
	ModeChanges []workspaceSkillsModeChange     `json:"mode_changes,omitempty"`
}

func (s *Controller) prepareWorkspaceSkillsPreservation(agentID, sourceRuntimeKind, targetRuntimeKind, role string) (*workspaceSkillsPreservation, error) {
	sourceRuntimeKind = strings.TrimSpace(sourceRuntimeKind)
	targetRuntimeKind = strings.TrimSpace(targetRuntimeKind)
	if sourceRuntimeKind == "" {
		sourceRuntimeKind = targetRuntimeKind
	}
	if targetRuntimeKind == "" {
		targetRuntimeKind = sourceRuntimeKind
	}
	if err := s.recoverWorkspaceSkillsTransaction(agentID); err != nil {
		return nil, fmt.Errorf("recover interrupted workspace skills transaction: %w", err)
	}
	sourceSkills, err := s.agentSkillsRoot(agentID, sourceRuntimeKind)
	if err != nil {
		return nil, err
	}
	targetSkills, err := s.agentSkillsRoot(agentID, targetRuntimeKind)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(sourceSkills)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat workspace skills: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrWorkspaceSymlinkDenied
	}
	if !info.IsDir() {
		return nil, nil
	}
	entryCount, err := validateWorkspaceSkillsTree(sourceSkills)
	if err != nil {
		return nil, err
	}
	if entryCount == 0 {
		return nil, nil
	}
	templateNames, err := managedWorkspaceSkillNames(targetRuntimeKind, role)
	if err != nil {
		return nil, err
	}
	agentHome, err := s.agentHomeDir(agentID)
	if err != nil {
		return nil, err
	}
	tempRoot := workspaceSkillsTransactionRoot(agentHome)
	if err := os.MkdirAll(filepath.Dir(agentHome), 0o755); err != nil {
		return nil, fmt.Errorf("create agent root for skills preservation: %w", err)
	}
	if err := os.Mkdir(tempRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create skills preservation dir: %w", err)
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}
	preserved := &workspaceSkillsPreservation{
		agentID:       canonicalAgentID(agentID),
		sourceRoot:    sourceSkills,
		targetRoot:    targetSkills,
		tempRoot:      tempRoot,
		preservedRoot: filepath.Join(tempRoot, "preserved"),
		displacedRoot: filepath.Join(tempRoot, "displaced"),
		rollbackRoot:  filepath.Join(tempRoot, "rollback"),
		statePath:     filepath.Join(tempRoot, workspaceSkillsStateFileName),
		sourceMode:    mode,
		phase:         workspaceSkillsStatePreparing,
	}
	if err := preserved.persistState(); err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, fmt.Errorf("record skills preservation state: %w", err)
	}
	if err := os.Rename(sourceSkills, preserved.preservedRoot); err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, fmt.Errorf("preserve workspace skills with rename: %w", err)
	}
	if err := os.MkdirAll(sourceSkills, mode); err != nil {
		return nil, preserved.rollbackPreparation(fmt.Errorf("recreate workspace skills root: %w", err))
	}

	managedNames := make([]string, 0, len(templateNames))
	for name := range templateNames {
		managedNames = append(managedNames, name)
	}
	sort.Strings(managedNames)
	for _, name := range managedNames {
		if err := validateWorkspaceSkillEntryName(name); err != nil {
			return nil, preserved.rollbackPreparation(err)
		}
		from := filepath.Join(preserved.preservedRoot, name)
		if _, err := os.Lstat(from); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, preserved.rollbackPreparation(fmt.Errorf("inspect managed skill %q: %w", name, err))
		}
		if err := os.Rename(from, filepath.Join(sourceSkills, name)); err != nil {
			return nil, preserved.rollbackPreparation(fmt.Errorf("retain managed skill %q: %w", name, err))
		}
	}
	entries, err := os.ReadDir(preserved.preservedRoot)
	if err != nil {
		return nil, preserved.rollbackPreparation(fmt.Errorf("read preserved workspace skills: %w", err))
	}
	if len(entries) == 0 {
		if err := preserved.Commit(); err != nil {
			if preserved.phase == workspaceSkillsStateCommitted {
				return nil, nil
			}
			return nil, err
		}
		return nil, nil
	}
	preserved.entries = make([]workspaceSkillsPreservedEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if err := validateWorkspaceSkillEntryName(name); err != nil {
			return nil, preserved.rollbackPreparation(err)
		}
		preserved.entries = append(preserved.entries, workspaceSkillsPreservedEntry{Name: name})
	}
	preserved.phase = workspaceSkillsStatePrepared
	if err := preserved.persistState(); err != nil {
		return nil, preserved.rollbackPreparation(fmt.Errorf("record prepared workspace skills: %w", err))
	}
	return preserved, nil
}

func workspaceSkillsTransactionRoot(agentHome string) string {
	return filepath.Join(filepath.Dir(agentHome), "."+filepath.Base(agentHome)+workspaceSkillsTransactionSuffix)
}

func (s *Controller) recoverWorkspaceSkillsTransactions() error {
	root := strings.TrimSpace(s.agentsRoot)
	if root == "" {
		var err error
		root, err = config.DefaultAgentsDir()
		if err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read agents root for skills recovery: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), workspaceSkillsTransactionSuffix) {
			continue
		}
		if err := s.recoverWorkspaceSkillsTransactionAt(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (s *Controller) recoverWorkspaceSkillsTransaction(agentID string) error {
	agentHome, err := s.agentHomeDir(agentID)
	if err != nil {
		return err
	}
	tempRoot := workspaceSkillsTransactionRoot(agentHome)
	if _, err := os.Lstat(tempRoot); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return s.recoverWorkspaceSkillsTransactionAt(tempRoot)
}

func (s *Controller) recoverWorkspaceSkillsTransactionAt(tempRoot string) error {
	statePath := filepath.Join(tempRoot, workspaceSkillsStateFileName)
	data, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(tempRoot)
		if readErr != nil {
			return readErr
		}
		if len(entries) == 0 {
			return os.Remove(tempRoot)
		}
		if len(entries) == 1 && entries[0].Name() == workspaceSkillsStateFileName+".tmp" {
			tempStatePath := filepath.Join(tempRoot, entries[0].Name())
			info, statErr := os.Lstat(tempStatePath)
			if statErr != nil {
				return statErr
			}
			if info.Mode().IsRegular() {
				if removeErr := os.Remove(tempStatePath); removeErr != nil {
					return removeErr
				}
				return os.Remove(tempRoot)
			}
		}
		return fmt.Errorf("skills transaction %q has no state file", tempRoot)
	}
	if err != nil {
		return fmt.Errorf("read skills transaction state: %w", err)
	}
	var state workspaceSkillsTransactionState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode skills transaction state %q: %w", statePath, err)
	}
	if state.Version != workspaceSkillsTransactionVersion {
		return fmt.Errorf("skills transaction %q has unsupported version %d", tempRoot, state.Version)
	}
	agentHome, err := s.agentHomeDir(state.AgentID)
	if err != nil {
		return err
	}
	if filepath.Clean(tempRoot) != filepath.Clean(workspaceSkillsTransactionRoot(agentHome)) {
		return fmt.Errorf("skills transaction path %q does not match agent %q", tempRoot, state.AgentID)
	}
	for _, path := range []string{state.SourceRoot, state.TargetRoot} {
		if err := validateWorkspaceTransactionPath(agentHome, path); err != nil {
			return err
		}
	}
	if state.AppliedRoot != "" {
		if err := validateWorkspaceTransactionPath(agentHome, state.AppliedRoot); err != nil {
			return err
		}
	}
	preserved := &workspaceSkillsPreservation{
		agentID:       state.AgentID,
		sourceRoot:    state.SourceRoot,
		targetRoot:    state.TargetRoot,
		tempRoot:      tempRoot,
		preservedRoot: filepath.Join(tempRoot, "preserved"),
		displacedRoot: filepath.Join(tempRoot, "displaced"),
		rollbackRoot:  filepath.Join(tempRoot, "rollback"),
		statePath:     statePath,
		sourceMode:    os.FileMode(state.SourceMode),
		entries:       state.Entries,
		appliedRoot:   state.AppliedRoot,
		phase:         state.Phase,
		modeChanges:   state.ModeChanges,
	}
	if err := preserved.validateRecordedPaths(agentHome); err != nil {
		return err
	}
	switch preserved.phase {
	case workspaceSkillsStateCommitted:
		return removeWorkspaceSkillsTransaction(tempRoot)
	case workspaceSkillsStatePreparing:
		return preserved.recoverPreparation()
	case workspaceSkillsStatePrepared, workspaceSkillsStateApplying:
		return preserved.Rollback()
	default:
		return fmt.Errorf("skills transaction %q has unknown phase %q", tempRoot, preserved.phase)
	}
}

func validateWorkspaceTransactionPath(agentHome, path string) error {
	agentHome = filepath.Clean(strings.TrimSpace(agentHome))
	path = filepath.Clean(strings.TrimSpace(path))
	if agentHome == "." || path == "." {
		return ErrWorkspacePathUnsafe
	}
	rel, err := filepath.Rel(agentHome, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, path)
	}
	return nil
}

func (p *workspaceSkillsPreservation) validateRecordedPaths(agentHome string) error {
	for _, change := range p.modeChanges {
		if err := validateWorkspaceTransactionPath(agentHome, change.Path); err != nil && !pathWithinRoot(p.tempRoot, change.Path) {
			return err
		}
	}
	for _, entry := range p.entries {
		if err := validateWorkspaceSkillEntryName(entry.Name); err != nil {
			return err
		}
		for _, move := range entry.Merged {
			if !pathWithinRoot(p.displacedRoot, move.From) {
				return fmt.Errorf("%w: %s", ErrWorkspacePathUnsafe, move.From)
			}
			if err := validateWorkspaceTransactionPath(agentHome, move.To); err != nil {
				return err
			}
		}
	}
	return nil
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (p *workspaceSkillsPreservation) persistState() error {
	if p == nil {
		return nil
	}
	state := workspaceSkillsTransactionState{
		Version:     workspaceSkillsTransactionVersion,
		Phase:       p.phase,
		AgentID:     p.agentID,
		SourceRoot:  p.sourceRoot,
		TargetRoot:  p.targetRoot,
		SourceMode:  uint32(p.sourceMode.Perm()),
		AppliedRoot: p.appliedRoot,
		Entries:     p.entries,
		ModeChanges: p.modeChanges,
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tempPath := p.statePath + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, p.statePath)
}

func (p *workspaceSkillsPreservation) rollbackPreparation(cause error) error {
	if p == nil {
		return cause
	}
	return errors.Join(cause, p.recoverPreparation())
}

func (p *workspaceSkillsPreservation) recoverPreparation() error {
	if p == nil {
		return nil
	}
	if _, err := os.Lstat(p.preservedRoot); errors.Is(err, os.ErrNotExist) {
		return removeWorkspaceSkillsTransaction(p.tempRoot)
	} else if err != nil {
		return err
	}
	if err := mergeWorkspaceDirectoryEntries(p.sourceRoot, p.preservedRoot, p.rollbackRoot, nil); err != nil {
		return fmt.Errorf("recover workspace skills preparation: %w", err)
	}
	if err := os.Remove(p.sourceRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove temporary workspace skills root: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.sourceRoot), 0o755); err != nil {
		return err
	}
	if err := os.Rename(p.preservedRoot, p.sourceRoot); err != nil {
		return fmt.Errorf("restore workspace skills after preparation interruption: %w", err)
	}
	return removeWorkspaceSkillsTransaction(p.tempRoot)
}

func (p *workspaceSkillsPreservation) Restore() error {
	if p == nil {
		return nil
	}
	p.phase = workspaceSkillsStateApplying
	p.appliedRoot = p.targetRoot
	if err := p.persistState(); err != nil {
		return err
	}
	return p.apply(p.targetRoot)
}

func (p *workspaceSkillsPreservation) apply(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("workspace skills restore destination is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create target workspace skills dir: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("stat target workspace skills dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrWorkspaceSymlinkDenied
	}
	if !info.IsDir() {
		return fmt.Errorf("target workspace skills path %q is not a directory", root)
	}

	for index := range p.entries {
		entry := &p.entries[index]
		preserved := filepath.Join(p.preservedRoot, entry.Name)
		target := filepath.Join(root, entry.Name)
		if _, err := os.Lstat(target); err == nil {
			if err := os.MkdirAll(p.displacedRoot, 0o700); err != nil {
				return p.revertApply(fmt.Errorf("create displaced skills dir: %w", err))
			}
			entry.Displaced = true
			if err := p.persistState(); err != nil {
				return p.revertApply(err)
			}
			if err := os.Rename(target, filepath.Join(p.displacedRoot, entry.Name)); err != nil {
				return p.revertApply(fmt.Errorf("preserve replacement skill %q: %w", entry.Name, err))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return p.revertApply(fmt.Errorf("inspect target skill %q: %w", entry.Name, err))
		}
		entry.Restored = true
		if err := p.persistState(); err != nil {
			return p.revertApply(err)
		}
		if err := os.Rename(preserved, target); err != nil {
			return p.revertApply(fmt.Errorf("restore workspace skill %q with rename: %w", entry.Name, err))
		}
		if entry.Displaced {
			plan, err := planWorkspaceSkillsMerge(filepath.Join(p.displacedRoot, entry.Name), target)
			if err != nil {
				return p.revertApply(fmt.Errorf("plan replacement skill merge %q: %w", entry.Name, err))
			}
			entry.Merged = plan
			if err := p.recordMergeDirectoryModes(plan); err != nil {
				return p.revertApply(fmt.Errorf("record replacement skill permissions %q: %w", entry.Name, err))
			}
			if err := p.persistState(); err != nil {
				return p.revertApply(err)
			}
			if err := p.makeRecordedDirectoriesWritable(); err != nil {
				return p.revertApply(fmt.Errorf("make replacement skill directories writable %q: %w", entry.Name, err))
			}
			if err := executeWorkspaceSkillsMerge(plan); err != nil {
				return p.revertApply(fmt.Errorf("merge replacement skill %q with rename: %w", entry.Name, err))
			}
			if err := p.restoreRecordedDirectoryModes(); err != nil {
				return p.revertApply(fmt.Errorf("restore replacement skill permissions %q: %w", entry.Name, err))
			}
		}
	}
	return nil
}

func planWorkspaceSkillsMerge(displacedRoot, targetRoot string) ([]workspaceSkillsRename, error) {
	displacedInfo, err := os.Lstat(displacedRoot)
	if err != nil {
		return nil, err
	}
	targetInfo, err := os.Lstat(targetRoot)
	if err != nil {
		return nil, err
	}
	if !displacedInfo.IsDir() || !targetInfo.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(displacedRoot)
	if err != nil {
		return nil, err
	}
	var plan []workspaceSkillsRename
	for _, entry := range entries {
		from := filepath.Join(displacedRoot, entry.Name())
		to := filepath.Join(targetRoot, entry.Name())
		toInfo, err := os.Lstat(to)
		if errors.Is(err, os.ErrNotExist) {
			plan = append(plan, workspaceSkillsRename{From: from, To: to})
			continue
		}
		if err != nil {
			return nil, err
		}
		fromInfo, err := os.Lstat(from)
		if err != nil {
			return nil, err
		}
		if fromInfo.IsDir() && toInfo.IsDir() {
			nested, err := planWorkspaceSkillsMerge(from, to)
			if err != nil {
				return nil, err
			}
			plan = append(plan, nested...)
		}
	}
	return plan, nil
}

func executeWorkspaceSkillsMerge(plan []workspaceSkillsRename) error {
	for _, move := range plan {
		fromExists, err := workspacePathExists(move.From)
		if err != nil {
			return err
		}
		toExists, err := workspacePathExists(move.To)
		if err != nil {
			return err
		}
		switch {
		case fromExists && !toExists:
			if err := os.Rename(move.From, move.To); err != nil {
				return err
			}
		case !fromExists && toExists:
			continue
		default:
			return fmt.Errorf("workspace merge paths have unexpected state: from=%t to=%t", fromExists, toExists)
		}
	}
	return nil
}

func (p *workspaceSkillsPreservation) recordMergeDirectoryModes(plan []workspaceSkillsRename) error {
	recorded := make(map[string]struct{}, len(p.modeChanges))
	for _, change := range p.modeChanges {
		recorded[filepath.Clean(change.Path)] = struct{}{}
	}
	for _, move := range plan {
		for _, parent := range []string{filepath.Clean(filepath.Dir(move.From)), filepath.Clean(filepath.Dir(move.To))} {
			if _, ok := recorded[parent]; ok {
				continue
			}
			info, err := os.Lstat(parent)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return ErrWorkspaceSymlinkDenied
			}
			if !info.IsDir() {
				return fmt.Errorf("workspace merge parent %q is not a directory", parent)
			}
			if info.Mode().Perm()&0o200 == 0 {
				p.modeChanges = append(p.modeChanges, workspaceSkillsModeChange{Path: parent, Mode: uint32(info.Mode().Perm())})
				recorded[parent] = struct{}{}
			}
		}
	}
	return nil
}

func (p *workspaceSkillsPreservation) makeRecordedDirectoriesWritable() error {
	var chmodErr error
	for _, change := range p.modeChanges {
		info, err := os.Lstat(change.Path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			chmodErr = errors.Join(chmodErr, err)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			chmodErr = errors.Join(chmodErr, ErrWorkspaceSymlinkDenied)
			continue
		}
		if err := os.Chmod(change.Path, info.Mode().Perm()|0o200); err != nil {
			chmodErr = errors.Join(chmodErr, err)
		}
	}
	return chmodErr
}

func (p *workspaceSkillsPreservation) restoreRecordedDirectoryModes() error {
	var chmodErr error
	changes := append([]workspaceSkillsModeChange(nil), p.modeChanges...)
	sort.SliceStable(changes, func(i, j int) bool {
		return strings.Count(changes[i].Path, string(filepath.Separator)) > strings.Count(changes[j].Path, string(filepath.Separator))
	})
	for _, change := range changes {
		if err := os.Chmod(change.Path, os.FileMode(change.Mode)); err != nil && !errors.Is(err, os.ErrNotExist) {
			chmodErr = errors.Join(chmodErr, err)
		}
	}
	return chmodErr
}

func (p *workspaceSkillsPreservation) RevertRestore() error {
	if p == nil || p.appliedRoot == "" {
		return nil
	}
	if err := p.makeRecordedDirectoriesWritable(); err != nil {
		return err
	}
	for index := len(p.entries) - 1; index >= 0; index-- {
		entry := &p.entries[index]
		if err := revertWorkspaceSkillsMergePlan(entry.Merged); err != nil {
			return fmt.Errorf("revert merged replacement skill %q: %w", entry.Name, err)
		}
		entry.Merged = nil
		if err := p.persistState(); err != nil {
			return err
		}
	}
	if err := p.restoreRecordedDirectoryModes(); err != nil {
		return err
	}
	p.modeChanges = nil

	root := p.appliedRoot
	for index := len(p.entries) - 1; index >= 0; index-- {
		entry := &p.entries[index]
		target := filepath.Join(root, entry.Name)
		preserved := filepath.Join(p.preservedRoot, entry.Name)
		if entry.Restored {
			targetExists, err := workspacePathExists(target)
			if err != nil {
				return err
			}
			preservedExists, err := workspacePathExists(preserved)
			if err != nil {
				return err
			}
			switch {
			case targetExists && !preservedExists:
				if err := os.MkdirAll(p.preservedRoot, 0o700); err != nil {
					return err
				}
				if err := os.Rename(target, preserved); err != nil {
					return err
				}
			case !targetExists && preservedExists:
			case targetExists && preservedExists:
				return fmt.Errorf("restored and preserved skill %q both exist", entry.Name)
			default:
				return fmt.Errorf("restored skill %q is missing", entry.Name)
			}
			entry.Restored = false
			if err := p.persistState(); err != nil {
				return err
			}
		}
		if entry.Displaced {
			displaced := filepath.Join(p.displacedRoot, entry.Name)
			displacedExists, err := workspacePathExists(displaced)
			if err != nil {
				return err
			}
			targetExists, err := workspacePathExists(target)
			if err != nil {
				return err
			}
			switch {
			case displacedExists && !targetExists:
				if err := os.Rename(displaced, target); err != nil {
					return err
				}
			case !displacedExists && targetExists:
			case displacedExists && targetExists:
				return fmt.Errorf("displaced and target skill %q both exist", entry.Name)
			default:
				return fmt.Errorf("displaced skill %q is missing", entry.Name)
			}
			entry.Displaced = false
			if err := p.persistState(); err != nil {
				return err
			}
		}
	}
	p.appliedRoot = ""
	p.phase = workspaceSkillsStatePrepared
	return p.persistState()
}

func revertWorkspaceSkillsMergePlan(plan []workspaceSkillsRename) error {
	for index := len(plan) - 1; index >= 0; index-- {
		move := plan[index]
		fromExists, err := workspacePathExists(move.From)
		if err != nil {
			return err
		}
		toExists, err := workspacePathExists(move.To)
		if err != nil {
			return err
		}
		switch {
		case !fromExists && toExists:
			if err := os.MkdirAll(filepath.Dir(move.From), 0o755); err != nil {
				return err
			}
			if err := os.Rename(move.To, move.From); err != nil {
				return err
			}
		case fromExists && !toExists:
			continue
		default:
			return fmt.Errorf("workspace merge rollback paths have unexpected state: from=%t to=%t", fromExists, toExists)
		}
	}
	return nil
}

func workspacePathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (p *workspaceSkillsPreservation) revertApply(cause error) error {
	if err := p.RevertRestore(); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (p *workspaceSkillsPreservation) Rollback() error {
	if p == nil {
		return nil
	}
	if p.phase == workspaceSkillsStateApplying || p.appliedRoot != "" {
		if err := p.RevertRestore(); err != nil {
			return err
		}
	}
	preservedExists, err := workspacePathExists(p.preservedRoot)
	if err != nil {
		return err
	}
	if !preservedExists {
		sourceExists, err := workspacePathExists(p.sourceRoot)
		if err != nil {
			return err
		}
		if !sourceExists {
			return fmt.Errorf("original and preserved workspace skills are both missing")
		}
		if err := p.Commit(); err != nil && p.phase != workspaceSkillsStateCommitted {
			return err
		}
		return nil
	}
	if err := mergeWorkspaceDirectoryEntries(p.sourceRoot, p.preservedRoot, p.rollbackRoot, p.entries); err != nil {
		return fmt.Errorf("restore original workspace skills: %w; preserved data remains at %s", err, p.tempRoot)
	}
	if err := os.Remove(p.sourceRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove replacement workspace skills root: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.sourceRoot), 0o755); err != nil {
		return err
	}
	if err := os.Rename(p.preservedRoot, p.sourceRoot); err != nil {
		return fmt.Errorf("restore original workspace skills: %w; preserved data remains at %s", err, p.tempRoot)
	}
	if err := p.Commit(); err != nil && p.phase != workspaceSkillsStateCommitted {
		return err
	}
	return nil
}

func mergeWorkspaceDirectoryEntries(sourceRoot, preservedRoot, rollbackRoot string, preservedEntries []workspaceSkillsPreservedEntry) error {
	if _, err := os.Lstat(sourceRoot); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(preservedRoot, 0o700); err != nil {
		return err
	}
	preservedNames := make(map[string]struct{}, len(preservedEntries))
	for _, entry := range preservedEntries {
		preservedNames[entry.Name] = struct{}{}
	}
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(sourceRoot, entry.Name())
		to := filepath.Join(preservedRoot, entry.Name())
		if _, isPreserved := preservedNames[entry.Name()]; isPreserved {
			if err := os.MkdirAll(rollbackRoot, 0o700); err != nil {
				return err
			}
			if err := os.Rename(from, filepath.Join(rollbackRoot, entry.Name())); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Lstat(to); err == nil {
			return fmt.Errorf("workspace recovery destination %q already exists", to)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
	}
	return nil
}

func (p *workspaceSkillsPreservation) Commit() error {
	if p == nil {
		return nil
	}
	previousPhase := p.phase
	p.phase = workspaceSkillsStateCommitted
	if err := p.persistState(); err != nil {
		p.phase = previousPhase
		return err
	}
	return removeWorkspaceSkillsTransaction(p.tempRoot)
}

func (p *workspaceSkillsPreservation) Cleanup() {
	if p == nil {
		return
	}
	for _, entry := range p.entries {
		if !entry.Restored {
			return
		}
	}
	_ = removeWorkspaceSkillsTransaction(p.tempRoot)
}

func removeWorkspaceSkillsTransaction(root string) error {
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chmod(path, info.Mode().Perm()|0o700)
	}); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.RemoveAll(root)
}
