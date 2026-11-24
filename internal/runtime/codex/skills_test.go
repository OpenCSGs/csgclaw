package codex

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
	skill "csgclaw/internal/skill/state"
)

func TestSkillStatesSurviveConfigRefreshAndReenable(t *testing.T) {
	home := t.TempDir()
	disabled := map[string]skill.State{"reviewer": {Enabled: false}}
	content := renderSkillStates("model = \"old\"\n", home, disabled)
	content = configureCodexHomeConfig(content, agentruntime.Profile{Provider: "api", BaseURL: "https://example.com", APIKey: "test", ModelID: "new"}, map[string]any{})
	if !strings.Contains(content, "path = "+strconv.Quote(filepath.Join(home, "skills", "reviewer", "SKILL.md"))) || !strings.Contains(content, "enabled = false") {
		t.Fatalf("配置刷新后缺少禁用状态：%s", content)
	}
	if got := renderSkillStates(content, home, disabled); strings.Count(got, "[[skills.config]]") != 1 {
		t.Fatalf("重复写入产生重复配置：%s", got)
	}
	enabled := renderSkillStates(content, home, map[string]skill.State{"reviewer": {Enabled: true}})
	if strings.Contains(enabled, "[[skills.config]]") || !strings.Contains(enabled, `approval_policy = "on-request"`) {
		t.Fatalf("重新启用后的配置错误：%s", enabled)
	}
}

func TestExplicitEnablePreservesCommentsAndUnrelatedTOML(t *testing.T) {
	for _, cfg := range []string{
		"[[skills.config]]\nenabled = false # target\npath = '/home/skills/demo/SKILL.md'\n[[skills.config]]\npath = '/other/SKILL.md'\nenabled = false # keep\n",
		"[skills]\nconfig = [{ path='/other/SKILL.md',enabled=false }, {enabled=false,path='/home/skills/demo/SKILL.md'}] # keep\n",
		"skills = { config = [{enabled=false,path='/home/skills/demo/SKILL.md'}] }\n[other]\nenabled=false\n",
	} {
		got, err := enableSkillConfigEntries(cfg, filepath.FromSlash("/home/skills/demo/SKILL.md"))
		if err != nil || got == cfg || strings.Replace(got, "true", "false", 1) != cfg {
			t.Fatalf("unexpected TOML edit: %q, %v", got, err)
		}
		if strings.Contains(got, "true # keep") || strings.Contains(got, "'/other/SKILL.md',enabled=true") {
			t.Fatal("changed another skill")
		}
	}
}
