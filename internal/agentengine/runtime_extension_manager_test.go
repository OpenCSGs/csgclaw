package agentengine

import (
	"context"
	"csgclaw/internal/agentengine/lifecycle"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

type extensionBackendStub struct {
	lifecycle          lifecycle.Coordinator
	items              map[string]json.RawMessage
	desired            []agentruntime.ExtensionDesired
	reconciled         agentruntime.ExtensionResult
	observed           agentruntime.ExtensionResult
	deletedProjections int
}

func (b *extensionBackendStub) LoadRuntimeExtension(_, name string) (json.RawMessage, bool, error) {
	raw, ok := b.items[name]
	return append(json.RawMessage(nil), raw...), ok, nil
}
func (b *extensionBackendStub) ListRuntimeExtensions(string) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(b.items))
	for name, raw := range b.items {
		out[name] = append(json.RawMessage(nil), raw...)
	}
	return out, nil
}
func (b *extensionBackendStub) StoreRuntimeExtension(_, name string, raw json.RawMessage) error {
	if b.items == nil {
		b.items = make(map[string]json.RawMessage)
	}
	b.items[name] = append(json.RawMessage(nil), raw...)
	return nil
}
func (b *extensionBackendStub) RemoveRuntimeExtension(_, name string) error {
	delete(b.items, name)
	return nil
}
func (b *extensionBackendStub) ReconcileRuntimeExtension(_ context.Context, _ string, desired agentruntime.ExtensionDesired) (agentruntime.ExtensionResult, error) {
	b.desired = append(b.desired, desired)
	return b.reconciled, nil
}
func (b *extensionBackendStub) ObserveRuntimeExtension(_ context.Context, _ string, desired agentruntime.ExtensionDesired) (agentruntime.ExtensionResult, error) {
	b.desired = append(b.desired, desired)
	return b.observed, nil
}
func (b *extensionBackendStub) DeleteRuntimeExtensionProjection(context.Context, string, string, string) (agentruntime.ExtensionResult, error) {
	b.deletedProjections++
	return agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, CheckedAt: time.Now().UTC()}, nil
}

type extensionSourceStub struct {
	revision string
	payload  json.RawMessage
	err      error
}

func (b *extensionBackendStub) WithAgentMutation(ctx context.Context, id string, fn func(context.Context) error) error {
	return b.lifecycle.Mutate(ctx, id, fn)
}

func (s extensionSourceStub) Resolve(context.Context, string, string) (ResolvedRuntimeExtension, error) {
	return ResolvedRuntimeExtension{SourceRevision: s.revision, Payload: append(json.RawMessage(nil), s.payload...)}, s.err
}

func TestRuntimeExtensionResourcePersistsOnlySourceReference(t *testing.T) {
	backend := &extensionBackendStub{
		items:      make(map[string]json.RawMessage),
		reconciled: agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, RuntimeLoaded: true, CheckedAt: time.Now().UTC()},
		observed:   agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, RuntimeLoaded: true, CheckedAt: time.Now().UTC()},
	}
	manager := newRuntimeExtensionManager(backend)
	if err := manager.registerSource("participant", extensionSourceStub{revision: "rev-1", payload: json.RawMessage(`{"secret":"must-not-persist"}`)}); err != nil {
		t.Fatal(err)
	}
	extensions := manager.Scope("agent-a")
	created, err := extensions.Apply(context.Background(), RuntimeExtensionApplyRequest{Spec: RuntimeExtensionSpec{
		Name: "docs", Kind: "lark-cli", Source: RuntimeExtensionSourceRef{Provider: "participant", Ref: "pt-a"},
	}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if created.Status.State != RuntimeExtensionConfigured || created.Status.Generation != 1 || created.Status.ObservedGeneration != 1 || !created.Status.RuntimeLoaded {
		t.Fatalf("Apply() = %+v", created)
	}
	if len(backend.desired) != 1 || string(backend.desired[0].Payload) != `{"secret":"must-not-persist"}` {
		t.Fatalf("resolved desired = %#v", backend.desired)
	}
	if raw := string(backend.items["docs"]); containsSecret(raw) {
		t.Fatalf("persisted resource leaked resolved payload: %s", raw)
	}
	got, err := extensions.Get(context.Background(), "docs")
	if err != nil || got.Status.State != RuntimeExtensionConfigured {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	listed, err := extensions.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].Spec.Name != "docs" {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
	if _, err := extensions.Apply(context.Background(), RuntimeExtensionApplyRequest{Spec: created.Spec, ResourceVersion: "stale"}); ErrorCodeOf(err) != ErrorRuntimeExtensionConflict {
		t.Fatalf("stale Apply() error = %v", err)
	}
	backend.desired = nil
	if err := manager.reconcileAll(context.Background(), "agent-a"); err != nil {
		t.Fatalf("reconcileAll() error = %v", err)
	}
	if len(backend.desired) != 1 || !backend.desired[0].DeferRuntimeReload {
		t.Fatalf("recreate desired = %#v, want deferred Runtime reload", backend.desired)
	}
	if err := extensions.Delete(context.Background(), "docs"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := extensions.Get(context.Background(), "docs"); ErrorCodeOf(err) != ErrorRuntimeExtensionNotFound {
		t.Fatalf("Get() after delete error = %v", err)
	}
}

func TestRuntimeExtensionSourceFailurePersistsErrorWithoutPayload(t *testing.T) {
	backend := &extensionBackendStub{items: make(map[string]json.RawMessage)}
	manager := newRuntimeExtensionManager(backend)
	if err := manager.registerSource("missing", extensionSourceStub{payload: json.RawMessage(`{"secret":"must-not-persist"}`), err: errors.New("source unavailable")}); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Scope("agent-a").Apply(context.Background(), RuntimeExtensionApplyRequest{Spec: RuntimeExtensionSpec{
		Name: "docs", Kind: "lark-cli", Source: RuntimeExtensionSourceRef{Provider: "missing", Ref: "pt-a"},
	}})
	if err == nil || item.Status.State != RuntimeExtensionError || item.Status.Reason != "source_unavailable" {
		t.Fatalf("Apply() = %+v, %v", item, err)
	}
	if raw := string(backend.items["docs"]); containsSecret(raw) {
		t.Fatalf("persisted error resource leaked resolved payload: %s", raw)
	}
}

func TestDeleteRuntimeExtensionRequiresAResource(t *testing.T) {
	backend := &extensionBackendStub{items: make(map[string]json.RawMessage)}
	manager := newRuntimeExtensionManager(backend)
	engine := &Engine{extensions: manager}
	if err := engine.RuntimeExtensions("agent-a").Delete(context.Background(), "feishu-lark-cli"); ErrorCodeOf(err) != ErrorRuntimeExtensionNotFound {
		t.Fatalf("delete without resource = %v", err)
	}
	if backend.deletedProjections != 0 {
		t.Fatalf("projection deletes = %d, want no implicit legacy cleanup", backend.deletedProjections)
	}
}

func containsSecret(value string) bool {
	return json.Valid([]byte(value)) && strings.Contains(value, "must-not-persist")
}
