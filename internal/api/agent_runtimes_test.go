package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"csgclaw/internal/dshcli"
	"csgclaw/internal/runtimecatalog"
)

type bundledCodexResolver struct{}

func (bundledCodexResolver) Ensure(context.Context) (string, error) {
	return "/opt/csgclaw/bin/codex", nil
}

func TestAgentRuntimesListReportsBundledCodex(t *testing.T) {
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService(
		runtimecatalog.WithCodexResolver(bundledCodexResolver{}),
		runtimecatalog.WithDSHResolver(bundledCodexResolver{}),
	))

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/agent-runtimes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var runtimes []runtimecatalog.Runtime
	if err := json.NewDecoder(recorder.Body).Decode(&runtimes); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(runtimes) != 3 {
		t.Fatalf("runtimes = %+v, want Codex, DSH, and Claude Code", runtimes)
	}
	if got := runtimes[0]; got.Name != runtimecatalog.RuntimeCodex || !got.Installed || got.Installable || got.Status != "installed" || got.Path != "/opt/csgclaw/bin/codex" {
		t.Fatalf("Codex runtime = %+v, want bundled installed runtime", got)
	}
}

type mutableAPIDSHResolver struct {
	info dshcli.Info
	err  error
}

func (r *mutableAPIDSHResolver) Ensure(context.Context) (string, error)       { return r.info.Path, r.err }
func (r *mutableAPIDSHResolver) Resolve(context.Context) (dshcli.Info, error) { return r.info, r.err }

type apiDSHInstaller struct {
	resolver *mutableAPIDSHResolver
	err      error
}

func (i apiDSHInstaller) Install(context.Context) (dshcli.Info, error) {
	if i.err != nil {
		return dshcli.Info{}, i.err
	}
	i.resolver.err = nil
	i.resolver.info = dshcli.Info{Path: "/home/test/.local/bin/dsh", Version: "0.1.5-rc.2"}
	return i.resolver.info, nil
}

func TestAgentRuntimeInstallReportsMissingNodePrerequisite(t *testing.T) {
	resolver := &mutableAPIDSHResolver{err: errors.New("missing")}
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService(
		runtimecatalog.WithDSHResolver(resolver),
		runtimecatalog.WithDSHInstaller(apiDSHInstaller{resolver: resolver, err: dshcli.ErrNPMNotFound}),
	))

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agent-runtimes/dsh/install", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	installation := waitForAgentRuntimeInstallation(t, handler)
	if installation.Status != runtimecatalog.InstallStatusFailed || installation.ErrorCode != "dsh_node_required" {
		t.Fatalf("installation = %+v, want missing Node.js prerequisite failure", installation)
	}
}

func TestAgentRuntimeInstallRejectsNonInstallableRuntime(t *testing.T) {
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService())

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agent-runtimes/codex/install", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentRuntimeInstallInstallsDSH(t *testing.T) {
	resolver := &mutableAPIDSHResolver{err: errors.New("missing")}
	handler := &Handler{}
	handler.SetAgentRuntimeService(runtimecatalog.NewService(
		runtimecatalog.WithDSHResolver(resolver),
		runtimecatalog.WithDSHInstaller(apiDSHInstaller{resolver: resolver}),
	))

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/agent-runtimes/dsh/install", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	installation := waitForAgentRuntimeInstallation(t, handler)
	if installation.Status != runtimecatalog.InstallStatusSucceeded || installation.Runtime == nil {
		t.Fatalf("installation = %+v, want succeeded DSH installation", installation)
	}
	if !installation.Runtime.Installed || installation.Runtime.Path != resolver.info.Path || installation.Runtime.Version != resolver.info.Version {
		t.Fatalf("runtime = %+v, want installed DSH", installation.Runtime)
	}
}

func waitForAgentRuntimeInstallation(t *testing.T, handler *Handler) runtimecatalog.Installation {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		recorder := httptest.NewRecorder()
		handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/agent-runtimes/dsh/install", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET install status = %d; body=%s", recorder.Code, recorder.Body.String())
		}
		var installation runtimecatalog.Installation
		if err := json.NewDecoder(recorder.Body).Decode(&installation); err != nil {
			t.Fatalf("decode install status: %v", err)
		}
		if installation.Status != runtimecatalog.InstallStatusRunning {
			return installation
		}
		if time.Now().After(deadline) {
			t.Fatalf("installation did not finish: %+v", installation)
		}
		time.Sleep(time.Millisecond)
	}
}
