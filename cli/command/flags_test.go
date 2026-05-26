package command

import (
	"flag"
	"testing"
)

func TestParseFlexibleFlagsAfterPositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "max results")
	if err := ParseFlexible(fs, []string{"gitlab", "--limit", "10"}); err != nil {
		t.Fatalf("ParseFlexible() error = %v", err)
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "gitlab" {
		t.Fatalf("Args() = %v, want [gitlab]", got)
	}
	if *limit != 10 {
		t.Fatalf("limit = %d, want 10", *limit)
	}
}

func TestParseFlexibleFlagsBeforePositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "max results")
	if err := ParseFlexible(fs, []string{"--limit", "10", "gitlab"}); err != nil {
		t.Fatalf("ParseFlexible() error = %v", err)
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "gitlab" {
		t.Fatalf("Args() = %v, want [gitlab]", got)
	}
	if *limit != 10 {
		t.Fatalf("limit = %d, want 10", *limit)
	}
}

func TestParseFlexibleBoolFlagBeforePositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite")
	if err := ParseFlexible(fs, []string{"--force", "my-skill"}); err != nil {
		t.Fatalf("ParseFlexible() error = %v", err)
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "my-skill" {
		t.Fatalf("Args() = %v, want [my-skill]", got)
	}
	if !*force {
		t.Fatal("force = false, want true")
	}
}

func TestParseFlexibleBoolFlagAfterPositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite")
	if err := ParseFlexible(fs, []string{"my-skill", "--force"}); err != nil {
		t.Fatalf("ParseFlexible() error = %v", err)
	}
	if got := fs.Args(); len(got) != 1 || got[0] != "my-skill" {
		t.Fatalf("Args() = %v, want [my-skill]", got)
	}
	if !*force {
		t.Fatal("force = false, want true")
	}
}

func newSkillSearchFlags() (*flag.FlagSet, *int) {
	fs := flag.NewFlagSet("skill search", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "maximum number of results")
	return fs, limit
}

func newSkillGetFlags() (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet("skill get", flag.ContinueOnError)
	version := fs.String("version", "", "show a specific published version")
	registry := fs.String("registry", "", "registry: opencsg or clawhub")
	return fs, version, registry
}

func newSkillInstallFlags() (*flag.FlagSet, *string, *string, *bool) {
	fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
	fs.String("skills-dir", "", "workspace skills directory")
	version := fs.String("version", "", "install this semver version")
	registry := fs.String("registry", "", "registry: opencsg or clawhub")
	force := fs.Bool("force", false, "overwrite an existing skill directory")
	return fs, version, registry, force
}

func newSkillVersionsFlags() (*flag.FlagSet, *int, *string) {
	fs := flag.NewFlagSet("skill versions", flag.ContinueOnError)
	limit := fs.Int("limit", 50, "maximum number of versions to list")
	registry := fs.String("registry", "", "registry: opencsg or clawhub")
	return fs, limit, registry
}

func TestParseFlexibleSkillSubcommands(t *testing.T) {
	t.Run("search flags after query", func(t *testing.T) {
		fs, limit := newSkillSearchFlags()
		if err := ParseFlexible(fs, []string{"gitlab", "--limit", "5"}); err != nil {
			t.Fatalf("ParseFlexible() error = %v", err)
		}
		if got := fs.Args(); len(got) != 1 || got[0] != "gitlab" {
			t.Fatalf("Args() = %v, want [gitlab]", got)
		}
		if *limit != 5 {
			t.Fatalf("limit = %d, want 5", *limit)
		}
	})

	t.Run("get version after slug", func(t *testing.T) {
		fs, version, _ := newSkillGetFlags()
		if err := ParseFlexible(fs, []string{"my-skill", "--version", "1.0.0"}); err != nil {
			t.Fatalf("ParseFlexible() error = %v", err)
		}
		if got := fs.Args(); len(got) != 1 || got[0] != "my-skill" {
			t.Fatalf("Args() = %v, want [my-skill]", got)
		}
		if *version != "1.0.0" {
			t.Fatalf("version = %q, want 1.0.0", *version)
		}
	})

	t.Run("get registry after slug", func(t *testing.T) {
		fs, _, registry := newSkillGetFlags()
		if err := ParseFlexible(fs, []string{"my-skill", "--registry", "opencsg"}); err != nil {
			t.Fatalf("ParseFlexible() error = %v", err)
		}
		if got := fs.Args(); len(got) != 1 || got[0] != "my-skill" {
			t.Fatalf("Args() = %v, want [my-skill]", got)
		}
		if *registry != "opencsg" {
			t.Fatalf("registry = %q, want opencsg", *registry)
		}
	})

	t.Run("install flags after slug", func(t *testing.T) {
		fs, version, registry, force := newSkillInstallFlags()
		if err := ParseFlexible(fs, []string{"my-skill", "--version", "2.0.0", "--registry", "clawhub", "--force"}); err != nil {
			t.Fatalf("ParseFlexible() error = %v", err)
		}
		if got := fs.Args(); len(got) != 1 || got[0] != "my-skill" {
			t.Fatalf("Args() = %v, want [my-skill]", got)
		}
		if *version != "2.0.0" || *registry != "clawhub" || !*force {
			t.Fatalf("version=%q registry=%q force=%t", *version, *registry, *force)
		}
	})

	t.Run("versions flags after slug", func(t *testing.T) {
		fs, limit, registry := newSkillVersionsFlags()
		if err := ParseFlexible(fs, []string{"my-skill", "--limit", "10", "--registry", "opencsg"}); err != nil {
			t.Fatalf("ParseFlexible() error = %v", err)
		}
		if got := fs.Args(); len(got) != 1 || got[0] != "my-skill" {
			t.Fatalf("Args() = %v, want [my-skill]", got)
		}
		if *limit != 10 || *registry != "opencsg" {
			t.Fatalf("limit=%d registry=%q", *limit, *registry)
		}
	})
}
