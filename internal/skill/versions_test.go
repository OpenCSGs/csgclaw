package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListVersionsFromSkillGet(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/demo-skill/versions":
			http.NotFound(w, r)
		case "/api/v1/skills/demo-skill":
			_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill","displayName":"Demo"},"latestVersion":{"version":"1.0.2","createdAt":3},"versions":[{"version":"1.0.2","createdAt":3,"changelog":"c2"},{"version":"1.0.1","createdAt":2,"changelog":"c1"},{"version":"1.0.0","createdAt":1,"changelog":"c0"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(singleRegistryConfig(srv.URL), srv.Client())
	versions, err := client.ListVersions(context.Background(), "demo-skill", 10)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("len = %d, want 3", len(versions))
	}
	if versions[0].Version != "1.0.2" {
		t.Fatalf("first version = %q, want 1.0.2", versions[0].Version)
	}
}

func TestClientListVersionsFromVersionsEndpoint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills/demo-skill/versions" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"version":"2.0.0","createdAt":2},{"version":"1.0.0","createdAt":1}]}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(singleRegistryConfig(srv.URL), srv.Client())
	versions, err := client.ListVersions(context.Background(), "demo-skill", 10)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 2 || versions[0].Version != "2.0.0" {
		t.Fatalf("versions = %#v", versions)
	}
}

func TestServiceListVersions(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills/demo-skill" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"skill":{"slug":"demo-skill","displayName":"Demo"},"versions":[{"version":"1.0.0","createdAt":1}]}`))
	}))
	t.Cleanup(srv.Close)

	svc := NewService(singleRegistryConfig(srv.URL), srv.Client())
	list, err := svc.ListVersions(context.Background(), "demo-skill", "", 10)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if list.Registry != RegistryOpenCSG || len(list.Versions) != 1 {
		t.Fatalf("list = %#v", list)
	}
}
