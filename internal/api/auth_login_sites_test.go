package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLoginSites(t *testing.T) {
	const site = `{"sites":[{"id":"main","url":"https://opencsg.com","icon":"data:image/png;base64,AAAA","locales":{"en":{"title":"OpenCSG Main Site"},"zh":{"title":"OpenCSG主站"}}}]}`
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{name: "public catalog", status: 200, body: site},
		{name: "upstream error", status: 503, wantError: true},
		{name: "invalid catalog", status: 200, body: `{"sites":[{"id":"main","url":"javascript:alert(1)"}]}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/relay/v1/login-sites" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("sent credentials with a pre-login request")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer proxy.Close()
			got, err := fetchLoginSites(t.Context(), proxy.Client(), proxy.URL)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error %v", err)
			}
			if err == nil && (len(got.Sites) != 1 || got.Sites[0].ID != "main") {
				t.Fatalf("unexpected catalog %+v", got)
			}
		})
	}
}
