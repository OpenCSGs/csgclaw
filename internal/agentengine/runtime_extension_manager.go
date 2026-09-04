package agentengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

type runtimeExtensionScopes interface {
	Scope(agentID string) RuntimeExtensionInterface
}

type runtimeExtensionBackend interface {
	WithAgentMutation(context.Context, string, func(context.Context) error) error
	LoadRuntimeExtension(agentID, name string) (json.RawMessage, bool, error)
	ListRuntimeExtensions(agentID string) (map[string]json.RawMessage, error)
	StoreRuntimeExtension(agentID, name string, raw json.RawMessage) error
	RemoveRuntimeExtension(agentID, name string) error
	ReconcileRuntimeExtension(context.Context, string, agentruntime.ExtensionDesired) (agentruntime.ExtensionResult, error)
	ObserveRuntimeExtension(context.Context, string, agentruntime.ExtensionDesired) (agentruntime.ExtensionResult, error)
	DeleteRuntimeExtensionProjection(context.Context, string, string, string) (agentruntime.ExtensionResult, error)
}

type runtimeExtensionManager struct {
	backend runtimeExtensionBackend

	mu      sync.RWMutex
	sources map[string]RuntimeExtensionSource
}

type runtimeExtensions struct {
	manager *runtimeExtensionManager
	agentID string
}

func newRuntimeExtensionManager(backend runtimeExtensionBackend) *runtimeExtensionManager {
	return &runtimeExtensionManager{backend: backend, sources: make(map[string]RuntimeExtensionSource)}
}

func (m *runtimeExtensionManager) Scope(agentID string) RuntimeExtensionInterface {
	return &runtimeExtensions{manager: m, agentID: strings.TrimSpace(agentID)}
}

func (m *runtimeExtensionManager) registerSource(provider string, source RuntimeExtensionSource) error {
	provider = strings.TrimSpace(provider)
	if provider == "" || source == nil {
		return fmt.Errorf("runtime extension source provider and implementation are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sources[provider]; exists {
		return fmt.Errorf("runtime extension source %q is already registered", provider)
	}
	m.sources[provider] = source
	return nil
}

func (m *runtimeExtensionManager) source(provider string) RuntimeExtensionSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sources[strings.TrimSpace(provider)]
}

func (m *runtimeExtensionManager) reconcileAll(ctx context.Context, agentID string) error {
	return m.backend.WithAgentMutation(ctx, agentID, func(ctx context.Context) error { return m.reconcileAllLocked(ctx, agentID) })
}

func (m *runtimeExtensionManager) reconcileAllLocked(ctx context.Context, agentID string) error {
	raw, err := m.backend.ListRuntimeExtensions(agentID)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		scoped := &runtimeExtensions{manager: m, agentID: agentID}
		item, found, loadErr := scoped.load(name)
		if loadErr != nil || !found {
			return loadErr
		}
		if extensionDeleting(item) {
			if err := scoped.deleteLocked(ctx, name); err != nil {
				return err
			}
			continue
		}
		resolved, resolveErr := scoped.resolve(ctx, item)
		if resolveErr != nil {
			item.Status.State = RuntimeExtensionError
			item.Status.Reason = "source_unavailable"
			item.Status.Message = "The Runtime extension source is unavailable"
			item.Status.RuntimeLoaded = false
			item.Status.CheckedAt = time.Now().UTC()
			if _, cleanupErr := m.backend.DeleteRuntimeExtensionProjection(ctx, agentID, name, item.Spec.Kind); cleanupErr != nil {
				item.Status.Reason = "cleanup_failed"
				item.Status.Message = "The previous extension configuration could not be disabled"
				if storeErr := scoped.store(item); storeErr != nil {
					return storeErr
				}
				return errors.New(item.Status.Message)
			}
			if err := scoped.store(item); err != nil {
				return err
			}
			if item.Spec.FailurePolicy == RuntimeExtensionBlockRuntime {
				return errors.New(item.Status.Message)
			}
			continue
		}
		result, reconcileErr := m.backend.ReconcileRuntimeExtension(ctx, agentID, agentruntime.ExtensionDesired{
			Name: item.Spec.Name, Kind: item.Spec.Kind, Generation: item.Status.Generation,
			SourceRevision: resolved.SourceRevision, Payload: append(json.RawMessage(nil), resolved.Payload...), DeferRuntimeReload: true,
		})
		applyRuntimeExtensionResult(&item, result, reconcileErr)
		item.Status.SourceRevision = resolved.SourceRevision
		if reconcileErr == nil && item.Status.State == RuntimeExtensionConfigured {
			item.Status.ObservedGeneration = item.Status.Generation
			item.Status.AppliedAt = item.Status.CheckedAt
		}
		if err := scoped.store(item); err != nil {
			return err
		}
		if item.Spec.FailurePolicy == RuntimeExtensionBlockRuntime && (reconcileErr != nil || item.Status.State != RuntimeExtensionConfigured) {
			return fmt.Errorf("required runtime extension %q is not configured: %s", name, item.Status.Reason)
		}
	}
	return nil
}

