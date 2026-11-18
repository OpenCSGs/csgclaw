package cliproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/modelcap"

	"github.com/gin-gonic/gin"
	cliproxyapi "github.com/router-for-me/CLIProxyAPI/v8/sdk/api"
	sdkhandlers "github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

func startModelTestService(t *testing.T, ctx context.Context, svc *Service, transport *modelTransportFixture) *coreauth.Manager {
	t.Helper()
	ready := make(chan *coreauth.Manager, 1)
	if err := svc.ensureStarted(ctx, cliproxyapi.WithRouterConfigurator(func(_ *gin.Engine, handler *sdkhandlers.BaseAPIHandler, _ *sdkconfig.Config) {
		manager := handler.AuthManager
		if transport != nil {
			manager.SetRoundTripperProvider(transport)
		}
		ready <- manager
	})); err != nil {
		t.Fatal(err)
	}
	manager := <-ready
	t.Cleanup(func() {
		if err := svc.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
		for _, auth := range manager.List() {
			cliproxysdk.GlobalModelRegistry().UnregisterClient(auth.ID)
		}
	})
	return manager
}

type modelTransportFixture struct {
	mu              sync.Mutex
	provider, model string
}

func (f *modelTransportFixture) RoundTripperFor(*coreauth.Auth) http.RoundTripper { return f }

func (f *modelTransportFixture) RoundTrip(request *http.Request) (*http.Response, error) {
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.model = payload.Model
	f.mu.Unlock()
	body := `data: {"type":"response.completed","response":{"id":"resp-latest","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"
	if f.provider == "claude" {
		body = `event: message_start
data: {"type":"message_start","message":{"id":"msg-latest","type":"message","role":"assistant","model":"` + payload.Model + `","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {"type":"message_stop"}

`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

// Use the real embedded HTTP server, file auth loader, registry, scheduler and
// request handlers and provider executors. Mock only the upstream HTTP transport.
func TestEmbeddedCLIProxyLatestModels(t *testing.T) {
	for _, test := range []struct {
		provider, model, plan string
		unavailable           []string
	}{
		{ProviderCodex, "gpt-6.2-astra", "pro", nil},
		{ProviderCodex, "gpt-6.2-sol", "plus", nil},
		{ProviderCodex, "gpt-6.2-astra", "team", nil},
		{ProviderCodex, "gpt-6.2-sol", "prolite", nil},
		{ProviderCodex, "gpt-6-luna", "free", []string{"gpt-6.2-astra", "gpt-6.2-sol", "gpt-6.1-sol"}},
		{"claude", "claude-opus-5-5", "", nil},
		{"claude", "claude-sonnet-5-5", "", nil},
		{"claude", "claude-fable-5-2", "", nil},
		{"claude", "claude-haiku-5-5", "", nil},
	} {
		t.Run(strings.TrimSuffix(test.model+"/"+test.plan, "/"), func(t *testing.T) {
			clearStandardProxyEnv(t)
			t.Setenv(systemProxyURLEnv, "")
			t.Setenv("HOME", t.TempDir())
			t.Setenv(disableKeychainEnv, "true")
			t.Setenv(configDirEnv, t.TempDir())
			authDir := t.TempDir()
			t.Setenv(authDirEnv, authDir)
			metadata := map[string]any{
				"type": test.provider, "access_token": "fixture-token", "refresh_token": "fixture-refresh",
				"expired": "2030-01-01T00:00:00Z", "plan_type": test.plan,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if _, err := saveMetadataAuth(ctx, authDir, test.provider, "fixture.json", metadata); err != nil {
				t.Fatal(err)
			}
			svc := &Service{client: &http.Client{Timeout: 5 * time.Second}}
			transport := &modelTransportFixture{provider: test.provider}
			manager := startModelTestService(t, ctx, svc, transport)
			models, err := svc.ListModels(ctx, test.provider)
			if err != nil || !containsString(models, test.model) {
				t.Fatalf("model discovery missing %s: models=%v err=%v", test.model, models, err)
			}
			for _, unavailable := range test.unavailable {
				if slices.Contains(models, unavailable) {
					t.Errorf("%s plan exposes %s", test.plan, unavailable)
				}
			}
			if strings.HasPrefix(test.model, "gpt-6.2-") {
				window := modelcap.Resolve(test.provider, "", test.model, modelcap.Metadata{}, modelcap.Metadata{}).ContextWindow
				for _, info := range cliproxysdk.GlobalModelRegistry().GetAvailableModelsByProvider(test.provider) {
					if info.ID == test.model && int64(info.ContextLength) != window {
						t.Fatalf("proxy context metadata differs from Codex profile: proxy=%d profile=%d", info.ContextLength, window)
					}
				}
			}
			baseURL, err := svc.BaseURL(ctx)
			if err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/responses", strings.NewReader(`{"model":"`+test.model+`","input":"hello","stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+LocalAPIKey)
			request.Header.Set("Content-Type", "application/json")
			response, err := svc.client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "latest") {
				t.Fatalf("model request failed: status=%d body=%s err=%v", response.StatusCode, body, err)
			}
			transport.mu.Lock()
			gotModel := transport.model
			transport.mu.Unlock()
			if gotModel != test.model {
				t.Fatalf("routed model = %s, want %s", gotModel, test.model)
			}
			// The model must survive a credential reload and disappear on removal.
			metadata["email"] = "refreshed@example.test"
			if _, err := saveMetadataAuth(ctx, authDir, test.provider, "fixture.json", metadata); err != nil {
				t.Fatal(err)
			}
			waitForModelAuth(t, ctx, manager, test.provider, test.model, "refreshed@example.test", true)
			// An atomic auth replacement can emit a removal event on macOS.
			// Wait out the SDK's one-second debounce before deleting the file.
			time.Sleep(time.Second)
			if err := os.Remove(filepath.Join(authDir, "fixture.json")); err != nil {
				t.Fatal(err)
			}
			waitForModelAuth(t, ctx, manager, test.provider, test.model, "", false)
		})
	}
}

func waitForModelAuth(t *testing.T, ctx context.Context, manager *coreauth.Manager, provider, model, email string, present bool) {
	t.Helper()
	registry := cliproxysdk.GlobalModelRegistry()
	for ctx.Err() == nil {
		activeFixture := false
		for _, auth := range manager.List() {
			if auth.FileName == "fixture.json" && !auth.Disabled {
				activeFixture = true
			}
			if auth.Disabled || auth.FileName != "fixture.json" || (email != "" && auth.Metadata["email"] != email) {
				continue
			}
			if present && registry.ClientSupportsModel(auth.ID, model) {
				return
			}
		}
		if !present && !activeFixture && !registry.ClientSupportsModel("fixture.json", model) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	auth, exists := manager.GetByID("fixture.json")
	t.Fatalf("auth/model state did not converge: model=%s email=%s present=%v auth exists=%v disabled=%v model registered=%v models=%v", model, email, present, exists, auth != nil && auth.Disabled, registry.ClientSupportsModel("fixture.json", model), registeredModels(provider))
}
