package dsh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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
	larkextension "csgclaw/internal/runtimeextension/larkcli"
)

func TestValidateConfigAcceptsCatalogProviderReferences(t *testing.T) {
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			return dshcli.Info{Path: "/test/dsh", Version: "0.1.5-rc.2"}, nil
		},
	})

	for _, test := range []struct {
		name     string
		provider string
		baseURL  string
		apiKey   string
	}{
		{name: "OpenCSG", provider: "csghub"},
		{name: "CSGHub Lite", provider: "csghub_lite", baseURL: "http://127.0.0.1:11435/v1", apiKey: "local"},
		{name: "Codex", provider: "codex"},
		{name: "Claude Code", provider: "claude_code"},
		{name: "OpenAI compatible", provider: "api", baseURL: "https://api.example/v1", apiKey: "secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{
				Profile: agentruntime.RuntimeProfileConfig{
					Provider: test.provider,
					BaseURL:  test.baseURL,
					APIKey:   test.apiKey,
					ModelID:  "test-model",
				},
			})
			if err != nil {
				t.Fatalf("ValidateConfig() error = %v", err)
			}
		})
	}
}

func TestValidateConfigRequiresModelID(t *testing.T) {
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			return dshcli.Info{Path: "/test/dsh", Version: "0.1.5-rc.2"}, nil
		},
	})

	err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{
		Profile: agentruntime.RuntimeProfileConfig{Provider: "csghub"},
	})
	if err == nil || !strings.Contains(err.Error(), "model ID") {
		t.Fatalf("ValidateConfig() error = %v, want missing model ID", err)
	}
}

func TestValidateConfigRejectsIncompleteOpenAICompatibleProvider(t *testing.T) {
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			return dshcli.Info{Path: "/test/dsh", Version: "0.1.5-rc.2"}, nil
		},
	})

	err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{
		Profile: agentruntime.RuntimeProfileConfig{Provider: "api", ModelID: "test-model"},
	})
	if err == nil || !strings.Contains(err.Error(), "requires API key and base URL") {
		t.Fatalf("ValidateConfig() error = %v, want incomplete OpenAI-compatible provider", err)
	}
}

func TestProvisionRequiresMaterializedExecutionProfile(t *testing.T) {
	rt := New(Dependencies{
		AgentHome: func(string) (string, error) { return t.TempDir(), nil },
	})

	for _, test := range []struct {
		name    string
		profile agentruntime.Profile
	}{
		{name: "missing bridge URL", profile: agentruntime.Profile{APIKey: "agent-token", ModelID: "test-model"}},
		{name: "missing Agent token", profile: agentruntime.Profile{BaseURL: "http://127.0.0.1:18080/api/v1/agents/u-test/llm", ModelID: "test-model"}},
		{name: "missing model", profile: agentruntime.Profile{BaseURL: "http://127.0.0.1:18080/api/v1/agents/u-test/llm", APIKey: "agent-token"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
				RuntimeID: "rt-u-test",
				AgentID:   "u-test",
				Profile:   test.profile,
			})
			if err == nil || !strings.Contains(err.Error(), "DSH runtime profile requires") {
				t.Fatalf("Provision() error = %v, want incomplete execution profile", err)
			}
		})
	}
}

