package mcpschema

import "testing"

func TestServerIdentitiesHandleUnicodeSpacesAndCollisions(t *testing.T) {
	input := map[string]any{"搜索": map[string]any{"url": "https://example.test"}, "Web Fetch": map[string]any{"command": "fetch"}, "Web_Fetch": map[string]any{"command": "fetch"}}
	result, err := WithServerIdentities(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatal("colliding names overwrote entries")
	}
	for id, raw := range result {
		if !ValidServerID(id) || ServerDisplayName(id, raw) == "" {
			t.Fatal("invalid identity")
		}
	}
	if _, exists := result["Web_Fetch"]; !exists {
		t.Fatal("legal ID changed")
	}
	if _, exists := input["搜索"].(map[string]any)[DisplayNameKey]; exists {
		t.Fatal("input mutated")
	}
	for id, raw := range result {
		raw.(map[string]any)[DisplayNameKey] = "renamed " + id
	}
	renamed, err := WithServerIdentities(result)
	if err != nil {
		t.Fatal(err)
	}
	for id := range result {
		if _, exists := renamed[id]; !exists {
			t.Fatal("rename changed identity")
		}
	}
	if id := NewServerID("Web_Fetch", func(id string) bool { return id == "Web_Fetch" }); id == "Web_Fetch" || !ValidServerID(id) {
		t.Fatal("occupied identity reused")
	}
}

func TestTemplateServerIDsPreserveRenamesAndOlderImports(t *testing.T) {
	incoming := map[string]any{"搜索 docs": map[string]any{"url": "https://example.test"}}
	first, err := TemplateServerIDs(nil, incoming)
	if err != nil {
		t.Fatal(err)
	}
	for id, raw := range first {
		raw.(map[string]any)[DisplayNameKey] = "custom name"
		again, err := TemplateServerIDs(first, incoming)
		if err != nil || again[id] == nil {
			t.Fatalf("second import = %v, %v", again, err)
		}
		if _, overwritesName := again[id].(map[string]any)[DisplayNameKey]; overwritesName {
			t.Fatal("template overwrote a display name it did not supply")
		}
	}
	older, err := WithServerIdentities(incoming)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := TemplateServerIDs(older, incoming)
	if err != nil {
		t.Fatal(err)
	}
	for id := range older {
		if replay[id] == nil {
			t.Fatal("existing runtime ID was lost")
		}
	}
	older["mcp_1234567890abcdef1234567890abcdef"] = map[string]any{DisplayNameKey: "搜索 docs"}
	if _, err := TemplateServerIDs(older, incoming); err == nil {
		t.Fatal("ambiguous legacy server selected")
	}
}
