package onboard

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/bot"
	"csgclaw/internal/config"
)

func TestRunInteractiveDefaultUsesCSGHubLiteModels(t *testing.T) {
	restore := stubBootstrap(t, func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		if got, want := cfg.Models.Default, "csghub-lite.Qwen/Qwen3-0.6B-GGUF"; got != want {
			t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
		}
		if got, want := cfg.Sandbox.Provider, config.DefaultSandboxProvider; got != want {
			t.Fatalf("cfg.Sandbox.Provider = %q, want %q", got, want)
		}
		if got, want := cfg.Sandbox.HomeDirName, config.DefaultSandboxHomeDirName; got != want {
			t.Fatalf("cfg.Sandbox.HomeDirName = %q, want %q", got, want)
		}
		provider, ok := cfg.Models.Providers["csghub-lite"]
		if !ok {
			t.Fatal(`cfg.Models.Providers["csghub-lite"] missing`)
		}
		if provider.BaseURL == "" || provider.APIKey != "local" {
			t.Fatalf("provider = %#v, want csghub-lite base URL and local API key", provider)
		}
		if got, want := strings.Join(provider.Models, ","), "Qwen/Qwen3-0.6B-GGUF,Qwen/Qwen3-1.7B-GGUF"; got != want {
			t.Fatalf("provider models = %q, want %q", got, want)
		}
		return bot.Bot{}, nil
	})
	defer restore()

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer local" {
			t.Fatalf("Authorization = %q, want Bearer local", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"Qwen/Qwen3-0.6B-GGUF"},{"id":"Qwen/Qwen3-1.7B-GGUF"}]}`))
	}))
	defer modelServer.Close()

	restoreInteractive := enableInteractiveTest(t, modelServer.URL+"/v1")
	defer restoreInteractive()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	run := interactiveTestContext("\n\n")
	if err := NewCmd().Run(context.Background(), run, nil, command.GlobalOptions{Config: configPath, Output: "table"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := run.Stdout.(*fakeTerminalBuffer).String()
	for _, want := range []string{
		"CSGClaw model provider setup",
		"Options:",
		"csghub-lite - local default at " + modelServer.URL + "/v1",
		"csghub-lite run <model>",
		"CSGHub-lite base URL [" + modelServer.URL + "/v1]",
		"Checking CSGHub-lite models",
		"Using CSGHub-lite provider",
		"Default model: csghub-lite.Qwen/Qwen3-0.6B-GGUF",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("prompt output missing %q:\n%s", want, output)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"`,
		`[models.providers.csghub-lite]`,
		`api_key = "local"`,
		`models = ["Qwen/Qwen3-0.6B-GGUF", "Qwen/Qwen3-1.7B-GGUF"]`,
		`[sandbox]`,
		fmt.Sprintf(`provider = %q`, config.DefaultSandboxProvider),
		fmt.Sprintf(`home_dir_name = %q`, config.DefaultSandboxHomeDirName),
		fmt.Sprintf(`boxlite_cli_path = %q`, config.DefaultBoxLiteCLIPath),
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("saved config missing %q:\n%s", want, content)
		}
	}
}

func TestSandboxServiceOptionsSupportsConfiguredProvider(t *testing.T) {
	opts, err := sandboxServiceOptions(config.SandboxConfig{
		Provider:          config.BoxLiteCLIProvider,
		HomeDirName:       "sandbox-home",
		BoxLiteCLIPath:    "/opt/boxlite/bin/boxlite",
	}, config.BootstrapConfig{DebianRegistries: []string{"registry.a"}})
	if err != nil {
		t.Fatalf("sandboxServiceOptions() error = %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d, want 2", len(opts))
	}
}