func TestReconcileConfigUsesResolvedExecutionProfile(t *testing.T) {
	root := t.TempDir()
	profile := agentruntime.Profile{
		Provider: "csghub",
		BaseURL:  "http://127.0.0.1:18080/api/v1/agents/u-test/llm",
		APIKey:   "agent-token",
		ModelID:  "next-model",
	}
	rt := New(Dependencies{
		AgentHome: func(string) (string, error) { return filepath.Join(root, "agent"), nil },
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "u-test", Instructions: "Use the bridge.", Profile: profile}, nil
		},
	})
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-u-test",
		AgentID:   "u-test",
		Profile:   profile,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	err := rt.ReconcileConfig(context.Background(), agentruntime.Handle{RuntimeID: "rt-u-test"}, agentruntime.RuntimeConfigChange{
		Current: agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{
			Provider: "csghub",
			ModelID:  "next-model",
		}},
	})
	if err != nil {
		t.Fatalf("ReconcileConfig() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "agent", hostStateDirName, homeDirName, settingsFileName))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]struct {
		BaseURL string `json:"baseURL"`
		Models  []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got := settings["llm-deepseek"].BaseURL; got != profile.BaseURL {
		t.Fatalf("settings baseURL = %q, want resolved bridge %q", got, profile.BaseURL)
	}
	if got := settings["llm-deepseek"].Models[0].ID; got != profile.ModelID {
		t.Fatalf("settings model = %q, want %q", got, profile.ModelID)
	}
}

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
	profile := agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "secret-key", ModelID: "test-model", InputModalities: []string{"text", "image"}, ReasoningEffort: "auto"}
	ref := AgentRef{ID: "agent-test", RuntimeID: "rt-agent-test", Profile: profile}
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
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
		Models    []struct {
			InputModalities []string `json:"inputModalities"`
		} `json:"models"`
	}
	if err := json.Unmarshal(settings, &decodedSettings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got := decodedSettings["llm-deepseek"].APIKeyEnv; got != llmAPIKeyEnvName {
		t.Fatalf("llm-deepseek.apiKeyEnv = %q, want %q", got, llmAPIKeyEnvName)
	}
	if got := decodedSettings["llm-deepseek"].Models[0].InputModalities; len(got) != 2 || got[0] != "text" || got[1] != "image" {
		t.Fatalf("llm-deepseek model inputModalities = %v, want text and image", got)
	}
	legacySettings := []byte(`{"llm-deepseek":{"protocol":"chat-completions","apiKeyEnv":"DEEPSEEK_API_KEY"}}`)
	if err := os.WriteFile(filepath.Join(root, "agent", hostStateDirName, homeDirName, settingsFileName), legacySettings, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.New(context.Background(), agentruntime.Spec{RuntimeID: "rt-agent-test", AgentID: "agent-test", Profile: profile}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	proc, err := rt.process("rt-agent-test")
	if err != nil || !proc.imagePrompts {
		t.Fatalf("DSH prompt image capability = %v, error = %v", proc != nil && proc.imagePrompts, err)
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
	if len(events) != 5 || events[0].Kind != contract.TurnEventActivityUpdate || events[1].Kind != contract.TurnEventThoughtDelta ||
		events[2].Kind != contract.TurnEventToolCallStart || events[3].Kind != contract.TurnEventToolCallUpdate ||
		events[4].Kind != contract.TurnEventTextDelta {
		t.Fatalf("events = %+v", events)
	}
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
	}
}

func TestNewRecreatesRuntimeDirectoriesAfterDelete(t *testing.T) {
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
	profile := agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "secret-key", ModelID: "test-model"}
	ref := AgentRef{ID: "agent-test", RuntimeID: "rt-agent-test", Profile: profile}
	agentHome := filepath.Join(root, "agent")
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			return dshcli.Info{Path: launcher, Version: "0.1.5-rc.2"}, nil
		},
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) { return ref, nil },
		AgentHome:    func(string) (string, error) { return agentHome, nil },
	})
	t.Cleanup(func() { _ = rt.Close() })
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-agent-test", AgentID: "agent-test", AgentName: "test", Profile: profile,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	runtimeRoot := filepath.Join(agentHome, hostStateDirName)
	workspaceFile := filepath.Join(runtimeRoot, workspaceDirName, "project.txt")
	if err := os.WriteFile(workspaceFile, []byte("keep me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, workspaceDirName, "index.html"), []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.New(context.Background(), agentruntime.Spec{RuntimeID: "rt-agent-test", AgentID: "agent-test", Profile: profile}); err != nil {
		t.Fatalf("New() before recreate error = %v", err)
	}
	request := contract.TurnRequest{
		ID: "turn-before-recreate", ConversationKey: "room-1", Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "before recreate"}}, Interaction: contract.InteractionResolve,
	}
	if result := rt.Conversation("rt-agent-test").Run(context.Background(), request, nil); result.Status != contract.TurnSucceeded {
		t.Fatalf("Run() before recreate = %+v", result)
	}
	durableSessionFile := filepath.Join(runtimeRoot, homeDirName, sessionsDirName, "--workspace--", "session-1", "v3.jsonl")
	if err := os.MkdirAll(filepath.Dir(durableSessionFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(durableSessionFile, []byte("session state\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSH_TEST_REQUIRE_RESUME_FILE", durableSessionFile)
	if err := rt.Delete(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-test"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if data, err := os.ReadFile(workspaceFile); err != nil || string(data) != "keep me\n" {
		t.Fatalf("workspace after Delete() = %q, %v; want preserved", data, err)
	}
	if data, err := os.ReadFile(durableSessionFile); err != nil || string(data) != "session state\n" {
		t.Fatalf("DSH session after Delete() = %q, %v; want preserved", data, err)
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, homeDirName, settingsFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated DSH settings after Delete() error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, homeDirName, "skills", "agent-teams", "SKILL.md")); err != nil {
		t.Fatalf("DSH template skill after Delete() error = %v", err)
	}

	if _, err := rt.New(context.Background(), agentruntime.Spec{RuntimeID: "rt-agent-test", AgentID: "agent-test", Profile: profile}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(runtimeRoot, homeDirName, settingsFileName),
		filepath.Join(runtimeRoot, patchFileName),
		filepath.Join(runtimeRoot, runtimeFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("regenerated runtime file %s stat error = %v", path, err)
		}
	}
	request.ID = "turn-after-recreate"
	request.Input[0].Text = "after recreate"
	request.Continuation = contract.ContinuationRequireExisting
	if result := rt.Conversation("rt-agent-test").Run(context.Background(), request, nil); result.Status != contract.TurnSucceeded {
		t.Fatalf("Run() after recreate = %+v", result)
	}
}

func TestDSHLaunchArgsEnablePresentOverlay(t *testing.T) {
	root := filepath.Join("tmp", "agent", hostStateDirName)
	wantPatch := filepath.Join(root, patchFileName)
	args := dshLaunchArgs(root, true)
	if len(args) != 6 || args[0] != "--profile" || args[1] != "acp" || args[2] != "--patch" || args[3] != filepath.Join(root, contextPatchFileName) || args[5] != wantPatch {
		t.Fatalf("dshLaunchArgs() = %q", args)
	}
	fallback := dshLaunchArgs(root, false)
	if len(fallback) != 4 || fallback[0] != "--profile" || fallback[1] != "acp" || fallback[3] != filepath.Join(root, contextPatchFileName) {
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
	t.Setenv(permissionModeEnvName, PermissionModeDangerFullAccess)
	t.Setenv("DEEPSEEK_API_KEY", "ambient-search-key")
	t.Setenv("DEEPSEEK_BASE_URL", "https://ambient-chat.example/v1")
	t.Setenv("DEEPSEEK_SEARCH_BASE_URL", "https://search.example/anthropic/v1")
	t.Setenv("LARK_CHANNEL", "1")
	t.Setenv("LARK_CHANNEL_PROFILE", "host-profile")
	t.Setenv("lark_channel_home", "/ambient/lark-home")

	tests := []struct {
		name          string
		profileEnv    map[string]string
		wantSearchKey string
	}{
		{name: "inherits search credential from server environment", wantSearchKey: "ambient-search-key"},
		{
			name:          "agent environment overrides search credential",
			profileEnv:    map[string]string{"DEEPSEEK_API_KEY": "agent-search-key", "lark_channel_config": "/profile/lark.json"},
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
			}, "/isolated/dsh-home", PermissionModeReadOnly))

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
			if _, found := env["LARK_CHANNEL"]; found {
				t.Fatal("DSH inherited the host LARK_CHANNEL")
			}
			if _, found := env["LARK_CHANNEL_PROFILE"]; found {
				t.Fatal("DSH inherited the host LARK_CHANNEL_PROFILE")
			}
			for key := range env {
				switch agentruntime.CanonicalEnvironmentKey(key) {
				case "LARK_CHANNEL_HOME", "LARK_CHANNEL_CONFIG":
					t.Fatalf("DSH inherited protected environment key %q", key)
				}
			}
			if got := env[permissionModeEnvName]; got != PermissionModeReadOnly {
				t.Fatalf("%s = %q, want runtime option to override ambient value", permissionModeEnvName, got)
			}
		})
	}
}

