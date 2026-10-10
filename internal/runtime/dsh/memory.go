package dsh

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const (
	memoryFileName      = "memory_summary.md"
	memoryPatchFileName = "csgclaw-memory.patch.yml"
	maxMemoryBytes      = 32 * 1024
)

//go:embed embed/memory.mjs
var memoryModule []byte

var _ agentruntime.MemoryController = (*Runtime)(nil)

func (r *Runtime) ReadMemoryDocument(_ context.Context, agentHome string, raw map[string]any) (agentruntime.MemoryDocument, error) {
	opts, err := DecodeRuntimeOptions(raw)
	if err != nil {
		return agentruntime.MemoryDocument{}, err
	}
	if strings.TrimSpace(agentHome) == "" {
		return agentruntime.MemoryDocument{}, fmt.Errorf("agent home is required")
	}
	document := agentruntime.MemoryDocument{Enabled: opts.MemoryMode == MemoryModeEnabled, Name: memoryFileName, Location: "$DSH_HOME/memories/" + memoryFileName}
	root, err := os.OpenRoot(filepath.Join(agentHome, hostStateDirName, homeDirName))
	if errors.Is(err, os.ErrNotExist) {
		return document, nil
	}
	if err != nil {
		return document, err
	}
	defer root.Close()
	dirInfo, err := root.Lstat("memories")
	if errors.Is(err, os.ErrNotExist) {
		return document, nil
	}
	if err != nil {
		return document, err
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return document, fmt.Errorf("DSH memory directory must be a real directory")
	}
	fileInfo, err := root.Lstat(filepath.Join("memories", memoryFileName))
	if errors.Is(err, os.ErrNotExist) {
		return document, nil
	}
	if err != nil {
		return document, err
	}
	if !fileInfo.Mode().IsRegular() {
		return document, fmt.Errorf("DSH memory must be a regular file")
	}
	file, err := root.Open(filepath.Join("memories", memoryFileName))
	if errors.Is(err, os.ErrNotExist) {
		return document, nil
	}
	if err != nil {
		return document, fmt.Errorf("read DSH memory: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return document, err
	}
	if !info.Mode().IsRegular() {
		return document, fmt.Errorf("DSH memory must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxMemoryBytes+1))
	if err != nil {
		return document, err
	}
	if len(data) > maxMemoryBytes {
		return document, fmt.Errorf("DSH memory exceeds %d bytes", maxMemoryBytes)
	}
	document.Ready, document.Content = true, string(data)
	return document, nil
}

func (r *Runtime) ConfigureMemory(raw map[string]any, enabled bool) (map[string]any, error) {
	if _, err := DecodeRuntimeOptions(raw); err != nil {
		return nil, err
	}
	next := maps.Clone(raw)
	if next == nil {
		next = make(map[string]any)
	}
	mode := MemoryModeDisabled
	if enabled {
		mode = MemoryModeEnabled
	}
	next[MemoryModeOptionKey] = mode
	return next, nil
}

func writeMemoryPatch(root string, raw map[string]any) error {
	opts, err := DecodeRuntimeOptions(raw)
	if err != nil {
		return err
	}
	modulePath := filepath.Join(root, "memory.mjs")
	if err := os.WriteFile(modulePath, memoryModule, 0o600); err != nil {
		return err
	}
	patch := fmt.Sprintf("- insert:\n    - id: csgclaw-memory\n      name: %q\n      config:\n        enabled: %t\n", modulePath, opts.MemoryMode == MemoryModeEnabled)
	return os.WriteFile(filepath.Join(root, memoryPatchFileName), []byte(patch), 0o600)
}

func provisionMemory(root string, req agentruntime.ProvisionRequest) error {
	if err := writeMemoryPatch(root, req.RuntimeOptions); err != nil {
		return err
	}
	opts, err := DecodeRuntimeOptions(req.RuntimeOptions)
	if err != nil {
		return err
	}
	if !req.TemplateMemorySet || opts.MemoryMode != MemoryModeEnabled {
		return nil
	}
	if len(req.TemplateMemory) > maxMemoryBytes {
		return fmt.Errorf("DSH template memory exceeds %d bytes", maxMemoryBytes)
	}
	dir := filepath.Join(root, homeDirName, "memories")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("DSH memory directory must be a real directory")
	}
	home, err := os.OpenRoot(filepath.Join(root, homeDirName))
	if err != nil {
		return err
	}
	defer home.Close()
	// Reprovisioning retains the learned summary instead of replacing it with the seed.
	file, err := home.OpenFile(filepath.Join("memories", memoryFileName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(req.TemplateMemory)
	return errors.Join(writeErr, file.Close())
}
