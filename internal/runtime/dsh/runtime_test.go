package dsh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/agentengine/interactionstate"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
)

func TestRuntimeRunsACPConversation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX launcher")
	}
	root := t.TempDir()
	launcher := filepath.Join(root, "dsh-test")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexec \"$DSH_TEST_BINARY\" -test.run=TestDSHHelperProcess\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GO_WANT_DSH_HELPER_PROCESS", "1")
	t.Setenv("DSH_TEST_BINARY", os.Args[0])
	profile := agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "secret-key", ModelID: "test-model", ReasoningEffort: "auto"}
	ref := AgentRef{ID: "agent-test", RuntimeID: "rt-agent-test", Profile: profile}
	rt := New(Dependencies{
		ResolveBinary: func(context.Context, string) (dshcli.Info, error) {
			return dshcli.Info{Path: launcher, Version: "0.1.5-rc.2"}, nil
		},
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) { return ref, nil },
		AgentHome:    func(string) (string, error) { return filepath.Join(root, "agent"), nil },
	})
	t.Cleanup(func() { _ = rt.Close() })
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-agent-test", AgentID: "agent-test", AgentName: "test", Instructions: "Be exact.", Profile: profile,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	patch, err := os.ReadFile(filepath.Join(root, "agent", hostStateDirName, patchFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch), "@deepseek-ai/dsh-tool-present") {
		t.Fatalf("DSH patch does not enable present tool: %s", patch)
	}
	workspace := filepath.Join(root, "agent", hostStateDirName, workspaceDirName)
	if err := os.WriteFile(filepath.Join(workspace, "index.html"), []byte("<!doctype html><title>DSH preview</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := os.ReadFile(filepath.Join(root, "agent", hostStateDirName, homeDirName, settingsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settings), "secret-key") {
		t.Fatal("settings persisted the API key")
	}
	var decodedSettings map[string]struct {
		APIKeyEnv string `json:"apiKeyEnv"`
	}
	if err := json.Unmarshal(settings, &decodedSettings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got := decodedSettings["llm-deepseek"].APIKeyEnv; got != llmAPIKeyEnvName {
		t.Fatalf("llm-deepseek.apiKeyEnv = %q, want %q", got, llmAPIKeyEnvName)
	}
	legacySettings := []byte(`{"llm-deepseek":{"protocol":"chat-completions","apiKeyEnv":"DEEPSEEK_API_KEY"}}`)
	if err := os.WriteFile(filepath.Join(root, "agent", hostStateDirName, homeDirName, settingsFileName), legacySettings, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.New(context.Background(), agentruntime.Spec{RuntimeID: "rt-agent-test", AgentID: "agent-test", Profile: profile}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	settings, err = os.ReadFile(filepath.Join(root, "agent", hostStateDirName, homeDirName, settingsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(settings, &decodedSettings); err != nil {
		t.Fatalf("decode refreshed settings: %v", err)
	}
	if got := decodedSettings["llm-deepseek"].APIKeyEnv; got != llmAPIKeyEnvName {
		t.Fatalf("refreshed llm-deepseek.apiKeyEnv = %q, want %q", got, llmAPIKeyEnvName)
	}

	var mu sync.Mutex
	var events []contract.TurnEvent
	result := rt.Conversation("rt-agent-test").Run(context.Background(), contract.TurnRequest{
		ID: "turn-1", ConversationKey: "room-1", Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "hello"}}, Interaction: contract.InteractionResolve,
	}, contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
		return nil
	}))
	if result.Status != contract.TurnSucceeded || result.Output != "hello from dsh" || !result.Dispatched {
		t.Fatalf("Run() = %+v", result)
	}
	if len(result.RuntimeFiles) != 1 {
		t.Fatalf("RuntimeFiles = %+v, want one presented file", result.RuntimeFiles)
	}
	t.Cleanup(result.RuntimeFiles[0].Cleanup)
	if result.RuntimeFiles[0].Name != "index.html" || result.RuntimeFiles[0].MediaType != "text/html" {
		t.Fatalf("presented file metadata = %+v", result.RuntimeFiles[0].Metadata())
	}
	download, err := result.RuntimeFiles[0].Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(download)
	if closeErr := download.Close(); err == nil {
		err = closeErr
	}
	if err != nil || !strings.Contains(string(data), "DSH preview") {
		t.Fatalf("presented file content = %q, err = %v", data, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 4 || events[0].Kind != contract.TurnEventThoughtDelta ||
		events[1].Kind != contract.TurnEventToolCallStart || events[2].Kind != contract.TurnEventToolCallUpdate ||
		events[3].Kind != contract.TurnEventTextDelta {
		t.Fatalf("events = %+v", events)
	}
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
	}
}

func TestDSHLaunchArgsEnablePresentOverlay(t *testing.T) {
	root := filepath.Join("tmp", "agent", hostStateDirName)
	wantPatch := filepath.Join(root, patchFileName)
	args := dshLaunchArgs(root, true)
	if len(args) != 4 || args[0] != "--profile" || args[1] != "acp" || args[2] != "--patch" || args[3] != wantPatch {
		t.Fatalf("dshLaunchArgs() = %q", args)
	}
	fallback := dshLaunchArgs(root, false)
	if len(fallback) != 2 || fallback[0] != "--profile" || fallback[1] != "acp" {
		t.Fatalf("fallback dshLaunchArgs() = %q", fallback)
	}
}

func TestAuthorizePresentedFilesRejectsPathsOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, turnErr := authorizePresentedFiles(context.Background(), "rt-agent-test", workspace, []presentedFile{{Path: outside}})
	if len(files) != 0 || turnErr == nil || turnErr.Code != contract.ErrorFileUnavailable {
		t.Fatalf("authorizePresentedFiles() = files %#v, error %#v", files, turnErr)
	}
}

func TestAuthorizePresentedFilesRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated permissions")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "target.html")
	if err := os.WriteFile(target, []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(workspace, "preview.html")); err != nil {
		t.Fatal(err)
	}

	files, turnErr := authorizePresentedFiles(context.Background(), "rt-agent-test", workspace, []presentedFile{{Path: "preview.html"}})
	if len(files) != 0 || turnErr == nil || turnErr.Code != contract.ErrorFileUnavailable {
		t.Fatalf("authorizePresentedFiles() = files %#v, error %#v", files, turnErr)
	}
}