func TestValidateConfigAcceptsBridgeManagedProfile(t *testing.T) {
	resolved := false
	rt := New(Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			resolved = true
			return dshcli.Info{Path: "/opt/dsh", Version: "0.1.5-rc.2"}, nil
		},
	})
	err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{
		Profile: agentruntime.RuntimeProfileConfig{Provider: "csghub", ModelID: "test-model"},
	})
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if !resolved {
		t.Fatal("ResolveBinary() was not called")
	}
}

func TestProvisionRejectsIncompleteRuntimeProfile(t *testing.T) {
	rt := New(Dependencies{
		AgentHome: func(string) (string, error) {
			t.Fatal("AgentHome() called for an incomplete Runtime profile")
			return "", nil
		},
	})
	err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-agent-test",
		AgentID:   "agent-test",
		Profile:   agentruntime.Profile{ModelID: "test-model"},
	})
	if err == nil || !strings.Contains(err.Error(), "DSH runtime profile requires") {
		t.Fatalf("Provision() error = %v", err)
	}
}

func TestStartRejectsIncompleteResolvedRuntimeProfile(t *testing.T) {
	rt := New(Dependencies{
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "agent-test", RuntimeID: "rt-agent-test", Profile: agentruntime.Profile{ModelID: "test-model"}}, nil
		},
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			t.Fatal("ResolveBinary() called for an incomplete Runtime profile")
			return dshcli.Info{}, nil
		},
		AgentHome: func(string) (string, error) {
			t.Fatal("AgentHome() called for an incomplete Runtime profile")
			return "", nil
		},
	})
	_, err := rt.Start(context.Background(), agentruntime.Handle{RuntimeID: "rt-agent-test"})
	if err == nil || !strings.Contains(err.Error(), "DSH runtime profile requires") {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestDecodeRuntimeOptionsPermissionMode(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		want    string
		wantErr bool
	}{
		{name: "empty defaults to workspace", want: PermissionModeWorkspaceWrite},
		{name: "read only", raw: map[string]any{PermissionModeOptionKey: " read-only "}, want: PermissionModeReadOnly},
		{name: "full access", raw: map[string]any{PermissionModeOptionKey: PermissionModeDangerFullAccess}, want: PermissionModeDangerFullAccess},
		{name: "legacy executable ignored", raw: map[string]any{"executable_path": "/opt/dsh"}, want: PermissionModeWorkspaceWrite},
		{name: "invalid", raw: map[string]any{PermissionModeOptionKey: "auto"}, wantErr: true},
		{name: "non string", raw: map[string]any{PermissionModeOptionKey: true}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, err := DecodeRuntimeOptions(test.raw)
			if (err != nil) != test.wantErr {
				t.Fatalf("DecodeRuntimeOptions() error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && opts.PermissionMode != test.want {
				t.Fatalf("PermissionMode = %q, want %q", opts.PermissionMode, test.want)
			}
		})
	}
}

