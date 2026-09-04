package agentengine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
)

// This fixture exercises the real Engine, repository, lifecycle coordinator and
// generation store. Only the tool executable and process restart are simulated.
type projectionRuntime struct {
	*fakeConversationRuntime
	root           string
	unavailable    bool
	forceStage     bool
	renderFailures int
	starts, stops  atomic.Int32
	loaded         map[string]string
}

func (r *projectionRuntime) store(id string) *extensionstate.Store {
	s, err := extensionstate.New(filepath.Join(r.root, id))
	if err != nil {
		panic(err)
	}
	return s
}
func (r *projectionRuntime) RuntimeExtensionDriver(kind string) (runtime.ExtensionDriver, bool) {
	return r, kind == "fixture"
}
func (r *projectionRuntime) ExtensionProjections(id string) ([]runtime.ExtensionProjection, error) {
	return r.store(id).List()
}
func (r *projectionRuntime) PrepareExtensionDelete(_ context.Context, id, name string) (runtime.PreparedExtension, error) {
	return r.store(id).Delete(name)
}
func (r *projectionRuntime) RenderExtensions(_ context.Context, id string, items []runtime.ExtensionProjection) error {
	if r.renderFailures > 0 {
		r.renderFailures--
		return errors.New("injected render failure")
	}
	var fragments []string
	for _, item := range items {
		fragments = append(fragments, item.Instructions)
	}
	return os.WriteFile(filepath.Join(r.root, id+".instructions"), []byte(strings.Join(fragments, "\n")), 0600)
}
func (r *projectionRuntime) PrepareExtension(_ context.Context, id string, desired runtime.ExtensionDesired) (runtime.PreparedExtension, runtime.ExtensionResult, error) {
	result := runtime.ExtensionResult{State: runtime.ExtensionStateConfigured, Reason: "configured"}
	if r.unavailable {
		return nil, runtime.ExtensionResult{State: runtime.ExtensionStateUnavailable, Reason: "executable_unavailable", Message: "Install the fixture executable"}, nil
	}
	store := r.store(id)
	if previous, found, err := store.Load(desired.Name); err != nil {
		return nil, result, err
	} else if found && previous.SourceRevision == desired.SourceRevision && !r.forceStage {
		previous.Generation = desired.Generation
		change, err := store.Revise(previous)
		return change, result, err
	}
	change, err := store.Stage(desired.Name)
	if err != nil {
		return nil, result, err
	}
	change.SetProjection(runtime.ExtensionProjection{Name: desired.Name, Kind: desired.Kind, Generation: desired.Generation, SourceRevision: desired.SourceRevision, Environment: map[string]string{"FIXTURE_TOOL": desired.SourceRevision}, Instructions: desired.Name})
	if err := os.WriteFile(filepath.Join(change.Directory(), "config"), desired.Payload, 0600); err != nil {
		return nil, result, err
	}
	return change, result, nil
}
func (r *projectionRuntime) ObserveExtension(_ context.Context, id string, desired runtime.ExtensionDesired) (runtime.ExtensionResult, error) {
	item, found, err := r.store(id).Load(desired.Name)
	if err != nil {
		return runtime.ExtensionResult{}, err
	}
	if !found {
		return runtime.ExtensionResult{State: runtime.ExtensionStateError, Reason: "binding_missing"}, nil
	}
	return runtime.ExtensionResult{State: runtime.ExtensionStateConfigured, Reason: "configured", RuntimeLoaded: r.loaded[desired.Name] == item.Digest}, nil
}
func (r *projectionRuntime) Start(_ context.Context, h runtime.Handle) (runtime.State, error) {
	r.starts.Add(1)
	r.state = runtime.StateRunning
	r.loaded = map[string]string{}
	id := strings.TrimPrefix(h.RuntimeID, "rt-")
	items, err := r.store(id).List()
	if err != nil {
		return "", err
	}
	for _, item := range items {
		r.loaded[item.Name] = item.Digest
	}
	return r.state, nil
}
func (r *projectionRuntime) Stop(context.Context, runtime.Handle) (runtime.State, error) {
	r.stops.Add(1)
	r.state = runtime.StateStopped
	r.loaded = nil
	return r.state, nil
}

type projectionSource struct{}

func (projectionSource) Resolve(_ context.Context, _ string, ref string) (ResolvedRuntimeExtension, error) {
	return ResolvedRuntimeExtension{SourceRevision: ref, Payload: json.RawMessage(`{"value":"transient-source"}`)}, nil
}
func newProjectionEngine(t *testing.T, running bool) (*Engine, *projectionRuntime) {
	t.Helper()
	state := runtime.StateStopped
	if running {
		state = runtime.StateRunning
	}
	r := &projectionRuntime{fakeConversationRuntime: &fakeConversationRuntime{state: state}, root: t.TempDir()}
	controller := newTestAgentService(t, []agent.Agent{{ID: "agent-a", Name: "A", RuntimeID: "rt-agent-a", RuntimeKind: runtime.KindCodex, Role: agent.RoleWorker, Status: string(state), ProfileComplete: true}}, r)
	engine := New(controller)
	if err := engine.RegisterRuntimeExtensionSource("fixture", projectionSource{}); err != nil {
		t.Fatal(err)
	}
	return engine, r
}
func projectionRequest(name, revision string) RuntimeExtensionApplyRequest {
	return RuntimeExtensionApplyRequest{Spec: RuntimeExtensionSpec{Name: name, Kind: "fixture", Source: RuntimeExtensionSourceRef{Provider: "fixture", Ref: revision}, FailurePolicy: RuntimeExtensionOptional}}
}