func (m *runtimeExtensionManager) deleteAll(ctx context.Context, agentID string) error {
	raw, err := m.backend.ListRuntimeExtensions(agentID)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := m.Scope(agentID).Delete(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// RegisterRuntimeExtensionSource connects an integration-owned fact source to
// the Engine without making the Engine depend on that integration package.
func (e *Engine) RegisterRuntimeExtensionSource(provider string, source RuntimeExtensionSource) error {
	if e == nil || e.extensions == nil {
		return fmt.Errorf("runtime extension service is unavailable")
	}
	manager, ok := e.extensions.(*runtimeExtensionManager)
	if !ok {
		return fmt.Errorf("runtime extension source registration is unsupported")
	}
	return manager.registerSource(provider, source)
}

func (r *runtimeExtensions) Apply(ctx context.Context, request RuntimeExtensionApplyRequest) (item RuntimeExtension, err error) {
	if r == nil || r.manager == nil || r.manager.backend == nil || r.agentID == "" {
		return item, &TurnError{Code: ErrorInvalidRequest, Message: "agent ID is required"}
	}
	err = r.manager.backend.WithAgentMutation(ctx, r.agentID, func(ctx context.Context) error {
		var applyErr error
		item, applyErr = r.applyLocked(ctx, request)
		return applyErr
	})
	return item, err
}

func (r *runtimeExtensions) applyLocked(ctx context.Context, request RuntimeExtensionApplyRequest) (RuntimeExtension, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || r.manager == nil || r.manager.backend == nil || r.agentID == "" {
		return RuntimeExtension{}, &TurnError{Code: ErrorInvalidRequest, Message: "agent ID is required"}
	}
	spec := normalizeRuntimeExtensionSpec(request.Spec)
	if err := validateRuntimeExtensionSpec(spec); err != nil {
		return RuntimeExtension{}, err
	}
	current, found, err := r.load(spec.Name)
	if err != nil {
		return RuntimeExtension{}, err
	}
	if found && request.ResourceVersion != "" && request.ResourceVersion != current.ResourceVersion {
		return RuntimeExtension{}, &TurnError{Code: ErrorRuntimeExtensionConflict, Message: "runtime extension resource version does not match"}
	}
	if !found && request.ResourceVersion != "" {
		return RuntimeExtension{}, &TurnError{Code: ErrorRuntimeExtensionConflict, Message: "runtime extension does not exist for the supplied resource version"}
	}

	now := time.Now().UTC()
	item := current
	if !found {
		item = RuntimeExtension{AgentID: r.agentID, CreatedAt: now}
	}
	item.Spec = spec
	item.UpdatedAt = now
	item.ResourceVersion = now.Format(time.RFC3339Nano)
	item.Status.Generation++
	item.Status.CheckedAt = now
	item.Status.State = RuntimeExtensionError
	item.Status.Reason = "reconciling"
	item.Status.Message = "Configuring the Runtime extension"
	item.Status.RuntimeLoaded = false
	if err := r.store(item); err != nil {
		return RuntimeExtension{}, err
	}

	resolved, resolveErr := r.resolve(ctx, item)
	if resolveErr != nil {
		item.Status.State = RuntimeExtensionError
		item.Status.Reason = "source_unavailable"
		item.Status.Message = "The Runtime extension source is unavailable"
		item.Status.RuntimeLoaded = false
		item.Status.CheckedAt = time.Now().UTC()
		if found {
			if _, cleanupErr := r.manager.backend.DeleteRuntimeExtensionProjection(ctx, r.agentID, spec.Name, spec.Kind); cleanupErr != nil {
				item.Status.Reason = "cleanup_failed"
				item.Status.Message = "The previous extension configuration could not be disabled"
			}
		}
		if err := r.store(item); err != nil {
			return item, err
		}
		return item, errors.New(item.Status.Message)
	}
	item.Status.SourceRevision = resolved.SourceRevision
	result, reconcileErr := r.manager.backend.ReconcileRuntimeExtension(ctx, r.agentID, agentruntime.ExtensionDesired{
		Name:           spec.Name,
		Kind:           spec.Kind,
		Generation:     item.Status.Generation,
		SourceRevision: resolved.SourceRevision,
		Payload:        append(json.RawMessage(nil), resolved.Payload...),
	})
	applyRuntimeExtensionResult(&item, result, reconcileErr)
	if reconcileErr == nil && item.Status.State == RuntimeExtensionConfigured {
		item.Status.ObservedGeneration = item.Status.Generation
		item.Status.AppliedAt = item.Status.CheckedAt
	}
	if err := r.store(item); err != nil {
		return RuntimeExtension{}, err
	}
	return item, reconcileErr
}

// Get is a side-effect-free resource read. Probing is explicit reconciliation.
func (r *runtimeExtensions) Get(ctx context.Context, name string) (RuntimeExtension, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return RuntimeExtension{}, err
		}
	}
	item, found, err := r.load(strings.TrimSpace(name))
	if err != nil {
		return RuntimeExtension{}, err
	}
	if !found {
		return RuntimeExtension{}, &TurnError{Code: ErrorRuntimeExtensionNotFound, Message: fmt.Sprintf("runtime extension %q not found", name)}
	}
	return item, nil
}

