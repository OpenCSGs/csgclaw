package apps

import (
	"bytes"
	"context"
	feishutransport "csgclaw/internal/channel/feishu/transport"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"csgclaw/internal/localstore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGlobalResourceBindingsRefreshAndRevokeIndependently(t *testing.T) {
	ctx := context.Background()
	upstream := upstreamServer(t)
	s := newTestService(t, Options{})
	resource, err := s.Create(ctx, "", CreateRequest{AppID: "gitlab", Name: "Shared GitLab", Config: Config{URL: upstream.URL, AuthMode: "bearer"}, Credentials: Credentials{Token: "shared-secret-one"}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Bind(ctx, "a", BindRequest{ResourceID: resource.InstallationID, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Bind(ctx, "b", BindRequest{ResourceID: resource.InstallationID, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.InstallationID == b.InstallationID {
		t.Fatal("shared runtime identity")
	}
	if _, err = s.Bind(ctx, "a", BindRequest{ResourceID: resource.InstallationID}); err != ErrConflict {
		t.Fatal("duplicate binding accepted")
	}
	current, err := s.Get(ctx, "", resource.InstallationID)
	if err != nil || len(current.Tools) != 1 || current.Tools[0].Name != "inspect" {
		t.Fatal("global resource omitted discovered tools")
	}
	ac, bc := gatewayClient(t, s, "a"), gatewayClient(t, s, "b")
	check := func(client *mcp.ClientSession, binding Installation, token string) {
		t.Helper()
		r, err := client.CallTool(ctx, &mcp.CallToolParams{Name: toolName(binding.InstallationID, "inspect"), Arguments: map[string]any{}})
		if err != nil || r.IsError {
			t.Fatalf("tool call failed: %v", err)
		}
		if r.Content[0].(*mcp.TextContent).Text != "Bearer "+token {
			t.Fatal("stale or wrong credential")
		}
	}
	check(ac, a, "shared-secret-one")
	check(bc, b, "shared-secret-one")
	data, _ := os.ReadFile(s.path)
	if bytes.Count(data, []byte("shared-secret-one")) != 1 {
		t.Fatal("credentials copied into bindings")
	}
	if _, err = s.Disconnect(ctx, "a", a.InstallationID); err != nil {
		t.Fatal(err)
	}
	token := Credentials{Token: "shared-secret-two"}
	if _, err = s.Update(ctx, "", resource.InstallationID, UpdateRequest{Credentials: &token}); err != nil {
		t.Fatal(err)
	}
	check(bc, b, "shared-secret-two")
	tools, _ := ac.ListTools(ctx, nil)
	if len(tools.Tools) != 0 {
		t.Fatal("global refresh revived disconnected agent")
	}
	disabled := false
	if _, err = s.Update(ctx, "", resource.InstallationID, UpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	tools, _ = bc.ListTools(ctx, nil)
	if len(tools.Tools) != 0 {
		t.Fatal("global disable retained tools")
	}
	enabled := true
	if _, err = s.Update(ctx, "", resource.InstallationID, UpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	check(bc, b, "shared-secret-two")
	if err = s.DeleteAgent(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	global, err := s.Get(ctx, "", resource.InstallationID)
	if err != nil || len(global.Bindings) != 1 {
		t.Fatal("agent deletion removed global resource or another binding")
	}
	if err = s.Delete(ctx, "", resource.InstallationID); err != nil {
		t.Fatal(err)
	}
	tools, _ = bc.ListTools(ctx, nil)
	if len(tools.Tools) != 0 {
		t.Fatal("global deletion retained tools")
	}
	if items, _ := s.List(ctx, "b"); len(items) != 0 {
		t.Fatal("global deletion retained binding")
	}
}

func TestUpdateResourceToConnectorClearsAppOwnedCredentials(t *testing.T) {
	ctx := context.Background()
	upstream := upstreamServer(t)
	s := newTestService(t, Options{ResolveConnectorHTTP: func(context.Context, string, string, Config) (ConnectorHTTPConfig, error) {
		return ConnectorHTTPConfig{Endpoint: upstream.URL, Token: "managed-pat", TokenHeader: "Authorization", TokenPrefix: "Bearer "}, nil
	}})
	resource, err := s.Create(ctx, "", CreateRequest{
		AppID: "gitlab", Name: "GitLab", Config: Config{URL: upstream.URL, AuthMode: "bearer"},
		Credentials: Credentials{Token: "stale-app-token", AppID: "stale-app-id", AppSecret: "stale-secret", Env: map[string]string{"TOKEN": "stale-env"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	connectorConfig := Config{Transport: "http", URL: upstream.URL, GitLabBaseURL: "https://gitlab.example.com", AuthMode: "connector", ConnectorID: "gitlab"}
	updated, err := s.Update(ctx, "", resource.InstallationID, UpdateRequest{Config: &connectorConfig})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"token", "app_id", "app_secret", "env.TOKEN"} {
		if updated.CredentialsSet[key] {
			t.Fatalf("connector resource retained App-owned credential %q: %+v", key, updated.CredentialsSet)
		}
	}
	binding, err := s.Bind(ctx, "agent", BindRequest{ResourceID: resource.InstallationID, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := gatewayClient(t, s, "agent").CallTool(ctx, &mcp.CallToolParams{Name: toolName(binding.InstallationID, "inspect"), Arguments: map[string]any{}})
	if err != nil || result.Content[0].(*mcp.TextContent).Text != "Bearer managed-pat" {
		t.Fatalf("connector binding used stale App credential: result=%+v err=%v", result, err)
	}
}

func TestExistingInstallationBecomesResourceWithoutChangingToolIdentity(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t, Options{})
	path := s.path
	_ = s.Close()
	id := "00112233445566778899aabbccddeeff"
	pkg, _ := loadPackages()
	now := time.Now().UTC()
	old := record{Installation: Installation{InstallationID: id, AgentID: "manager", AppID: "gitlab", Name: "Existing", Enabled: true, Disconnected: true, Config: Config{URL: "http://localhost:9999/mcp", AuthMode: "bearer"}, CreatedAt: now, UpdatedAt: now}, Package: pkg["gitlab"], Credentials: Credentials{Token: "existing-secret"}}
	if err := localstore.WriteSection(path, installationsSection, map[string]record{id: old}); err != nil {
		t.Fatal(err)
	}
	restored, err := NewService(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	binding, err := restored.Get(ctx, "manager", id)
	if err != nil || binding.ResourceID == "" || binding.InstallationID != id || !binding.Disconnected {
		t.Fatal("existing binding identity or intent lost")
	}
	global, err := restored.Get(ctx, "", binding.ResourceID)
	if err != nil || global.Name != "Existing" || !global.CredentialsSet["token"] {
		t.Fatal("existing resource not preserved")
	}
	data, _ := os.ReadFile(path)
	if bytes.Count(data, []byte("existing-secret")) != 1 {
		t.Fatal("secret persisted more than once")
	}
	_ = restored.Close()
	again, err := NewService(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	items, _ := again.List(ctx, "")
	if len(items) != 1 {
		t.Fatal("resource conversion was repeated")
	}
}

func TestGlobalFeishuResourceUsesItsOwnIdentityForEveryAgent(t *testing.T) {
	ctx := context.Background()
	upstream := mcp.NewServer(&mcp.Implementation{Name: "feishu", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "identity", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: r.Extra.Header.Get("lark-access-token")}}}, nil
	})
	server := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, nil))
	t.Cleanup(server.Close)
	s := newTestService(t, Options{FeishuTokenSource: func(appID, secret string) feishutransport.TenantTokenSource {
		return &fakeTenantSource{prefix: appID, generation: 1}
	}})
	resource, err := s.Create(ctx, "", CreateRequest{AppID: "feishu", Name: "Team Feishu", Config: Config{URL: server.URL, AuthMode: "feishu", CredentialSource: "manual"}, Credentials: Credentials{AppID: "shared-app", AppSecret: "global-feishu-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if resource.Status != "configured" {
		t.Fatal("global resource claimed an Agent identity")
	}
	for _, agentID := range []string{"alpha", "beta"} {
		binding, err := s.Bind(ctx, agentID, BindRequest{ResourceID: resource.InstallationID, Connect: true})
		if err != nil {
			t.Fatal(err)
		}
		client := gatewayClient(t, s, agentID)
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: toolName(binding.InstallationID, "identity"), Arguments: map[string]any{}})
		if err != nil || result.Content[0].(*mcp.TextContent).Text != "shared-app-1" {
			t.Fatal("Agent used another channel identity")
		}
	}
	data, _ := os.ReadFile(s.path)
	if bytes.Contains(data, []byte("private-alpha")) || bytes.Contains(data, []byte("private-beta")) {
		t.Fatal("channel secrets copied into shared resource or binding")
	}
}

func TestAgentChannelReferencesAreRejected(t *testing.T) {
	s := newTestService(t, Options{})
	_, err := s.Probe(t.Context(), "agent", ProbeRequest{AppID: "feishu", Config: Config{URL: "http://localhost:9999/mcp", AuthMode: "feishu", CredentialSource: "feishu_channel"}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("Agent channel reference accepted")
	}
}

func TestDisabledResourceRetainsNewBindingConnectIntent(t *testing.T) {
	ctx := t.Context()
	upstream := upstreamServer(t)
	s := newTestService(t, Options{})
	resource, err := s.Create(ctx, "", CreateRequest{AppID: "gitlab", Name: "Disabled shared", Config: Config{URL: upstream.URL}, Credentials: Credentials{Token: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	enabled := false
	if _, err = s.Update(ctx, "", resource.InstallationID, UpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	binding, err := s.Bind(ctx, "agent", BindRequest{ResourceID: resource.InstallationID, Connect: true})
	if err != nil || binding.Status != "disabled" {
		t.Fatal("disabled resource binding failed")
	}
	enabled = true
	if _, err = s.Update(ctx, "", resource.InstallationID, UpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	binding, err = s.Get(ctx, "agent", binding.InstallationID)
	if err != nil || binding.Status != "connected" {
		t.Fatal("connect intent lost when resource was disabled")
	}
}

func TestExistingResourceNamesRemainUniqueAfterConversion(t *testing.T) {
	s := newTestService(t, Options{})
	path := s.path
	_ = s.Close()
	pkg, _ := loadPackages()
	records := map[string]record{}
	for i, item := range []struct{ agent, name string }{{"a", "Work (c)"}, {"b", "Work"}, {"c", "Work"}} {
		id := fmt.Sprintf("%032x", i+1)
		records[id] = record{Installation: Installation{InstallationID: id, AgentID: item.agent, AppID: "gitlab", Name: item.name, Enabled: true}, Package: pkg["gitlab"]}
	}
	if err := localstore.WriteSection(path, installationsSection, records); err != nil {
		t.Fatal(err)
	}
	restored, err := NewService(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	items, _ := restored.List(t.Context(), "")
	names := map[string]bool{}
	for _, item := range items {
		if names[item.Name] {
			t.Fatal("converted resource names collide")
		}
		names[item.Name] = true
	}
	if len(items) != 3 {
		t.Fatal("conversion merged independent resources")
	}
}

func TestVerifiedDraftRejectsConcurrentCredentialChanges(t *testing.T) {
	ctx := t.Context()
	server := upstreamServer(t)
	s := newTestService(t, Options{})
	resource, err := s.Create(ctx, "", CreateRequest{AppID: "llm-wiki", Name: "Versioned", Config: Config{URL: server.URL}, Credentials: Credentials{Token: "old"}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := resource.Config
	if err = s.ProbeResourceUpdate(ctx, resource.ResourceID, resource.UpdatedAt, UpdateRequest{Config: &cfg}); err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Token: "changed"}
	if _, err = s.Update(ctx, "", resource.ResourceID, UpdateRequest{Credentials: &creds}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateResourceAt(ctx, resource.ResourceID, UpdateRequest{Config: &cfg}, resource.UpdatedAt); !errors.Is(err, ErrChanged) {
		t.Fatal("unchecked concurrent credentials accepted")
	}
}

func TestDraftProbeUsesCurrentCredentialsWithoutSaving(t *testing.T) {
	upstream := upstreamServer(t)
	resolved := false
	s := newTestService(t, Options{ResolveConnectorHTTP: func(_ context.Context, _, _ string, cfg Config) (ConnectorHTTPConfig, error) {
		resolved = true
		return ConnectorHTTPConfig{Endpoint: cfg.URL, Token: "managed", TokenHeader: "PRIVATE-TOKEN"}, nil
	}})
	resource, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: "Original", Config: Config{URL: upstream.URL, AuthMode: "bearer"}, Credentials: Credentials{Token: "saved-bearer"}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{URL: upstream.URL, AuthMode: "connector", GitLabBaseURL: "https://gitlab.example.com"}
	for _, token := range []string{"", "draft-pat"} {
		resolved = false
		result, err := s.Probe(t.Context(), "", ProbeRequest{InstallationID: resource.ResourceID, Config: cfg, Credentials: Credentials{Token: token, Headers: map[string]string{"Authorization": "Bearer fixture"}}})
		if err != nil || !result.Connected {
			t.Fatalf("probe: %v", err)
		}
		if resolved != (token == "") {
			t.Fatal("probe did not use the current credential source")
		}
		after, err := os.ReadFile(s.path)
		if err != nil || string(before) != string(after) {
			t.Fatal("probe changed persisted settings")
		}
	}
}

func TestFeishuViewShowsAppIDWithoutSecrets(t *testing.T) {
	s := newTestService(t, Options{})
	item, err := s.Create(t.Context(), "", CreateRequest{AppID: "feishu", Name: "Feishu", Config: Config{URL: "https://example.com/mcp", AuthMode: "feishu"}, Credentials: Credentials{AppID: "cli_visible_id", AppSecret: "private-app-secret", Token: "private-token"}})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Get(t.Context(), "", item.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []Installation{item, saved} {
		if resource.FeishuAppID != "cli_visible_id" {
			t.Fatal("saved App ID is not visible")
		}
		data, err := json.Marshal(resource)
		if err != nil || bytes.Contains(data, []byte("private-app-secret")) || bytes.Contains(data, []byte("private-token")) {
			t.Fatal("view exposed secrets")
		}
	}
}
