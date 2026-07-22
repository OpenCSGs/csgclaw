package agent

import (
	"slices"
	"testing"
)

func TestDefaultSystemSkillNamesExcludeManagerOnlyDemo(t *testing.T) {
	names, err := defaultSystemSkillNames()
	if err != nil {
		t.Fatalf("defaultSystemSkillNames() error = %v", err)
	}
	if slices.Contains(names, "csgclaw-interactive-output-demo") {
		t.Fatalf("defaultSystemSkillNames() = %#v, want Manager-only demo excluded", names)
	}
	for _, name := range []string{"skill-creator", "skill-installer"} {
		if !slices.Contains(names, name) {
			t.Fatalf("defaultSystemSkillNames() = %#v, want %s", names, name)
		}
	}
}