func TestRunInteractiveCSGHubLiteUsesSpecifiedBaseURL(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"Qwen/Qwen3-0.6B-GGUF"}]}`))
	}))
	defer modelServer.Close()

	restore := stubBootstrap(t, func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		provider := cfg.Models.Providers["csghub-lite"]
		if got, want := provider.BaseURL, modelServer.URL+"/v1"; got != want {
			t.Fatalf("provider.BaseURL = %q, want %q", got, want)
		}
		if got, want := cfg.Models.Default, "csghub-lite.Qwen/Qwen3-0.6B-GGUF"; got != want {
			t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
		}
		return bot.Bot{}, nil
	})
	defer restore()
	restoreInteractive := enableInteractiveTest(t, "http://unused.test/v1")
	defer restoreInteractive()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	run := interactiveTestContext("1\n" + modelServer.URL + "/v1\n")
	if err := NewCmd().Run(context.Background(), run, nil, command.GlobalOptions{Config: configPath, Output: "table"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := run.Stdout.(*fakeTerminalBuffer).String()
	for _, want := range []string{
		"CSGHub-lite base URL [http://unused.test/v1]",
		"Checking CSGHub-lite models at " + modelServer.URL + "/v1/models",
		"Base URL: " + modelServer.URL + "/v1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("prompt output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRequiresLLMFlagsForFirstTimeSetup(t *testing.T) {
	origCreateManager := CreateManagerBot
	origEnsureIMBootstrapState := EnsureIMBootstrapState
	t.Cleanup(func() {
		CreateManagerBot = origCreateManager
		EnsureIMBootstrapState = origEnsureIMBootstrapState
	})

	CreateManagerBot = func(context.Context, string, string, config.Config, bool) (bot.Bot, error) {
		t.Fatal("bot manager create should not run when config is incomplete")
		return bot.Bot{}, nil
	}
	EnsureIMBootstrapState = func(string) error {
		t.Fatal("IM bootstrap should not run when config is incomplete")
		return nil
	}

	run := testContext()
	err := NewCmd().Run(context.Background(), run, nil, command.GlobalOptions{Config: filepath.Join(t.TempDir(), "config.toml")})
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--base-url") || !strings.Contains(err.Error(), "--api-key") || !strings.Contains(err.Error(), "--models") {
		t.Fatalf("Run() error = %q, want all required LLM flags", err)
	}
}

func TestRunInteractiveExistingConfigKeepsCurrentProviderByDefault(t *testing.T) {
	restore := stubBootstrap(t, func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		if got, want := cfg.Models.Default, "default.gpt-test"; got != want {
			t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
		}
		if _, ok := cfg.Models.Providers["csghub-lite"]; ok {
			t.Fatal("csghub-lite provider should not be added when keeping existing config")
		}
		return bot.Bot{}, nil
	})
	defer restore()
	restoreInteractive := enableInteractiveTest(t, "http://unused.test/v1")
	defer restoreInteractive()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := writeConfig(configPath, `# Generated by csgclaw onboard.