func TestBuildEnvironmentSeparatesBridgeAndWebSearchCredentials(t *testing.T) {
	t.Setenv("DSH_HOME", "/ambient/dsh-home")
	t.Setenv("DSH_AGENTS_HOME", "/ambient/dsh-agents")
	t.Setenv(llmAPIKeyEnvName, "ambient-bridge-key")
	t.Setenv("DEEPSEEK_API_KEY", "ambient-search-key")
	t.Setenv("DEEPSEEK_BASE_URL", "https://ambient-chat.example/v1")
	t.Setenv("DEEPSEEK_SEARCH_BASE_URL", "https://search.example/anthropic/v1")

	tests := []struct {
		name          string
		profileEnv    map[string]string
		wantSearchKey string
	}{
		{name: "inherits search credential from server environment", wantSearchKey: "ambient-search-key"},
		{
			name:          "agent environment overrides search credential",
			profileEnv:    map[string]string{"DEEPSEEK_API_KEY": "agent-search-key"},
			wantSearchKey: "agent-search-key",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := environmentMap(buildEnvironment(agentruntime.Profile{
				BaseURL: "http://127.0.0.1:18080/api/v1/agents/agent-test/llm",
				APIKey:  "bridge-key",
				ModelID: "test-model",
				Env:     test.profileEnv,
			}, "/isolated/dsh-home"))

			if got := env[llmAPIKeyEnvName]; got != "bridge-key" {
				t.Fatalf("%s = %q, want bridge credential", llmAPIKeyEnvName, got)
			}
			if got := env["DEEPSEEK_API_KEY"]; got != test.wantSearchKey {
				t.Fatalf("DEEPSEEK_API_KEY = %q, want %q", got, test.wantSearchKey)
			}
			if got := env["DEEPSEEK_BASE_URL"]; got != "https://ambient-chat.example/v1" {
				t.Fatalf("DEEPSEEK_BASE_URL = %q, want ambient value", got)
			}
			if got := env["DEEPSEEK_SEARCH_BASE_URL"]; got != "https://search.example/anthropic/v1" {
				t.Fatalf("DEEPSEEK_SEARCH_BASE_URL = %q, want search endpoint", got)
			}
			if got := env["DSH_HOME"]; got != "/isolated/dsh-home" {
				t.Fatalf("DSH_HOME = %q", got)
			}
			if got := env["DSH_AGENTS_HOME"]; got != "/isolated/dsh-home/agents" {
				t.Fatalf("DSH_AGENTS_HOME = %q", got)
			}
		})
	}
}

func environmentMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, item := range env {
		key, value, found := strings.Cut(item, "=")
		if found {
			values[key] = value
		}
	}
	return values
}

func TestDSHHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DSH_HELPER_PROCESS") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var frame struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &frame) != nil {
			os.Exit(2)
		}
		respond := func(result any) {
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": result})
		}
		switch frame.Method {
		case "initialize":
			respond(map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"mcpCapabilities": map[string]any{"http": true}}})
		case "session/new", "session/resume":
			respond(map[string]any{"sessionId": "session-1", "configOptions": helperConfigOptions()})
		case "session/set_config_option":
			respond(map[string]any{"configOptions": helperConfigOptions()})
		case "session/prompt":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "method": "session/update",
				"params": map[string]any{
					"sessionId": "session-1",
					"update": map[string]any{
						"sessionUpdate": "agent_thought_chunk",
						"content":       map[string]any{"type": "text", "text": "thinking"},
					},
				},
			})
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "method": "session/update",
				"params": map[string]any{
					"sessionId": "session-1",
					"update": map[string]any{
						"sessionUpdate": "tool_call", "toolCallId": "present-1", "title": "present", "kind": "other", "status": "in_progress",
						"rawInput": map[string]any{"files": []any{map[string]any{"path": "index.html", "description": "HTML preview"}}},
					},
				},
			})
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "method": "session/update",
				"params": map[string]any{
					"sessionId": "session-1",
					"update": map[string]any{
						"sessionUpdate": "tool_call_update", "toolCallId": "present-1", "status": "completed",
						"content": []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "Presented index.html"}}},
					},
				},
			})
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0", "method": "session/update",
				"params": map[string]any{
					"sessionId": "session-1",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]any{"type": "text", "text": "hello from dsh"},
					},
				},
			})
			respond(map[string]any{"stopReason": "end_turn"})
		case "session/close":
			respond(map[string]any{})
		default:
			if frame.ID != 0 {
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "error": map[string]any{"code": -32601, "message": fmt.Sprintf("unknown %s", frame.Method)}})
			}
		}
	}
	os.Exit(0)
}

func helperConfigOptions() []map[string]any {
	return []map[string]any{{
		"id": "model", "type": "select", "currentValue": `["deepseek-official","test-model"]`,
		"options": []map[string]any{{"name": "Test Model", "value": `["deepseek-official","test-model"]`}},
	}}
}

func TestBuildACPMCPServersRejectsRelativeCommand(t *testing.T) {
	_, err := buildACPMCPServers(map[string]any{"local": map[string]any{"command": "npx", "args": []any{"server"}}})
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("buildACPMCPServers() error = %v", err)
	}
}

