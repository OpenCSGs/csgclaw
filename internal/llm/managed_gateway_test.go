package llm

import (
	"context"
	"csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestCSGHubGatewayTracingPreservesKeyRefreshAndRedirectGuard(t *testing.T) {
	var lookups, redirects atomic.Int32
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		if r.Header.Get("Authorization") != "Bearer hub-token" {
			t.Error("incorrect Hub credential")
		}
		fmt.Fprint(w, `{"data":{"token":"gk_refreshed"}}`)
	}))
	defer hub.Close()
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirects.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()
	var redirect atomic.Bool
	var mu sync.Mutex
	var spans []trace.SpanContext
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		mu.Lock()
		spans = append(spans, trace.SpanContextFromContext(ctx))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer gk_refreshed" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
			return
		}
		fmt.Fprint(w, `{"id":"reply","output":[]}`)
	}))
	defer gateway.Close()
	_, port, err := net.SplitHostPort(gateway.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	gatewayAddress := net.JoinHostPort("aigateway.opencsg.com", port)
	gatewayURL := "http://" + gatewayAddress + "/v1"

	service := NewCSGHubService(config.ModelConfig{}, nil, CSGHubGatewayConfig{
		GatewayURL: gatewayURL, HubURL: hub.URL,
		UserToken: "hub-token", UserName: "owner", UserUUID: "owner-uuid",
	})
	dialer := &net.Dialer{}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		switch address {
		case gatewayAddress:
			address = gateway.Listener.Addr().String()
		case destination.Listener.Addr().String():
		default:
			return nil, fmt.Errorf("unexpected test connection to %s", address)
		}
		return dialer.DialContext(ctx, network, address)
	}}
	defer transport.CloseIdleConnections()
	service.gatewayClient.Transport = otelhttp.NewTransport(transport,
		otelhttp.WithTracerProvider(service.gatewayTracer), otelhttp.WithPropagators(propagation.TraceContext{}))
	ctx := WithTaskProfile(t.Context(), agents.AgentProfile{
		Provider: agents.ProviderAPI, ModelProviderID: "remote", BaseURL: gatewayURL,
		APIKey: "gk_stale", ModelID: "fixture",
	})
	for _, status := range []int{http.StatusOK, http.StatusTemporaryRedirect} {
		redirect.Store(status == http.StatusTemporaryRedirect)
		response, err := service.Responses(ctx, "manager", []byte(`{"input":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("Responses status = %d, want %d", response.StatusCode, status)
		}
	}
	if lookups.Load() != 1 || redirects.Load() != 0 {
		t.Fatalf("Hub lookups = %d, followed redirects = %d", lookups.Load(), redirects.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(spans) != 3 || !spans[0].IsValid() || !spans[1].IsValid() || !spans[2].IsValid() ||
		spans[0].TraceID() != spans[1].TraceID() || spans[0].SpanID() == spans[1].SpanID() {
		t.Fatalf("rotated-key attempts did not preserve tracing: %+v", spans)
	}
}

func TestCSGHubGatewayRefreshesRotatedKey(t *testing.T) {
	var generation atomic.Int32
	var lookups atomic.Int32
	var hubRejectsToken atomic.Bool
	var requests atomic.Int32
	generation.Store(1)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		if r.URL.Path != "/api/v1/namespaces/owner-uuid/apikeys/builtin" ||
			r.URL.Query().Get("current_user") != "owner" || r.Header.Get("Authorization") != "Bearer hub-token" {
			t.Error("key lookup did not use the sandbox's owner and user token")
		}
		if hubRejectsToken.Load() {
			http.Error(w, "token revoked", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `{"data":{"token":"gk_fixture_%d"}}`, generation.Load())
	}))
	defer hub.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/responses" && r.URL.Path != "/v1/chat/completions" {
			t.Error("unexpected gateway URL")
		}
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer gk_fixture_%d", generation.Load()) {
			http.Error(w, "key rejected", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"id":"reply","output":[]}`)
	}))
	defer gateway.Close()
	cfg := CSGHubGatewayConfig{GatewayURL: gateway.URL + "/v1", HubURL: hub.URL,
		UserToken: "hub-token", UserName: "owner", UserUUID: "owner-uuid"}
	service := NewCSGHubService(config.ModelConfig{}, nil, cfg)
	ctx := WithTaskProfile(t.Context(), agents.AgentProfile{Provider: agents.ProviderAPI,
		ModelProviderID: "remote", BaseURL: cfg.GatewayURL, APIKey: "gk_fixture_1", ModelID: "fixture"})

	responses := func(want int) {
		response, err := service.Responses(ctx, "manager", []byte(`{"input":"hello"}`))
		if err != nil {
			t.Error(err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Errorf("responses status = %d, want %d", response.StatusCode, want)
		}
	}
	hubRejectsToken.Store(true)
	responses(http.StatusOK)
	if lookups.Load() != 0 || requests.Load() != 1 {
		t.Fatal("first inference did not use the configured key without contacting the Hub")
	}
	hubRejectsToken.Store(false)

	generation.Store(2)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() { responses(http.StatusOK) })
	}
	group.Wait()
	if lookups.Load() != 1 {
		t.Fatalf("concurrent requests made %d total lookups, want 1", lookups.Load())
	}

	generation.Store(3)
	hubRejectsToken.Store(true)
	responses(http.StatusUnauthorized)
	hubRejectsToken.Store(false)
	chat, err := service.ChatCompletionsStream(ctx, "manager", []byte(`{"messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer chat.Body.Close()
	if chat.StatusCode != http.StatusOK || lookups.Load() != 3 {
		t.Fatalf("chat failed to refresh using the recovered Hub access: status %d, lookups %d", chat.StatusCode, lookups.Load())
	}
	responses(http.StatusOK)
	if lookups.Load() != 3 {
		t.Fatal("next request did not reuse the refreshed key")
	}
}

func TestCSGHubGatewaySharesRefreshResultAndAllowsCancellation(t *testing.T) {
	for _, lookupFails := range []bool{false, true} {
		name := "unchanged key"
		if lookupFails {
			name = "lookup error"
		}
		t.Run(name, func(t *testing.T) {
			const rejected = "gk_rejected"
			var lookups atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if lookups.Add(1) == 1 {
					close(started)
				}
				<-release
				if lookupFails {
					http.Error(w, "Hub unavailable", http.StatusServiceUnavailable)
					return
				}
				fmt.Fprintf(w, `{"data":{"token":%q}}`, rejected)
			}))
			defer hub.Close()
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			gateway := &managedGateway{config: CSGHubGatewayConfig{HubURL: hub.URL,
				UserToken: "hub-token", UserName: "owner", UserUUID: "owner-uuid"}}

			type result struct {
				key string
				err error
			}
			refresh := func(ctx context.Context) <-chan result {
				done := make(chan result, 1)
				go func() {
					key, err := gateway.key(ctx, hub.Client(), rejected)
					done <- result{key, err}
				}()
				return done
			}
			leader, cancel := context.WithCancel(t.Context())
			defer cancel()
			leaderResult := refresh(leader)
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("the Hub lookup did not start")
			}
			var waiters []<-chan result
			for range 6 {
				ctx := &gatewayRefreshWaitContext{Context: t.Context(), waiting: make(chan struct{})}
				waiters = append(waiters, refresh(ctx))
				select {
				case <-ctx.waiting:
				case <-time.After(2 * time.Second):
					t.Fatal("a waiter could not reach the shared result while the Hub was blocked")
				}
			}
			cancel()
			select {
			case got := <-leaderResult:
				if !errors.Is(got.err, context.Canceled) {
					t.Errorf("cancelled leader returned %v, want context.Canceled", got.err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("the cancelled caller did not exit while the Hub was blocked")
			}
			unblock()
			for _, waiter := range waiters {
				select {
				case got := <-waiter:
					if lookupFails && got.err == nil || !lookupFails && (got.err != nil || got.key != rejected) {
						t.Errorf("waiter returned %+v for lookupFails = %t", got, lookupFails)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("a waiter did not receive the shared result")
				}
			}
			if got := lookups.Load(); got != 1 {
				t.Errorf("Hub lookups = %d, want 1", got)
			}
		})
	}
}

type gatewayRefreshWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *gatewayRefreshWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestCSGHubGatewayUsesOnlyTheManagedRemoteProvider(t *testing.T) {
	var calls atomic.Int32
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"data":{"token":"gk_managed"}}`)
	}))
	defer hub.Close()
	static := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gk_custom" {
			t.Error("custom provider did not use its own credential")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer static.Close()
	service := NewCSGHubService(config.ModelConfig{}, nil, CSGHubGatewayConfig{
		GatewayURL: static.URL + "/v1", HubURL: hub.URL,
		UserToken: "hub-token", UserName: "owner", UserUUID: "owner-uuid",
	})
	for _, profile := range []agents.AgentProfile{
		{Provider: agents.ProviderAPI, ModelProviderID: "custom", BaseURL: static.URL + "/v1", APIKey: "gk_custom", ModelID: "fixture"},
		{Provider: agents.ProviderAPI, ModelProviderID: "remote", BaseURL: static.URL + "/other/v1", APIKey: "gk_custom", ModelID: "fixture"},
	} {
		ctx := WithTaskProfile(t.Context(), profile)
		response, err := service.Responses(ctx, "manager", []byte(`{"input":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("responses status = %d, want 401", response.StatusCode)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("static provider triggered a managed credential lookup")
	}
}
