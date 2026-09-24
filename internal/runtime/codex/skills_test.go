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