func TestPermissionModeChangeDropsPersistedSessionMappings(t *testing.T) {
	persisted := runtimeMetadata{
		PermissionMode: PermissionModeWorkspaceWrite,
		Sessions:       map[string]string{"room-1": "session-1"},
	}

	unchanged, changed := sessionsForPermissionMode(persisted, PermissionModeWorkspaceWrite)
	if changed || unchanged["room-1"] != "session-1" {
		t.Fatalf("unchanged permission sessions = %v, changed = %v", unchanged, changed)
	}
	unchanged["room-1"] = "replacement"
	if persisted.Sessions["room-1"] != "session-1" {
		t.Fatal("sessionsForPermissionMode() aliased persisted session metadata")
	}

	reset, changed := sessionsForPermissionMode(persisted, PermissionModeReadOnly)
	if !changed || len(reset) != 0 {
		t.Fatalf("changed permission sessions = %v, changed = %v; want empty reset", reset, changed)
	}

	legacy := runtimeMetadata{Sessions: map[string]string{"room-legacy": "session-legacy"}}
	legacySessions, changed := sessionsForPermissionMode(legacy, PermissionModeWorkspaceWrite)
	if changed || legacySessions["room-legacy"] != "session-legacy" {
		t.Fatalf("legacy default permission sessions = %v, changed = %v", legacySessions, changed)
	}
}