func TestRuntimeExtensionIsolationConflictAndStoppedApply(t *testing.T) {
	engine, rt := newProjectionEngine(t, false)
	ctx := context.Background()
	extensions := engine.RuntimeExtensions("agent-a")
	for _, name := range []string{"z-tool", "a-tool"} {
		if item, err := extensions.Apply(ctx, projectionRequest(name, "one")); err != nil || item.Status.State != RuntimeExtensionConfigured {
			t.Fatalf("Apply = %+v %v", item, err)
		}
	}
	instructions, err := os.ReadFile(filepath.Join(rt.root, "agent-a.instructions"))
	if err != nil || string(instructions) != "a-tool\nz-tool" {
		t.Fatalf("instructions = %q %v", instructions, err)
	}
	items, _ := rt.ExtensionProjections("agent-a")
	if len(items) != 2 || items[0].Root == items[1].Root {
		t.Fatalf("isolated projections = %+v", items)
	}
	item, err := extensions.Apply(ctx, projectionRequest("conflicting", "two"))
	if err == nil || item.Status.Reason != "environment_conflict" {
		t.Fatalf("conflict = %+v %v", item, err)
	}
	items, _ = rt.ExtensionProjections("agent-a")
	if len(items) != 2 {
		t.Fatalf("conflict damaged another extension: %+v", items)
	}
	if err := extensions.Delete(ctx, "a-tool"); err != nil {
		t.Fatal(err)
	}
	items, _ = rt.ExtensionProjections("agent-a")
	if len(items) != 1 || items[0].Name != "z-tool" {
		t.Fatalf("Delete damaged another extension: %+v", items)
	}
	if rt.starts.Load() != 0 || rt.stops.Load() != 0 {
		t.Fatal("stopped Agent was started")
	}
}
func TestRuntimeExtensionRetryRollbackAndSourceInvalidation(t *testing.T) {
	engine, rt := newProjectionEngine(t, true)
	ctx := context.Background()
	extensions := engine.RuntimeExtensions("agent-a")
	first, err := extensions.Apply(ctx, projectionRequest("tool", "one"))
	if err != nil || !first.Status.RuntimeLoaded {
		t.Fatalf("initial = %+v %v", first, err)
	}
	if rt.starts.Load() != 1 || rt.stops.Load() != 1 {
		t.Fatal("Apply did not restart exactly once")
	}
	second, err := extensions.Apply(ctx, projectionRequest("tool", "one"))
	if err != nil || second.Status.Generation != 2 || !second.Status.RuntimeLoaded || rt.starts.Load() != 1 {
		t.Fatalf("idempotent retry = %+v %v", second, err)
	}
	previous, _, _ := rt.store("agent-a").Load("tool")
	rt.forceStage = true
	rt.renderFailures = 1
	failed, err := extensions.Apply(ctx, projectionRequest("tool", "one"))
	if err == nil || failed.Status.Reason != "instructions_refresh_failed" {
		t.Fatalf("injected render failure = %+v %v", failed, err)
	}
	active, found, _ := rt.store("agent-a").Load("tool")
	if !found || active.Digest != previous.Digest {
		t.Fatal("same-source rollback lost previous generation")
	}
	rt.unavailable = true
	unavailable, err := extensions.Apply(ctx, projectionRequest("tool", "one"))
	if err != nil || unavailable.Status.State != RuntimeExtensionUnavailable {
		t.Fatalf("dependency failure = %+v %v", unavailable, err)
	}
	active, found, _ = rt.store("agent-a").Load("tool")
	if !found || active.Digest != previous.Digest {
		t.Fatal("same-source retry removed active generation")
	}
	unavailable, err = extensions.Apply(ctx, projectionRequest("tool", "new-bot"))
	if err != nil || unavailable.Status.State != RuntimeExtensionUnavailable {
		t.Fatalf("changed-source failure = %+v %v", unavailable, err)
	}
	if _, found, _ = rt.store("agent-a").Load("tool"); found {
		t.Fatal("changed-source failure retained stale credentials")
	}
	if len(rt.loaded) != 0 || rt.starts.Load() != 2 {
		t.Fatal("running Runtime retained old environment")
	}
}
func TestRuntimeExtensionDeleteFailureIsRetryableAndDoesNotReapply(t *testing.T) {
	engine, rt := newProjectionEngine(t, true)
	ctx := context.Background()
	extensions := engine.RuntimeExtensions("agent-a")
	if _, err := extensions.Apply(ctx, projectionRequest("tool", "one")); err != nil {
		t.Fatal(err)
	}
	rt.renderFailures = 1
	if err := extensions.Delete(ctx, "tool"); err == nil {
		t.Fatal("failed cleanup was acknowledged")
	}
	item, err := extensions.Get(ctx, "tool")
	if err != nil || item.Status.Reason != "delete_failed" {
		t.Fatalf("retry resource = %+v %v", item, err)
	}
	if rt.state != runtime.StateStopped {
		t.Fatal("unsafe Runtime was left running")
	}
	rt.unavailable = true
	if err := engine.extensions.(*runtimeExtensionManager).PrepareRuntime(ctx, "agent-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := extensions.Get(ctx, "tool"); ErrorCodeOf(err) != ErrorRuntimeExtensionNotFound {
		t.Fatalf("deletion was reapplied: %v", err)
	}
	if rt.starts.Load() != 1 {
		t.Fatal("cleanup retry started the stopped Runtime")
	}
}
