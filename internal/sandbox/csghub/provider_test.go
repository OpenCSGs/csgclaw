package csghub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/csghub/csghubsdk"
)

func setRequiredEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("CSGHUB_API_BASE_URL", baseURL)
	t.Setenv("CSGHUB_USER_TOKEN", "token-test")
	t.Setenv("CSGCLAW_PVC_MOUNT_PATH", "/opt/csgclaw")
	t.Setenv("CSGCLAW_CLUSTER_ID", "cluster-a")
	t.Setenv("CSGCLAW_RESOURCE_ID", "77")
	t.Setenv("CSGCLAW_SANDBOX_PORT", "8080")
	t.Setenv("CSGCLAW_SANDBOX_TIMEOUT", "600")
	t.Setenv("CSGCLAW_SANDBOX_READY_TIMEOUT", "30s")
	t.Setenv("CSGCLAW_SANDBOX_POLL_INTERVAL", "1s")
}

func writeSandboxState(w http.ResponseWriter, name, image, status string) {
	_, _ = fmt.Fprintf(w, `{
		"spec": {"sandbox_name":%q,"image":%q},
		"state": {"status":%q,"created_at":"2026-04-22T00:00:00Z"}
	}`, name, image, status)
}

func TestOpenRequiresBaseURL(t *testing.T) {
	t.Setenv("CSGHUB_API_BASE_URL", "")
	t.Setenv("CSGHUB_USER_TOKEN", "token-test")
	_, err := NewProvider().Open(context.Background(), "")
	if err == nil {
		t.Fatal("Open() error = nil, want missing base url error")
	}
	if !strings.Contains(err.Error(), "CSGHUB_API_BASE_URL is required") {
		t.Fatalf("Open() error = %q", err)
	}
}

