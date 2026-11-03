package retiredclaws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"csgclaw/internal/localstore"
	toml "github.com/pelletier/go-toml/v2"
)

const cleanupTimeout = 5 * time.Second

type sandboxHome struct {
	path    string
	agentID string
	name    string
	info    fs.FileInfo
	parent  fs.FileInfo
}

type cleanupReference struct {
	path string
	data []byte
}

// CleanupAgentSandboxHomes runs under the application instance lock, before
// native runtimes start. It deletes only managed native Agents' old homes,
// without creating a backup.
// Nonempty homes require an inventory of every old instance to prove inactivity.
// The checker must not re-enter the root-state lock. Errors are warnings at the
// startup boundary; unrelated Agents and later retries remain usable.
func CleanupAgentSandboxHomes(ctx context.Context, root string, checkStopped func(context.Context, string) error, configPaths ...string) error {
	ctx, cancel := context.WithTimeout(ctx, cleanupTimeout)
	defer cancel()
	return localstore.WithRootStateLock(func() error { return cleanupAgentSandboxHomes(ctx, root, checkStopped, configPaths) })
}

func cleanupAgentSandboxHomes(ctx context.Context, root string, checkStopped func(context.Context, string) error, configPaths []string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	agentsDir := filepath.Join(root, "agents")
	info, err := os.Lstat(agentsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("retired home cleanup requires a real agents directory")
	}
	agentsRoot, err := os.OpenRoot(agentsDir)
	if err != nil {
		return err
	}
	defer agentsRoot.Close()
	opened, err := agentsRoot.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return fmt.Errorf("agents directory changed during retired home cleanup")
	}
	statePath := filepath.Join(root, localstore.RootStateFileName)
	data, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("read retired home cleanup state: %w", err)
	}
	agents, _ := state["agents"].(map[string]any)
	items, _ := agents["items"].([]any)
	var homes []sandboxHome
	var failures []error
	seen := map[string]bool{}
	for _, item := range items {
		a, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid Agent record during retired home cleanup")
		}
		id, _ := a["id"].(string)
		if id == "" || id == "." || id == ".." || id != strings.TrimSpace(id) || strings.TrimRight(id, ". ") != id || strings.ContainsAny(id, "/\\:") {
			return fmt.Errorf("invalid Agent identity during retired home cleanup")
		}
		if seen[strings.ToLower(id)] {
			return fmt.Errorf("duplicate Agent identity during retired home cleanup")
		}
		seen[strings.ToLower(id)] = true
		rt, _ := a["runtime"].(map[string]any)
		kind := ""
		for _, raw := range []any{rt["kind"], a["runtime_kind"], rt["name"], a["runtime_name"]} {
			if v, ok := raw.(string); ok && strings.TrimSpace(v) != "" {
				kind = strings.ToLower(strings.TrimSpace(v))
				break
			}
		}
		parentPath := filepath.Join(agentsDir, id)
		parent, err := os.Lstat(parentPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
			failures = append(failures, fmt.Errorf("skip retired homes under non-directory or linked Agent home %q", id))
			continue
		}
		for _, name := range []string{".boxlite", "boxlite"} {
			path := filepath.Join(parentPath, name)
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if kind != "codex" && kind != "dsh" {
				failures = append(failures, fmt.Errorf("Agent %q has not completed native runtime migration; keeping %s", id, path))
				continue
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				failures = append(failures, fmt.Errorf("skip linked or non-directory retired home %s", path))
				continue
			}
			homes = append(homes, sandboxHome{path, id, name, info, parent})
		}
	}
	if len(homes) == 0 {
		return errors.Join(failures...)
	}
	references := []cleanupReference{{statePath, data}}
	referenceValues := []any{state}
	paths := append([]string{filepath.Join(root, "im", "state.json")}, configPaths...)
	for _, path := range paths {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			references = append(references, cleanupReference{path, nil})
			continue
		}
		if err != nil {
			return errors.Join(errors.Join(failures...), err)
		}
		var v any
		if filepath.Ext(path) == ".toml" {
			err = toml.Unmarshal(data, &v)
		} else {
			err = json.Unmarshal(data, &v)
		}
		if err != nil {
			return errors.Join(errors.Join(failures...), fmt.Errorf("read cleanup references %s: %w", path, err))
		}
		references = append(references, cleanupReference{path, data})
		referenceValues = append(referenceValues, v)
	}
	// WalkDir never follows links. Resolve their targets separately, and skip the
	// retired trees themselves so their internal metadata cannot protect itself.
	links, err := cleanupSymlinkTargets(ctx, agentsDir, homes)
	if err != nil {
		return errors.Join(errors.Join(failures...), err)
	}
	referenceValues = append(referenceValues, links)
	for _, home := range homes {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		if cleanupReferenced(referenceValues, home.path) {
			failures = append(failures, fmt.Errorf("retired home is still referenced; keeping %s", home.path))
			continue
		}
		entries, err := os.ReadDir(home.path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if len(entries) > 0 {
			if checkStopped == nil {
				failures = append(failures, fmt.Errorf("cannot confirm old instances have stopped; keeping %s", home.path))
				continue
			}
			if err := checkStopped(ctx, home.path); err != nil {
				failures = append(failures, fmt.Errorf("keep retired home %s: %w", home.path, err))
				continue
			}
		}
		if err := cleanupReferencesUnchanged(references); err != nil {
			failures = append(failures, err)
			break
		}
		current, err := os.Lstat(home.path)
		parent, parentErr := os.Lstat(filepath.Dir(home.path))
		if err != nil || parentErr != nil || !os.SameFile(home.info, current) || !os.SameFile(home.parent, parent) || current.Mode()&os.ModeSymlink != 0 || parent.Mode()&os.ModeSymlink != 0 {
			failures = append(failures, fmt.Errorf("retired home changed during cleanup; keeping %s", home.path))
			continue
		}
		if err := removeRetiredSandboxHome(agentsRoot, home); err != nil {
			failures = append(failures, fmt.Errorf("remove retired home %s: %w", home.path, err))
			continue
		}
		slog.Info("retired Agent sandbox home removed", "agent_id", home.agentID, "path", home.path)
	}
	return errors.Join(failures...)
}

