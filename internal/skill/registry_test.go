package skill

import (
	"testing"
)

func TestMergeSearchResultsPrefersEarlierRegistry(t *testing.T) {
	t.Parallel()

	batches := [][]SearchResult{
		{
			{Registry: RegistryOpenCSG, Slug: "shared", Score: 1},
			{Registry: RegistryOpenCSG, Slug: "opencsg-only", Score: 2},
		},
		{
			{Registry: RegistryClawHub, Slug: "shared", Score: 9},
			{Registry: RegistryClawHub, Slug: "clawhub-only", Score: 3},
		},
	}
	merged := mergeSearchResults(batches)
	if len(merged) != 3 {
		t.Fatalf("len = %d, want 3", len(merged))
	}
	if merged[0].Slug != "shared" || merged[0].Registry != RegistryOpenCSG {
		t.Fatalf("shared = %#v", merged[0])
	}
	if merged[1].Slug != "opencsg-only" {
		t.Fatalf("second = %#v", merged[1])
	}
	if merged[2].Slug != "clawhub-only" {
		t.Fatalf("third = %#v", merged[2])
	}
}

func TestParseRegistry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want RegistryID
	}{
		{"", ""},
		{"opencsg", RegistryOpenCSG},
		{"clawhub", RegistryClawHub},
		{"official", RegistryClawHub},
	}
	for _, tc := range cases {
		got, err := ParseRegistry(tc.in)
		if err != nil {
			t.Fatalf("ParseRegistry(%q) error = %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseRegistry(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
