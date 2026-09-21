package modelcap

import "testing"

func TestSearchCapabilityRequiresKnownProviderAndModel(t *testing.T) {
	for _, test := range []struct {
		provider, model string
		want            bool
	}{
		{"codex", "gpt-5.5", true}, {"codex", "gpt-5.6-sol", true},
		{"api", "gpt-5.5", false}, {"csghub", "gpt-5.5", false},
		{"codex", "unknown", false}, {"codex", "gpt-4.1", false},
	} {
		if got := ForProviderModel(test.provider, test.model).SupportsSearchTool; got != test.want {
			t.Errorf("%s/%s search = %v", test.provider, test.model, got)
		}
	}
}