func (r *runtimeExtensions) List(ctx context.Context) ([]RuntimeExtension, error) {
	raw, err := r.manager.backend.ListRuntimeExtensions(r.agentID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]RuntimeExtension, 0, len(names))
	for _, name := range names {
		item, getErr := r.Get(ctx, name)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *runtimeExtensions) Delete(ctx context.Context, name string) error {
	return r.manager.backend.WithAgentMutation(ctx, r.agentID, func(ctx context.Context) error { return r.deleteLocked(ctx, name) })
}

func (r *runtimeExtensions) deleteLocked(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	item, found, err := r.load(name)
	if err != nil {
		return err
	}
	if !found {
		return &TurnError{Code: ErrorRuntimeExtensionNotFound, Message: fmt.Sprintf("runtime extension %q not found", name)}
	}
	item.Status.State = RuntimeExtensionError
	item.Status.Reason = "deleting"
	item.Status.Message = "Removing the Runtime extension"
	item.Status.RuntimeLoaded = false
	item.Status.CheckedAt = time.Now().UTC()
	item.ResourceVersion = item.Status.CheckedAt.Format(time.RFC3339Nano)
	item.UpdatedAt = item.Status.CheckedAt
	if err := r.store(item); err != nil {
		return err
	}
	result, deleteErr := r.manager.backend.DeleteRuntimeExtensionProjection(ctx, r.agentID, item.Spec.Name, item.Spec.Kind)
	if deleteErr == nil {
		deleteErr = r.manager.backend.RemoveRuntimeExtension(r.agentID, item.Spec.Name)
	}
	if deleteErr != nil {
		applyRuntimeExtensionResult(&item, result, deleteErr)
		item.Status.State = RuntimeExtensionError
		item.Status.Reason = "delete_failed"
		item.Status.Message = "Runtime extension cleanup is incomplete; retry cleanup"
		item.Status.RuntimeLoaded = false
		if err := r.store(item); err != nil {
			return err
		}
		return errors.New(item.Status.Message)
	}
	return nil
}

// A failed deletion remains desired deletion across process restart and Recreate.
// Apply explicitly cancels that intent by writing a new desired generation.
func extensionDeleting(item RuntimeExtension) bool {
	return item.Status.Reason == "deleting" || item.Status.Reason == "delete_failed"
}

func (r *runtimeExtensions) resolve(ctx context.Context, item RuntimeExtension) (ResolvedRuntimeExtension, error) {
	source := r.manager.source(item.Spec.Source.Provider)
	if source == nil {
		return ResolvedRuntimeExtension{}, fmt.Errorf("runtime extension source %q is unavailable", item.Spec.Source.Provider)
	}
	return source.Resolve(ctx, r.agentID, item.Spec.Source.Ref)
}

func (r *runtimeExtensions) load(name string) (RuntimeExtension, bool, error) {
	raw, found, err := r.manager.backend.LoadRuntimeExtension(r.agentID, name)
	if err != nil || !found {
		return RuntimeExtension{}, found, err
	}
	var item RuntimeExtension
	if err := json.Unmarshal(raw, &item); err != nil {
		return RuntimeExtension{}, false, fmt.Errorf("decode runtime extension %q: %w", name, err)
	}
	return item, true, nil
}

