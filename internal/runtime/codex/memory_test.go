package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadMemoryDocumentUsesReadableCodexMemoryFile(t *testing.T) {
	agentHome := t.TempDir()
	rt := New(Dependencies{})

	missing, err := rt.ReadMemoryDocument(context.Background(), agentHome, nil)
	if err != nil {
		t.Fatalf("ReadMemoryDocument() missing error = %v", err)
	}
	if !missing.Enabled || missing.Ready || missing.Name != readableMemoryFileName || missing.Location != readableMemoryLocation || missing.Content != "" {
		t.Fatalf("ReadMemoryDocument() missing = %#v, want enabled and not ready", missing)
	}

	memoryPath := filepath.Join(agentHome, filepath.FromSlash(hostStateDirName), homeDirName, "memories", readableMemoryFileName)
	if err := os.MkdirAll(filepath.Dir(memoryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memoryPath, []byte("# Durable memory\n\nRemember this.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	document, err := rt.ReadMemoryDocument(context.Background(), agentHome, map[string]any{memoryModeOptionKey: MemoryModeDisabled})
	if err != nil {
		t.Fatalf("ReadMemoryDocument() error = %v", err)
	}
	if document.Enabled || !document.Ready || document.Name != readableMemoryFileName || document.Location != readableMemoryLocation || document.Content != "# Durable memory\n\nRemember this.\n" {
		t.Fatalf("ReadMemoryDocument() = %#v", document)
	}
}

func TestConfigureMemoryPreservesOtherRuntimeOptions(t *testing.T) {
	rt := New(Dependencies{})
	original := map[string]any{
		executionModeOptionKey:     ExecutionModeReadOnly,
		localWorkspaceDirOptionKey: "/tmp/project",
	}

	disabled, err := rt.ConfigureMemory(original, false)
	if err != nil {
		t.Fatalf("ConfigureMemory(false) error = %v", err)
	}
	if disabled[memoryModeOptionKey] != MemoryModeDisabled || disabled[executionModeOptionKey] != ExecutionModeReadOnly || disabled[localWorkspaceDirOptionKey] != "/tmp/project" {
		t.Fatalf("ConfigureMemory(false) = %#v", disabled)
	}
	if _, exists := original[memoryModeOptionKey]; exists {
		t.Fatalf("ConfigureMemory() mutated original options: %#v", original)
	}

	enabled, err := rt.ConfigureMemory(disabled, true)
	if err != nil {
		t.Fatalf("ConfigureMemory(true) error = %v", err)
	}
	if enabled[memoryModeOptionKey] != MemoryModeEnabled {
		t.Fatalf("ConfigureMemory(true) = %#v", enabled)
	}
}
