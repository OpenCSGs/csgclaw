package conversationresource

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/apps"
	"csgclaw/internal/localstore"
	"csgclaw/internal/resourcequery"
	skillstate "csgclaw/internal/skill/state"
)

func TestCLISelectionChecksAuthorizationAndAppliedConfiguration(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*apps.Installation)
		reason string
		code   string
	}{
		{name: "ready", change: func(*apps.Installation) {}},
		{name: "binding_disabled", change: func(a *apps.Installation) { a.Enabled = false }, reason: "disabled", code: ResourceDisabled},
		{name: "resource_disabled", change: func(a *apps.Installation) { a.ResourceEnabled = false }, reason: "disabled", code: ResourceDisabled},
		{name: "disconnected", change: func(a *apps.Installation) { a.Disconnected = true }, reason: "disconnected", code: ResourceUnavailable},
		{name: "unauthorized", change: func(a *apps.Installation) { a.Authorized = false }, reason: "authorization_required", code: ResourceUnavailable},
		{name: "applying", change: func(a *apps.Installation) { a.Status = "applying" }, reason: "applying", code: ResourceUnavailable},
		{name: "failed", change: func(a *apps.Installation) { a.Status = "error" }, reason: "connection_error", code: ResourceUnavailable},
		{name: "stale_revision", change: func(a *apps.Installation) { a.AppliedRevision-- }, reason: "connecting", code: ResourceUnavailable},
		{name: "unapplied", change: func(a *apps.Installation) { a.Revision, a.AppliedRevision = 0, 0 }, reason: "connecting", code: ResourceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := apps.Installation{
				InstallationID: "feishu-binding", AppID: "feishu", Name: "Feishu", Kind: apps.KindCLI,
				Enabled: true, ResourceEnabled: true, Status: "connected", Authorized: true, Revision: 2, AppliedRevision: 2,
				Skills:     []skillstate.ManagedSkill{{Name: "lark-doc"}, {Name: "disabled"}, {Name: "removed"}},
				AuthStatus: json.RawMessage(`{"private":"private-fixture"}`),
			}
			test.change(&app)
			appReader := &appFixture{items: []apps.Installation{app}}
			agentReader := &agentFixture{item: contract.Agent{ID: "agent", Spec: contract.AgentSpec{
				Runtime: contract.RuntimeSpec{Adapter: "codex"}, Skills: []string{"lark-doc", "disabled"},
				SkillStates: map[string]skillstate.State{"disabled": {Enabled: false}},
			}}}
			service := New(agentReader, appReader)
			items, _, err := service.List(t.Context(), "agent", apitypes.ResourceTypeConnector, resourcequery.Query{Mode: "cursor", Size: 20})
			if err != nil || len(items) != 1 || items[0].Available != (test.reason == "") || items[0].UnavailableReason != test.reason {
				t.Fatalf("CLI list result: items=%+v err=%v", items, err)
			}
			selection := &apitypes.ResourceSelection{Version: 1, AgentID: "agent", Refs: []apitypes.ResourceSnapshot{items[0].ResourceSnapshot}}
			_, prompt, err := service.PrepareInput(t.Context(), "agent", "Read the document", selection)
			if test.code != "" {
				var failure *Error
				if !errors.As(err, &failure) || failure.Code != test.code {
					t.Fatalf("error=%v, want %s", err, test.code)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var input ResolvedSelection
				if err := json.Unmarshal([]byte(prompt[strings.LastIndex(prompt, "\n")+1:]), &input); err != nil {
					t.Fatal(err)
				}
				if len(input.Resources) != 1 || input.Resources[0].AppKind != apps.KindCLI ||
					!slices.Equal(input.Resources[0].SkillNames, []string{"lark-doc"}) || len(input.Resources[0].ToolNames) != 0 {
					t.Fatalf("CLI invocation details missing: %+v", input)
				}
				if strings.Contains(prompt, "private-fixture") {
					t.Fatal("CLI authorization data included in prompt")
				}
				appReader.items[0].Authorized = false
				if _, _, err := service.PrepareInput(t.Context(), "agent", "Read again", selection); err == nil {
					t.Fatal("authorization loss was not revalidated")
				}
			}
			if len(appReader.toolCalls) != 0 {
				t.Fatalf("CLI selection queried MCP tools: %v", appReader.toolCalls)
			}
		})
	}
}

func TestConnectedCLIInstallationCanBeSelected(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	initial, err := apps.NewService(statePath, apps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := initial.EnsureFeishuCLI(t.Context())
	closeErr := initial.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("create CLI fixture: %v, close: %v", err, closeErr)
	}
	const bindingID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := localstore.UpdateObjectSection(statePath, "app_installations", func(values map[string]json.RawMessage) error {
		var global map[string]any
		if err := json.Unmarshal(values[resource.InstallationID], &global); err != nil {
			return err
		}
		global["cli_state"].(map[string]any)["ready"] = true
		data, err := json.Marshal(global)
		if err != nil {
			return err
		}
		values[resource.InstallationID] = data
		data, err = json.Marshal(apps.Installation{
			InstallationID: bindingID, ResourceID: resource.InstallationID, AgentID: "agent",
			Kind: apps.KindCLI, Enabled: true, AppliedRevision: resource.Revision,
			Skills: []skillstate.ManagedSkill{{Name: "lark-doc", Managed: true}},
		})
		if err != nil {
			return err
		}
		values[bindingID] = data
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	appService, err := apps.NewService(statePath, apps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer appService.Close()
	for _, runtime := range []string{"codex", "dsh"} {
		t.Run(runtime, func(t *testing.T) {
			reader := &agentFixture{item: contract.Agent{ID: "agent", Spec: contract.AgentSpec{
				Runtime: contract.RuntimeSpec{Adapter: runtime}, Skills: []string{"lark-doc"},
			}}}
			service := New(reader, appService)
			items, _, err := service.List(t.Context(), "agent", apitypes.ResourceTypeConnector, resourcequery.Query{Mode: "page", Page: 1, Size: 20})
			if err != nil || len(items) != 1 || !items[0].Available {
				t.Fatalf("connected CLI unavailable: items=%+v err=%v", items, err)
			}
			resolved, err := service.Resolve(t.Context(), "agent", []apitypes.ResourceReference{items[0].ResourceReference})
			if err != nil || len(resolved.Resources) != 1 || resolved.Resources[0].AppKind != apps.KindCLI ||
				!slices.Equal(resolved.Resources[0].SkillNames, []string{"lark-doc"}) {
				t.Fatalf("CLI selection failed: resources=%+v err=%v", resolved, err)
			}
		})
	}
}
