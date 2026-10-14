package dsh

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestDSHMemoryConfigurationAndPersistence(t *testing.T) {
	rt := New(Dependencies{})
	home := t.TempDir()
	document, err := rt.ReadMemoryDocument(context.Background(), home, nil)
	if err != nil || !document.Enabled || document.Ready {
		t.Fatalf("empty memory = %+v, %v", document, err)
	}
	root := filepath.Join(home, hostStateDirName)
	if err := os.MkdirAll(filepath.Join(root, homeDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	req := agentruntime.ProvisionRequest{TemplateMemorySet: true, TemplateMemory: "A durable preference"}
	if err := provisionMemory(root, req); err != nil {
		t.Fatal(err)
	}
	req.TemplateMemory = "Replacement seed"
	if err := provisionMemory(root, req); err != nil {
		t.Fatal(err)
	}
	document, err = rt.ReadMemoryDocument(context.Background(), home, nil)
	if err != nil || !document.Ready || document.Content != "A durable preference" {
		t.Fatalf("seed overwrote memory: %+v, %v", document, err)
	}
	raw := map[string]any{PermissionModeOptionKey: PermissionModeReadOnly, localWorkspaceDirOptionKey: "/example"}
	disabled, err := rt.ConfigureMemory(raw, false)
	if err != nil || disabled[MemoryModeOptionKey] != MemoryModeDisabled || raw[MemoryModeOptionKey] != nil || disabled[localWorkspaceDirOptionKey] != "/example" {
		t.Fatalf("ConfigureMemory = %#v, %v", disabled, err)
	}
	document, err = rt.ReadMemoryDocument(context.Background(), home, disabled)
	if err != nil || document.Enabled || !document.Ready {
		t.Fatalf("disabled memory = %+v, %v", document, err)
	}
	persistentFiles := map[string]string{
		filepath.Join(homeDirName, "memories", "maintenance.json"):                 `{"version":1,"checkpoints":{},"pending":[{"id":"abcdefabcdefabcdefabcdef","sessionId":"s","route":{"provider":"p","model":"m"},"transcript":"user: Chinese replies.","capturedAt":100,"dueAt":3660100,"manualGeneration":0}],"buffered":{},"manualGeneration":0}`,
		filepath.Join(homeDirName, "memories", "rollout_summaries", "previous.md"): "A previous extraction record.",
	}
	for relative, content := range persistentFiles {
		filename := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	preserved, err := preserveRecreateState(root)
	if err != nil {
		t.Fatal(err)
	}
	defer preserved.Cleanup()
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := preserved.Restore(); err != nil {
		t.Fatal(err)
	}
	document, err = rt.ReadMemoryDocument(context.Background(), home, nil)
	if err != nil || document.Content != "A durable preference" {
		t.Fatalf("recreated memory = %+v, %v", document, err)
	}
	for relative, content := range persistentFiles {
		data, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil || string(data) != content {
			t.Fatalf("recreated memory state %s was not retained: %v", relative, err)
		}
	}
	for _, invalid := range []any{"automatic", true} {
		if _, err := DecodeRuntimeOptions(map[string]any{MemoryModeOptionKey: invalid}); err == nil {
			t.Fatalf("accepted invalid memory mode %v", invalid)
		}
	}
}

func TestDSHMemoryRejectsUnsafeSummary(t *testing.T) {
	for _, kind := range []string{"symlink-file", "symlink-directory", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, hostStateDirName)
			memoryDir := filepath.Join(root, homeDirName, "memories")
			if err := os.MkdirAll(memoryDir, 0o700); err != nil {
				t.Fatal(err)
			}
			summary := filepath.Join(memoryDir, memoryFileName)
			if kind == "oversized" {
				if err := os.WriteFile(summary, []byte(strings.Repeat("x", maxMemoryBytes+1)), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				target := filepath.Join(root, homeDirName, "unrelated-file")
				if err := os.WriteFile(target, []byte("unrelated managed state"), 0o600); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink-directory" {
					if err := os.Remove(memoryDir); err != nil {
						t.Fatal(err)
					}
					summary, target = memoryDir, t.TempDir()
				}
				if err := os.Symlink(target, summary); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			if _, err := New(Dependencies{}).ReadMemoryDocument(context.Background(), home, nil); err == nil {
				t.Fatal("accepted an unsafe memory summary")
			}
			if kind == "symlink-directory" {
				if err := provisionMemory(root, agentruntime.ProvisionRequest{TemplateMemorySet: true, TemplateMemory: "seed"}); err == nil {
					t.Fatal("seeded memory through a symlink directory")
				}
			}
		})
	}
}

func TestDSHMemoryPlugin(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for the managed memory plugin test")
	}
	root := t.TempDir()
	module := filepath.Join(root, "memory.mjs")
	if err := writeMemoryPatch(root, nil, agentruntime.Profile{}); err != nil {
		t.Fatal(err)
	}
	const script = `
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
const {apply} = await import(pathToFileURL(process.argv[1]));
const tools = new Map(), sections = [], contexts = [];
const ctx = {tools: {register: tool => tools.set(tool.name, tool)}, systemPrompt: {section: value => sections.push(value), context: value => contexts.push(value)}, on: () => {}, effect: value => value(), logger: {warn: () => {}}, llm: {}};
const config = {enabled: true, checkpointIntervalMs:3600000, checkpointIdleMs:3660000, retryDelayMs:300000, requestTimeoutMs:120000};
const exec = {signal: new AbortController().signal};
apply(ctx, config);
assert.equal(tools.size, 3);
let current = tools.get('memory_read').execute({}, exec);
assert.equal(current.content, '');
const saved = tools.get('memory_update').execute({content:'The user prefers Chinese. {{literal}}', expected_revision:current.revision}, exec);
assert.ok(contexts[0].text().includes(saved.content));
assert.equal(contexts[0].interpolate, false);
assert.throws(() => tools.get('memory_update').execute({content:'Lost update', expected_revision:current.revision}, exec), /revision conflict/);
assert.throws(() => tools.get('memory_update').execute({content:'x'.repeat(32769), expected_revision:saved.revision}, exec), /exceeds/);
const summary = path.join(process.env.DSH_HOME, 'memories', 'memory_summary.md');
assert.equal(fs.readFileSync(summary, 'utf8'), saved.content);
assert.equal(tools.get('memory_read').execute({document:'handbook'}, exec).content, saved.content);
assert.ok(tools.get('memory_search').execute({query:'Chinese'}, exec).matches.length > 0);
const detailed = tools.get('memory_read').execute({document:'handbook'}, exec);
assert.throws(() => tools.get('memory_update').execute({content:saved.content, expected_revision:saved.revision, handbook:'Missing revision'}, exec), /supplied together/);
tools.get('memory_update').execute({content:saved.content, expected_revision:saved.revision, handbook:'# Chinese documents\nUse project-specific terminology.', expected_handbook_revision:detailed.revision}, exec);
assert.ok(tools.get('memory_read').execute({document:'handbook'}, exec).content.includes('project-specific'));
assert.ok(!contexts[0].text().includes('project-specific'));
tools.clear();
apply(ctx, {enabled: false});
assert.equal(tools.size, 0);
assert.equal(fs.readFileSync(summary, 'utf8'), saved.content);
tools.clear();
apply(ctx, config);
assert.equal(tools.get('memory_read').execute({}, exec).content, saved.content);
if (process.platform !== 'win32') {
  fs.unlinkSync(summary);
  fs.symlinkSync(process.argv[1], summary);
  assert.throws(() => tools.get('memory_read').execute({}, exec), /Invalid memory/);
}

`
	cmd := exec.Command(node, "--input-type=module", "-e", script, module)
	cmd.Env = append(os.Environ(), "DSH_HOME="+root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("memory plugin: %v\n%s", err, output)
	}
}

func TestDSHBackgroundMemoryMaintenance(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for the managed memory plugin test")
	}
	root := t.TempDir()
	if err := writeMemoryPatch(root, nil, agentruntime.Profile{}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--test", "testdata/memory-maintenance.test.mjs")
	cmd.Env = append(os.Environ(), "CSGCLAW_MEMORY_MODULE_ROOT="+root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("background memory: %v\n%s", err, output)
	}
}

func TestDSHMemoryCheckpointTimingMatchesCodex(t *testing.T) {
	root := t.TempDir()
	if err := writeMemoryPatch(root, nil, agentruntime.Profile{ModelID: "configured-model"}); err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(filepath.Join(root, memoryPatchFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"checkpointIntervalMs: 3600000", "checkpointIdleMs: 3660000", "retryDelayMs: 300000", "model: \"configured-model\""} {
		if !strings.Contains(string(patch), setting) {
			t.Fatalf("missing default memory setting %q", setting)
		}
	}
}
