package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrExtensionUnsupported = errors.New("runtime extension is unsupported")

// ExtensionDesired is the transient, resolved Runtime-facing view of one
// Agent Engine RuntimeExtension. Payload may contain secrets and must not be
// persisted or logged by Runtime implementations.
type ExtensionDesired struct {
	Name               string
	Kind               string
	Generation         int64
	SourceRevision     string
	Payload            json.RawMessage
	DeferRuntimeReload bool
}

type ExtensionResult struct {
	State           string
	Reason          string
	Message         string
	RuntimeLoaded   bool
	RestartRequired bool
	CheckedAt       time.Time
}

// ExtensionProjection is Runtime-private derived state. It is never part of an
// Engine resource response and contains no resolved Source payload.
type ExtensionProjection struct {
	Name           string            `json:"name"`
	Kind           string            `json:"kind"`
	Generation     int64             `json:"generation"`
	SourceRevision string            `json:"source_revision"`
	Digest         string            `json:"digest"`
	Root           string            `json:"root"`
	Executable     string            `json:"executable,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	Instructions   string            `json:"instructions,omitempty"`
}

// PreparedExtension owns only managed staging. Activate is reversible until
// Cleanup; Cleanup retains an activated generation and removes obsolete staging.
type PreparedExtension interface {
	Projection() ExtensionProjection
	Activate(context.Context) error
	Rollback(context.Context) error
	Cleanup(context.Context) error
}

// ExtensionPreparer validates/probes and stages without changing active state.
type ExtensionPreparer interface {
	PrepareExtension(context.Context, string, ExtensionDesired) (PreparedExtension, ExtensionResult, error)
}

// ExtensionHost projects the complete, Engine-ordered contribution set. Deletion
// does not require an installed Driver or executable, so failed cleanup is retryable.
type ExtensionHost interface {
	ExtensionProjections(string) ([]ExtensionProjection, error)
	RenderExtensions(context.Context, string, []ExtensionProjection) error
	PrepareExtensionDelete(context.Context, string, string) (PreparedExtension, error)
}

const (
	ExtensionStateConfigured  = "configured"
	ExtensionStateUnavailable = "unavailable"
	ExtensionStateError       = "error"
)

// ExtensionDriver owns Runtime-specific validation, layout, staging,
// activation, observation, and cleanup for one extension kind.
type ExtensionDriver interface {
	ExtensionPreparer
	ObserveExtension(ctx context.Context, agentID string, desired ExtensionDesired) (ExtensionResult, error)
}

// ExtensionDriverProvider is implemented by Runtime Adapters that support
// independently reconcilable Runtime extensions.
type ExtensionDriverProvider interface {
	RuntimeExtensionDriver(kind string) (ExtensionDriver, bool)
}

// CanonicalEnvironmentKey returns the comparison form used for protected and
// extension-owned environment variables. Using one form on every platform
// keeps a configuration portable to Windows, where names are case-insensitive.
func CanonicalEnvironmentKey(key string) string {
	return strings.ToUpper(strings.TrimSpace(key))
}

// ExtensionInstructions returns the ordered instruction contributions from
// the active Runtime projections.
func ExtensionInstructions(projections []ExtensionProjection) []string {
	fragments := make([]string, 0, len(projections))
	for _, projection := range projections {
		if projection.Instructions != "" {
			fragments = append(fragments, projection.Instructions)
		}
	}
	return fragments
}

// MergeExtensionEnvironment applies Runtime projections to a base process
// environment and returns the effective projection digests.
func MergeExtensionEnvironment(base []string, profile map[string]string, projections []ExtensionProjection) ([]string, map[string]string, error) {
	values := make(map[string]string, len(base))
	effectiveKeys := make(map[string]string, len(base))
	for _, entry := range base {
		key, value, found := strings.Cut(entry, "=")
		if found {
			canonical := CanonicalEnvironmentKey(key)
			if previous := effectiveKeys[canonical]; previous != "" && previous != key {
				delete(values, previous)
			}
			values[key] = value
			effectiveKeys[canonical] = key
		}
	}
	profileValues := make(map[string]string, len(profile))
	for key, value := range profile {
		profileValues[CanonicalEnvironmentKey(key)] = value
	}
	contributed := make(map[string]string)
	digests := make(map[string]string, len(projections))
	for _, projection := range projections {
		for key, value := range projection.Environment {
			canonical := CanonicalEnvironmentKey(key)
			if previous, ok := contributed[canonical]; ok && previous != value {
				return nil, nil, fmt.Errorf("conflicting extension environment key %q", key)
			}
			if previous, ok := profileValues[canonical]; ok && previous != value {
				return nil, nil, fmt.Errorf("extension environment key %q conflicts with the Agent profile", key)
			}
			if previous := effectiveKeys[canonical]; previous != "" && previous != key {
				delete(values, previous)
			}
			contributed[canonical] = value
			values[key] = value
			effectiveKeys[canonical] = key
		}
		digests[projection.Name] = projection.Digest
	}
	for _, projection := range projections {
		if projection.Executable != "" {
			pathKey := effectiveKeys["PATH"]
			if pathKey == "" {
				pathKey = "PATH"
			}
			dir := filepath.Dir(projection.Executable)
			parts := filepath.SplitList(values[pathKey])
			found := false
			for _, part := range parts {
				if filepath.Clean(part) == dir {
					found = true
					break
				}
			}
			if !found {
				values[pathKey] = dir + string(os.PathListSeparator) + values[pathKey]
			}
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment, digests, nil
}
