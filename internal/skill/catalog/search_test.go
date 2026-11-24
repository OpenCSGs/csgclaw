package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"csgclaw/internal/skill/remote"
)

func TestDiscoveryStopsAtFirstHit(t *testing.T) {
	for _, tc := range []struct {
		hit   string
		order []string
	}{
		{Installed, []string{Installed}}, {Global, []string{Installed, Global}}, {Site, []string{Installed, Global, Site}}, {Public, []string{Installed, Global, Site, Public}}, {"", []string{Installed, Global, Site, Public}},
	} {
		var calls []string
		lookup := func(source string) Lookup {
			return func(context.Context, string) ([]Candidate, error) {
				calls = append(calls, source)
				if source == tc.hit {
					return []Candidate{{Name: "demo", Source: source}}, nil
				}
				return nil, nil
			}
		}
		out, err := Search(t.Context(), "demo", "", Lookups{lookup(Installed), lookup(Global), lookup(Site), lookup(Public)})
		if err != nil || !slices.Equal(calls, tc.order) || tc.hit != "" && (len(out.Candidates) != 1 || out.Candidates[0].Source != tc.hit) {
			t.Fatalf("hit=%q order=%v result=%+v err=%v", tc.hit, calls, out, err)
		}
	}
	items := Match([]Candidate{{Name: "demo", Key: "demo", URL: "https://skills.sh/org/repo/demo-old"}, {Name: "demo", Key: "demo", URL: "https://skills.sh/org/repo/demo"}}, "https://skills.sh/org/repo/demo")
	if len(items) != 1 || items[0].URL != "https://skills.sh/org/repo/demo" {
		t.Fatalf("explicit origin match: %+v", items)
	}
	result, err := Search(t.Context(), "internal/project", "", Lookups{Site: func(context.Context, string) ([]Candidate, error) { return nil, remote.ErrAgenticHubSignInRequired }, Public: func(context.Context, string) ([]Candidate, error) {
		t.Fatal("public lookup after a failed private lookup")
		return nil, nil
	}})
	if err != nil || result.Sources[Site] != "login_required" {
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
		out, err := Search(t.Context(), "demo", Public, Lookups{Public: func(ctx context.Context, q string) ([]Candidate, error) {
			return searchPublic(ctx, server.Client(), server.URL, q)
		}})
		server.Close()
		if err != nil || out.Sources[Public] != tc.want || len(out.Candidates) != tc.count {
			t.Fatalf("%+v %v", out, err)
		}
	}
}
