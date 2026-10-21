package api

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apps"
	"csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	skill "csgclaw/internal/skill/state"
)

type connectorConfigurationBridge struct{ engine *agentengine.Engine }

func (b connectorConfigurationBridge) State(context.Context, string, string) (bool, error) {
	return false, nil
}
func (b connectorConfigurationBridge) Update(ctx context.Context, id string, resume bool, apply func(context.Context) error) error {
	return b.engine.UpdateRuntimeConfiguration(ctx, id, resume, apply)
}
func (connectorConfigurationBridge) Configure(context.Context, string, string, *apps.CLIProjection) error {
	return nil
}
func (connectorConfigurationBridge) PlanSkills(string, string, string, []skill.ManagedSkill, bool) (skill.SkillPlan, error) {
	return skill.SkillPlan{}, nil
}
func (connectorConfigurationBridge) ApplySkills(context.Context, string, skill.SkillPlan) error {
	return nil
}

func TestUnconfiguredConnectorDoesNotStrandManagerWithoutRuntime(t *testing.T) {
	rt := fakeCompatRuntime{kind: runtime.KindCodex, stop: func(context.Context, runtime.Handle) (runtime.State, error) {
		return runtime.StateUnknown, sandbox.ErrNotFound
	}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: agent.ManagerUserID, Name: "manager", Role: agent.RoleManager, RuntimeKind: runtime.KindCodex, RuntimeID: "rt-agent-manager", Status: "profile_incomplete"}}, agent.WithRuntime(rt))
	engine := agentengine.New(controller)
	statePath := filepath.Join(t.TempDir(), "state.json")
	options := apps.Options{Runtime: connectorConfigurationBridge{engine: engine}}
	service, err := apps.NewService(statePath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	engine.SetRuntimeConfigurationGuard(service.CheckCLIReady)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resource, err := service.EnsureFeishuCLI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.Bind(ctx, agent.ManagerUserID, apps.BindRequest{ResourceID: resource.InstallationID})
	if err != nil {
		t.Fatal(err)
	}
	wait := func(status string) {
		t.Helper()
		for ctx.Err() == nil {
			items, e := service.List(ctx, agent.ManagerUserID)
			if e != nil {
				t.Fatal(e)
			}
			for _, item := range items {
				if item.InstallationID == binding.InstallationID {
					if item.Status == "error" {
						t.Fatalf("connector cleanup failed: %s; admission=%v", item.LastErrorCode, service.CheckCLIReady(agent.ManagerUserID))
					}
					if item.Status == status {
						return
					}
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("binding did not settle as %s: %v", status, ctx.Err())
	}
	wait("needs_configuration")
	if _, err = service.SetCLIEnabled(ctx, agent.ManagerUserID, binding.InstallationID, false); err != nil {
		t.Fatal(err)
	}
	wait("disabled")
	if err = service.CheckCLIReady(agent.ManagerUserID); err != nil {
		t.Fatalf("disabled unconfigured connector blocked manager: %v", err)
	}
	globals, err := service.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(globals) != 1 || globals[0].Status != "needs_configuration" {
		t.Fatalf("global connector state changed: %+v", globals)
	}
	// Restore the disabled, unfinished binding left by the previous release.
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	var installations map[string]json.RawMessage
	if err = json.Unmarshal(stored["app_installations"], &installations); err != nil {
		t.Fatal(err)
	}
	var previous map[string]any
	if err = json.Unmarshal(installations[binding.InstallationID], &previous); err != nil {
		t.Fatal(err)
	}
	previous["cli_apply"] = map[string]any{"pending": true, "resume": false, "resume_recorded": true}
	installations[binding.InstallationID], err = json.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	stored["app_installations"], err = json.Marshal(installations)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(statePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := apps.NewService(statePath, options)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	engine.SetRuntimeConfigurationGuard(restored.CheckCLIReady)
	if err = restored.RestoreAll(ctx); err != nil {
		t.Fatalf("old pending state did not recover on restart: %v", err)
	}
	if err = restored.CheckCLIReady(agent.ManagerUserID); err != nil {
		t.Fatal(err)
	}
	items, err := restored.List(ctx, agent.ManagerUserID)
	if err != nil || len(items) != 1 || items[0].Enabled || items[0].Status != "disabled" {
		t.Fatalf("restored binding=%+v err=%v", items, err)
	}
	// This only settles connector state; missing model configuration stays missing.
	selected, err := engine.Agents().Get(ctx, agent.ManagerUserID, agentengine.AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Status.Model.ProfileComplete {
		t.Fatal(errors.New("connector cleanup invented a model profile"))
	}
}

func TestConnectorConfigurationPreservesStopFailure(t *testing.T) {
	failure := errors.New("process stop failed")
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: agent.ManagerUserID, Name: "manager", Role: agent.RoleManager, RuntimeKind: runtime.KindCodex, RuntimeID: "rt-agent-manager", Status: "unknown"}}, agent.WithRuntime(fakeCompatRuntime{kind: runtime.KindCodex, stop: func(context.Context, runtime.Handle) (runtime.State, error) { return runtime.StateUnknown, failure }}))
	engine := agentengine.New(controller)
	applied := false
	err := engine.UpdateRuntimeConfiguration(t.Context(), agent.ManagerUserID, false, func(context.Context) error { applied = true; return nil })
	if !errors.Is(err, failure) || applied {
		t.Fatalf("err=%v applied=%t", err, applied)
	}
}
