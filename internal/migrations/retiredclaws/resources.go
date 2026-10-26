package retiredclaws

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/mcpschema"
)

type retiredMemoryImport struct {
	path string
	data []byte
}

func retiredRuntimeName(kind string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(kind)), "_sandbox")
}

func importRetiredMCP(root, id, kind, workspace string, agent map[string]json.RawMessage) error {
	if _, explicit := agent["mcpServers"]; explicit {
		return nil
	}
	name := retiredRuntimeName(kind)
	filename := "openclaw.json"
	if name == "picoclaw" {
		filename = "config.json"
	}
	path := filepath.Join(root, "agents", id, "."+name, filename)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var native map[string]json.RawMessage
	if err := json.Unmarshal(data, &native); err != nil {
		return fmt.Errorf("decode retired runtime config %s: %w", path, err)
	}
	if name == "picoclaw" {
		var tools map[string]json.RawMessage
		if raw := native["tools"]; len(raw) == 0 {
			return nil
		} else if err := json.Unmarshal(raw, &tools); err != nil {
			return err
		}
		native = tools
	}
	var mcp map[string]json.RawMessage
	if raw := native["mcp"]; len(raw) == 0 || string(raw) == "null" {
		return nil
	} else if err := json.Unmarshal(raw, &mcp); err != nil {
		return err
	}
	if raw := mcp["enabled"]; string(raw) == "false" {
		return nil
	}
	var servers map[string]map[string]json.RawMessage
	if raw := mcp["servers"]; len(raw) == 0 || string(raw) == "null" {
		return nil
	} else if err := json.Unmarshal(raw, &servers); err != nil {
		return err
	}
	for _, entry := range servers {
		if entry == nil {
			return fmt.Errorf("retired runtime MCP server must be an object")
		}
		for source, target := range map[string]string{"connectionTimeoutMs": "startup_timeout_sec", "requestTimeoutMs": "tool_timeout_sec"} {
			raw, ok := entry[source]
			if !ok {
				continue
			}
			if _, exists := entry[target]; !exists {
				var milliseconds float64
				if err := json.Unmarshal(raw, &milliseconds); err != nil || milliseconds <= 0 || math.IsInf(milliseconds, 0) || milliseconds > float64(math.MaxInt64) {
					return fmt.Errorf("retired runtime MCP %s must be a positive timeout", source)
				}
				entry[target], _ = json.Marshal(int64(math.Ceil(milliseconds / 1000)))
			}
			delete(entry, source)
		}
		for _, key := range []string{"command", "cwd", "args", "env"} {
			if raw, exists := entry[key]; exists && workspace != "" {
				var value any
				if err := json.Unmarshal(raw, &value); err != nil {
					return err
				}
				entry[key], err = json.Marshal(rewriteRetiredWorkspace(value, workspace))
				if err != nil {
					return err
				}
			}
		}
	}
	encoded, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return err
	}
	normalized, err := mcpschema.NormalizeMCPServers(generic)
	if err != nil {
		return fmt.Errorf("import retired runtime MCP config: %w", err)
	}
	agent["mcpServers"], err = json.Marshal(normalized)
	return err
}

func rewriteRetiredWorkspace(value any, workspace string) any {
	switch v := value.(type) {
	case string:
		for _, guest := range []string{"/home/node/.openclaw/workspace", "/home/picoclaw/.picoclaw/workspace"} {
			v = strings.ReplaceAll(v, guest, workspace)
		}
		if v == "/workspace" || strings.HasPrefix(v, "/workspace/") {
			v = workspace + strings.TrimPrefix(v, "/workspace")
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = rewriteRetiredWorkspace(item, workspace)
		}
		return v
	case map[string]any:
		for key, item := range v {
			v[key] = rewriteRetiredWorkspace(item, workspace)
		}
		return v
	default:
		return value
	}
}

func planRetiredMemory(root, id, kind, workspace string) (*retiredMemoryImport, error) {
	if workspace == "" {
		return nil, nil
	}
	target := filepath.Join(root, "agents", id, ".codex", "home", "memories", "memory_summary.md")
	if _, err := os.Stat(target); err == nil {
		return nil, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	source := filepath.Join(workspace, "MEMORY.md")
	if retiredRuntimeName(kind) == "picoclaw" {
		source = filepath.Join(workspace, "memory", "MEMORY.md")
	}
	data, err := os.ReadFile(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	return &retiredMemoryImport{target, data}, nil
}

func writeRetiredMemory(item retiredMemoryImport) error {
	if err := os.MkdirAll(filepath.Dir(item.path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(item.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = file.Write(item.data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(item.path)
	}
	return err
}
