package localstore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/mcpschema"
)

func TestMigrateMCPIdentitiesPreservesSnapshotsAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := []byte(`{
 "version":1,"unrelated":{"large":9007199254740993},
 "mcpServers":{"必应搜索中文":{"url":"https://catalog.example","headers":{"Authorization":"Bearer fixture-secret"}},"Web Fetch":{"command":"fetch","args":["--stdio"],"enabled":false},"legal-id":{"url":"https://legal.example"}},
 "agents":{"extra":true,"items":[
  {"id":"agent-one","model_config":{"provider":"api"},"mcpServers":{"必应搜索中文":{"url":"https://agent.example","tool_timeout_sec":80},"Web Fetch":{"command":"fetch"},"legal-id":{"url":"https://legal.example"}}},
  {"id":"agent-two","mcpServers":{"必应搜索中文":{"url":"https://second.example"}}},
  {"id":"agent-empty","mcpServers":{}},{"id":"agent-unmanaged"},{"id":"agent-null","mcpServers":null}
 ]}}
 `)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateMCPIdentities(path); err != nil {
		t.Fatal(err)
	}
	migrated, _ := os.ReadFile(path)
	if !bytes.Contains(migrated, []byte("9007199254740993")) {
		t.Fatal("unrelated number lost precision")
	}
	var state map[string]any
	if err := json.Unmarshal(migrated, &state); err != nil {
		t.Fatal(err)
	}
	catalog := state["mcpServers"].(map[string]any)
	ids := map[string]string{}
	for id, raw := range catalog {
		if !mcpschema.ValidServerID(id) {
			t.Fatalf("illegal ID %q", id)
		}
		ids[mcpschema.ServerDisplayName(id, raw)] = id
	}
	if ids["legal-id"] != "legal-id" || ids["必应搜索中文"] == "" || ids["Web Fetch"] == "" {
		t.Fatalf("migration IDs: %#v", ids)
	}
	source := catalog[ids["必应搜索中文"]].(map[string]any)
	if source["headers"].(map[string]any)["Authorization"] != "Bearer fixture-secret" {
		t.Fatal("credential changed")
	}
	items := state["agents"].(map[string]any)["items"].([]any)
	for i := range 2 {
		agent := items[i].(map[string]any)
		snapshot := agent["mcpServers"].(map[string]any)
		if _, ok := snapshot[ids["必应搜索中文"]]; !ok {
			t.Fatal("catalog and agent IDs diverged")
		}
		if agent["model_config"].(map[string]any)["env_restart_required"] != true {
			t.Fatal("rekeyed runtime was not marked for refresh")
		}
	}
	if got := items[0].(map[string]any)["mcpServers"].(map[string]any)[ids["必应搜索中文"]].(map[string]any)["url"]; got != "https://agent.example" {
		t.Fatal("Agent snapshot overwritten with catalog config")
	}
	if len(items[2].(map[string]any)["mcpServers"].(map[string]any)) != 0 {
		t.Fatal("empty map lost")
	}
	if _, ok := items[3].(map[string]any)["mcpServers"]; ok {
		t.Fatal("unmanaged agent became managed")
	}
	if items[4].(map[string]any)["mcpServers"] != nil {
		t.Fatal("null map lost")
	}
	assertMCPMigrationLeavesOnlyState(t, path)
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("insecure state mode")
	}
	if err := MigrateMCPIdentities(path); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if !bytes.Equal(again, migrated) {
		t.Fatal("second migration changed state")
	}
	assertMCPMigrationLeavesOnlyState(t, path)
}

func TestMigrateMCPIdentitiesRejectsMalformedStateWithoutWriting(t *testing.T) {
	for _, body := range []string{`{"mcpServers":{"中文":{"url":"test"}},"agents":{"items":[{"mcpServers":{"broken":42}}]}}`, `{"mcpServers":{"valid":{"display_name":42,"url":"test"}}}`} {
		path := filepath.Join(t.TempDir(), "state.json")
		_ = os.WriteFile(path, []byte(body), 0600)
		if err := MigrateMCPIdentities(path); err == nil {
			t.Fatal("expected migration failure")
		}
		actual, _ := os.ReadFile(path)
		if string(actual) != body {
			t.Fatal("failed migration changed state")
		}
		assertMCPMigrationLeavesOnlyState(t, path)
	}
}

func assertMCPMigrationLeavesOnlyState(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("migration left backup or temporary files: %v", entries)
	}
}
