package template

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
)

func TestRemoteStoreListGetAndFetchWorkspace(t *testing.T) {
	t.Parallel()

	var archive []byte
	{
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{
			Name:     "AGENTS.md",
			Mode:     0o644,
			Size:     5,
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}
		if _, err := tw.Write([]byte("hello")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := tw.Close(); err != nil {
			t.Fatalf("tar Close() error = %v", err)
		}
		if err := gz.Close(); err != nil {
			t.Fatalf("gzip Close() error = %v", err)
		}
		archive = buf.Bytes()
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/hub/templates":
			_ = json.NewEncoder(w).Encode([]apitypes.HubTemplate{
				{
					ID:          "review-bot",
					Name:        "review-bot",
					Role:        "worker",
					RuntimeKind: "codex",
					Source:      apitypes.HubTemplateSource{Name: config.DefaultOfficialHubRegistryName, Kind: "remote"},
					Workspace:   apitypes.HubTemplateWorkspace{Kind: "dir"},
				},
			})
		case "/api/v1/hub/templates/review-bot":
			_ = json.NewEncoder(w).Encode(apitypes.HubTemplate{
				ID:          "review-bot",
				Name:        "review-bot",
				Description: "code review helper",
				Role:        "worker",
				RuntimeKind: "codex",
				Source:      apitypes.HubTemplateSource{Name: config.DefaultOfficialHubRegistryName, Kind: "remote"},
				Workspace:   apitypes.HubTemplateWorkspace{Kind: "dir"},
			})
		case "/api/v1/hub/templates/review-bot/workspace.tar.gz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	store := NewRemoteStore(srv.URL, "")
	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "review-bot"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}

	item, err := store.Get(context.Background(), "review-bot")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got, want := item.Description, "code review helper"; got != want {
		t.Fatalf("Get().Description = %q, want %q", got, want)
	}
	if got, want := item.Role, TemplateRoleWorker; got != want {
		t.Fatalf("Get().Role = %q, want %q", got, want)
	}

	workspace, err := store.FetchWorkspace(context.Background(), "review-bot")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workspace.Path) })

	if got, want := workspace.Kind, WorkspaceKindDir; got != want {
		t.Fatalf("FetchWorkspace().Kind = %q, want %q", got, want)
	}
	data, err := os.ReadFile(filepath.Join(workspace.Path, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read extracted AGENTS.md: %v", err)
	}
	if got, want := string(data), "hello"; got != want {
		t.Fatalf("AGENTS.md = %q, want %q", got, want)
	}
}

func TestDefaultStoreFactoryCreatesRemoteStore(t *testing.T) {
	t.Parallel()

	store, err := DefaultStoreFactory(config.HubRegistryConfig{
		Kind:  RegistryKindRemote,
		URL:   "https://hub.example.com",
		Token: "secret",
	})
	if err != nil {
		t.Fatalf("DefaultStoreFactory() error = %v", err)
	}
	if _, ok := store.(*RemoteStore); !ok {
		t.Fatalf("store type = %T, want *RemoteStore", store)
	}
}

func TestRemoteStoreGetJSONRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	oversized := make([]byte, defaultRemoteMaxJSONBytes+1)
	for i := range oversized {
		oversized[i] = ' '
	}
	oversized[0] = '['
	oversized[len(oversized)-1] = ']'

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/hub/templates" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(oversized)
	}))
	t.Cleanup(srv.Close)

	store := NewRemoteStore(srv.URL, "")
	_, err := store.List(context.Background())
	if err == nil {
		t.Fatal("List() error = nil, want oversized response error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("List() error = %v, want response size limit message", err)
	}
}

func TestDefaultStoreFactoryRemoteRequiresURL(t *testing.T) {
	t.Parallel()

	_, err := DefaultStoreFactory(config.HubRegistryConfig{Kind: RegistryKindRemote})
	if !errors.Is(err, ErrRegistryURLRequired) {
		t.Fatalf("DefaultStoreFactory() error = %v, want ErrRegistryURLRequired", err)
	}
}
