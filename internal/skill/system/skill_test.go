package system

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNamesReadsEmbeddedDirectory(t *testing.T) {
	got, err := Names()
	if err != nil {
		t.Fatalf("Names() error = %v", err)
	}
	if !slicesContains(got, "skill-installer") {
		t.Fatalf("Names() = %#v, want embedded skill-installer", got)
	}
	if !slicesContains(got, "skill-creator") {
		t.Fatalf("Names() = %#v, want embedded skill-creator", got)
	}
	if !slicesContains(got, "csgclaw-interactive-output-demo") {
		t.Fatalf("Names() = %#v, want embedded interactive output demo", got)
	}
}

func TestDefaultNamesExcludeManagerOnlyDemo(t *testing.T) {
	got, err := DefaultNames()
	if err != nil {
		t.Fatalf("DefaultNames() error = %v", err)
	}
	if slicesContains(got, "csgclaw-interactive-output-demo") {
		t.Fatalf("DefaultNames() = %#v, want Manager-only demo excluded", got)
	}
	for _, name := range []string{"skill-creator", "skill-installer"} {
		if !slicesContains(got, name) {
			t.Fatalf("DefaultNames() = %#v, want %s", got, name)
		}
	}
}

func TestResolveSource(t *testing.T) {
	source, err := ResolveSource("skill-installer")
	if err != nil {
		t.Fatalf("ResolveSource() error = %v", err)
	}
	data, err := source.FS.Open(source.RootPath + "/SKILL.md")
	if err != nil {
		t.Fatalf("Open(system skill) error = %v", err)
	}
	defer data.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(data); err != nil {
		t.Fatalf("ReadFrom(system skill) error = %v", err)
	}
	if !strings.Contains(buf.String(), "registry skill search") {
		t.Fatalf("system skill content = %q, want skill-installer instructions", buf.String())
	}
}

func TestSkillCreatorUsesRuntimeNeutralDefaultPath(t *testing.T) {
	source, err := ResolveSource("skill-creator")
	if err != nil {
		t.Fatalf("ResolveSource(skill-creator) error = %v", err)
	}
	data, err := source.FS.Open(source.RootPath + "/SKILL.md")
	if err != nil {
		t.Fatalf("Open(skill-creator) error = %v", err)
	}
	defer data.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(data); err != nil {
		t.Fatalf("ReadFrom(skill-creator) error = %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "~/.openclaw/workspace/skills") || !strings.Contains(content, "~/.picoclaw/workspace/skills") {
		t.Fatalf("skill-creator content = %q, want runtime-neutral workspace skills paths", content)
	}
}

func TestResolveInteractiveOutputDemoUsesManagerTemplate(t *testing.T) {
	source, err := ResolveSource("csgclaw-interactive-output-demo")
	if err != nil {
		t.Fatalf("ResolveSource(csgclaw-interactive-output-demo) error = %v", err)
	}
	data, err := source.FS.Open(source.RootPath + "/scripts/emit_demo.py")
	if err != nil {
		t.Fatalf("Open(emit_demo.py) error = %v", err)
	}
	defer data.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(data); err != nil {
		t.Fatalf("ReadFrom(emit_demo.py) error = %v", err)
	}
	if !strings.Contains(buf.String(), "::csgclaw-output::") {
		t.Fatalf("emit_demo.py content = %q, want structured output emitter", buf.String())
	}
}

func TestListRejectsEmbeddedDirectoryWithoutSkillFile(t *testing.T) {
	_, err := listFS(fstest.MapFS{
		"embed/broken/README.md": {Data: []byte("# Broken\n")},
	}, "embed")
	if err == nil || !strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("listFS() error = %v, want missing SKILL.md error", err)
	}
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