[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = ""
access_token = "your_access_token"

[bootstrap]
manager_image = "img"

[models]
default = "default.gpt-test"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret"
models = ["gpt-test"]
`); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}

	run := interactiveTestContext("\n")
	if err := NewCmd().Run(context.Background(), run, nil, command.GlobalOptions{Config: configPath, Output: "table"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := run.Stdout.(*fakeTerminalBuffer).String()
	for _, want := range []string{
		"Current model configuration:",
		"Provider: default",
		"Default model: default.gpt-test",
		"keep current - leave the existing model provider unchanged",
		"Keeping current model provider",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("prompt output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "secret") {
		t.Fatalf("prompt output should not contain API key:\n%s", output)
	}
}

func TestRunInteractiveCustomProvider(t *testing.T) {
	restore := stubBootstrap(t, func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		if got, want := cfg.Models.Default, "default.gpt-custom"; got != want {
			t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
		}
		provider := cfg.Models.Providers["default"]
		if provider.BaseURL != "http://llm.test/v1" || provider.APIKey != "secret" {
			t.Fatalf("provider = %#v, want custom base URL/API key", provider)
		}
		if got, want := strings.Join(provider.Models, ","), "gpt-custom,gpt-mini"; got != want {
			t.Fatalf("provider models = %q, want %q", got, want)
		}
		return bot.Bot{}, nil
	})
	defer restore()
	restoreInteractive := enableInteractiveTest(t, "http://unused.test/v1")
	defer restoreInteractive()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	run := interactiveTestContext("2\nhttp://llm.test/v1\nsecret\ngpt-custom,gpt-mini\n")
	if err := NewCmd().Run(context.Background(), run, nil, command.GlobalOptions{Config: configPath, Output: "table"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := run.Stdout.(*fakeTerminalBuffer).String()
	for _, want := range []string{
		"Custom OpenAI-compatible provider",
		"Base URL example: https://api.openai.com/v1",
		"Using custom provider",
		"Default model: default.gpt-custom",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("prompt output missing %q:\n%s", want, output)
		}
	}
}

func TestRunInteractiveCustomProviderKeepsExistingSecretWithoutEchoing(t *testing.T) {
	restore := stubBootstrap(t, func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		provider := cfg.Models.Providers["default"]
		if provider.BaseURL != "http://llm.test/v1" || provider.APIKey != "secret-token" {
			t.Fatalf("provider = %#v, want existing base URL/API key", provider)
		}
		if got, want := cfg.Models.Default, "default.gpt-new"; got != want {
			t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
		}
		return bot.Bot{}, nil
	})
	defer restore()
	restoreInteractive := enableInteractiveTest(t, "http://unused.test/v1")
	defer restoreInteractive()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := writeConfig(configPath, `# Generated by csgclaw onboard.

[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = ""
access_token = "your_access_token"

[bootstrap]
manager_image = "img"

[models]
default = "default.gpt-old"

[models.providers.default]
base_url = "http://llm.test/v1"
api_key = "secret-token"
models = ["gpt-old"]
`); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}

	run := interactiveTestContext("3\n\n\ngpt-new\n")
	if err := NewCmd().Run(context.Background(), run, nil, command.GlobalOptions{Config: configPath, Output: "table"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := run.Stdout.(*fakeTerminalBuffer).String()
	if strings.Contains(output, "secret-token") {
		t.Fatalf("prompt output should not contain existing API key:\n%s", output)
	}
	if !strings.Contains(output, "API key [keep existing]:") {
		t.Fatalf("prompt output missing keep-existing API key prompt:\n%s", output)
	}
}

func TestRunCSGHubLiteProviderUnavailableReturnsStartHint(t *testing.T) {
	restore := stubBootstrap(t, func(context.Context, string, string, config.Config, bool) (bot.Bot, error) {
		t.Fatal("bot manager create should not run when csghub-lite is unavailable")
		return bot.Bot{}, nil
	})
	defer restore()

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer modelServer.Close()

	run := testContext()
	err := NewCmd().Run(context.Background(), run, []string{
		"--provider", "csghub-lite",
		"--base-url", modelServer.URL + "/v1",
	}, command.GlobalOptions{Config: filepath.Join(t.TempDir(), "config.toml")})
	if err == nil {
		t.Fatal("Run() error = nil, want csghub-lite availability error")
	}
	if !strings.Contains(err.Error(), "csghub-lite is not reachable") || !strings.Contains(err.Error(), "csghub-lite run <model>") {
		t.Fatalf("Run() error = %q, want start hint", err)
	}
}

func TestRunReusesExistingLLMConfig(t *testing.T) {
	origCreateManager := CreateManagerBot
	origEnsureIMBootstrapState := EnsureIMBootstrapState
	t.Cleanup(func() {
		CreateManagerBot = origCreateManager
		EnsureIMBootstrapState = origEnsureIMBootstrapState
	})

	callCount := 0
	CreateManagerBot = func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		callCount++
		if got, want := cfg.Models.Default, "default.gpt-test"; got != want {
			t.Fatalf("cfg.Models.Default = %q, want %q", got, want)
		}
		provider, ok := cfg.Models.Providers["default"]
		if !ok {
			t.Fatal(`cfg.Models.Providers["default"] missing`)
		}
		if provider.BaseURL != "http://llm.test" || provider.APIKey != "secret" {
			t.Fatalf("provider config = %#v, want preserved base_url/api_key", provider)
		}
		if got, want := strings.Join(provider.Models, ","), "gpt-test,gpt-test-mini"; got != want {
			t.Fatalf("provider models = %q, want %q", got, want)
		}
		if got, want := provider.ReasoningEffort, "medium"; got != want {
			t.Fatalf("provider reasoning_effort = %q, want %q", got, want)
		}
		if cfg.Model.BaseURL != "http://llm.test" || cfg.Model.APIKey != "secret" || cfg.Model.ModelID != "gpt-test" || cfg.Model.ReasoningEffort != "medium" {
			t.Fatalf("model config = %#v, want resolved default values", cfg.Model)
		}
		return bot.Bot{}, nil
	}
	EnsureIMBootstrapState = func(string) error { return nil }

	configPath := filepath.Join(t.TempDir(), "config.toml")
	run := testContext()
	cmd := NewCmd()

	args := []string{
		"--base-url", "http://llm.test",
		"--api-key", "secret",
		"--models", "gpt-test,gpt-test-mini",
		"--reasoning-effort", "medium",
	}
	if err := cmd.Run(context.Background(), run, args, command.GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("initial Run() error = %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`[models]`,
		`default = "default.gpt-test"`,
		`[models.providers.default]`,
		`models = ["gpt-test", "gpt-test-mini"]`,
		`reasoning_effort = "medium"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("saved config missing %q:\n%s", want, content)
		}
	}

	if err := cmd.Run(context.Background(), run, nil, command.GlobalOptions{Config: configPath}); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if callCount != 2 {
		t.Fatalf("agent bootstrap call count = %d, want 2", callCount)
	}
}