// The filesystem root confines recursive removal to this exact Agent home.
// RemoveAll unlinks nested symlinks rather than following their targets.
func removeRetiredSandboxHome(agentsRoot *os.Root, home sandboxHome) error {
	agentRoot, err := agentsRoot.OpenRoot(home.agentID)
	if err != nil {
		return err
	}
	defer agentRoot.Close()
	parent, err := agentRoot.Stat(".")
	if err != nil || !os.SameFile(parent, home.parent) {
		return fmt.Errorf("Agent directory changed before removal")
	}
	current, err := agentRoot.Lstat(home.name)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !os.SameFile(current, home.info) {
		return fmt.Errorf("retired home changed before removal")
	}
	return agentRoot.RemoveAll(home.name)
}

func cleanupSymlinkTargets(ctx context.Context, agentsDir string, homes []sandboxHome) ([]any, error) {
	var targets []any
	err := filepath.WalkDir(agentsDir, func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return err
		}
		for _, home := range homes {
			if path == home.path {
				return filepath.SkipDir
			}
		}
		if d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		targets = append(targets, target)
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			targets = append(targets, resolved)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
	return targets, err
}

func cleanupReferenced(value any, candidate string) bool {
	return cleanupValueReferenced(value, []string{candidate, cleanupResolvedPath(candidate)})
}

func cleanupValueReferenced(value any, candidates []string) bool {
	switch v := value.(type) {
	case string:
		values := []string{v, cleanupExpandedReference(v)}
		for _, value := range append([]string(nil), values...) {
			if filepath.IsAbs(value) {
				values = append(values, cleanupResolvedPath(value))
			}
		}
		for _, value := range values {
			for _, target := range candidates {
				value = filepath.ToSlash(filepath.Clean(value))
				target = filepath.ToSlash(filepath.Clean(target))
				if runtime.GOOS == "windows" {
					value = strings.ToLower(value)
					target = strings.ToLower(target)
				}
				if cleanupPathMentioned(value, target) {
					return true
				}
				if filepath.IsAbs(value) && strings.HasPrefix(target, strings.TrimRight(value, "/")+"/") {
					return true
				}
			}
		}
	case []any:
		for _, item := range v {
			if cleanupValueReferenced(item, candidates) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if cleanupValueReferenced(item, candidates) {
				return true
			}
		}
	}
	return false
}

func cleanupReferencesUnchanged(references []cleanupReference) error {
	for _, ref := range references {
		data, err := os.ReadFile(ref.path)
		if errors.Is(err, os.ErrNotExist) && ref.data == nil {
			continue
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(data, ref.data) {
			return fmt.Errorf("configuration changed during retired home cleanup; retry on next startup")
		}
	}
	return nil
}

// A configured workspace or MCP path need not exist yet. Resolve the existing
// prefix so aliases such as /var and /private/var still protect its old home.
func cleanupResolvedPath(path string) string {
	var suffix []string
	current := filepath.Clean(path)
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

var cleanupWindowsEnv = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)

func cleanupExpandedReference(value string) string {
	value = os.Expand(value, func(key string) string {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return "${" + key + "}"
	})
	if runtime.GOOS == "windows" {
		value = cleanupWindowsEnv.ReplaceAllStringFunc(value, func(token string) string {
			if v, ok := os.LookupEnv(strings.Trim(token, "%")); ok {
				return v
			}
			return token
		})
	}
	if home, err := os.UserHomeDir(); err == nil {
		value = strings.ReplaceAll(value, "~/", filepath.ToSlash(home)+"/")
	}
	return value
}

func cleanupPathMentioned(value, target string) bool {
	index := strings.Index(value, target)
	if index < 0 {
		return false
	}
	rest := value[index+len(target):]
	return rest == "" || strings.ContainsRune("/ \t\r\n\"'", rune(rest[0]))
}
