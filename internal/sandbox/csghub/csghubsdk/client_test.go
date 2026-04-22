package csghubsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL, Token: "sk-test"}), srv
}

func TestCreateSendsBodyAndParsesEnvelope(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"msg": "ok",
			"data": {
				"spec": {
					"sandbox_name": "csgclaw-t1-u1",
					"image": "hub/csgclaw:latest",
					"environments": {"CSGCLAW_ROLE": "server"},
					"volumes": [],
					"port": 8080
				},
				"state": {
					"status": "Pending",
					"exited_code": 0,
					"created_at": "2026-04-20T08:00:00Z"
				}
			}
		}`))
	}))

	spec := CreateRequest{
		Image:        "hub/csgclaw:latest",
		SandboxName:  "csgclaw-t1-u1",
		ResourceID:   77,
		Environments: map[string]string{"CSGCLAW_ROLE": "server"},
		Volumes: []VolumeSpec{{
			SandboxMountSubpath: "tenants/t1",
			SandboxMountPath:    "/home/picoclaw/.picoclaw",
		}},
		Port: 8080,
	}
	got, err := cli.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.Spec.SandboxName != "csgclaw-t1-u1" {
		t.Fatalf("Create() name = %q", got.Spec.SandboxName)
	}
	if got.State.Status != "Pending" {
		t.Fatalf("Create() state = %q", got.State.Status)
	}
	if gotPath != "/api/v1/sandboxes" {
		t.Fatalf("Create() path = %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Create() Authorization = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"sandbox_name":"csgclaw-t1-u1"`) {
		t.Fatalf("Create() body missing sandbox_name: %q", gotBody)
	}
	if !strings.Contains(gotBody, `"resource_id":77`) {
		t.Fatalf("Create() body missing resource_id: %q", gotBody)
	}
	if !strings.Contains(gotBody, `"sandbox_mount_path":"/home/picoclaw/.picoclaw"`) {
		t.Fatalf("Create() body missing volume mount: %q", gotBody)
	}
}

func TestGetParsesDirectShape(t *testing.T) {
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sandboxes/csgclaw-t1-u1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spec":{"sandbox_name":"csgclaw-t1-u1","image":"img"},"state":{"status":"Running","created_at":"2026-04-20T08:00:00Z"}}`))
	}))

	got, err := cli.Get(context.Background(), "csgclaw-t1-u1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.State.Status != "Running" {
		t.Fatalf("Get() status = %q", got.State.Status)
	}
}

func TestGetNotFoundMapsToIsNotFound(t *testing.T) {
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"sandbox not found"}`))
	}))

	_, err := cli.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("Get() error = nil, want HTTPError")
	}
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false", err)
	}
}

func TestGetBadRequestNotFoundAlsoMapsToIsNotFound(t *testing.T) {
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"message":"not found"}`))
	}))

	_, err := cli.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("Get() error = nil, want HTTPError")
	}
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false", err)
	}
}

func TestStartPutsToStatusStart(t *testing.T) {
	var gotMethod, gotPath string
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spec":{"sandbox_name":"csgclaw-t1-u1","image":"img"},"state":{"status":"Starting","created_at":"2026-04-20T08:00:00Z"}}`))
	}))

	if _, err := cli.Start(context.Background(), "csgclaw-t1-u1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("Start() method = %q", gotMethod)
	}
	if gotPath != "/api/v1/sandboxes/csgclaw-t1-u1/status/start" {
		t.Fatalf("Start() path = %q", gotPath)
	}
}

func TestStopPutsToStatusStop(t *testing.T) {
	var gotPath string
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	if err := cli.Stop(context.Background(), "csgclaw-t1-u1"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if gotPath != "/api/v1/sandboxes/csgclaw-t1-u1/status/stop" {
		t.Fatalf("Stop() path = %q", gotPath)
	}
}

func TestUpdateConfigSendsPatchAndOmitsDefaults(t *testing.T) {
	var gotMethod, gotBody string
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spec":{"sandbox_name":"csgclaw-t1-u1","image":"img2"},"state":{"status":"Running","created_at":"2026-04-20T08:00:00Z"}}`))
	}))

	_, err := cli.UpdateConfig(context.Background(), "csgclaw-t1-u1", UpdateConfigRequest{
		Image: "img2",
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("UpdateConfig() method = %q", gotMethod)
	}
	// resource_id omitempty when zero, image present.
	if strings.Contains(gotBody, `"resource_id"`) {
		t.Fatalf("UpdateConfig() body should omit zero resource_id: %q", gotBody)
	}
	if !strings.Contains(gotBody, `"image":"img2"`) {
		t.Fatalf("UpdateConfig() body missing image: %q", gotBody)
	}
}

