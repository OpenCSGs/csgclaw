package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestClientSearch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "postgres" {
			t.Fatalf("q = %q", got)
		}
		_, _ = w.Write([]byte(`{"results":[{"slug":"pg-backup","displayName":"PG Backup","summary":"backup","version":"1.0.0","score":0.9}]}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(config.SkillConfig{BaseURL: srv.URL, NonSuspiciousOnly: true}, srv.Client())
	items, err := client.Search(context.Background(), "postgres", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || items[0].Slug != "pg-backup" {
		t.Fatalf("items = %#v", items)
	}
}

func TestClientGetSkillPreservesSlugCase(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills/AIWizards--gitlab-fullstack-pro" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"skill":{"slug":"AIWizards--gitlab-fullstack-pro","displayName":"Demo"},"latestVersion":{"version":"1.0.0"}}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(config.SkillConfig{BaseURL: srv.URL}, srv.Client())
	detail, err := client.Get(context.Background(), "AIWizards--gitlab-fullstack-pro")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.Skill.Slug != "AIWizards--gitlab-fullstack-pro" {
		t.Fatalf("slug = %q", detail.Skill.Slug)
	}
}

func TestClientGetSkill(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills/demo-skill" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill","displayName":"Demo"},"latestVersion":{"version":"1.0.0"}}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(config.SkillConfig{BaseURL: srv.URL}, srv.Client())
	detail, err := client.Get(context.Background(), "demo-skill")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.LatestVersion == nil || detail.LatestVersion.Version != "1.0.0" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestClientDownloadPathFallback(t *testing.T) {
	t.Parallel()

	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/download/demo-skill":
			http.NotFound(w, r)
		case "/api/v1/download":
			if got := r.URL.Query().Get("slug"); got != "demo-skill" {
				t.Fatalf("slug = %q", got)
			}
			_, _ = w.Write([]byte("zip-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(config.SkillConfig{BaseURL: srv.URL}, srv.Client())
	body, err := client.Download(context.Background(), "demo-skill", "1.0.0", "")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if string(body) != "zip-bytes" {
		t.Fatalf("body = %q", body)
	}
	if len(calls) < 2 {
		t.Fatalf("calls = %v, want path then query download", calls)
	}
}

func TestClientGetNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(config.SkillConfig{BaseURL: srv.URL}, srv.Client())
	_, err := client.Get(context.Background(), "missing")
	if err == nil || !IsNotFound(err) {
		t.Fatalf("Get() error = %v, want ErrSkillNotFound", err)
	}
}

func TestClientGetVersionPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills/demo-skill/versions/1.0.0" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill"},"version":{"version":"1.0.0","changelog":"init"}}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(config.SkillConfig{BaseURL: srv.URL}, srv.Client())
	detail, err := client.GetVersion(context.Background(), "demo-skill", "1.0.0")
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if detail.Version.Version != "1.0.0" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestClientSearchRegistryUnavailable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(config.SkillConfig{BaseURL: srv.URL}, srv.Client())
	_, err := client.Search(context.Background(), "git", 10)
	if err == nil || !strings.Contains(err.Error(), "registry API not found") {
		t.Fatalf("Search() error = %v, want registry unavailable", err)
	}
}
