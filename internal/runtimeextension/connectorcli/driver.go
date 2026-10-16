package connectorcli

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os/exec"
	"path/filepath"
	"time"

	"csgclaw/internal/identity"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
)

const Kind = "connector-cli"
const SourceProvider = "connector-cli"

type Payload struct {
	AgentID      string            `json:"agent_id"`
	ResourceID   string            `json:"resource_id"`
	ConnectorID  string            `json:"connector_id"`
	ConfigDir    string            `json:"config_dir"`
	Executable   string            `json:"executable"`
	Environment  map[string]string `json:"environment"`
	Instructions string            `json:"instructions,omitempty"`
	VersionArgs  []string          `json:"version_args"`
}

type Driver struct {
	ResolveHome   func(string) (string, error)
	RuntimeLoaded func(string, agentruntime.ExtensionProjection) bool
}

func (d Driver) store(agentID string) (*extensionstate.Store, error) {
	home, err := d.ResolveHome(agentID)
	if err != nil {
		return nil, err
	}
	return extensionstate.New(filepath.Join(home, "runtime-extensions"))
}
func (d Driver) projection(ctx context.Context, id string, desired agentruntime.ExtensionDesired) (agentruntime.ExtensionProjection, error) {
	var p Payload
	if err := json.Unmarshal(desired.Payload, &p); err != nil {
		return agentruntime.ExtensionProjection{}, err
	}
	if identity.CanonicalAgentID(p.AgentID) != identity.CanonicalAgentID(id) || p.ResourceID == "" || !filepath.IsAbs(p.Executable) || len(p.VersionArgs) == 0 {
		return agentruntime.ExtensionProjection{}, errors.New("invalid CLI projection")
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(probe, p.Executable, p.VersionArgs...).Run(); err != nil {
		return agentruntime.ExtensionProjection{}, errors.New("CLI executable is unavailable")
	}
	return agentruntime.ExtensionProjection{Name: desired.Name, Kind: desired.Kind, Generation: desired.Generation, SourceRevision: desired.SourceRevision, Executable: p.Executable, Environment: p.Environment, Instructions: p.Instructions}, nil
}
func same(a, b agentruntime.ExtensionProjection) bool {
	return a.SourceRevision == b.SourceRevision && a.Kind == b.Kind && a.Executable == b.Executable && a.Instructions == b.Instructions && maps.Equal(a.Environment, b.Environment)
}
func result(err error) agentruntime.ExtensionResult {
	value := agentruntime.ExtensionResult{State: agentruntime.ExtensionStateConfigured, Reason: "configured", CheckedAt: time.Now().UTC()}
	if err != nil {
		value.State = agentruntime.ExtensionStateError
		value.Reason = "connector_unavailable"
		value.Message = err.Error()
	}
	return value
}
func (d Driver) PrepareExtension(ctx context.Context, id string, desired agentruntime.ExtensionDesired) (agentruntime.PreparedExtension, agentruntime.ExtensionResult, error) {
	p, err := d.projection(ctx, id, desired)
	if err != nil {
		return nil, result(err), err
	}
	store, err := d.store(id)
	if err != nil {
		return nil, result(err), err
	}
	previous, found, err := store.Load(desired.Name)
	if err != nil {
		return nil, result(err), err
	}
	if found && same(previous, p) {
		previous.Generation = p.Generation
		change, err := store.Revise(previous)
		return change, result(err), err
	}
	change, err := store.Stage(desired.Name)
	if err != nil {
		return nil, result(err), err
	}
	change.SetProjection(p)
	return change, result(nil), nil
}
func (d Driver) ObserveExtension(ctx context.Context, id string, desired agentruntime.ExtensionDesired) (agentruntime.ExtensionResult, error) {
	p, err := d.projection(ctx, id, desired)
	if err != nil {
		return result(err), nil
	}
	store, err := d.store(id)
	if err != nil {
		return result(err), err
	}
	current, found, err := store.Load(desired.Name)
	if err != nil {
		return result(err), err
	}
	if !found || !same(current, p) || current.Generation != p.Generation {
		return result(errors.New("CLI projection does not match")), nil
	}
	value := result(nil)
	value.RuntimeLoaded = d.RuntimeLoaded != nil && d.RuntimeLoaded(id, current)
	return value, nil
}
