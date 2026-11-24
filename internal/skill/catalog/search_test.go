package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"csgclaw/internal/skill/remote"
)

func TestDiscoveryOrderAndIndependentFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	never := func(context.Context, string) ([]Candidate, error) { t.Error("unexpected lookup"); return nil, nil }
	local := func(context.Context, string) ([]Candidate, error) { return []Candidate{{Name: "local"}}, nil }
	result, err := Search(ctx, "local", "", local, never, never)
	if err != nil || len(result.Candidates) != 1 || len(result.Sources) != 1 {
		t.Fatalf("local: %+v %v", result, err)
	}
	items := Match([]Candidate{{Name: "demo", Key: "demo", URL: "https://skills.sh/org/repo/demo-old"}, {Name: "demo", Key: "demo", URL: "https://skills.sh/org/repo/demo"}}, "https://skills.sh/org/repo/demo")
	if len(items) != 1 || items[0].URL != "https://skills.sh/org/repo/demo" {
		t.Fatalf("explicit origin match: %+v", items)
	}
	empty := func(context.Context, string) ([]Candidate, error) { return nil, nil }
	started := make(chan struct{})
	arrived := make(chan struct{})
	site := func(ctx context.Context, _ string) ([]Candidate, error) {
		close(started)
		select {
		case <-arrived:
			return nil, remote.ErrAgenticHubSignInRequired
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	public := func(ctx context.Context, _ string) ([]Candidate, error) {
		close(arrived)
		select {
		case <-started:
			return []Candidate{{Key: "org/repo/demo"}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	result, err = Search(ctx, "demo", "", empty, site, public)
	if err != nil || len(result.Candidates) != 1 || result.Sources[Site] != "login_required" || result.Sources[Public] != "ok" {
		t.Fatalf("remote: %+v %v", result, err)
	}
}

func TestPublicCatalogSeparatesNoMatchesFromFailure(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
		count      int
	}{
		{200, `{"skills":[]}`, "ok", 0}, {200, `{"skills":[{"id":"one/repo/demo","name":"Demo","source":"one/repo"}]}`, "ok", 1},
		{200, `{}`, "unavailable", 0}, {429, `{}`, "rate_limited", 0}, {503, `{}`, "unavailable", 0},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("q") != "demo" || r.Header.Get("Authorization") != "" {
				t.Error("unexpected query or credential")
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		out, err := Search(t.Context(), "demo", Public, nil, nil, func(ctx context.Context, q string) ([]Candidate, error) {
			return searchPublic(ctx, server.Client(), server.URL, q)
		})
		server.Close()
		if err != nil || out.Sources[Public] != tc.want || len(out.Candidates) != tc.count {
			t.Fatalf("%+v %v", out, err)
		}
	}
}