func TestRuntimeCreateBuildsRequestAndMapsMounts(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			writeSandboxState(w, "worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/worker-1/":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	rt := rtAny.(*Runtime)

	_, err = rt.Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
		Env: map[string]string{
			"A": "B",
		},
		Mounts: []sandbox.Mount{
			{
				HostPath:  "/opt/csgclaw/tenant-a/projects",
				GuestPath: "/home/picoclaw/.picoclaw/workspace/projects",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if gotBody["sandbox_name"] != "worker-1" {
		t.Fatalf("sandbox_name = %v", gotBody["sandbox_name"])
	}
	if gotBody["image"] != "img:1" {
		t.Fatalf("image = %v", gotBody["image"])
	}
	if gotBody["resource_id"] != float64(77) {
		t.Fatalf("resource_id = %v", gotBody["resource_id"])
	}
	if gotBody["cluster_id"] != "cluster-a" {
		t.Fatalf("cluster_id = %v", gotBody["cluster_id"])
	}
	volumes, ok := gotBody["volumes"].([]any)
	if !ok || len(volumes) != 1 {
		t.Fatalf("volumes = %#v", gotBody["volumes"])
	}
	volume := volumes[0].(map[string]any)
	if volume["sandbox_mount_subpath"] != "tenant-a/projects" {
		t.Fatalf("sandbox_mount_subpath = %v", volume["sandbox_mount_subpath"])
	}
}

func TestCreateRequestInjectsCSGHubEnvironment(t *testing.T) {
	rt := &Runtime{
		cfg: runtimeConfig{
			clientCfg: csghubsdk.Config{
				BaseURL: " https://hub.example.test ",
				Token:   " token-test ",
			},
		},
	}

	req, err := rt.createRequest(sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
		Env: map[string]string{
			"A": "B",
		},
	})
	if err != nil {
		t.Fatalf("createRequest() error = %v", err)
	}

	if got, want := req.Environments["A"], "B"; got != want {
		t.Fatalf("A = %q, want %q", got, want)
	}
	if got, want := req.Environments["CSGHUB_API_BASE_URL"], "https://hub.example.test"; got != want {
		t.Fatalf("CSGHUB_API_BASE_URL = %q, want %q", got, want)
	}
	if got, want := req.Environments["CSGHUB_USER_TOKEN"], "token-test"; got != want {
		t.Fatalf("CSGHUB_USER_TOKEN = %q, want %q", got, want)
	}
}

func TestRuntimeOpenWithPVCMountPathOverridesEnvMountPath(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			writeSandboxState(w, "worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			writeSandboxState(w, "worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/worker-1/":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider(WithPVCMountPath("/shared/csgclaw")).Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	rt := rtAny.(*Runtime)

	_, err = rt.Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
		Mounts: []sandbox.Mount{
			{
				HostPath:  "/shared/csgclaw/tenant-a/projects",
				GuestPath: "/home/picoclaw/.picoclaw/workspace/projects",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	volumes, ok := gotBody["volumes"].([]any)
	if !ok || len(volumes) != 1 {
		t.Fatalf("volumes = %#v", gotBody["volumes"])
	}
	volume := volumes[0].(map[string]any)
	if volume["sandbox_mount_subpath"] != "tenant-a/projects" {
		t.Fatalf("sandbox_mount_subpath = %v", volume["sandbox_mount_subpath"])
	}
}

func TestRuntimeOpenWithPVCMountSubpathPrefix(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			writeSandboxState(w, "worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			writeSandboxState(w, "worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/worker-1/":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider(
		WithPVCMountSubpathPrefix("/tenant-a-data/"),
	).Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	rt := rtAny.(*Runtime)

	_, err = rt.Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
		Mounts: []sandbox.Mount{
			{
				HostPath:  "/opt/csgclaw/agents/manager/workspace",
				GuestPath: "/home/picoclaw/.picoclaw/workspace",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	volumes, ok := gotBody["volumes"].([]any)
	if !ok || len(volumes) != 1 {
		t.Fatalf("volumes = %#v", gotBody["volumes"])
	}
	volume := volumes[0].(map[string]any)
	if volume["sandbox_mount_subpath"] != "tenant-a-data/agents/manager/workspace" {
		t.Fatalf("sandbox_mount_subpath = %v", volume["sandbox_mount_subpath"])
	}
}

func TestRuntimeOpenReadsPVCSubpathPrefixFromEnv(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			writeSandboxState(w, "worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			writeSandboxState(w, "worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/worker-1/":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)
	t.Setenv("CSGCLAW_PVC_SUBPATH_PREFIX", "tenant-b-space")

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	rt := rtAny.(*Runtime)

	_, err = rt.Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
		Mounts: []sandbox.Mount{
			{
				HostPath:  "/opt/csgclaw/agents/manager/workspace",
				GuestPath: "/home/picoclaw/.picoclaw/workspace",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	volumes, ok := gotBody["volumes"].([]any)
	if !ok || len(volumes) != 1 {
		t.Fatalf("volumes = %#v", gotBody["volumes"])
	}
	volume := volumes[0].(map[string]any)
	if volume["sandbox_mount_subpath"] != "tenant-b-space/agents/manager/workspace" {
		t.Fatalf("sandbox_mount_subpath = %v", volume["sandbox_mount_subpath"])
	}
}

func TestRuntimeCreatePrefixesSandboxNameWhenCSGCLAWNameProvided(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"cluster-a-worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/cluster-a-worker-1/status/start":
			writeSandboxState(w, "cluster-a-worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/cluster-a-worker-1":
			writeSandboxState(w, "cluster-a-worker-1", "img:1", "running")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/cluster-a-worker-1/":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)
	t.Setenv("CSGCLAW_NAME", "cluster-a")

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = rtAny.(*Runtime).Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if gotBody["sandbox_name"] != "cluster-a-worker-1" {
		t.Fatalf("sandbox_name = %v", gotBody["sandbox_name"])
	}
}

func TestRuntimeGetUsesPrefixedSandboxNameWhenCSGCLAWNameProvided(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/cluster-a-worker-1" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"cluster-a-worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)
	t.Setenv("CSGCLAW_NAME", "cluster-a")

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = rtAny.(*Runtime).Get(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestRuntimeCreateStartsAndWaitsHealthWhenNotRunning(t *testing.T) {
	var started bool
	var healthChecked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"created","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			started = true
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"starting","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/worker-1/":
			healthChecked = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = rtAny.(*Runtime).Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !started {
		t.Fatal("Create() did not call start for non-running status")
	}
	if !healthChecked {
		t.Fatal("Create() did not wait for runtime health")
	}
}

func TestRuntimeCreateStartsWhenDeployingThenWaitsForRunning(t *testing.T) {
	var startCalls, getCalls int
	var healthChecked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"deploying","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			startCalls++
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"starting","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			getCalls++
			if getCalls == 1 {
				_, _ = w.Write([]byte(`{
					"spec": {"sandbox_name":"worker-1","image":"img:1"},
					"state": {"status":"deploying","created_at":"2026-04-22T00:00:00Z"}
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"running","created_at":"2026-04-22T00:00:00Z"}
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/worker-1/":
			healthChecked = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = rtAny.(*Runtime).Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("start called = %d, want 1", startCalls)
	}
	if getCalls < 1 {
		t.Fatalf("polling get count = %d, want >=1", getCalls)
	}
	if !healthChecked {
		t.Fatal("Create() did not wait for runtime health")
	}
}

func TestRuntimeCreateConflictAppliesStartsAndWaitsPastStopped(t *testing.T) {
	var patchCalls, startCalls, getCalls int
	var healthChecked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sandboxes":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"msg":"the record already exists"}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/sandboxes/worker-1":
			patchCalls++
			writeSandboxState(w, "worker-1", "img:2", "running")
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			startCalls++
			writeSandboxState(w, "worker-1", "img:2", "stopped")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			getCalls++
			if getCalls == 1 {
				writeSandboxState(w, "worker-1", "img:2", "stopped")
				return
			}
			writeSandboxState(w, "worker-1", "img:2", "deploying")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/worker-1/":
			healthChecked = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)
	t.Setenv("CSGCLAW_SANDBOX_POLL_INTERVAL", "500ms")

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = rtAny.(*Runtime).Create(context.Background(), sandbox.CreateSpec{
		Image: "img:2",
		Name:  "worker-1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if patchCalls != 1 {
		t.Fatalf("patch calls = %d, want 1", patchCalls)
	}
	if startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", startCalls)
	}
	if getCalls < 2 {
		t.Fatalf("get calls = %d, want >=2", getCalls)
	}
	if !healthChecked {
		t.Fatal("Create() did not wait for runtime health")
	}
}

func TestRuntimeRemoveWaitsForStopped(t *testing.T) {
	var stopCalls, getCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/stop":
			stopCalls++
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			getCalls++
			if getCalls == 1 {
				writeSandboxState(w, "worker-1", "img:1", "running")
				return
			}
			writeSandboxState(w, "worker-1", "img:1", "stopped")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)
	t.Setenv("CSGCLAW_SANDBOX_POLL_INTERVAL", "500ms")

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	err = rtAny.(*Runtime).Remove(context.Background(), "worker-1", sandbox.RemoveOptions{Force: true})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", stopCalls)
	}
	if getCalls < 2 {
		t.Fatalf("get calls = %d, want >=2", getCalls)
	}
}

func TestInstanceStopWaitsForStopped(t *testing.T) {
	var stopCalls, getCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/stop":
			stopCalls++
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			getCalls++
			if getCalls == 1 {
				writeSandboxState(w, "worker-1", "img:1", "running")
				return
			}
			writeSandboxState(w, "worker-1", "img:1", "stopped")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)
	t.Setenv("CSGCLAW_SANDBOX_POLL_INTERVAL", "500ms")

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	instance := &Instance{runtime: rtAny.(*Runtime), name: "worker-1"}
	err = instance.Stop(context.Background(), sandbox.StopOptions{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", stopCalls)
	}
	if getCalls < 2 {
		t.Fatalf("get calls = %d, want >=2", getCalls)
	}
}

func TestRuntimeCreateRejectsMountOutsidePVCPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	rt := rtAny.(*Runtime)
	_, err = rt.Create(context.Background(), sandbox.CreateSpec{
		Image: "img:1",
		Name:  "worker-1",
		Mounts: []sandbox.Mount{
			{
				HostPath:  "/tmp/not-in-pvc",
				GuestPath: "/workspace",
			},
		},
	})
	if err == nil {
		t.Fatal("Create() error = nil, want invalid mount error")
	}
	if !strings.Contains(err.Error(), "must be under") {
		t.Fatalf("Create() error = %q", err)
	}
}

func TestRuntimeGetNotFoundMapsSandboxNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/sandboxes/missing" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"missing"}`))
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = rtAny.(*Runtime).Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("Get() error = nil, want not found")
	}
	if !sandbox.IsNotFound(err) {
		t.Fatalf("Get() error = %v, want sandbox.ErrNotFound mapping", err)
	}
}

func TestInstanceRunStreamsOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/sandboxes/worker-1/execute"):
			_, _ = w.Write([]byte("line-1\nline-2\n"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	inst := &Instance{
		runtime: rtAny.(*Runtime),
		name:    "worker-1",
	}
	var out bytes.Buffer
	result, err := inst.Run(context.Background(), sandbox.CommandSpec{
		Name:   "tail",
		Args:   []string{"-n", "2", "/tmp/log"},
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() ExitCode = %d, want 0", result.ExitCode)
	}
	if got := out.String(); !strings.Contains(got, "line-1\n") || !strings.Contains(got, "line-2\n") {
		t.Fatalf("Run() output = %q", got)
	}
}

func TestInstanceRunReturnsErrorWhenGatewayEmitsErrorLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	inst := &Instance{
		runtime: rtAny.(*Runtime),
		name:    "worker-1",
	}
	var out bytes.Buffer
	result, err := inst.Run(context.Background(), sandbox.CommandSpec{
		Name:   "tail",
		Args:   []string{"-n", "2", "/tmp/log"},
		Stdout: &out,
	})
	if err == nil {
		t.Fatal("Run() error = nil, want stream error")
	}
	if result.ExitCode != 1 {
		t.Fatalf("Run() ExitCode = %d, want 1", result.ExitCode)
	}
	if got := out.String(); !strings.Contains(got, "ERROR: HTTP 500") {
		t.Fatalf("Run() output = %q, want ERROR line", got)
	}
}

func TestOpenClampsDurations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)
	t.Setenv("CSGCLAW_SANDBOX_READY_TIMEOUT", "1s")
	t.Setenv("CSGCLAW_SANDBOX_POLL_INTERVAL", "60s")

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	rt := rtAny.(*Runtime)
	if rt.cfg.readyTimeout != minReadyTimeout {
		t.Fatalf("readyTimeout = %v, want %v", rt.cfg.readyTimeout, minReadyTimeout)
	}
	if rt.cfg.pollInterval != maxPollInterval {
		t.Fatalf("pollInterval = %v, want %v", rt.cfg.pollInterval, maxPollInterval)
	}
}

func TestParseDurationEnvSupportsSeconds(t *testing.T) {
	t.Setenv("X_DURATION", "12")
	got, err := parseDurationEnv("X_DURATION", time.Second)
	if err != nil {
		t.Fatalf("parseDurationEnv() error = %v", err)
	}
	if got != 12*time.Second {
		t.Fatalf("parseDurationEnv() = %v, want %v", got, 12*time.Second)
	}
}

func TestStartSandboxIdempotentAcceptsDuplicateStartAfterGet(t *testing.T) {
	var getCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			w.WriteHeader(http.StatusBadRequest)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			getCalls++
			_, _ = w.Write([]byte(`{
				"spec": {"sandbox_name":"worker-1","image":"img:1"},
				"state": {"status":"deploying","created_at":"2026-04-22T00:00:00Z"}
			}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	runtime := rtAny.(*Runtime)

	resp, err := runtime.startSandboxIdempotent(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("startSandboxIdempotent() error = %v", err)
	}
	if resp != nil {
		t.Fatalf("startSandboxIdempotent() response = %#v, want nil state response", resp)
	}
	if getCalls != 1 {
		t.Fatalf("startSandboxIdempotent() GET calls = %d, want 1", getCalls)
	}
}

func TestStartSandboxIdempotentDuplicateStartReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/sandboxes/worker-1/status/start":
			w.WriteHeader(http.StatusBadRequest)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sandboxes/worker-1":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"msg":"sandbox not found"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	setRequiredEnv(t, server.URL)

	rtAny, err := NewProvider().Open(context.Background(), "")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	runtime := rtAny.(*Runtime)

	_, err = runtime.startSandboxIdempotent(context.Background(), "worker-1")
	if !sandbox.IsNotFound(err) {
		t.Fatalf("startSandboxIdempotent() error = %v, want sandbox not found", err)
	}
}
