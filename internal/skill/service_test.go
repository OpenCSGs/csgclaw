package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func singleRegistryConfig(baseURL string) config.SkillConfig {
	return config.SkillConfig{
		BaseURL:            baseURL,
		OfficialBaseURLSet: true,
	}
}

func TestServiceSearchReturnsOpenCSGWithoutQueryingClawHub(t *testing.T) {
	t.Parallel()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"slug":"opencsg-only","displayName":"OpenCSG","score":1}]}`))
	}))
	t.Cleanup(primary.Close)

	officialCalled := false
	official := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		officialCalled = true
		http.NotFound(w, r)
	}))
	t.Cleanup(official.Close)

	svc := NewService(config.SkillConfig{
		BaseURL:            primary.URL,
		OfficialBaseURL:    official.URL,
		OfficialBaseURLSet: true,
	}, primary.Client())

	items, err := svc.Search(context.Background(), "demo", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || items[0].Registry != RegistryOpenCSG {
		t.Fatalf("items = %#v", items)
	}
	if officialCalled {
		t.Fatal("clawhub registry was queried despite opencsg hits")
	}
}

func TestServiceSearchFallsBackToClawHubWhenOpenCSGEmpty(t *testing.T) {
	t.Parallel()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(primary.Close)

	official := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"slug":"official-only","displayName":"Official","score":2}]}`))
	}))
	t.Cleanup(official.Close)

	svc := NewService(config.SkillConfig{
		BaseURL:            primary.URL,
		OfficialBaseURL:    official.URL,
		OfficialBaseURLSet: true,
	}, primary.Client())

	items, err := svc.Search(context.Background(), "demo", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || items[0].Slug != "official-only" || items[0].Registry != RegistryClawHub {
		t.Fatalf("items = %#v", items)
	}
}

func TestServiceInstallPinnedVersion(t *testing.T) {
	t.Parallel()

	zipBytes := mustServiceZip(t, map[string]string{
		"SKILL.md": "# Demo\n",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/demo-skill":
			_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill"},"latestVersion":{"version":"2.0.0"}}`))
		case "/api/v1/skills/demo-skill/versions/1.0.0":
			_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill"},"version":{"version":"1.0.0"}}`))
		case "/api/v1/download/demo-skill":
			if got := r.URL.Query().Get("version"); got != "1.0.0" {
				t.Fatalf("version = %q", got)
			}
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(singleRegistryConfig(srv.URL), srv.Client())
	result, err := svc.Install(context.Background(), "demo-skill", "1.0.0", "", t.TempDir(), false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Version != "1.0.0" {
		t.Fatalf("Version = %q, want 1.0.0", result.Version)
	}
}

func TestServiceInstallDefaultsToLatestVersion(t *testing.T) {
	t.Parallel()

	zipBytes := mustServiceZip(t, map[string]string{
		"SKILL.md": "# Demo\n",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/demo-skill":
			_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill"},"latestVersion":{"version":"2.0.0"},"versions":[{"version":"1.0.0","createdAt":1},{"version":"2.0.0","createdAt":2}]}`))
		case "/api/v1/download/demo-skill":
			if got := r.URL.Query().Get("version"); got != "2.0.0" {
				t.Fatalf("version = %q, want 2.0.0", got)
			}
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(singleRegistryConfig(srv.URL), srv.Client())
	result, err := svc.Install(context.Background(), "demo-skill", "", "", t.TempDir(), false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Version != "2.0.0" {
		t.Fatalf("Version = %q, want 2.0.0", result.Version)
	}
}

func TestServiceInstall(t *testing.T) {
	t.Parallel()

	zipBytes := mustServiceZip(t, map[string]string{
		"SKILL.md": "# Demo\n",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/demo-skill":
			_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill","displayName":"Demo"},"latestVersion":{"version":"1.0.0"}}`))
		case "/api/v1/download/demo-skill", "/api/v1/download":
			if r.URL.Path == "/api/v1/download" {
				if got := r.URL.Query().Get("slug"); got != "demo-skill" {
					t.Fatalf("slug = %q", got)
				}
			}
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(singleRegistryConfig(srv.URL), srv.Client())

	skillsRoot := t.TempDir()
	result, err := svc.Install(context.Background(), "demo-skill", "", "", skillsRoot, false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Version != "1.0.0" {
		t.Fatalf("Version = %q, want 1.0.0", result.Version)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "demo-skill", "SKILL.md")); err != nil {
		t.Fatalf("installed skill missing: %v", err)
	}
}

func TestServiceInstallRejectsUnsafeSlug(t *testing.T) {
	t.Parallel()

	svc := NewService(singleRegistryConfig("http://example.test"), http.DefaultClient)
	_, err := svc.Install(context.Background(), "../escape", "", "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), ErrWorkspacePathUnsafe.Error()) {
		t.Fatalf("Install() error = %v, want unsafe slug error", err)
	}
}

func TestServiceInstallUsesCanonicalSlugFromRegistry(t *testing.T) {
	t.Parallel()

	zipBytes := mustServiceZip(t, map[string]string{
		"SKILL.md": "# Demo\n",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/AIWizards--gitlab-fullstack-pro":
			_, _ = w.Write([]byte(`{"skill":{"slug":"AIWizards--gitlab-fullstack-pro","displayName":"Demo"},"latestVersion":{"version":"1.0.0"}}`))
		case "/api/v1/download/AIWizards--gitlab-fullstack-pro":
			_, _ = w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := NewService(singleRegistryConfig(srv.URL), srv.Client())
	skillsRoot := t.TempDir()
	result, err := svc.Install(context.Background(), "AIWizards--gitlab-fullstack-pro", "", "", skillsRoot, false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Slug != "AIWizards--gitlab-fullstack-pro" {
		t.Fatalf("Slug = %q", result.Slug)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "AIWizards--gitlab-fullstack-pro", "SKILL.md")); err != nil {
		t.Fatalf("installed skill missing: %v", err)
	}
}

func mustServiceZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}