func TestBuildACPMCPServersPreservesRequiredArraysOnWire(t *testing.T) {
	command := filepath.Join(t.TempDir(), "server")
	servers, err := buildACPMCPServers(map[string]any{
		"empty-http":  map[string]any{"url": "https://example.com/mcp"},
		"empty-stdio": map[string]any{"command": command},
		"full-http":   map[string]any{"url": "https://example.com/full", "headers": map[string]any{"Authorization": "Bearer token"}},
		"full-stdio":  map[string]any{"command": command, "args": []any{"serve"}, "env": map[string]any{"MODE": "test"}},
	})
	if err != nil {
		t.Fatalf("buildACPMCPServers() error = %v", err)
	}
	raw, err := json.Marshal(servers)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	byName := make(map[string]map[string]any, len(decoded))
	for _, item := range decoded {
		byName[item["name"].(string)] = item
	}
	for _, field := range []string{"args", "env"} {
		value, ok := byName["empty-stdio"][field].([]any)
		if !ok || len(value) != 0 {
			t.Fatalf("empty stdio %s = %#v, want []", field, byName["empty-stdio"][field])
		}
	}
	if _, ok := byName["empty-stdio"]["headers"]; ok {
		t.Fatalf("stdio wire unexpectedly includes headers: %#v", byName["empty-stdio"])
	}
	headers, ok := byName["empty-http"]["headers"].([]any)
	if !ok || len(headers) != 0 {
		t.Fatalf("empty HTTP headers = %#v, want []", byName["empty-http"]["headers"])
	}
	if _, ok := byName["empty-http"]["args"]; ok {
		t.Fatalf("HTTP wire unexpectedly includes args: %#v", byName["empty-http"])
	}
	if args := byName["full-stdio"]["args"].([]any); len(args) != 1 || args[0] != "serve" {
		t.Fatalf("full stdio args = %#v", args)
	}
	if env := byName["full-stdio"]["env"].([]any); len(env) != 1 {
		t.Fatalf("full stdio env = %#v", env)
	}
	if headers := byName["full-http"]["headers"].([]any); len(headers) != 1 {
		t.Fatalf("full HTTP headers = %#v", headers)
	}
}

func TestRequiresHTTPMCP(t *testing.T) {
	if requiresHTTPMCP([]acpMCPServer{{Name: "local", Command: "/usr/bin/server"}}) {
		t.Fatal("stdio MCP unexpectedly requires HTTP capability")
	}
	if !requiresHTTPMCP([]acpMCPServer{{Type: "http", Name: "remote", URL: "https://example.com/mcp"}}) {
		t.Fatal("HTTP MCP capability was not required")
	}
}

func TestPreparePromptInputStagesAndCleansAuthorizedFile(t *testing.T) {
	workspace := t.TempDir()
	file, err := contract.NewOutputFile(context.Background(), contract.OutputFileMetadata{
		Name: "notes.txt", MediaType: "text/plain", SizeBytes: int64(len("hello from file")),
	}, strings.NewReader("hello from file"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Cleanup()

	blocks, cleanup, turnErr := preparePromptInput(context.Background(), "turn/file", workspace, []contract.InputPart{
		{Kind: contract.InputPartText, Text: "read the attachment"},
		{Kind: contract.InputPartFile, File: &contract.InputFile{ID: file.ID, Resolved: file}},
	})
	if turnErr != nil {
		t.Fatalf("preparePromptInput() error = %v", turnErr)
	}
	if len(blocks) != 2 || blocks[0].Text != "read the attachment" || !strings.Contains(blocks[1].Text, "notes.txt") {
		t.Fatalf("prompt blocks = %#v", blocks)
	}
	matches, err := filepath.Glob(filepath.Join(workspace, ".csgclaw", "engine-inputs", "*", "*notes.txt"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("staged input matches = %q, error = %v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil || string(data) != "hello from file" {
		t.Fatalf("staged input = %q, error = %v", data, err)
	}
	if !strings.Contains(blocks[1].Text, matches[0]) {
		t.Fatalf("file prompt %q does not contain staged path %q", blocks[1].Text, matches[0])
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(matches[0])); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("turn input directory survived cleanup: %v", err)
	}
}

func TestPreparePromptInputRejectsUnresolvedFile(t *testing.T) {
	_, cleanup, turnErr := preparePromptInput(context.Background(), "turn-1", t.TempDir(), []contract.InputPart{{
		Kind: contract.InputPartFile, File: &contract.InputFile{ID: "missing"},
	}})
	cleanup()
	if turnErr == nil || turnErr.Code != contract.ErrorFileUnavailable {
		t.Fatalf("preparePromptInput() error = %+v", turnErr)
	}
}

func TestProcessMetadataUpdatesAreSerialized(t *testing.T) {
	root := t.TempDir()
	proc := &process{
		root:  root,
		meta:  runtimeMetadata{RuntimeID: "rt-agent-test", Sessions: map[string]string{}},
		ready: map[string]bool{},
	}
	const sessionCount = 40
	created := make([]chan struct{}, sessionCount)
	for index := range created {
		created[index] = make(chan struct{})
	}
	errorsCh := make(chan error, sessionCount*2)
	var wait sync.WaitGroup
	for index := range sessionCount {
		index := index
		key := fmt.Sprintf("conversation-%02d", index)
		sessionID := fmt.Sprintf("session-%02d", index)
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsCh <- proc.updateMetadata(func(meta *runtimeMetadata) {
				meta.Sessions[key] = sessionID
				proc.ready[sessionID] = true
			})
			close(created[index])
		}()
		if index%2 == 0 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-created[index]
				errorsCh <- proc.updateMetadata(func(meta *runtimeMetadata) {
					delete(meta.Sessions, key)
					delete(proc.ready, sessionID)
				})
			}()
		}
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("updateMetadata() error = %v", err)
		}
	}
	persisted, err := readMetadata(root)
	if err != nil {
		t.Fatalf("readMetadata() error = %v", err)
	}
	if len(persisted.Sessions) != sessionCount/2 {
		t.Fatalf("persisted sessions = %d, want %d", len(persisted.Sessions), sessionCount/2)
	}
	for index := range sessionCount {
		key := fmt.Sprintf("conversation-%02d", index)
		_, found := persisted.Sessions[key]
		if found != (index%2 == 1) {
			t.Fatalf("persisted session %q found = %v", key, found)
		}
	}
}