func TestApplyRoutesThroughUpdateConfig(t *testing.T) {
	var gotPath, gotBody string
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spec":{"sandbox_name":"csgclaw-t1-u1","image":"img"},"state":{"status":"Running","created_at":"2026-04-20T08:00:00Z"}}`))
	}))

	_, err := cli.Apply(context.Background(), CreateRequest{
		Image:       "img",
		SandboxName: "csgclaw-t1-u1",
		ResourceID:  77,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if gotPath != "/api/v1/sandboxes/csgclaw-t1-u1" {
		t.Fatalf("Apply() path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"resource_id":77`) {
		t.Fatalf("Apply() body = %q", gotBody)
	}
}

func TestHTTPErrorDecodesMessageField(t *testing.T) {
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"message":"bad spec"}`))
	}))

	_, err := cli.Create(context.Background(), CreateRequest{Image: "img", SandboxName: "n"})
	if err == nil {
		t.Fatal("Create() error = nil, want HTTPError")
	}
	he, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if he.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", he.StatusCode)
	}
	if !strings.Contains(he.Detail, "bad spec") {
		t.Fatalf("detail = %q", he.Detail)
	}
}

func TestStreamExecuteEmitsLines(t *testing.T) {
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/sandboxes/sb-1/execute"; got != want {
			t.Errorf("path = %q", got)
		}
		if got, want := r.URL.Query().Get("port"), "8888"; got != want {
			t.Errorf("port = %q", got)
		}
		var req struct {
			Command string `json:"command"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Command != "echo hello" {
			t.Errorf("command = %q", req.Command)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "hello\n\nworld\n")
	}))

	var lines []string
	err := cli.StreamExecute(context.Background(), "sb-1", "echo hello", func(line string) error {
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamExecute() error = %v", err)
	}
	if strings.Join(lines, "|") != "hello|world" {
		t.Fatalf("lines = %v", lines)
	}
}

func TestStreamExecuteEmitsErrorLineOnHTTPFailure(t *testing.T) {
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"boom"}`)
	}))

	var lines []string
	err := cli.StreamExecute(context.Background(), "sb-1", "x", func(line string) error {
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamExecute() error = %v", err)
	}
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "ERROR: HTTP 500") {
		t.Fatalf("lines = %v", lines)
	}
}

func TestUploadFileSendsMultipart(t *testing.T) {
	var gotField, gotFilename, gotContent string
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		for field, files := range r.MultipartForm.File {
			gotField = field
			gotFilename = files[0].Filename
			f, _ := files[0].Open()
			defer f.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, f)
			gotContent = buf.String()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"uploaded"}`))
	}))

	got, err := cli.UploadFile(context.Background(), "sb-1", "readme.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if got.Message != "uploaded" {
		t.Fatalf("Message = %q", got.Message)
	}
	if gotField != "file" {
		t.Fatalf("field = %q", gotField)
	}
	if gotFilename != "readme.txt" {
		t.Fatalf("filename = %q", gotFilename)
	}
	if gotContent != "hello" {
		t.Fatalf("content = %q", gotContent)
	}
}

func TestUploadFilesBatchEncodesBase64(t *testing.T) {
	var gotPayload map[string]any
	cli, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))

	_, err := cli.UploadFilesBatch(context.Background(), "sb-1", []FilePayload{
		{Path: "a.txt", Content: []byte("hi")},
	})
	if err != nil {
		t.Fatalf("UploadFilesBatch() error = %v", err)
	}
	files, ok := gotPayload["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("files = %v", gotPayload["files"])
	}
	entry, _ := files[0].(map[string]any)
	if entry["path"] != "a.txt" {
		t.Fatalf("path = %v", entry["path"])
	}
	// base64("hi") == aGk=
	if entry["content"] != "aGk=" {
		t.Fatalf("content = %v", entry["content"])
	}
}

func TestRuntimeHealthUsesGatewayBase(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sb-1/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(gw.Close)

	cli := New(Config{BaseURL: "http://unused.invalid", AIGatewayURL: gw.URL, Token: "sk"})
	if err := cli.RuntimeHealth(context.Background(), "sb-1"); err != nil {
		t.Fatalf("RuntimeHealth() error = %v", err)
	}
}

func TestConfigFallsBackToBaseURLWhenGatewayEmpty(t *testing.T) {
	cfg := Config{BaseURL: "https://hub.example.com"}
	if got, want := cfg.aigatewayBase(), "https://hub.example.com"; got != want {
		t.Fatalf("aigatewayBase() = %q, want %q", got, want)
	}
	cfg.AIGatewayURL = "https://gw.example.com/"
	if got, want := cfg.aigatewayBase(), "https://gw.example.com"; got != want {
		t.Fatalf("aigatewayBase() = %q, want %q", got, want)
	}
	if got, want := cfg.apiSandboxesRoot(), "https://hub.example.com/api/v1/sandboxes"; got != want {
		t.Fatalf("apiSandboxesRoot() = %q, want %q", got, want)
	}
}

// Compile-time guard: the multipart writer import path is actually used by
// UploadFile; also exercise ParseResponse fallback without wrapper.
var _ = multipart.Writer{}
