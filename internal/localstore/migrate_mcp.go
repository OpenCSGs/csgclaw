package localstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"csgclaw/internal/mcpschema"
)

// MigrateMCPIdentities upgrades both catalog and Agent snapshots in one atomic
// write before services load state. It retains legal runtime keys, shares the
// replacement of an old key across snapshots, and never changes credentials.
// A synced temporary file replaces state.json without keeping history files.
func MigrateMCPIdentities(path string) error {
	rootStateMu.Lock()
	defer rootStateMu.Unlock()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("decode MCP migration state: %w", err)
	}
	type serverMap map[string]map[string]json.RawMessage
	decode := func(raw json.RawMessage) (serverMap, error) {
		var result serverMap
		err := json.Unmarshal(raw, &result)
		return result, err
	}
	var catalog serverMap
	if raw := root["mcpServers"]; len(raw) > 0 {
		if catalog, err = decode(raw); err != nil {
			return fmt.Errorf("decode MCP catalog: %w", err)
		}
	}
	var agents struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	var agentSection map[string]json.RawMessage
	if raw := root["agents"]; len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &agents); err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &agentSection); err != nil {
			return err
		}
	}
	snapshots := make([]serverMap, len(agents.Items))
	maps := []serverMap{catalog}
	for i, agent := range agents.Items {
		if raw := agent["mcpServers"]; len(raw) > 0 {
			snapshots[i], err = decode(raw)
			if err != nil {
				return fmt.Errorf("decode Agent MCP snapshot: %w", err)
			}
			maps = append(maps, snapshots[i])
		}
	}
	names := map[string]bool{}
	for _, servers := range maps {
		for name, entry := range servers {
			if entry == nil {
				return fmt.Errorf("MCP server %q must be an object", name)
			}
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	ids := map[string]string{}
	for _, name := range ordered {
		id := name
		if !mcpschema.ValidServerID(id) {
			id = mcpschema.NewServerID("", func(candidate string) bool { return names[candidate] })
			names[id] = true
		}
		ids[name] = id
	}
	changed := false
	convert := func(servers serverMap) (bool, error) {
		rekeyed := false
		for old, entry := range servers {
			// Iterate original names only; newly inserted IDs are already complete.
			id, original := ids[old]
			if !original {
				continue
			}
			if raw, exists := entry[mcpschema.DisplayNameKey]; !exists {
				entry[mcpschema.DisplayNameKey], _ = json.Marshal(old)
				changed = true
			} else {
				var label string
				if err := json.Unmarshal(raw, &label); err != nil || label == "" {
					return false, fmt.Errorf("MCP server %q has invalid display_name", old)
				}
			}
			if old != id {
				delete(servers, old)
				servers[id] = entry
				changed = true
				rekeyed = true
			}
		}
		return rekeyed, nil
	}
	if _, err = convert(catalog); err != nil {
		return err
	}
	for i, servers := range snapshots {
		rekeyed, convertErr := convert(servers)
		if convertErr != nil {
			return convertErr
		}
		if servers != nil {
			agents.Items[i]["mcpServers"], _ = json.Marshal(servers)
		}
		if rekeyed {
			var profile map[string]json.RawMessage
			if raw := agents.Items[i]["model_config"]; len(raw) > 0 {
				if err = json.Unmarshal(raw, &profile); err != nil {
					return err
				}
			}
			if profile == nil {
				profile = map[string]json.RawMessage{}
			}
			profile["env_restart_required"] = json.RawMessage("true")
			agents.Items[i]["model_config"], _ = json.Marshal(profile)
		}
	}
	if !changed {
		return nil
	}
	if catalog != nil {
		root["mcpServers"], _ = json.Marshal(catalog)
	}
	if agentSection != nil {
		agentSection["items"], _ = json.Marshal(agents.Items)
		root["agents"], _ = json.Marshal(agentSection)
	}
	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	if bytes.Equal(data, updated) {
		return nil
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".mcp-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(updated); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
