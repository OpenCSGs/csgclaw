package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/uiassets"
)

func TestRunWithOptionalPresentation(t *testing.T) {
	for _, ui := range []string{"", "workbuddy", "alternative UI"} {
		t.Run("ui="+ui, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := Options{Listener: listener, Context: ctx, AccessToken: "test-access-token"}
			if ui != "" {
				opts.UI = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, ui) })
			}
			done := make(chan error, 1)
			go func() { done <- Run(opts) }()
			t.Cleanup(func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(6 * time.Second):
					t.Error("server did not shut down")
				}
			})
			client := &http.Client{Timeout: 5 * time.Second}
			uiStatus := http.StatusOK
			if ui == "" {
				uiStatus = http.StatusNotFound
			}
			for _, tc := range []struct {
				path, token string
				status      int
			}{
				{"/healthz", "", http.StatusOK},
				{"/api/v1/version", "test-access-token", http.StatusOK},

				{"/api/unknown", "", http.StatusNotFound},
				{"/", "", uiStatus},
				{"/workspace", "", uiStatus},
			} {
				req, _ := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+tc.path, nil)
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if res.StatusCode != tc.status {
					t.Fatalf("%s: status=%d body=%s", tc.path, res.StatusCode, body)
				}
				if ui != "" && (tc.path == "/" || tc.path == "/workspace") && strings.TrimSpace(string(body)) != ui {
					t.Fatalf("presentation was not injected: %s", body)
				}
			}
		})
	}
}

func TestBothUIsUseTheExistingBackendRouter(t *testing.T) {
	page := func(body string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
	}
	ui := uiassets.WithCustom(page("existing UI"), page("custom UI"))
	router := newHandler(Options{UI: ui, AccessToken: "ui-test-token"}).Routes()
	router.Handle("/*", apiAwareFallbackHandler(ui))
	s := httptest.NewServer(router)
	defer s.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/", "existing UI", 200},
		{"/workspace", "existing UI", 200},
		{uiassets.CustomPath, "custom UI", 200},
		{"/api/v1/version", `"version"`, 200},
		{"/api/unknown", "404 page not found", 404},
	} {
		req, _ := http.NewRequest("GET", s.URL+tc.path, nil)
		req.Header.Set("Authorization", "Bearer ui-test-token")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != tc.status || !strings.Contains(string(body), tc.body) {
			t.Fatalf("%s: status=%d body=%s err=%v", tc.path, res.StatusCode, body, err)
		}
	}
}
