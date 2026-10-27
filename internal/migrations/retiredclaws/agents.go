package retiredclaws

import (
	"context"
	"csgclaw/internal/localstore"
	"csgclaw/internal/sandbox"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isRetiredRuntimeKind identifies persisted runtime names for one-time upgrades.
// It does not make these runtimes available for new Agents or templates.
func isRetiredRuntimeKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "openclaw", "openclaw_sandbox", "picoclaw", "picoclaw_sandbox":
		return true
	default:
		return false
	}
}

// MigrateAgents runs before services load the store. User identity,
// model settings, resources, conversations and workspace files remain intact.
// Workers stay stopped when their retired sandbox is replaced by a host runtime.
func MigrateAgents(ctx context.Context, root string, openSandbox func(context.Context, string) (sandbox.Runtime, error)) error {
	return localstore.WithRootStateLock(func() error { return migrateAgents(ctx, root, openSandbox) })
}

func migrateAgents(ctx context.Context, root string, openSandbox func(context.Context, string) (sandbox.Runtime, error)) error {
	path := filepath.Join(root, localstore.RootStateFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode runtime migration state: %w", err)
	}
	var section map[string]json.RawMessage
	if raw := state["agents"]; len(raw) == 0 || string(raw) == "null" {
		return nil
	} else if err := json.Unmarshal(raw, &section); err != nil {
		return err
	}
	var agents []map[string]json.RawMessage
	if raw := section["items"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &agents); err != nil {
			return err
		}
	}
	changed := false
	var memories []retiredMemoryImport
	for _, agent := range agents {
		var runtime map[string]json.RawMessage
		if raw := agent["runtime"]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &runtime); err != nil {
				return err
			}
		}
		kind := migrationString(runtime["kind"])
		if kind == "" {
			kind = migrationString(agent["runtime_kind"])
		}
		if kind == "" {
			kind = migrationString(runtime["name"])
		}
		if kind == "" {
			kind = migrationString(agent["runtime_name"])
		}
		if !isRetiredRuntimeKind(kind) {
			continue
		}
		id := migrationString(agent["id"])
		if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
			return fmt.Errorf("invalid retired runtime Agent id %q", id)
		}
		if err := stopRetiredSandboxes(ctx, id, agent, runtime, openSandbox); err != nil {
			return err
		}
		workspace, err := retiredRuntimeWorkspace(root, id, kind)
		if err != nil {
			return err
		}
		if err := importRetiredMCP(root, id, kind, workspace, agent); err != nil {
			return err
		}
		memory, err := planRetiredMemory(root, id, kind, workspace)
		if err != nil {
			return err
		}
		if memory != nil {
			memories = append(memories, *memory)
		}
		next := map[string]any{"kind": "codex", "state": "stopped"}
		if workspace != "" {
			next["options"] = map[string]string{"local_workspace_dir": workspace}
		}
		agent["runtime"], err = json.Marshal(next)
		if err != nil {
			return err
		}
		agent["desired_state"] = json.RawMessage(`"stopped"`)
		for _, key := range []string{"runtime_kind", "runtime_name", "runtime_id", "box_id", "sandbox_id", "sandbox_enabled", "image", "runtime_options", "status"} {
			delete(agent, key)
		}
		if err := cleanRetiredProfile(agent); err != nil {
			return err
		}
		// Older runtimes kept user instructions only in their workspace file.
		if migrationString(agent["instructions"]) == "" && workspace != "" {
			name := "AGENTS.md"
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(kind)), "picoclaw") {
				name = "AGENT.md"
			}
			text, readErr := os.ReadFile(filepath.Join(workspace, name))
			if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
				return readErr
			}
			if len(text) > 0 {
				agent["instructions"], err = json.Marshal(string(text))
				if err != nil {
					return err
				}
			}
		}
		changed = true
	}
	if !changed {
		return nil
	}
	section["items"], err = json.Marshal(agents)
	if err != nil {
		return err
	}
	state["agents"], err = json.Marshal(section)
	if err != nil {
		return err
	}
	updated, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := backupOnce(root); err != nil {
		return fmt.Errorf("backup retired runtime state: %w", err)
	}
	for _, memory := range memories {
		if err := writeRetiredMemory(memory); err != nil {
			return fmt.Errorf("import retired runtime memory: %w", err)
		}
	}
	if err := writeRuntimeMigrationFile(path, append(updated, '\n')); err != nil {
		return fmt.Errorf("write runtime migration state: %w", err)
	}
	return nil
}

func migrationString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func retiredRuntimeWorkspace(root, id, kind string) (string, error) {
	kind = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(kind)), "_sandbox")
	workspace := filepath.Join(root, "agents", id, "."+kind, "workspace")
	info, err := os.Stat(workspace)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("retired runtime workspace is not a directory: %s", workspace)
	}
	return filepath.Abs(workspace)
}

func cleanRetiredProfile(agent map[string]json.RawMessage) error {
	for _, key := range []string{"model_config", "agent_profile"} {
		var profile map[string]json.RawMessage
		if raw := agent[key]; len(raw) == 0 || string(raw) == "null" {
			continue
		} else if err := json.Unmarshal(raw, &profile); err != nil {
			return err
		}
		delete(profile, "image_upgrade_required")
		delete(profile, "env_restart_required")
		if raw := profile["env"]; len(raw) > 0 && string(raw) != "null" {
			var env map[string]json.RawMessage
			if err := json.Unmarshal(raw, &env); err != nil {
				return err
			}
			for name := range env {
				upper := strings.ToUpper(strings.TrimSpace(name))
				if strings.HasPrefix(upper, "OPENCLAW_") || strings.HasPrefix(upper, "PICOCLAW_") {
					delete(env, name)
				}
			}
			data, err := json.Marshal(env)
			if err != nil {
				return err
			}
			profile["env"] = data
		}
		data, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		agent[key] = data
	}
	return nil
}

func writeRuntimeMigrationFile(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".runtime-migration-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), path)
}