func TestRunDebianRegistriesFlagPersistsToConfig(t *testing.T) {
	restore := stubBootstrap(t, func(_ context.Context, _, _ string, cfg config.Config, _ bool) (bot.Bot, error) {
		if got, want := strings.Join(cfg.Bootstrap.DebianRegistries, ","), "registry.a,docker.io"; got != want {
			t.Fatalf("bootstrap cfg.Bootstrap.DebianRegistries = %q, want %q", got, want)
		}
		return bot.Bot{}, nil
	})
	defer restore()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	run := testContext()
	err := NewCmd().Run(context.Background(), run, []string{
		"--base-url", "http://llm.test",
		"--api-key", "secret",
		"--models", "gpt-test",
		"--debian-registries", "registry.a,docker.io",
	}, command.GlobalOptions{Config: configPath})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `debian_registries = ["registry.a", "docker.io"]`) {
		t.Fatalf("saved config should persist onboard --debian-registries:\n%s", string(data))
	}
}

func testContext() *command.Context {
	return &command.Context{
		Program: "csgclaw",
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}

func interactiveTestContext(input string) *command.Context {
	return &command.Context{
		Program: "csgclaw",
		Stdin:   fakeTerminalInput{Reader: strings.NewReader(input)},
		Stdout:  &fakeTerminalBuffer{},
		Stderr:  &bytes.Buffer{},
	}
}

type fakeTerminalInput struct {
	*strings.Reader
}

func (fakeTerminalInput) Fd() uintptr {
	return 1
}

type fakeTerminalBuffer struct {
	bytes.Buffer
}

func (*fakeTerminalBuffer) Fd() uintptr {
	return 1
}

func enableInteractiveTest(t *testing.T, baseURL string) func() {
	t.Helper()
	origTerminal := isTerminalFD
	origBaseURL := defaultCSGHubLiteBaseURL
	isTerminalFD = func(int) bool { return true }
	defaultCSGHubLiteBaseURL = baseURL
	return func() {
		isTerminalFD = origTerminal
		defaultCSGHubLiteBaseURL = origBaseURL
	}
}

func stubBootstrap(t *testing.T, create func(context.Context, string, string, config.Config, bool) (bot.Bot, error)) func() {
	t.Helper()
	origCreateManager := CreateManagerBot
	origEnsureIMBootstrapState := EnsureIMBootstrapState
	CreateManagerBot = create
	EnsureIMBootstrapState = func(string) error { return nil }
	return func() {
		CreateManagerBot = origCreateManager
		EnsureIMBootstrapState = origEnsureIMBootstrapState
	}
}

func writeConfig(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
