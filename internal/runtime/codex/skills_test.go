package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
	toml "github.com/pelletier/go-toml/v2"
)

func TestSkillStatesSurviveConfigRefreshAndReenable(t *testing.T) {
	home := t.TempDir()
	disabled := map[string]skill.State{"reviewer": {Enabled: false}}
	render := func(content string, states map[string]skill.State) string {
		t.Helper()
		got, err := renderSkillStates(content, home, states, "")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	content := render("model = \"old\"\n", disabled)
	content = configureCodexHomeConfig(content, agentruntime.Profile{Provider: "api", BaseURL: "https://example.com", APIKey: "test", ModelID: "new"}, map[string]any{})
	if !strings.Contains(content, "path = "+strconv.Quote(filepath.Join(home, "skills", "reviewer", "SKILL.md"))) || !strings.Contains(content, "enabled = false") {
		t.Fatalf("配置刷新后缺少禁用状态：%s", content)
	}
	if got := render(content, disabled); strings.Count(got, "[[skills.config]]") != 1 {
		t.Fatalf("重复写入产生重复配置：%s", got)
	}
	enabled := render(content, map[string]skill.State{"reviewer": {Enabled: true}})
	if strings.Contains(enabled, "[[skills.config]]") || !strings.Contains(enabled, `approval_policy = "on-request"`) {
		t.Fatalf("重新启用后的配置错误：%s", enabled)
	}
}

func TestUpdateSkillConfigMergesInlineArray(t *testing.T) {
	for _, form := range []string{
		"[skills]\nconfig = [%s] # array\n",
		"skills.config = [\n%s, # trailing comma\n] # array\n",
		"skills = { config = [%s] } # array\n",
	} {
		t.Run(form, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, configFileName)
			skillPath := func(name string) string { return filepath.Join(home, "skills", name, "SKILL.md") }
			userEntry := fmt.Sprintf("{path=%q, enabled=false, note='keep'}", skillPath("one"))
			original := fmt.Sprintf(form, userEntry)
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			r := New(Dependencies{})
			defer r.Close()
			states := map[string]skill.State{"two": {Enabled: false}}
			update := func(states map[string]skill.State) string {
				t.Helper()
				if err := r.updateSkillConfig(home, states, "one"); err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			content := update(states)
			var cfg struct {
				Skills struct {
					Config []struct {
						Path    string
						Enabled bool
						Note    string
					}
				}
			}
			if err := toml.Unmarshal([]byte(content), &cfg); err != nil {
				t.Fatalf("invalid file on disk: %v\n%s", err, content)
			}
			entries := cfg.Skills.Config
			if len(entries) != 2 || entries[0].Path != skillPath("one") || !entries[0].Enabled || entries[0].Note != "keep" ||
				entries[1].Path != skillPath("two") || entries[1].Enabled {
				t.Fatalf("unexpected skill states: %+v", entries)
			}
			if again := update(states); again != content {
				t.Fatalf("repeat reconcile changed config:\n%s", again)
			}
			if got, want := update(nil), strings.Replace(original, "enabled=false", "enabled=true", 1); got != want {
				t.Fatalf("removing managed entry changed user TOML:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestUpdateSkillConfigRejectsInvalidTOMLBeforeWrite(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, configFileName)
	const original = "model = 'first'\nmodel = 'duplicate'\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	r := New(Dependencies{})
	defer r.Close()
	if err := r.updateSkillConfig(home, map[string]skill.State{"two": {Enabled: false}}, "one"); err == nil {
		t.Fatal("accepted duplicate TOML key")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != original {
		t.Fatalf("original config not preserved: %q, %v", b, err)
	}
}

func TestExplicitEnablePreservesCommentsAndUnrelatedTOML(t *testing.T) {
	for _, cfg := range []string{
		"[[skills.config]]\nenabled = false # target\npath = '/home/skills/demo/SKILL.md'\n[[skills.config]]\npath = '/other/SKILL.md'\nenabled = false # keep\n",
		"[skills]\nconfig = [{ path='/other/SKILL.md',enabled=false }, {enabled=false,path='/home/skills/demo/SKILL.md'}] # keep\n",
		"skills = { config = [{enabled=false,path='/home/skills/demo/SKILL.md'}] }\n[other]\nenabled=false\n",
	} {
		got, _, err := editSkillConfigEntries(cfg, filepath.FromSlash("/home/skills/demo/SKILL.md"), nil)
		if err != nil || got == cfg || strings.Replace(got, "true", "false", 1) != cfg {
			t.Fatalf("unexpected TOML edit: %q, %v", got, err)
		}
		if strings.Contains(got, "true # keep") || strings.Contains(got, "'/other/SKILL.md',enabled=true") {
			t.Fatal("changed another skill")
		}
	}
}
