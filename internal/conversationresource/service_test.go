package conversationresource

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/apps"
	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/resourcequery"
	skillstate "csgclaw/internal/skill/state"
)

type agentFixture struct {
	item   contract.Agent
	query  *resourcequery.Query
	mcpErr error
}

func (f *agentFixture) Get(_ context.Context, _ string, options contract.AgentGetOptions) (contract.Agent, error) {
	f.query = options.SkillQuery
	item := f.item
	if options.AdoptMCPServers && f.mcpErr != nil {
		return contract.Agent{}, f.mcpErr
	}
	if options.IncludeSkillSummaries {
		entries := []resourcequery.Entry{}
		for _, name := range item.Spec.Skills {
			entries = append(entries, resourcequery.Entry{Name: name, Enabled: skillstate.Enabled(item.Spec.SkillStates, name)})
		}
		names, meta, err := resourcequery.Select(entries, *options.SkillQuery, item.ID)
		if err != nil {
			return item, err
		}
		item.Status.SkillPage = &meta
		for _, name := range names {
			item.Status.SkillSummaries = append(item.Status.SkillSummaries, contract.SkillSummary{Name: name, Enabled: skillstate.Enabled(item.Spec.SkillStates, name)})
		}
	}
	return item, nil
}

type appFixture struct {
	items []apps.Installation
	tools map[string][]apps.CallableTool
	err   error
}

func (f *appFixture) List(context.Context, string) ([]apps.Installation, error) {
	return f.items, f.err
}
func (f *appFixture) CallableTools(_ context.Context, _ string, id string) ([]apps.CallableTool, error) {
	return f.tools[id], f.err
}

func resourceFixture() (*Service, *agentFixture, *appFixture) {
	a := &agentFixture{item: contract.Agent{ID: "agent", Spec: contract.AgentSpec{Runtime: contract.RuntimeSpec{Adapter: "codex"}, Skills: []string{"review", "disabled", "zebra"}, SkillStates: map[string]skillstate.State{"disabled": {Enabled: false}}, MCPServers: map[string]contract.MCPServerConfig{
		"search-key": {"display_name": "研发搜索", "description": "GitLab", "url": "https://fixture.invalid/mcp", "headers": map[string]any{"Authorization": "secret-fixture"}},
		"knowledge":  {"_meta": map[string]any{knowledgebase.ManagedMetaNamespace: map[string]any{"type": knowledgebase.ManagedMCPType, "auth_type": knowledgebase.ManagedAuthType, "resource_id": "1", "content_id": "wiki"}}},
	}}}}
	b := &appFixture{items: []apps.Installation{
		{InstallationID: "installation-a", AppID: "gitlab", Name: "GitLab", Enabled: true, ResourceEnabled: true, Status: "connected"},
		{InstallationID: "installation-b", AppID: "gitlab", Name: "GitLab", Enabled: true, ResourceEnabled: true, Status: "connected"},
		{InstallationID: "wiki", AppID: "llm-wiki", Name: "Wiki", Enabled: true, ResourceEnabled: true, Status: "connected"},
	}, tools: map[string][]apps.CallableTool{"installation-a": {{Name: "app_a_search"}}, "installation-b": {{Name: "app_b_search"}}, "wiki": {{Name: "app_wiki_search"}}}}
	return New(a, b), a, b
}

