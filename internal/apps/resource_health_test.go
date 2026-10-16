package apps

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"
)

func TestResourceHealthWithoutAgentAndAcrossRestart(t *testing.T) {
	for _, appID := range []string{"gitlab", "feishu"} {
		t.Run(appID, func(t *testing.T) {
			upstream := upstreamServer(t)
			s := newTestService(t, Options{})
			resource, err := s.Create(t.Context(), "", CreateRequest{Kind: KindMCP, AppID: appID, Name: "Independent", Config: Config{URL: upstream.URL, AuthMode: "bearer"}, Credentials: Credentials{Token: "fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			checked, err := s.CheckResource(t.Context(), resource.InstallationID, false)
			if err != nil || checked.Status != "connected" || len(checked.Tools) == 0 || len(checked.Bindings) != 0 {
				t.Fatalf("independent health = %+v; err=%v", checked, err)
			}
			if !checked.UpdatedAt.Equal(resource.UpdatedAt) {
				t.Fatal("health check changed configuration revision")
			}
			s.mu.Lock()
			first := s.entries[resource.InstallationID].resourceHealth
			s.mu.Unlock()
			if _, err := s.CheckResource(t.Context(), resource.InstallationID, false); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			cached := s.entries[resource.InstallationID].resourceHealth
			s.mu.Unlock()
			if cached != first {
				t.Fatal("fresh resource health was not cached")
			}
			restored, err := NewService(s.path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restored.Close() })
			items, err := restored.CheckResources(t.Context())
			if err != nil || len(items) != 1 || items[0].Status != "connected" {
				t.Fatalf("restored health = %+v; err=%v", items, err)
			}
			upstream.Close()
			checked, err = s.CheckResource(t.Context(), resource.InstallationID, true)
			if err != nil || checked.Status != "error" || checked.LastErrorCode == "" {
				t.Fatalf("failed health = %+v; err=%v", checked, err)
			}
		})
	}
}

func TestCLIResourceHealthUsesAuthorizationState(t *testing.T) {
	s, _, id := newCLIFixture(t)
	op := startCLITest(t, s, id, "connect", "health-connect")
	if got := waitCLIOperation(t, s, id, op); got.Status != "completed" {
		t.Fatalf("operation=%+v", got)
	}
	checked, err := s.CheckResource(t.Context(), id, true)
	if err != nil || checked.Status != "connected" || !checked.Authorized {
		t.Fatalf("health=%+v err=%v", checked, err)
	}
	s.mu.Lock()
	health := s.entries[id].resourceHealth
	s.mu.Unlock()
	if health != nil {
		t.Fatal("CLI resource was passed to MCP discovery")
	}
}

func TestResourceHealthAuthorizationAndStaleSaveValidation(t *testing.T) {
	upstream := upstreamServer(t)
	s := newTestService(t, Options{})
	resource, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: "Independent", Config: Config{URL: upstream.URL, AuthMode: "bearer"}, Credentials: Credentials{Token: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer unauthorized.Close()
	config := resource.Config
	config.URL = unauthorized.URL
	if _, err := s.Update(t.Context(), "", resource.InstallationID, UpdateRequest{Config: &config}); err != nil {
		t.Fatal(err)
	}
	stale, err := s.RecordResourceProbe(resource, ProbeResult{Connected: true})
	if err != nil || stale.Status == "connected" {
		t.Fatalf("stale validation accepted: %+v, %v", stale, err)
	}
	checked, err := s.CheckResource(t.Context(), resource.InstallationID, true)
	if err != nil || checked.Status != "authorization_required" || checked.LastErrorHTTPStatus != 401 {
		t.Fatalf("auth status = %+v, %v", checked, err)
	}
}

func TestSaveProbeCannotRestoreHealthAfterCredentialsChange(t *testing.T) {
	for _, invalidate := range []string{"platform", "connector"} {
		t.Run(invalidate, func(t *testing.T) {
			upstream := upstreamServer(t)
			s := newTestService(t, Options{})
			resource, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: "Independent", Config: Config{URL: upstream.URL, AuthMode: "bearer"}, Credentials: Credentials{Token: "fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			probe, err := s.Probe(t.Context(), "", ProbeRequest{AppID: "gitlab", Config: resource.Config, Credentials: Credentials{Token: "fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			valid, err := s.RecordResourceProbe(resource, probe)
			if err != nil || valid.Status != "connected" {
				t.Fatalf("current probe rejected: %s %v", valid.Status, err)
			}
			// Simulate a save-time probe that finishes immediately before an account or PAT change.
			if invalidate == "platform" {
				if err := s.RefreshPlatformCredentials(t.Context()); err != nil {
					t.Fatal(err)
				}
			} else {
				s.InvalidateConnector("gitlab")
			}
			// Creation can commit after the account change while its earlier probe is pending.
			savedAfterChange, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: "Saved after change", Config: resource.Config, Credentials: Credentials{Token: "fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			stale, err := s.RecordResourceProbe(savedAfterChange, probe)
			if err != nil || stale.Status == "connected" {
				t.Fatalf("stale probe accepted: %s %v", stale.Status, err)
			}
			fresh, err := s.Probe(t.Context(), "", ProbeRequest{AppID: "gitlab", Config: resource.Config, Credentials: Credentials{Token: "fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			checked, err := s.RecordResourceProbe(savedAfterChange, fresh)
			if err != nil || checked.Status != "connected" {
				t.Fatalf("fresh probe rejected: %s %v", checked.Status, err)
			}
		})
	}
}

func TestResourceHealthHonorsConfiguredTimeouts(t *testing.T) {
	for _, method := range []string{"initialize", "tools/list"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			upstream := upstreamServer(t)
			target, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			delayed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						return
					}
					r.Body = io.NopCloser(bytes.NewReader(body))
					if bytes.Contains(body, []byte(`"method":"`+method+`"`)) {
						select {
						case <-time.After(11 * time.Second):
						case <-r.Context().Done():
							return
						}
					}
				}
				proxy.ServeHTTP(w, r)
			}))
			defer delayed.Close()
			s := newTestService(t, Options{})
			resource, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: "Slow service", Config: Config{URL: delayed.URL, AuthMode: "bearer", StartupTimeoutSec: 30, ToolTimeoutSec: 30}, Credentials: Credentials{Token: "fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			checked, err := s.CheckResource(t.Context(), resource.InstallationID, false)
			if err != nil || checked.Status != "connected" || len(checked.Tools) == 0 {
				t.Fatalf("configured timeout ignored: status=%s tools=%d err=%v", checked.Status, len(checked.Tools), err)
			}
		})
	}
}

func TestSaveProbeSurvivesOnlyItsOwnConnectorRotation(t *testing.T) {
	for _, rotation := range []string{"own", "before", "after", "other-connector"} {
		t.Run(rotation, func(t *testing.T) {
			upstream := upstreamServer(t)
			s := newTestService(t, Options{ResolveConnectorHTTP: func(context.Context, string, string, Config) (ConnectorHTTPConfig, error) {
				return ConnectorHTTPConfig{Endpoint: upstream.URL, Token: "fixture", TokenHeader: "Authorization", TokenPrefix: "Bearer "}, nil
			}})
			cfg := Config{URL: upstream.URL, AuthMode: "connector", ConnectorID: "gitlab", GitLabBaseURL: "https://gitlab.example.test"}
			resource, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: "Managed", Config: cfg})
			if err != nil {
				t.Fatal(err)
			}
			probe, err := s.Probe(t.Context(), "", ProbeRequest{AppID: "gitlab", Config: cfg})
			if err != nil {
				t.Fatal(err)
			}
			if rotation == "before" {
				s.InvalidateConnector("gitlab")
			}
			connectorID := "gitlab"
			if rotation == "other-connector" {
				connectorID = "feishu"
			}
			if err := s.RefreshConnectorAfterProbe(t.Context(), connectorID, &probe); err != nil {
				t.Fatal(err)
			}
			if rotation == "after" {
				s.InvalidateConnector("gitlab")
			}
			checked, err := s.RecordResourceProbe(resource, probe)
			if err != nil {
				t.Fatal(err)
			}
			if (checked.Status == "connected") != (rotation == "own") {
				t.Fatalf("rotation=%s status=%s", rotation, checked.Status)
			}
		})
	}
}