func TestDeletePreservesDSHRecreateState(t *testing.T) {
	agentHome := t.TempDir()
	root := filepath.Join(agentHome, hostStateDirName)
	preservedFiles := map[string]string{
		filepath.Join(root, workspaceDirName, "project.txt"):      "workspace state\n",
		filepath.Join(root, homeDirName, "agents", "session.log"): "session state\n",
		filepath.Join(root, homeDirName, "skills", "custom.md"):   "skill state\n",
	}
	ephemeralFiles := map[string]string{
		filepath.Join(root, homeDirName, settingsFileName): "generated settings\n",
		filepath.Join(root, patchFileName):                 "generated patch\n",
		filepath.Join(root, stderrFileName):                "runtime log\n",
	}
	writeFiles := func(files map[string]string) {
		for path, content := range files {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeFiles(preservedFiles)
	writeFiles(ephemeralFiles)
	if err := writeMetadata(root, runtimeMetadata{
		RuntimeID: "rt-agent-test", AgentID: "agent-test", Sessions: map[string]string{"room-1": "session-1"},
	}); err != nil {
		t.Fatal(err)
	}

	rt := New(Dependencies{})
	rt.roots["rt-agent-test"] = root
	if err := rt.Delete(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-test"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	for path, want := range preservedFiles {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("preserved file %s = %q, %v; want %q", path, data, err, want)
		}
	}
	meta, err := readMetadata(root)
	if err != nil || meta.Sessions["room-1"] != "session-1" {
		t.Fatalf("preserved runtime metadata = %+v, %v", meta, err)
	}
	for path := range ephemeralFiles {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ephemeral runtime file %s survived Delete(): %v", path, err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(agentHome, ".csgclaw-dsh-state-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary state preservation directories = %q, error = %v", matches, err)
	}
}

func TestHandleNotificationPreservesACPToolDetails(t *testing.T) {
	var events []contract.TurnEvent
	turn := &activeTurn{
		request: contract.TurnRequest{ID: "turn-1"},
		sink: contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
			events = append(events, event)
			return nil
		}),
	}
	proc := &process{active: map[string]*activeTurn{"session-1": turn}}
	runtime := &Runtime{}

	emitDSHUpdate(t, runtime, proc, map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    "call-1",
		"title":         "bash",
		"kind":          "other",
		"status":        "in_progress",
		"rawInput": map[string]any{
			"command":     "ls -la",
			"description": "List workspace root contents",
		},
	})
	emitDSHUpdate(t, runtime, proc, map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    "call-1",
		"status":        "completed",
		"content": []any{
			map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "AGENTS.md\n"}},
		},
	})

	if len(events) != 2 {
		t.Fatalf("events = %+v, want tool start and update", events)
	}
	start, completed := events[0], events[1]
	if start.Kind != contract.TurnEventToolCallStart || start.Tool == nil ||
		start.Tool.Kind != "exec_command" || start.Tool.Title != "bash" ||
		!strings.Contains(start.Tool.InputSummary, `"command":"ls -la"`) {
		t.Fatalf("tool start = %+v", start)
	}
	if completed.Kind != contract.TurnEventToolCallUpdate || completed.Tool == nil ||
		completed.Tool.ID != "call-1" || completed.Tool.Kind != "exec_command" ||
		completed.Tool.Title != "bash" || completed.Tool.Status != "completed" ||
		!strings.Contains(completed.Tool.InputSummary, `"command":"ls -la"`) ||
		!strings.Contains(completed.Tool.OutputSummary, "AGENTS.md") {
		t.Fatalf("tool completion = %+v", completed)
	}
	payload, ok := completed.Tool.Payload.(map[string]any)
	if !ok || payload["rawInput"] == nil || payload["content"] == nil {
		t.Fatalf("merged payload = %#v", completed.Tool.Payload)
	}
}