func (r *runtimeExtensions) store(item RuntimeExtension) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return r.manager.backend.StoreRuntimeExtension(r.agentID, item.Spec.Name, raw)
}

func applyRuntimeExtensionResult(item *RuntimeExtension, result agentruntime.ExtensionResult, err error) {
	if item == nil {
		return
	}
	checkedAt := result.CheckedAt.UTC()
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	item.Status.CheckedAt = checkedAt
	item.Status.Reason = strings.TrimSpace(result.Reason)
	item.Status.Message = strings.TrimSpace(result.Message)
	item.Status.RuntimeLoaded = result.RuntimeLoaded
	switch result.State {
	case agentruntime.ExtensionStateConfigured:
		item.Status.State = RuntimeExtensionConfigured
	case agentruntime.ExtensionStateUnavailable:
		item.Status.State = RuntimeExtensionUnavailable
	default:
		item.Status.State = RuntimeExtensionError
	}
	if err != nil {
		item.Status.State = RuntimeExtensionError
		if errors.Is(err, agentruntime.ErrExtensionUnsupported) {
			item.Status.Reason = "extension_unsupported"
		} else if item.Status.Reason == "" {
			item.Status.Reason = "reconcile_failed"
		}
		if item.Status.Message == "" {
			item.Status.Message = "The Runtime extension could not be reconciled"
		}
	}
}

func (m *runtimeExtensionManager) PrepareRuntime(ctx context.Context, agentID string) error {
	return m.reconcileAll(ctx, agentID)
}
func (m *runtimeExtensionManager) DeleteExtensions(ctx context.Context, agentID string) error {
	return m.deleteAll(ctx, agentID)
}
func (m *runtimeExtensionManager) RuntimeReady(agentID string) error {
	raw, err := m.backend.ListRuntimeExtensions(agentID)
	if err != nil {
		return err
	}
	for name, data := range raw {
		var item RuntimeExtension
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		if extensionDeleting(item) {
			continue
		}
		if item.Spec.FailurePolicy == RuntimeExtensionBlockRuntime && (item.Status.State != RuntimeExtensionConfigured || item.Status.ObservedGeneration != item.Status.Generation || !item.Status.RuntimeLoaded) {
			return fmt.Errorf("required runtime extension %q is not loaded", name)
		}
	}
	return nil
}
func (m *runtimeExtensionManager) RuntimeStopped(ctx context.Context, agentID string) error {
	raw, err := m.backend.ListRuntimeExtensions(agentID)
	if err != nil {
		return err
	}
	for name, data := range raw {
		var item RuntimeExtension
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		item.Status.RuntimeLoaded = false
		if err := m.Scope(agentID).(*runtimeExtensions).store(item); err != nil {
			return fmt.Errorf("save extension %q stopped state: %w", name, err)
		}
	}
	return nil
}
func (m *runtimeExtensionManager) RuntimeStarted(ctx context.Context, agentID string) error {
	raw, err := m.backend.ListRuntimeExtensions(agentID)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		scoped := m.Scope(agentID).(*runtimeExtensions)
		item, found, err := scoped.load(name)
		if err != nil {
			return err
		}
		if !found || extensionDeleting(item) {
			continue
		}
		resolved, resolveErr := scoped.resolve(ctx, item)
		if resolveErr != nil {
			item.Status.State = RuntimeExtensionError
			item.Status.Reason = "source_unavailable"
			item.Status.Message = "Runtime extension source is unavailable"
			item.Status.RuntimeLoaded = false
		} else {
			result, observeErr := m.backend.ObserveRuntimeExtension(ctx, agentID, agentruntime.ExtensionDesired{Name: name, Kind: item.Spec.Kind, Generation: item.Status.Generation, SourceRevision: resolved.SourceRevision, Payload: resolved.Payload})
			applyRuntimeExtensionResult(&item, result, observeErr)
			item.Status.SourceRevision = resolved.SourceRevision
			if observeErr == nil && item.Status.State == RuntimeExtensionConfigured && item.Status.RuntimeLoaded {
				if item.Status.ObservedGeneration != item.Status.Generation {
					item.Status.AppliedAt = item.Status.CheckedAt
				}
				item.Status.ObservedGeneration = item.Status.Generation
			}
		}
		if err := scoped.store(item); err != nil {
			return err
		}
	}
	// Observing a successfully started process is independent of admission.
	// Required extensions gate RuntimeReady, not cleanup-triggered reloads.
	return nil
}
