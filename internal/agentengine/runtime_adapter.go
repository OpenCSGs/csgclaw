package agentengine

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/agentengine/lifecycle"
	"csgclaw/internal/agentengine/registry"
	"csgclaw/internal/runtime"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type runtimeBackend struct {
	agents     AgentInterface
	repository *agent.Repository
	lifecycle  *lifecycle.Coordinator
	registry   *registry.Registry
	restart    func(context.Context, string) (agent.Agent, bool, error)
	stop       func(context.Context, string) (agent.Agent, error)
	ready      func(string) error
}

func (a runtimeBackend) WithAgentMutation(ctx context.Context, id string, fn func(context.Context) error) error {
	return a.lifecycle.Mutate(ctx, agent.CanonicalID(id), fn)
}

// New composes independent Agent, Repository, lifecycle and Runtime owners.
func New(controller *agent.Controller) *Engine {
	files := NewFileStore()
	engine := &Engine{files: files}
	if controller == nil {
		return engine
	}
	backend := runtimeBackend{agents: controller, repository: &controller.Repository, lifecycle: controller.LifecycleCoordinator(), registry: controller.RuntimeRegistry(), restart: controller.ReloadRuntimeIfRunning, stop: controller.Stop}
	extensions := newRuntimeExtensionManager(backend)
	backend.ready = extensions.RuntimeReady
	engine.runtimes = backend
	engine.extensions = extensions
	controller.AttachEngine(files, &engine.interactions, extensions)
	controller.RuntimeRegistry().Seal()
	engine.agents = controller
	return engine
}

func (a runtimeBackend) LoadRuntimeExtension(agentID, name string) (json.RawMessage, bool, error) {
	return a.repository.RuntimeExtension(agentID, name)
}

func (a runtimeBackend) ListRuntimeExtensions(agentID string) (map[string]json.RawMessage, error) {
	return a.repository.RuntimeExtensionList(agentID)
}

func (a runtimeBackend) StoreRuntimeExtension(agentID, name string, raw json.RawMessage) error {
	return a.repository.PutRuntimeExtension(agentID, name, raw)
}

func (a runtimeBackend) RemoveRuntimeExtension(agentID, name string) error {
	return a.repository.DeleteRuntimeExtension(agentID, name)
}