func TestNormalizedDSHToolKindUsesToolNameForGenericACPKind(t *testing.T) {
	tests := []struct {
		reported string
		title    string
		want     string
	}{
		{reported: "other", title: "bash", want: "exec_command"},
		{reported: "other", title: "glob", want: "glob"},
		{reported: "other", title: "Read File", want: "read_file"},
		{reported: "read", title: "anything", want: "read"},
	}
	for _, test := range tests {
		if got := normalizedDSHToolKind(test.reported, test.title); got != test.want {
			t.Errorf("normalizedDSHToolKind(%q, %q) = %q, want %q", test.reported, test.title, got, test.want)
		}
	}
}

func TestMergeDSHToolActivityKeepsExplicitKindAndRedactsSummaries(t *testing.T) {
	started := mergeDSHToolActivity(contract.ToolActivity{}, map[string]any{
		"toolCallId": "call-1",
		"title":      "Read File",
		"kind":       "read",
		"rawInput":   map[string]any{"authorization": "Bearer abc.def", "api_key": "sk-secret"},
	})
	completed := mergeDSHToolActivity(started, map[string]any{
		"toolCallId": "call-1",
		"status":     "completed",
	})
	if completed.Kind != "read" {
		t.Fatalf("completed kind = %q, want explicit start kind", completed.Kind)
	}
	if strings.Contains(completed.InputSummary, "abc.def") || strings.Contains(completed.InputSummary, "sk-secret") {
		t.Fatalf("input summary leaked credentials: %s", completed.InputSummary)
	}
}

func emitDSHUpdate(t *testing.T, runtime *Runtime, proc *process, update map[string]any) {
	t.Helper()
	params, err := json.Marshal(map[string]any{"sessionId": "session-1", "update": update})
	if err != nil {
		t.Fatal(err)
	}
	runtime.handleNotification(proc, notification{Method: "session/update", Params: params})
}

