package apps

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func healthProxy(t *testing.T, intercept func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	upstream := upstreamServer(t)
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !intercept(w, r) {
			proxy.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestHealthChecksDeleteRemoteSessions(t *testing.T) {
	var initialized, deleted atomic.Int32
	proxy := healthProxy(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete && r.Header.Get("Mcp-Session-Id") != "" {
			deleted.Add(1)
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			if bytes.Contains(body, []byte(`"method":"initialize"`)) {
				initialized.Add(1)
			}
		}
		return false
	})
	s := newTestService(t, Options{})
	item, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: "Session cleanup", Config: Config{URL: proxy.URL, AuthMode: "bearer"}, Credentials: Credentials{Token: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		checked, err := s.CheckResource(t.Context(), item.InstallationID, true)
		if err != nil || checked.Status != "connected" {
			t.Fatalf("health=%s err=%v", checked.Status, err)
		}
	}
	if initialized.Load() != 3 || deleted.Load() != 3 {
		t.Fatalf("initialized=%d deleted=%d", initialized.Load(), deleted.Load())
	}
}

func TestSlowResourceDoesNotBlockOtherHealthRefreshes(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{}, 1)
	var slowCalls atomic.Int32
	slow := healthProxy(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost {
			return false
		}
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if !bytes.Contains(body, []byte(`"method":"tools/list"`)) {
			return false
		}
		slowCalls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return false
		case <-r.Context().Done():
			return true
		}
	})
	var unauthorized atomic.Bool
	fast := healthProxy(t, func(w http.ResponseWriter, r *http.Request) bool {
		if unauthorized.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		return false
	})
	s := newTestService(t, Options{})
	create := func(name, endpoint string) Installation {
		t.Helper()
		item, err := s.Create(t.Context(), "", CreateRequest{AppID: "gitlab", Name: name, Config: Config{URL: endpoint, AuthMode: "bearer"}, Credentials: Credentials{Token: "fixture"}})
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	fastItem := create("Fast", fast.URL)
	if _, err := s.CheckResource(t.Context(), fastItem.InstallationID, false); err != nil {
		t.Fatal(err)
	}
	create("Slow", slow.URL)
	if _, err := s.ListResources(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow discovery did not start")
	}
	unauthorized.Store(true)
	s.mu.Lock()
	s.entries[fastItem.InstallationID].resourceHealth.checkedAt = time.Now().Add(-2 * resourceHealthTTL)
	s.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for {
		items, err := s.ListResources(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.InstallationID == fastItem.InstallationID && item.Status == "authorization_required" {
				if slowCalls.Load() != 1 {
					t.Fatalf("duplicate slow checks: %d", slowCalls.Load())
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("slow resource prevented expired fast health from refreshing")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type cleanupRoundTripper func(*http.Request) (*http.Response, error)

func (f cleanupRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSessionCleanupHasIndependentDeadline(t *testing.T) {
	lifetime, stop := context.WithCancel(t.Context())
	stop()
	var cleanupContext context.Context
	transport := &headerTransport{lifetime: lifetime, origin: "https://example.test", base: cleanupRoundTripper(func(req *http.Request) (*http.Response, error) {
		cleanupContext = req.Context()
		if req.Context().Err() != nil {
			t.Fatal("cleanup inherited cancellation")
		}
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("cleanup is not bounded")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})}
	req, _ := http.NewRequestWithContext(lifetime, http.MethodDelete, "https://example.test", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if cleanupContext.Err() != context.Canceled {
		t.Fatal("response close did not release cleanup context")
	}
	req.Method = http.MethodPost
	if _, err := transport.RoundTrip(req); err != context.Canceled {
		t.Fatalf("cancelled POST was not rejected: %v", err)
	}
}
