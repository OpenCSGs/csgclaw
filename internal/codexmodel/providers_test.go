package codexmodel

import "testing"

func TestMiMoMetadataUsesDocumentedToolMode(t *testing.T) {
	got := Metadata(Profile{Provider: "api", ModelID: "mimo-v2.6-pro"})
	if got["tool_mode"] != "code_mode_only" || got["apply_patch_tool_type"] != "freeform" ||
		got["supports_reasoning_summaries"] != true || got["default_reasoning_summary"] != "none" {
		t.Fatalf("MiMo catalog entry is incompatible: %+v", got)
	}
	if got := Metadata(Profile{Provider: "api", ModelID: "other-model"}); got["tool_mode"] != nil {
		t.Errorf("vendor tool mode leaked to other models: %+v", got)
	}
}