func TestRuntimeOptionsSchemaExposesPermissionModes(t *testing.T) {
	rt := New(Dependencies{})
	schemas := rt.RuntimeOptionsSchema()
	var schema *agentruntime.RuntimeOptionSchema
	for i := range schemas {
		if schemas[i].Path == "executable_path" || schemas[i].Path == "auto_compact" {
			t.Fatalf("runtime schema still exposes hidden option %q", schemas[i].Path)
		}
		if schemas[i].Path == PermissionModeOptionKey {
			schema = &schemas[i]
			break
		}
	}
	if schema == nil || schema.DefaultValue != PermissionModeWorkspaceWrite || len(schema.Choices) != 2 {
		t.Fatalf("permission mode schema = %#v", schema)
	}
	if schema.Choices[0].Value != PermissionModeWorkspaceWrite || schema.Choices[1].Value != PermissionModeReadOnly {
		t.Fatalf("permission mode choices = %#v", schema.Choices)
	}
	if len(schema.Options) != 2 || schema.Options[0] != PermissionModeWorkspaceWrite || schema.Options[1] != PermissionModeReadOnly {
		t.Fatalf("permission mode options = %#v", schema.Options)
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
	if recordPath := os.Getenv("DSH_TEST_ENV_RECORD"); recordPath != "" {
		record, _ := json.Marshal(map[string]string{
			"LARKSUITE_CLI_CONFIG_DIR": os.Getenv("LARKSUITE_CLI_CONFIG_DIR"),
			"LARK_CHANNEL":             os.Getenv("LARK_CHANNEL"),
			"LARK_CHANNEL_HOME":        os.Getenv("LARK_CHANNEL_HOME"),
			"LARK_CHANNEL_PROFILE":     os.Getenv("LARK_CHANNEL_PROFILE"),
			"LARK_CHANNEL_CONFIG":      os.Getenv("LARK_CHANNEL_CONFIG"),
		})
		if err := os.WriteFile(recordPath, record, 0o600); err != nil {
			os.Exit(8)
		}
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
			respond(map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{
				"promptCapabilities": map[string]any{"image": true},
				"mcpCapabilities":    map[string]any{"http": true},
			}})
		case "session/new":
			respond(map[string]any{"sessionId": "session-1", "configOptions": helperConfigOptions()})
		case "session/resume":
			if required := os.Getenv("DSH_TEST_REQUIRE_RESUME_FILE"); required != "" {
				if _, err := os.Stat(required); err != nil {
					_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "error": map[string]any{"code": -32602, "message": "persisted session file is unavailable"}})
					continue
				}
			}
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
			if os.Getenv("DSH_TEST_HANG_PROMPT") == "1" {
				continue
			}
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
	}, false)
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
	}}, false)
	cleanup()
	if turnErr == nil || turnErr.Code != contract.ErrorFileUnavailable {
		t.Fatalf("preparePromptInput() error = %+v", turnErr)
	}
}