func (a runtimeBackend) ReconcileRuntimeExtension(ctx context.Context, agentID string, desired runtime.ExtensionDesired) (result runtime.ExtensionResult, err error) {
	host, implementation, selected, err := a.extensionHost(ctx, agentID)
	if err != nil {
		return result, err
	}
	before, err := host.ExtensionProjections(agentID)
	if err != nil {
		return extensionFailure("projection_invalid", "The managed extension projections could not be read"), errors.New("managed extension projections are invalid")
	}
	previous, exists := findProjection(before, desired.Name)
	changedSource := exists && (previous.SourceRevision != desired.SourceRevision || previous.Kind != desired.Kind)
	provider, ok := implementation.(runtime.ExtensionDriverProvider)
	if !ok {
		return result, runtime.ErrExtensionUnsupported
	}
	driver, ok := provider.RuntimeExtensionDriver(desired.Kind)
	if !ok || driver == nil {
		if exists {
			if _, cleanupErr := a.DeleteRuntimeExtensionProjection(ctx, agentID, desired.Name, previous.Kind); cleanupErr != nil {
				return extensionFailure("cleanup_failed", "The previous extension configuration could not be disabled"), cleanupErr
			}
		}
		return result, runtime.ErrExtensionUnsupported
	}
	prepared, result, prepareErr := driver.PrepareExtension(ctx, agentID, desired)
	if prepareErr != nil || result.State != runtime.ExtensionStateConfigured {
		if changedSource {
			if _, cleanupErr := a.DeleteRuntimeExtensionProjection(ctx, agentID, desired.Name, desired.Kind); cleanupErr != nil {
				return extensionFailure("cleanup_failed", "The previous extension configuration could not be disabled"), cleanupErr
			}
		}
		return result, prepareErr
	}
	if prepared == nil {
		return extensionFailure("prepare_failed", "The Runtime did not prepare an extension projection"), errors.New("extension preparation is missing")
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	next := replaceProjection(before, prepared.Projection())
	if err := validateExtensionEnvironment(next, selected.Spec.Model.Env); err != nil {
		_ = prepared.Cleanup(cleanupCtx)
		if changedSource {
			if _, cleanupErr := a.DeleteRuntimeExtensionProjection(cleanupCtx, agentID, desired.Name, desired.Kind); cleanupErr != nil {
				return extensionFailure("cleanup_failed", "The previous extension configuration could not be disabled"), cleanupErr
			}
		}
		return extensionFailure("environment_conflict", err.Error()), err
	}
	rollback := func(reason, message string) (runtime.ExtensionResult, error) {
		rollbackErr := prepared.Rollback(cleanupCtx)
		restoreErr := host.RenderExtensions(cleanupCtx, agentID, before)
		_ = prepared.Cleanup(cleanupCtx)
		if changedSource {
			_, rollbackErr = a.DeleteRuntimeExtensionProjection(cleanupCtx, agentID, desired.Name, desired.Kind)
		}
		if rollbackErr != nil || restoreErr != nil {
			return extensionFailure("rollback_failed", "The extension could not be safely restored; retry cleanup"), errors.New("extension rollback failed")
		}
		return extensionFailure(reason, message), errors.New(message)
	}
	if err := prepared.Activate(ctx); err != nil {
		return rollback("activate_failed", "The extension configuration could not be activated")
	}
	if err := host.RenderExtensions(ctx, agentID, next); err != nil {
		return rollback("instructions_refresh_failed", "The extension instructions could not be refreshed")
	}
	observed, observeErr := driver.ObserveExtension(ctx, agentID, desired)
	running := selected.Status.State == AgentStateRunning
	needsReload := running && (!observed.RuntimeLoaded || observeErr != nil || !sameProjectionSet(before, next))
	if needsReload && !desired.DeferRuntimeReload {
		if _, _, restartErr := a.restart(ctx, agentID); restartErr != nil {
			result.RuntimeLoaded = false
			result.Reason = "restart_failed"
			result.Message = "The tool is configured, but the Runtime could not reload it. Start the Agent and retry."
		} else {
			observed, observeErr = driver.ObserveExtension(ctx, agentID, desired)
			result.RuntimeLoaded = observeErr == nil && observed.RuntimeLoaded
			if !result.RuntimeLoaded {
				result.Reason = "reload_unverified"
				result.Message = "The tool is configured, but the Runtime has not confirmed loading it. Retry."
			}
		}
	} else {
		result.RuntimeLoaded = running && observeErr == nil && observed.RuntimeLoaded
	}
	if cleanupErr := prepared.Cleanup(cleanupCtx); cleanupErr != nil {
		result.Reason = "cleanup_failed"
		result.Message = "The extension is configured; obsolete managed files still need cleanup."
	}
	return result, nil
}

func (a runtimeBackend) ObserveRuntimeExtension(ctx context.Context, agentID string, desired runtime.ExtensionDesired) (runtime.ExtensionResult, error) {
	_, implementation, _, err := a.extensionHost(ctx, agentID)
	if err != nil {
		return runtime.ExtensionResult{}, err
	}
	provider, ok := implementation.(runtime.ExtensionDriverProvider)
	if !ok {
		return runtime.ExtensionResult{}, runtime.ErrExtensionUnsupported
	}
	driver, ok := provider.RuntimeExtensionDriver(desired.Kind)
	if !ok || driver == nil {
		return runtime.ExtensionResult{}, runtime.ErrExtensionUnsupported
	}
	return driver.ObserveExtension(ctx, agentID, desired)
}

func (a runtimeBackend) DeleteRuntimeExtensionProjection(ctx context.Context, agentID, name, _ string) (result runtime.ExtensionResult, err error) {
	err = a.WithAgentMutation(ctx, agentID, func(ctx context.Context) error {
		host, _, selected, err := a.extensionHost(ctx, agentID)
		if errors.Is(err, runtime.ErrExtensionUnsupported) {
			// This Adapter has no extension projections, including after a
			// Runtime replacement. The desired child can still be removed.
			result = runtime.ExtensionResult{State: runtime.ExtensionStateConfigured, Reason: "deleted", CheckedAt: time.Now().UTC()}
			return nil
		}
		if err != nil {
			return err
		}
		before, readErr := host.ExtensionProjections(agentID)
		change, err := host.PrepareExtensionDelete(ctx, agentID, name)
		if err != nil {
			return err
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := change.Activate(ctx); err != nil {
			return err
		}
		// Removing the selected root can also repair its corrupted manifest.
		next, err := host.ExtensionProjections(agentID)
		if err != nil {
			_, _ = a.stop(cleanupCtx, agentID)
			_ = change.Cleanup(cleanupCtx)
			return errors.New("remaining extension projections are invalid; Runtime stopped")
		}
		if err := host.RenderExtensions(ctx, agentID, next); err != nil {
			_, _ = a.stop(cleanupCtx, agentID)
			_ = change.Cleanup(cleanupCtx)
			return errors.New("extension instructions could not be removed; Runtime stopped, retry cleanup")
		}
		if selected.Status.State == AgentStateRunning && (readErr != nil || !sameProjectionSet(before, next)) {
			if _, _, err := a.restart(ctx, agentID); err != nil {
				_ = change.Cleanup(cleanupCtx)
				return errors.New("extension removed, but Runtime reload failed; retry cleanup")
			}
		}
		if err := change.Cleanup(cleanupCtx); err != nil {
			return errors.New("managed extension files could not be removed")
		}
		result = runtime.ExtensionResult{State: runtime.ExtensionStateConfigured, Reason: "deleted", CheckedAt: time.Now().UTC()}
		return nil
	})
	return result, err
}

func (a runtimeBackend) extensionHost(ctx context.Context, agentID string) (runtime.ExtensionHost, runtime.Runtime, Agent, error) {
	selected, err := a.agents.Get(ctx, agentID, AgentGetOptions{ProbeRuntime: true})
	if err != nil {
		return nil, nil, selected, err
	}
	implementation, err := a.registry.Get(selected.Status.RuntimeKind)
	if err != nil {
		return nil, nil, selected, err
	}
	host, ok := implementation.(runtime.ExtensionHost)
	if !ok {
		return nil, nil, selected, runtime.ErrExtensionUnsupported
	}
	return host, implementation, selected, nil
}
func extensionFailure(reason, message string) runtime.ExtensionResult {
	return runtime.ExtensionResult{State: runtime.ExtensionStateError, Reason: reason, Message: message, CheckedAt: time.Now().UTC()}
}
func findProjection(items []runtime.ExtensionProjection, name string) (runtime.ExtensionProjection, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return runtime.ExtensionProjection{}, false
}
func removeProjection(items []runtime.ExtensionProjection, name string) []runtime.ExtensionProjection {
	out := make([]runtime.ExtensionProjection, 0, len(items))
	for _, item := range items {
		if item.Name != name {
			out = append(out, item)
		}
	}
	return out
}
func replaceProjection(items []runtime.ExtensionProjection, current runtime.ExtensionProjection) []runtime.ExtensionProjection {
	out := append(removeProjection(items, current.Name), current)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func sameProjectionSet(a, b []runtime.ExtensionProjection) bool {
	if len(a) != len(b) {
		return false
	}
	for _, item := range a {
		other, found := findProjection(b, item.Name)
		if !found || other.Digest != item.Digest {
			return false
		}
	}
	return true
}
func validateExtensionEnvironment(items []runtime.ExtensionProjection, user map[string]string) error {
	values := make(map[string]string)
	owners := make(map[string]string)
	for _, item := range items {
		for key, value := range item.Environment {
			if !extensionEnvironmentKey.MatchString(key) {
				return fmt.Errorf("extension %q contains an invalid environment key", item.Name)
			}
			if previous, exists := values[key]; exists && previous != value {
				return fmt.Errorf("environment key %q conflicts between extensions %q and %q", key, owners[key], item.Name)
			}
			if previous, exists := user[key]; exists && previous != value {
				return fmt.Errorf("environment key %q conflicts with the Agent profile", key)
			}
			values[key] = value
			owners[key] = item.Name
		}
	}
	return nil
}

var extensionEnvironmentKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (a runtimeBackend) conversationRuntime(ctx context.Context, agentID string) (conversationRuntimeAdapter, func(), *TurnError) {
	if a.agents == nil {
		return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is required"}
	}
	release, err := a.lifecycle.Execution(ctx, agent.CanonicalID(agentID))
	if err != nil {
		return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: err.Error()}
	}
	selected, getErr := a.agents.Get(ctx, agentID, AgentGetOptions{ProbeRuntime: true})
	if getErr != nil {
		release()
		return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: getErr.Error()}
	}
	if a.ready != nil {
		if err := a.ready(selected.ID); err != nil {
			release()
			return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: err.Error()}
		}
	}
	implementation, runtimeErr := a.registry.Get(selected.Status.RuntimeKind)
	if runtimeErr != nil {
		release()
		return nil, nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: runtimeErr.Error()}
	}
	if !strings.EqualFold(strings.TrimSpace(string(selected.Status.State)), string(runtime.StateRunning)) || strings.TrimSpace(selected.Status.RuntimeID) == "" {
		release()
		return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q is unavailable", agentID)}
	}
	provider, ok := implementation.(contract.ConversationProvider)
	if !ok {
		release()
		return nil, nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: fmt.Sprintf("runtime adapter %q does not support conversations", selected.Status.RuntimeKind)}
	}
	adapter := provider.Conversation(selected.Status.RuntimeID)
	if adapter == nil {
		release()
		return nil, nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: "runtime conversation adapter is unavailable"}
	}
	return adapter, release, nil
}
