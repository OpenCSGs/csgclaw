package agentengine_test

import (
	"context"
	"errors"
	"testing"

	"csgclaw/internal/agentengine"
)

func TestRuntimeConfigurationChangeUsesEngineLifecycle(t *testing.T) {
	engine := newRealContractEngine(t, []agentengine.Agent{{ID: "agent-a", Spec: agentengine.AgentSpec{Name: "a"}, Status: agentengine.AgentStatus{RuntimeID: "runtime-a", State: "running"}}}, nil).(*agentengine.Engine)
	pending := true
	engine.SetRuntimeConfigurationGuard(func(string) error {
		if pending {
			return errors.New("configuration pending")
		}
		return nil
	})
	ctx := context.Background()
	err := engine.UpdateRuntimeConfiguration(ctx, "agent-a", true, func(ctx context.Context) error {
		current, err := engine.Agents().Get(ctx, "agent-a", agentengine.AgentGetOptions{})
		if err != nil {
			return err
		}
		if current.Status.State != "stopped" {
			t.Fatalf("configuration ran in state %s", current.Status.State)
		}
		_, err = engine.RuntimeExtensions("agent-a").Apply(ctx, agentengine.RuntimeExtensionApplyRequest{Spec: agentengine.RuntimeExtensionSpec{Name: "connector", Kind: "contract", Source: agentengine.RuntimeExtensionSourceRef{Provider: "contract", Ref: "resource"}, FailurePolicy: agentengine.RuntimeExtensionBlockRuntime}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := engine.RuntimeExtensions("agent-a").Get(ctx, "connector")
	if err != nil || !item.Status.RuntimeLoaded {
		t.Fatalf("loaded=%+v error=%v", item.Status, err)
	}
	pending = false
	current, err := engine.Agents().Get(ctx, "agent-a", agentengine.AgentGetOptions{})
	if err != nil || current.Status.State != "running" {
		t.Fatalf("resumed=%+v error=%v", current.Status, err)
	}
	failure := errors.New("configuration failed")
	if err := engine.UpdateRuntimeConfiguration(ctx, "agent-a", true, func(context.Context) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("failure=%v", err)
	}
	current, err = engine.Agents().Get(ctx, "agent-a", agentengine.AgentGetOptions{})
	if err != nil || current.Status.State != "stopped" {
		t.Fatalf("failure state=%+v error=%v", current.Status, err)
	}
}

func TestRuntimeConfigurationResumeHonorsLatestStop(t *testing.T) {
	engine := newRealContractEngine(t, []agentengine.Agent{{ID: "agent-a", Spec: agentengine.AgentSpec{Name: "a", DesiredState: agentengine.AgentDesiredStateRunning}, Status: agentengine.AgentStatus{RuntimeID: "runtime-a", State: "running"}}}, nil).(*agentengine.Engine)
	ctx := context.Background()
	if err := engine.UpdateRuntimeConfiguration(ctx, "agent-a", false, nil); err != nil {
		t.Fatal(err)
	}
	current, err := engine.Agents().Get(ctx, "agent-a", agentengine.AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current.Spec.DesiredState = agentengine.AgentDesiredStateStopped
	if _, err = engine.Agents().Update(ctx, "agent-a", agentengine.AgentUpdateRequest{Spec: current.Spec, FieldMask: []string{"desired_state"}, ResourceVersion: current.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	applied := false
	if err = engine.UpdateRuntimeConfiguration(ctx, "agent-a", true, func(context.Context) error { applied = true; return nil }); err != nil {
		t.Fatal(err)
	}
	current, err = engine.Agents().Get(ctx, "agent-a", agentengine.AgentGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !applied || current.Status.State != "stopped" || current.Spec.DesiredState != agentengine.AgentDesiredStateStopped {
		t.Fatalf("applied=%v desired=%s actual=%s", applied, current.Spec.DesiredState, current.Status.State)
	}
}