func TestPreparePromptInputSendsSupportedImageInline(t *testing.T) {
	payload := []byte("not-a-real-png-but-an-immutable-image-payload")
	file, err := contract.NewOutputFile(context.Background(), contract.OutputFileMetadata{
		Name: "screen.png", MediaType: "image/png", SizeBytes: int64(len(payload)),
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Cleanup()

	blocks, cleanup, turnErr := preparePromptInput(context.Background(), "turn-image", "", []contract.InputPart{
		{Kind: contract.InputPartText, Text: "what is in this image?"},
		{Kind: contract.InputPartFile, File: &contract.InputFile{ID: file.ID, Resolved: file}},
	}, true)
	defer cleanup()
	if turnErr != nil {
		t.Fatalf("preparePromptInput() error = %v", turnErr)
	}
	if len(blocks) != 2 || blocks[1].Type != "image" || blocks[1].MIMEType != "image/png" {
		t.Fatalf("prompt blocks = %#v", blocks)
	}
	decoded, err := base64.StdEncoding.DecodeString(blocks[1].Data)
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Fatalf("inline image = %q, error = %v", decoded, err)
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
		filepath.Join(root, workspaceDirName, "project.txt"):                                        "workspace state\n",
		filepath.Join(root, homeDirName, "agents", "subagent.json"):                                 "agent state\n",
		filepath.Join(root, homeDirName, sessionsDirName, "--workspace--", "session-1", "v3.jsonl"): "session state\n",
		filepath.Join(root, homeDirName, "skills", "custom.md"):                                     "skill state\n",
		filepath.Join(root, homeDirName, "runtime-extensions", "feishu-lark-cli", "active.json"):    "extension state\n",
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

func TestPermissionRequestSkipUserInputAllowsManagedLarkCLISetup(t *testing.T) {
	trustedExecutable, environment := writeTestLarkCLIExecutable(t)
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	turn := &activeTurn{
		request: contract.TurnRequest{ID: "turn-lark", Interaction: contract.InteractionSkipUserInput},
	}
	proc := &process{
		client:               &acpClient{writer: &output},
		workspace:            workspace,
		environment:          environment,
		active:               map[string]*activeTurn{"session-1": turn},
		extensionDigests:     map[string]string{larkextension.Name: "digest-1"},
		extensionExecutables: map[string]string{larkextension.Name: trustedExecutable},
	}
	runtime := &Runtime{}
	emitDSHUpdate(t, runtime, proc, map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    "tool-1",
		"title":         "bash",
		"kind":          "other",
		"rawInput": map[string]any{
			"command": "lark-cli auth login --no-wait --json --recommend",
			"workdir": "project",
		},
	})
	params, err := json.Marshal(map[string]any{
		"sessionId": "session-1",
		"toolCall":  map[string]any{"toolCallId": "tool-1", "title": "Run command", "kind": "execute"},
		"options": []map[string]any{
			{"optionId": "allow-once", "kind": "allow_once"},
			{"optionId": "reject", "kind": "reject_once"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime.handleServerRequest(proc, serverRequest{ID: json.RawMessage("93"), Method: "session/request_permission", Params: params})

	if got := output.String(); !strings.Contains(got, `"outcome":"selected"`) || !strings.Contains(got, `"optionId":"allow-once"`) {
		t.Fatalf("ACP permission response = %s", got)
	}
}

func TestUnattendedLarkCLIPermissionRejectsExecutableFromDifferentWorkdir(t *testing.T) {
	workspace := t.TempDir()
	project := filepath.Join(workspace, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "lark-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	trustedExecutable := filepath.Join(workspace, name)
	if err := os.WriteFile(trustedExecutable, []byte("trusted"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, name), []byte("untrusted"), 0o755); err != nil {
		t.Fatal(err)
	}
	turn := &activeTurn{
		request: contract.TurnRequest{ID: "turn-lark", Interaction: contract.InteractionSkipUserInput},
		tools: map[string]contract.ToolActivity{
			"tool-1": {
				Kind: "exec_command",
				Payload: map[string]any{"rawInput": map[string]any{
					"command": "." + string(filepath.Separator) + name + " config strict-mode off",
					"workdir": "project",
				}},
			},
		},
	}
	proc := &process{
		workspace:            workspace,
		environment:          []string{"PATH=" + workspace},
		active:               map[string]*activeTurn{"session-1": turn},
		extensionDigests:     map[string]string{larkextension.Name: "digest-1"},
		extensionExecutables: map[string]string{larkextension.Name: trustedExecutable},
	}
	request := permissionRequestParams{SessionID: "session-1", Options: []permissionOption{{OptionID: "allow-once", Kind: "allow_once"}}}
	request.ToolCall.ToolCallID = "tool-1"

	if optionID, ok := unattendedLarkCLIPermission(proc, turn, request); ok || optionID != "" {
		t.Fatalf("unattendedLarkCLIPermission() = %q, %v", optionID, ok)
	}
}

func TestUnattendedLarkCLIPermissionSnapshotsConcurrentToolUpdates(t *testing.T) {
	trustedExecutable, environment := writeTestLarkCLIExecutable(t)
	turn := &activeTurn{request: contract.TurnRequest{ID: "turn-lark", Interaction: contract.InteractionSkipUserInput}}
	proc := &process{
		workspace:            t.TempDir(),
		environment:          environment,
		active:               map[string]*activeTurn{"session-1": turn},
		extensionDigests:     map[string]string{larkextension.Name: "digest-1"},
		extensionExecutables: map[string]string{larkextension.Name: trustedExecutable},
	}
	updateParams, err := json.Marshal(map[string]any{
		"sessionId": "session-1",
		"update": map[string]any{
			"sessionUpdate": "tool_call",
			"toolCallId":    "tool-1",
			"title":         "bash",
			"kind":          "other",
			"rawInput": map[string]any{
				"command": "lark-cli config strict-mode off",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := permissionRequestParams{SessionID: "session-1", Options: []permissionOption{{OptionID: "allow-once", Kind: "allow_once"}}}
	request.ToolCall.ToolCallID = "tool-1"
	rt := &Runtime{}
	rt.handleNotification(proc, notification{Method: "session/update", Params: updateParams})

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for range 1_000 {
			rt.handleNotification(proc, notification{Method: "session/update", Params: updateParams})
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for range 1_000 {
			_, _ = unattendedLarkCLIPermission(proc, turn, request)
		}
	}()
	close(start)
	workers.Wait()

	if optionID, ok := unattendedLarkCLIPermission(proc, turn, request); !ok || optionID != "allow-once" {
		t.Fatalf("unattendedLarkCLIPermission() = %q, %v", optionID, ok)
	}
}

func TestPermissionRequestSkipUserInputRejectsUnmanagedCommands(t *testing.T) {
	trustedExecutable, environment := writeTestLarkCLIExecutable(t)
	workspace := t.TempDir()
	untrustedExecutable := filepath.Join(workspace, "lark-cli")
	if runtime.GOOS == "windows" {
		untrustedExecutable += ".exe"
	}
	if err := os.WriteFile(untrustedExecutable, []byte("untrusted"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		command     string
		environment []string
		extensions  map[string]string
		executables map[string]string
	}{
		{name: "different command", command: "lark-cli docs +fetch --doc token", extensions: map[string]string{larkextension.Name: "digest-1"}, executables: map[string]string{larkextension.Name: trustedExecutable}},
		{name: "extension not loaded", command: "lark-cli config strict-mode off", extensions: map[string]string{}},
		{name: "workspace executable", command: "." + string(filepath.Separator) + filepath.Base(untrustedExecutable) + " config strict-mode off", extensions: map[string]string{larkextension.Name: "digest-1"}, executables: map[string]string{larkextension.Name: trustedExecutable}},
		{name: "absolute untrusted executable", command: untrustedExecutable + " auth login --no-wait --json --recommend", extensions: map[string]string{larkextension.Name: "digest-1"}, executables: map[string]string{larkextension.Name: trustedExecutable}},
		{name: "PATH shadow executable", command: "lark-cli config default-as auto", environment: []string{"PATH=" + workspace + string(os.PathListSeparator) + filepath.Dir(trustedExecutable)}, extensions: map[string]string{larkextension.Name: "digest-1"}, executables: map[string]string{larkextension.Name: trustedExecutable}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			turn := &activeTurn{
				request: contract.TurnRequest{ID: "turn-lark", Interaction: contract.InteractionSkipUserInput},
				tools: map[string]contract.ToolActivity{
					"tool-1": {Kind: "exec_command", Payload: map[string]any{"rawInput": map[string]any{"command": test.command}}},
				},
			}
			processEnvironment := test.environment
			if processEnvironment == nil {
				processEnvironment = environment
			}
			proc := &process{
				client:               &acpClient{writer: &output},
				workspace:            workspace,
				environment:          processEnvironment,
				active:               map[string]*activeTurn{"session-1": turn},
				extensionDigests:     test.extensions,
				extensionExecutables: test.executables,
			}
			params, err := json.Marshal(map[string]any{
				"sessionId": "session-1",
				"toolCall":  map[string]any{"toolCallId": "tool-1"},
				"options": []map[string]any{
					{"optionId": "allow-once", "kind": "allow_once"},
					{"optionId": "reject", "kind": "reject_once"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			(&Runtime{}).handleServerRequest(proc, serverRequest{ID: json.RawMessage("94"), Method: "session/request_permission", Params: params})

			got := output.String()
			if !strings.Contains(got, `"outcome":"selected"`) || !strings.Contains(got, `"optionId":"reject"`) {
				t.Fatalf("ACP permission response = %s", got)
			}
		})
	}
}

func TestProcessExecutableMatchesManagedExecutable(t *testing.T) {
	trustedExecutable, environment := writeTestLarkCLIExecutable(t)
	workspace := t.TempDir()
	proc := &process{workspace: workspace, environment: environment}
	if !processExecutableMatches(proc, workspace, "lark-cli", trustedExecutable) {
		t.Fatal("bare lark-cli did not resolve to the managed executable")
	}
	if !processExecutableMatches(proc, workspace, trustedExecutable, trustedExecutable) {
		t.Fatal("absolute managed lark-cli path did not match")
	}
	untrusted := filepath.Join(workspace, filepath.Base(trustedExecutable))
	if err := os.WriteFile(untrusted, []byte("untrusted"), 0o755); err != nil {
		t.Fatal(err)
	}
	if processExecutableMatches(proc, workspace, "."+string(filepath.Separator)+filepath.Base(untrusted), trustedExecutable) {
		t.Fatal("workspace executable matched the managed lark-cli executable")
	}
	proc.environment = []string{"PATH=" + workspace + string(os.PathListSeparator) + filepath.Dir(trustedExecutable)}
	if processExecutableMatches(proc, workspace, "lark-cli", trustedExecutable) {
		t.Fatal("PATH shadow executable matched the managed lark-cli executable")
	}
	project := filepath.Join(workspace, "project")
	relativeBin := filepath.Join(project, "bin")
	if err := os.MkdirAll(relativeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	relativeTrusted := filepath.Join(relativeBin, filepath.Base(trustedExecutable))
	if err := os.WriteFile(relativeTrusted, []byte("trusted"), 0o755); err != nil {
		t.Fatal(err)
	}
	proc.environment = []string{"PATH=bin"}
	if !processExecutableMatches(proc, project, "lark-cli", relativeTrusted) {
		t.Fatal("relative PATH did not resolve from the command workdir")
	}
}

func TestResolveProcessWorkdir(t *testing.T) {
	workspace := t.TempDir()
	absolute := t.TempDir()
	proc := &process{workspace: workspace}
	for _, test := range []struct {
		name     string
		rawInput map[string]any
		want     string
		wantErr  bool
	}{
		{name: "default", rawInput: map[string]any{}, want: workspace},
		{name: "relative", rawInput: map[string]any{"workdir": "project"}, want: filepath.Join(workspace, "project")},
		{name: "absolute", rawInput: map[string]any{"workdir": absolute}, want: absolute},
		{name: "invalid", rawInput: map[string]any{"workdir": 42}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveProcessWorkdir(proc, test.rawInput)
			if test.wantErr {
				if err == nil {
					t.Fatalf("resolveProcessWorkdir() = %q, nil", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("resolveProcessWorkdir() = %q, %v, want %q", got, err, test.want)
			}
		})
	}
}

func writeTestLarkCLIExecutable(t *testing.T) (string, []string) {
	t.Helper()
	directory := t.TempDir()
	name := "lark-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("trusted"), 0o755); err != nil {
		t.Fatal(err)
	}
	environment := []string{"PATH=" + directory}
	if runtime.GOOS == "windows" {
		environment = append(environment, "PATHEXT=.EXE;.CMD")
	}
	return path, environment
}
