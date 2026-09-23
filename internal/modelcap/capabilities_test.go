package modelcap

import "testing"

func TestSearchCapabilityRequiresKnownProviderAndModel(t *testing.T) {
	for _, test := range []struct {
		provider, model string
		want            bool
	}{
		{"codex", "gpt-5.5", true}, {"codex", "gpt-5.6-sol", true},
		{"codex", "gpt-6-sol", true}, {"codex", "gpt-6-luna", true},
		{"api", "gpt-6-sol", false}, {"api", "gpt-6-luna", false},
		{"api", "gpt-5.5", false}, {"csghub", "gpt-5.5", false},
		{"codex", "unknown", false}, {"codex", "gpt-4.1", false},
	} {
		if got := ForProviderModel(test.provider, test.model).SupportsSearchTool; got != test.want {
			t.Errorf("%s/%s search = %v", test.provider, test.model, got)
		}
	}
}

func TestKnownVisionModelsDeclareImageInput(t *testing.T) {
	for _, model := range []string{"qwen3.7-plus", "qwen3.7-plus-20260901", "qwen3-vl"} {
		got := ForProviderModel("api", model).InputModalities
		if len(got) != 2 || got[0] != "text" || got[1] != "image" {
			t.Errorf("api/%s input modalities = %v, want text and image", model, got)
		}
	}
	if got := ForProviderModel("api", "text-only").InputModalities; len(got) != 1 || got[0] != "text" {
		t.Fatalf("text-only input modalities = %v, want text", got)
	}
}
