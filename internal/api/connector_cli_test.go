package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apps"
	"csgclaw/internal/connectorcli"
	agentruntime "csgclaw/internal/runtime"
)

func TestCreateDeclarativeCLIResourceAPI(t *testing.T) {
	spec := connectorcli.Spec{
		Install: connectorcli.InstallSpec{
			Version: "1.0.0", Executable: "fixture", VersionArgs: []string{"--version"},
			Releases: map[string]connectorcli.Archive{"test": {URL: "https://example.com/fixture.tar.gz", SHA256: strings.Repeat("0", 64)}},
		},
		Status: []string{"status"}, StatusMatch: "ready",
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	service, err := apps.NewService("", apps.Options{Packages: fstest.MapFS{
		"fixture/plugin.json": {Data: []byte(`{"name":"fixture","version":"1.0.0","kind":"cli","cli":"./cli.json","interface":{"displayName":"Fixture"}}`)},
		"fixture/cli.json":    {Data: data},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	h := &Handler{apps: service}
	for _, kind := range []string{"", "cli"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/resources", strings.NewReader(`{"app_id":"fixture","kind":"`+kind+`"}`))
		response := httptest.NewRecorder()
		h.handleConnectorResources(response, req)
		if response.Code != http.StatusCreated {
			t.Fatalf("kind=%q: status=%d body=%s", kind, response.Code, response.Body)
		}
		var resource apps.Installation
		if err := json.Unmarshal(response.Body.Bytes(), &resource); err != nil {
			t.Fatal(err)
		}
		if resource.Kind != apps.KindCLI || resource.AppID != "fixture" || resource.Status != "needs_configuration" {
			t.Fatalf("resource=%+v", resource)
		}
	}
}

func TestConnectorCLIVerifyDisabledConfiguration(t *testing.T) {
	for _, status := range []string{"stopped", "running"} {
		t.Run(status, func(t *testing.T) {
			svc := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "worker", Name: "worker", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, Status: status}}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex, info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{State: agentruntime.State(status)}, nil
			}}))
			runtime := connectorRuntime{h: &Handler{svc: svc, agentEngine: agentengine.New(svc)}}
			loaded, err := runtime.Verify(context.Background(), "worker")
			if err != nil {
				t.Fatal(err)
			}
			if loaded != (status == "running") {
				t.Fatalf("loaded=%v for %s worker", loaded, status)
			}
		})
	}
}

func TestLegacyWorkerRunningUsesDesiredState(t *testing.T) {
	for _, tt := range []struct {
		name, desired, observed string
		running                 bool
	}{
		{"service restart", agent.DesiredStateRunning, "stopped", true},
		{"explicit stop", agent.DesiredStateStopped, "running", false},
		{"legacy running", "", "running", true},
		{"legacy stopped", "", "stopped", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := legacyWorkerRunning(agent.Agent{DesiredState: tt.desired, Status: tt.observed}); got != tt.running {
				t.Fatalf("running=%v, want %v", got, tt.running)
			}
		})
	}
}

func TestConnectorCLIDisconnectRoute(t *testing.T) {
	service, err := apps.NewService("", apps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	resource, err := service.EnsureFeishuCLI(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{apps: service, serverNoAuth: true}
	router := h.Routes()
	for _, tc := range []struct {
		action string
		status int
	}{
		{"repair", http.StatusNotFound},
		{"disconnect", http.StatusAccepted},
	} {
		t.Run(tc.action, func(t *testing.T) {
			body, err := json.Marshal(apps.CLIRequest{OperationID: tc.action, Revision: resource.Revision})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/resources/"+resource.InstallationID+"/cli/"+tc.action, strings.NewReader(string(body)))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}
