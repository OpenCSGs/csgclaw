package apps

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type platformRoundTripper func(*http.Request) (*http.Response, error)

func (f platformRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenCSGLoginSourceUsesLatestMatchingCredential(t *testing.T) {
	var credentials atomic.Value
	credentials.Store(OpenCSGCredentials{BaseURL: "https://opencsg-stg.com", Token: "login-one"})
	service := newTestService(t, Options{OpenCSGCredentials: func(context.Context) (OpenCSGCredentials, error) { return credentials.Load().(OpenCSGCredentials), nil }})
	url := "https://app.public.opencsg-stg.com/mcp"
	config := normalizeConfig(Config{URL: url, AuthMode: "bearer", PlatformCredentialSource: "opencsg_login"}, "feishu")
	client, err := connectorHTTPClient(config, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	service.bindPlatformLogin(config, client)
	var got []string
	client.Transport.(*headerTransport).base = platformRoundTripper(func(r *http.Request) (*http.Response, error) {
		got = append(got, r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	send := func(wantErr bool) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{}`))
		res, err := client.Do(req)
		if res != nil {
			res.Body.Close()
		}
		if (err != nil) != wantErr {
			t.Fatalf("request error=%v", err)
		}
	}
	send(false)
	credentials.Store(OpenCSGCredentials{BaseURL: "https://opencsg-stg.com", Token: "login-two"})
	send(false)
	credentials.Store(OpenCSGCredentials{BaseURL: "https://opencsg.com", Token: "production-private"})
	send(true)
	credentials.Store(OpenCSGCredentials{})
	send(true)
	if len(got) != 2 || got[0] != "Bearer login-one" || got[1] != "Bearer login-two" {
		t.Fatal("stale/cross-environment credential sent")
	}
}

func TestOpenCSGLoginSourceRejectsUntrustedTargets(t *testing.T) {
	service := newTestService(t, Options{OpenCSGCredentials: func(context.Context) (OpenCSGCredentials, error) {
		return OpenCSGCredentials{BaseURL: "https://opencsg-stg.com", Token: "private"}, nil
	}})
	for _, url := range []string{"http://app.public.opencsg-stg.com/mcp", "https://evil.example/mcp", "https://app.public.opencsg-stg.com.evil.example/mcp", "https://app.public.opencsg.com/mcp", "https://app.public.opencsg-stg.com:8443/mcp"} {
		if token, err := service.openCSGToken(context.Background(), url); err == nil || token != "" {
			t.Fatalf("credential released to %s", url)
		}
	}
}

func TestExpiredPlatformTokenAndHTTPFailuresRemainActionable(t *testing.T) {
	expired := "x." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(-time.Hour).Unix()))) + ".x"
	service := newTestService(t, Options{})
	var called atomic.Int32
	original := http.DefaultTransport
	http.DefaultTransport = platformRoundTripper(func(*http.Request) (*http.Response, error) { called.Add(1); return nil, fmt.Errorf("unexpected") })
	t.Cleanup(func() { http.DefaultTransport = original })
	item, err := service.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: "Expired", Config: Config{URL: "https://example.com/mcp", AuthMode: "bearer"}, Credentials: Credentials{Token: expired}, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	if called.Load() != 0 || item.Status != "authorization_required" || item.LastErrorCode != "app_platform_token_expired" || item.LastErrorHTTPStatus != 401 {
		t.Fatalf("expired credential lost its cause: %+v", item)
	}
	if strings.Contains(item.LastError, expired) {
		t.Fatal("secret exposed")
	}
	for _, tc := range []struct {
		status     int
		body, code string
		auth       bool
	}{
		{401, "", "app_platform_unauthorized", true}, {403, "", "app_platform_access_denied", true}, {404, "", "app_mcp_not_found", false},
		{500, `{"msg":"endpoint of deploy service is empty"}`, "app_mcp_backend_unavailable", false}, {503, "private upstream text", "app_mcp_http_error", false},
	} {
		e := upstreamHTTPError(tc.status, []byte(tc.body), false)
		if e.Code != tc.code || e.HTTPStatus != tc.status || errors.Is(e, errAuthentication) != tc.auth || strings.Contains(e.Message, "private upstream text") {
			t.Fatal("HTTP error classification failed")
		}
	}
}

func TestOpenCSGReferenceDoesNotPersistOrReviveCredentials(t *testing.T) {
	service := newTestService(t, Options{OpenCSGCredentials: func(context.Context) (OpenCSGCredentials, error) {
		t.Fatal("manually disconnected instance reconnected")
		return OpenCSGCredentials{}, nil
	}})
	item, err := service.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: "Login", Config: Config{URL: "https://app.public.opencsg-stg.com/mcp", AuthMode: "bearer", PlatformCredentialSource: "opencsg_login"}, Credentials: Credentials{Token: "old-secret", Headers: map[string]string{"Authorization": "Bearer old-secret"}}})
	if err != nil {
		t.Fatal(err)
	}
	if item.CredentialsSet["token"] || item.CredentialsSet["headers.Authorization"] {
		t.Fatal("login reference retained static credential")
	}
	if _, err := service.Disconnect(context.Background(), "agent", item.InstallationID); err != nil {
		t.Fatal(err)
	}
	if err := service.RefreshPlatformCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOmittedPlatformSourceRestoresAsManual(t *testing.T) {
	upstream := upstreamServer(t)
	service := newTestService(t, Options{})
	item := addHTTP(t, service, "agent", "gitlab", "Existing", upstream.URL, "fixture-token")
	service.mu.Lock()
	e := service.entries[item.InstallationID]
	e.record.Config.PlatformCredentialSource = ""
	if err := service.persistLocked(item.InstallationID, &e.record); err != nil {
		t.Fatal(err)
	}
	service.mu.Unlock()
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := NewService(service.path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	if err := restored.RestoreAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := restored.Get(context.Background(), "agent", item.InstallationID)
	if err != nil || got.Status != "connected" {
		t.Fatal("omitted optional credential source blocked restoration")
	}
}

func TestHTTPMethodNegotiationDoesNotBreakAppConnection(t *testing.T) {
	upstream := mcp.NewServer(&mcp.Implementation{Name: "post-only", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodDelete {
			w.WriteHeader(405)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	service := newTestService(t, Options{})
	item := addHTTP(t, service, "agent", "gitlab", "POST only", server.URL, "fixture")
	if item.Status != "connected" {
		t.Fatalf("GET negotiation rejected connection: %s", item.LastError)
	}
	client := gatewayClient(t, service, "agent")
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read"), Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("POST-only service failed: %v", err)
	}
}

func TestPlatformDefaultAcrossApps(t *testing.T) {
	for _, appID := range []string{"gitlab", "feishu", "llm-wiki"} {
		for _, mode := range []string{"none", "bearer", "header"} {
			t.Run(appID+"/"+mode, func(t *testing.T) {
				config := normalizeConfig(Config{URL: "https://demo.public.opencsg-stg.com/mcp", AuthMode: mode, TokenHeader: "PRIVATE-TOKEN"}, appID)
				if config.PlatformCredentialSource != "opencsg_login" {
					t.Fatal("missing automatic platform login")
				}
				if err := validateConfig(config, appID); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	for _, endpoint := range []string{"http://localhost/mcp", "https://example.com/mcp", "https://demo.public.opencsg-stg.com.evil.example/mcp", "http://demo.public.opencsg-stg.com/mcp", "https://demo.public.opencsg-stg.com:8443/mcp"} {
		if got := normalizeConfig(Config{URL: endpoint}, "gitlab"); got.PlatformCredentialSource != "manual" {
			t.Fatalf("unexpected login default: %s", endpoint)
		}
	}
	for _, config := range []Config{
		{URL: "https://demo.public.opencsg-stg.com/mcp", PlatformCredentialSource: "manual"},
		{Command: "mcp-server"},
		{URL: "https://demo.public.opencsg-stg.com/mcp", AuthMode: "oauth2"},
	} {
		if normalizeConfig(config, "gitlab").PlatformCredentialSource != "manual" {
			t.Fatal("explicit/manual transport choice overridden")
		}
	}
	for _, mode := range []string{"", "bearer", "feishu"} {
		got := normalizeConfig(Config{URL: "https://demo.public.opencsg-stg.com/mcp", AuthMode: mode}, "feishu", Credentials{Token: "explicit-token"})
		if got.PlatformCredentialSource != "manual" {
			t.Fatal("explicit credential overridden")
		}
	}
}

func TestPlatformLoginPreservesBusinessHeader(t *testing.T) {
	service := newTestService(t, Options{OpenCSGCredentials: func(context.Context) (OpenCSGCredentials, error) {
		return OpenCSGCredentials{BaseURL: "https://opencsg-stg.com", Token: "platform-fixture"}, nil
	}})
	config := normalizeConfig(Config{URL: "https://demo.public.opencsg-stg.com/mcp", AuthMode: "header", TokenHeader: "PRIVATE-TOKEN"}, "gitlab", Credentials{Token: "business-fixture"})
	item, err := service.Create(context.Background(), "agent", CreateRequest{AppID: "gitlab", Name: "Business", Config: config, Credentials: Credentials{Token: "business-fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	if !item.CredentialsSet["token"] {
		t.Fatal("business token erased from stored installation")
	}
	credentials, err := service.resolve(context.Background(), "agent", config, Credentials{Token: "business-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := connectorHTTPClient(config, credentials)
	if err != nil {
		t.Fatal(err)
	}
	service.bindPlatformLogin(config, client)
	called := false
	client.Transport.(*headerTransport).base = platformRoundTripper(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.Header.Get("Authorization") != "Bearer platform-fixture" || r.Header.Get("PRIVATE-TOKEN") != "business-fixture" {
			t.Fatal("platform/business credential missing or overwritten")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	req, _ := http.NewRequest(http.MethodPost, config.URL, strings.NewReader(`{}`))
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !called {
		t.Fatal("request was not sent")
	}
	config.TokenHeader = "authorization"
	var conflict *ConnectionError
	if err := validateConfig(config, "gitlab"); !errors.As(err, &conflict) || conflict.Code != "app_platform_header_conflict" {
		t.Fatal("Authorization conflict not rejected")
	}
}
