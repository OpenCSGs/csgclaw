package cliproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

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
	}{
		{ProviderCodex, "gpt-6.1-sol", "pro"},
		{"claude", "claude-opus-5-5", ""},
		{"claude", "claude-sonnet-5-5", ""},
	} {
		t.Run(test.model, func(t *testing.T) {
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
			if err := svc.EnsureStarted(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Shutdown(context.Background()) })
			models, err := svc.ListModels(ctx, test.provider)
			if err != nil || !containsString(models, test.model) {
				t.Fatalf("model discovery missing %s: models=%v err=%v", test.model, models, err)
			}
			transport := &modelTransportFixture{provider: test.provider}
			svc.catalog.manager.SetRoundTripperProvider(transport)
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
			// A credential file update rebuilds the SDK catalog. The supplement
			// must survive reload and disappear when the credential is removed.
			metadata["email"] = "refreshed@example.test"
			if _, err := saveMetadataAuth(ctx, authDir, test.provider, "fixture.json", metadata); err != nil {
				t.Fatal(err)
			}
			waitForModelAuth(t, ctx, svc, test.model, "refreshed@example.test", true)
			if err := os.Remove(filepath.Join(authDir, "fixture.json")); err != nil {
				t.Fatal(err)
			}
			waitForModelAuth(t, ctx, svc, test.model, "", false)
		})
	}
}

func waitForModelAuth(t *testing.T, ctx context.Context, svc *Service, model, email string, present bool) {
	t.Helper()
	registry := cliproxysdk.GlobalModelRegistry()
	for ctx.Err() == nil {
		for _, auth := range svc.catalog.manager.List() {
			if auth.FileName != "fixture.json" || (email != "" && auth.Metadata["email"] != email) {
				continue
			}
			if present && registry.ClientSupportsModel(auth.ID, model) {
				return
			}
		}
		if !present && len(svc.catalog.manager.List()) == 0 {
			if !containsString(registeredModels("codex"), model) && !containsString(registeredModels("claude"), model) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("auth/model state did not converge: model=%s email=%s present=%v", model, email, present)
}

func TestModelCatalogPreservesSubscriptionFiltering(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{ID: "catalog-free-test", Provider: ProviderCodex, Status: coreauth.StatusActive}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	registry := cliproxysdk.GlobalModelRegistry()
	registry.RegisterClient(auth.ID, auth.Provider, []*cliproxysdk.ModelInfo{{ID: "gpt-6-luna"}})
	defer registry.UnregisterClient(auth.ID)
	catalog := &modelCatalog{manager: manager}
	catalog.reconcile(context.Background())
	if registry.ClientSupportsModel(auth.ID, "gpt-6.1-sol") {
		t.Fatal("supplement exposed a paid model to the free model catalog")
	}
	// A delayed registration callback cannot resurrect an unregistered client.
	registry.UnregisterClient(auth.ID)
	catalog.OnModelsRegistered(context.Background(), auth.Provider, auth.ID, []*cliproxysdk.ModelInfo{{ID: "gpt-6-sol"}})
	if len(registry.GetModelsForClient(auth.ID)) != 0 {
		t.Fatal("delayed callback restored a removed model client")
	}
}