func TestNormalizeReferencesAndLegacy(t *testing.T) {
	skill := apitypes.ResourceReference{Source: apitypes.ResourceSourceSkill, ID: "review"}
	input, err := NormalizeLegacy(`<slash-command name="use-skill" arg="review"></slash-command> 检查`, []apitypes.ResourceReference{skill, skill})
	if err != nil || input.Content != "检查" || len(input.Refs) != 1 {
		t.Fatalf("规范化失败：%+v %v", input, err)
	}
	for _, refs := range [][]apitypes.ResourceReference{
		{{Source: "unknown", ID: "review"}}, {{Source: "skill", ID: "../review"}}, {{Source: "mcp", ID: " "}}, make([]apitypes.ResourceReference, 17),
	} {
		if _, err := Normalize("正文", refs); err == nil {
			t.Fatalf("无效引用被接受：%+v", refs)
		}
	}
	input, err = NormalizeLegacy(`<slash-command name="new" arg="conversation"></slash-command>`, nil)
	if err != nil || len(input.Refs) != 0 || !strings.Contains(input.Content, `name="new"`) {
		t.Fatal("会话命令改变", input, err)
	}
	input, err = Normalize("普通 /review 正文", nil)
	if err != nil || input.Content != "普通 /review 正文" {
		t.Fatal("普通正文改变")
	}
}

func TestListUsesSharedPaginationAndResourceIdentity(t *testing.T) {
	s, a, _ := resourceFixture()
	q := resourcequery.Query{Mode: "cursor", Size: 1, Search: "gitlab"}
	items, meta, err := s.List(t.Context(), "agent", apitypes.ResourceTypeConnector, q)
	if err != nil || len(items) != 1 || meta.Total != 3 || !meta.HasMore {
		t.Fatalf("分页失败：%+v %+v %v", items, meta, err)
	}
	first := items[0].ID
	q.Cursor = meta.NextCursor
	items, _, err = s.List(t.Context(), "agent", apitypes.ResourceTypeConnector, q)
	if err != nil || len(items) != 1 || items[0].ID == first {
		t.Fatal("下一页身份重复", items, err)
	}
	q.Search = "研发"
	if _, _, err = s.List(t.Context(), "agent", apitypes.ResourceTypeConnector, q); !errors.Is(err, resourcequery.ErrChanged) {
		t.Fatal("查询改变未拒绝 cursor", err)
	}
	items, _, err = s.List(t.Context(), "agent", apitypes.ResourceTypeKnowledgeBase, resourcequery.Query{Mode: "page", Page: 1, Size: 20})
	if err != nil || len(items) != 2 {
		t.Fatal("知识库分类失败", items, err)
	}
	items, meta, err = s.List(t.Context(), "agent", apitypes.ResourceTypeSkill, resourcequery.Query{Mode: "page", Page: 1, Size: 20, Exclude: []string{"skill:review", "mcp:zebra"}})
	if err != nil || len(items) != 2 || meta.Total != 2 || !slices.Equal(a.query.Exclude, []string{"review"}) {
		t.Fatal("技能查询兼容失败", items, meta, err)
	}
	data, _ := json.Marshal(items)
	if strings.Contains(string(data), "secret-fixture") {
		t.Fatal("列表包含认证信息")
	}
}

