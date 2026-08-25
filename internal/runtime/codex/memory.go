package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const (
	readableMemoryFileName = "MEMORY.md"
	readableMemoryLocation = "$CODEX_HOME/memories/MEMORY.md"
)

func (r *Runtime) ReadMemoryDocument(_ context.Context, agentHome string, rawOptions map[string]any) (agentruntime.MemoryDocument, error) {
	options, err := DecodeRuntimeOptions(rawOptions)
	if err != nil {
		return agentruntime.MemoryDocument{}, err
	}
	agentHome = strings.TrimSpace(agentHome)
	if agentHome == "" {
		return agentruntime.MemoryDocument{}, fmt.Errorf("agent home is required")
	}
	path := filepath.Join(agentHome, filepath.FromSlash(hostStateDirName), homeDirName, "memories", readableMemoryFileName)
	raw, err := r.readFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentruntime.MemoryDocument{Enabled: options.MemoryMode == MemoryModeEnabled, Name: readableMemoryFileName, Location: readableMemoryLocation}, nil
		}
		return agentruntime.MemoryDocument{}, fmt.Errorf("read Codex memory document %s: %w", path, err)
	}
	return agentruntime.MemoryDocument{
		Enabled:  options.MemoryMode == MemoryModeEnabled,
		Ready:    true,
		Name:     readableMemoryFileName,
		Location: readableMemoryLocation,
		Content:  string(raw),
	}, nil
}

func (r *Runtime) ConfigureMemory(rawOptions map[string]any, enabled bool) (map[string]any, error) {
	if _, err := DecodeRuntimeOptions(rawOptions); err != nil {
		return nil, err
	}
	next := cloneAnyMap(rawOptions)
	if next == nil {
		next = make(map[string]any)
	}
	mode := MemoryModeDisabled
	if enabled {
		mode = MemoryModeEnabled
	}
	next[memoryModeOptionKey] = mode
	return next, nil
}