func TestPermissionRequestCanBeResolved(t *testing.T) {
	var output bytes.Buffer
	client := &acpClient{writer: &output}
	var event contract.TurnEvent
	proc := &process{
		client: client,
		meta:   runtimeMetadata{RuntimeID: "rt-agent-test"},
		active: map[string]*activeTurn{
			"session-1": {
				request: contract.TurnRequest{ID: "turn-1", ConversationKey: "room-1", Interaction: contract.InteractionResolve},
				sink: contract.EventSinkFunc(func(_ context.Context, emitted contract.TurnEvent) error {
					event = emitted
					return nil
				}),
			},
		},
	}
	rt := &Runtime{pending: map[string]*pendingPermission{}}
	params, err := json.Marshal(map[string]any{
		"sessionId": "session-1",
		"toolCall":  map[string]any{"toolCallId": "tool-1", "title": "Run command", "kind": "execute"},
		"options": []map[string]any{
			{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
			{"optionId": "reject", "name": "Reject", "kind": "reject_once"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.handleServerRequest(proc, serverRequest{ID: json.RawMessage("91"), Method: "session/request_permission", Params: params})
	if event.Kind != contract.TurnEventInteractionRequest || event.Interaction == nil || event.Interaction.Kind != contract.InteractionPermission {
		t.Fatalf("permission event = %+v", event)
	}
	snapshot, ok := event.Interaction.Payload.(activity.ActivitySnapshot)
	if !ok {
		t.Fatalf("permission payload type = %T, want activity.ActivitySnapshot", event.Interaction.Payload)
	}
	if snapshot.ID != event.Interaction.ID || snapshot.Kind != activity.ActionKindPermission || snapshot.Status != activity.ActionStatusPending || snapshot.Title != "Run command" || snapshot.RequestedAt.IsZero() {
		t.Fatalf("permission snapshot = %+v", snapshot)
	}
	if len(snapshot.Options) != 2 || snapshot.Options[0].ID != "allow-once" || snapshot.Options[0].Label != "Allow once" || snapshot.Options[0].Kind != "allow_once" {
		t.Fatalf("permission options = %+v", snapshot.Options)
	}

	var coordinator interactionstate.Coordinator
	conversation := rt.Conversation("rt-agent-test")
	coordinator.Register("agent-test", "room-1", "turn-1", *event.Interaction, conversation.Resolve, nil)
	resolveErr := coordinator.Resolve(context.Background(), "agent-test", contract.InteractionResolution{
		ConversationKey: "room-1",
		InteractionID:   event.Interaction.ID,
		OptionID:        "allow-once",
	})
	if resolveErr != nil {
		t.Fatalf("channel-style Resolve() error = %v", resolveErr)
	}
	resolved, err := coordinator.Get("agent-test", "room-1", event.Interaction.ID)
	if err != nil {
		t.Fatalf("Get() resolved interaction error = %v", err)
	}
	resolvedSnapshot, ok := resolved.Payload.(activity.ActivitySnapshot)
	if !ok || resolvedSnapshot.Status != activity.ActionStatusAllowed || resolvedSnapshot.Decision == nil || resolvedSnapshot.Decision.OptionID != "allow-once" {
		t.Fatalf("resolved permission snapshot = %#v", resolved.Payload)
	}
	var response struct {
		ID     int64 `json:"id"`
		Result struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode ACP permission response: %v", err)
	}
	if response.ID != 91 || response.Result.Outcome.Outcome != "selected" || response.Result.Outcome.OptionID != "allow-once" {
		t.Fatalf("permission response = %+v", response)
	}
}

func TestPermissionRequestRejectPolicyCancelsTurn(t *testing.T) {
	var output bytes.Buffer
	client := &acpClient{writer: &output}
	turn := &activeTurn{request: contract.TurnRequest{
		ID: "turn-reject", ConversationKey: "room-reject", Interaction: contract.InteractionReject,
	}}
	proc := &process{client: client, active: map[string]*activeTurn{"session-1": turn}}
	rt := &Runtime{pending: map[string]*pendingPermission{}}
	params, err := json.Marshal(map[string]any{
		"sessionId": "session-1",
		"toolCall":  map[string]any{"toolCallId": "tool-1"},
		"options":   []map[string]any{{"optionId": "allow-once", "kind": "allow_once"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rt.handleServerRequest(proc, serverRequest{ID: json.RawMessage("92"), Method: "session/request_permission", Params: params})

	proc.mu.Lock()
	interactionErr := turn.interactionError
	proc.mu.Unlock()
	if interactionErr == nil || interactionErr.Code != contract.ErrorInteractionUnsupported {
		t.Fatalf("interaction error = %+v", interactionErr)
	}
	frames := strings.TrimSpace(output.String())
	if !strings.Contains(frames, `"outcome":"cancelled"`) || !strings.Contains(frames, `"method":"session/cancel"`) {
		t.Fatalf("ACP reject frames = %s", frames)
	}
}
