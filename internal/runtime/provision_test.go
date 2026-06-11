package runtime

import (
	"context"
	"reflect"
	"testing"
)

type runtimeOnlyStub struct{}

func (runtimeOnlyStub) Kind() string { return "stub" }
func (runtimeOnlyStub) Layout(agentHome string) Layout {
	return Layout{
		WorkspaceRoot: agentHome,
		SkillsRoot:    agentHome + "/skills",
	}
}
func (runtimeOnlyStub) New(context.Context, Spec) (Handle, error) { return Handle{}, nil }
func (runtimeOnlyStub) Start(context.Context, Handle) (State, error) {
	return StateRunning, nil
}
func (runtimeOnlyStub) Stop(context.Context, Handle) (State, error)  { return StateStopped, nil }
func (runtimeOnlyStub) Delete(context.Context, Handle) error         { return nil }
func (runtimeOnlyStub) State(context.Context, Handle) (State, error) { return StateRunning, nil }
func (runtimeOnlyStub) Info(context.Context, Handle) (Info, error)   { return Info{}, nil }

type provisioningStub struct {
	runtimeOnlyStub
	called bool
	got    ProvisionRequest
}

type workspaceRootStub struct {
	runtimeOnlyStub
	root string
}

func (p *provisioningStub) Provision(_ context.Context, req ProvisionRequest) error {
	p.called = true
	p.got = req
	return nil
}

func (w workspaceRootStub) WorkspaceRoot(string) string {
	return w.root
}

func TestProvisionerCapabilityIsOptional(t *testing.T) {
	t.Parallel()

	var rt Runtime = runtimeOnlyStub{}
	if _, ok := any(rt).(Provisioner); ok {
		t.Fatal("runtime without provisioning should not expose Provisioner")
	}
}

func TestProvisionerCanBeDiscoveredSeparatelyFromRuntime(t *testing.T) {
	t.Parallel()

	rt := &provisioningStub{}
	asRuntime := Runtime(rt)
	provisioner, ok := any(asRuntime).(Provisioner)
	if !ok {
		t.Fatal("runtime implementing Provisioner should expose the capability")
	}

	req := ProvisionRequest{
		RuntimeID: "rt-1",
		AgentID:   "agent-1",
		AgentName: "alice",
		Profile: Profile{
			Provider: "openai",
			ModelID:  "gpt-5",
		},
	}
	if err := provisioner.Provision(context.Background(), req); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if !rt.called {
		t.Fatal("Provision() was not called")
	}
	if !reflect.DeepEqual(rt.got, req) {
		t.Fatalf("Provision() request = %#v, want %#v", rt.got, req)
	}
}

func (w workspaceRootStub) Layout(string) Layout {
	return Layout{
		WorkspaceRoot: w.root,
		SkillsRoot:    w.root + "/skills",
	}
}

func TestRuntimeExposesLayout(t *testing.T) {
	t.Parallel()

	var rt Runtime = workspaceRootStub{root: "/tmp/custom-workspace"}
	got := rt.Layout("/tmp/agent")
	want := Layout{
		WorkspaceRoot: "/tmp/custom-workspace",
		SkillsRoot:    "/tmp/custom-workspace/skills",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Layout() = %#v, want %#v", got, want)
	}
}