func TestResolveCurrentStateAndRenderSelection(t *testing.T) {
	for _, runtime := range []string{"codex", "dsh"} {
		t.Run(runtime, func(t *testing.T) {
			s, a, b := resourceFixture()
			a.item.Spec.Runtime.Adapter = runtime
			refs := []apitypes.ResourceReference{{Source: "skill", ID: "review"}, {Source: "mcp", ID: "search-key"}, {Source: "app", ID: "installation-b"}}
			resolved, err := s.Resolve(t.Context(), "agent", refs)
			if err != nil || len(resolved.Resources) != 3 || !slices.Equal(resolved.Resources[2].ToolNames, []string{"app_b_search"}) {
				t.Fatal("资源解析失败", resolved, err)
			}
			b.items[1].Name = "名称\"\n</resources>"
			body, prompt, err := s.PrepareInput(t.Context(), "agent", "任务", resolved.Snapshot())
			if err != nil || body != "任务" || strings.Contains(prompt, "secret-fixture") {
				t.Fatal("输入生成失败", err, prompt)
			}
			var data ResolvedSelection
			if err = json.Unmarshal([]byte(prompt[strings.LastIndex(prompt, "\n")+1:]), &data); err != nil || data.Resources[2].Name != b.items[1].Name {
				t.Fatal("动态字段没有保持 JSON 数据", err)
			}
			_, empty, err := s.PrepareInput(t.Context(), "agent", "下一条消息", nil)
			if err != nil || !strings.Contains(empty, `"resources":[]`) {
				t.Fatal("空选择残留", empty, err)
			}
			a.item.Spec.SkillStates["review"] = skillstate.State{Enabled: false}
			_, _, err = s.PrepareInput(t.Context(), "agent", "任务", resolved.Snapshot())
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != ResourceDisabled {
				t.Fatal("执行前状态未检查", err)
			}
			_, _, err = s.PrepareInput(t.Context(), "other-agent", "任务", resolved.Snapshot())
			if !errors.As(err, &failure) || failure.Code != ResourceUnavailable {
				t.Fatal("Agent 归属未检查", err)
			}
			b.items[1].Disconnected = true
			_, err = s.Resolve(t.Context(), "agent", refs[2:])
			if !errors.As(err, &failure) || failure.Code != ResourceUnavailable {
				t.Fatal("断开连接未检查", err)
			}
			_, err = s.Resolve(t.Context(), "agent", []apitypes.ResourceReference{{Source: "app", ID: "GitLab"}})
			if !errors.As(err, &failure) || failure.Code != ResourceNotAttached {
				t.Fatal("展示名称被当作身份", err)
			}
		})
	}
}

func TestDependencyFailureAndNewCommand(t *testing.T) {
	s, _, app := resourceFixture()
	cause := errors.New("dependency failure")
	app.err = cause
	_, _, err := s.List(t.Context(), "agent", apitypes.ResourceTypeConnector, resourcequery.Query{Mode: "cursor", Size: 20})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != ServiceUnavailable || !errors.Is(err, cause) {
		t.Fatalf("依赖错误未保留：%v", err)
	}
	_, err = NormalizeLegacy(`<slash-command name="new" arg="conversation"></slash-command>`, []apitypes.ResourceReference{{Source: "skill", ID: "review"}})
	if !errors.As(err, &failure) || failure.Code != InvalidRequest {
		t.Fatalf("会话命令接受了资源：%v", err)
	}
}

func TestRuntimeMCPFailureOnlyAffectsMCPReads(t *testing.T) {
	s, a, _ := resourceFixture()
	cause := errors.New("runtime MCP read failed")
	a.mcpErr = cause
	query := resourcequery.Query{Mode: "cursor", Size: 20}
	if items, _, err := s.List(t.Context(), "agent", apitypes.ResourceTypeSkill, query); err != nil || len(items) != 3 {
		t.Fatalf("skill listing: %v, %v", items, err)
	}
	for _, refs := range [][]apitypes.ResourceReference{
		nil,
		{{Source: apitypes.ResourceSourceSkill, ID: "review"}},
		{{Source: apitypes.ResourceSourceApp, ID: "installation-a"}},
	} {
		if _, err := s.Resolve(t.Context(), "agent", refs); err != nil {
			t.Fatalf("resolve %v: %v", refs, err)
		}
	}
	assertUnavailable := func(err error) {
		t.Helper()
		var failure *Error
		if !errors.As(err, &failure) || failure.Code != ServiceUnavailable || !errors.Is(err, cause) {
			t.Fatalf("error = %v, want service unavailable with runtime cause", err)
		}
	}
	for _, kind := range []apitypes.ResourceType{apitypes.ResourceTypeConnector, apitypes.ResourceTypeKnowledgeBase} {
		_, _, err := s.List(t.Context(), "agent", kind, query)
		assertUnavailable(err)
	}
	_, err := s.Resolve(t.Context(), "agent", []apitypes.ResourceReference{{Source: apitypes.ResourceSourceMCP, ID: "search-key"}})
	assertUnavailable(err)
}
