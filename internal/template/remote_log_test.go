package template

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRemoteTemplateRequestLogsResponseAndRequestID(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-credential" {
			t.Error("request authentication changed")
		}
		w.Header().Set("X-Request-ID", "qa-request-123")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"missing template","access_token":"response-credential","echo":"test-credential"}`)
	}))
	defer server.Close()
	store := NewRemoteStore(server.URL, "test-credential")
	body, status, err := store.request(context.Background(), http.MethodGet, server.URL+"/api/v1/codes/agentichub/Opencsg-guide?ref=main&token=query-credential", 4096)
	if err != nil || status != 404 || !strings.Contains(string(body), "response-credential") {
		t.Fatalf("response changed: status=%d err=%v body=%s", status, err, body)
	}
	got := logs.String()
	for _, want := range []string{"remote template request failed", "qa-request-123", "Opencsg-guide", "ref=main", "missing template", `"status":404`, `"level":"WARN"`} {
		if !strings.Contains(got, want) {
			t.Errorf("logs missing %q: %s", want, got)
		}
	}
	for _, secret := range []string{"test-credential", "response-credential", "query-credential"} {
		if strings.Contains(got, secret) {
			t.Errorf("logs leaked %q", secret)
		}
	}
}

func TestRemoteTemplateLogRedactionAndTruncation(t *testing.T) {
	store := NewRemoteStore("https://example.com", "known-credential")
	for _, input := range []string{
		`{"nested":{"app_secret":"a secret with spaces"},"password":{"value":"hidden-object"}}`,
		"Authorization: Bearer unknown-credential\napp_secret = 'a secret with spaces'",
		strings.Repeat("x", remoteLogBodyLimit-5) + ` token="cross-boundary-secret"`,
	} {
		got := store.remoteLogText(input)
		for _, secret := range []string{"a secret with spaces", "hidden-object", "unknown-credential", "cross-boundary-secret"} {
			if strings.Contains(got, secret) {
				t.Errorf("leaked %q: %s", secret, got)
			}
		}
	}
	u, _ := url.Parse("https://user:pass@example.com/path?ref=main&api_key=hidden-query")
	if got := store.remoteLogURL(u); strings.Contains(got, "pass") || strings.Contains(got, "hidden-query") || !strings.Contains(got, "ref=main") {
		t.Fatalf("unsafe URL: %s", got)
	}
	if got := store.remoteLogText(strings.Repeat("x", 5000)); !strings.HasSuffix(got, "[truncated]") {
		t.Fatal("large body was not marked truncated")
	}
}

func TestRemoteTemplateSuccessfulRequestsDoNotLog(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "archive-request")
		io.WriteString(w, "PK-binary-content")
	}))
	defer server.Close()
	store := NewRemoteStore(server.URL, "")
	body, status, err := store.requestAccept(context.Background(), http.MethodGet, server.URL, "application/zip", 1024)
	if err != nil || status != 200 || string(body) != "PK-binary-content" {
		t.Fatalf("archive response changed: %d %v %q", status, err, body)
	}
	if _, _, err := store.request(context.Background(), http.MethodGet, server.URL+"/blob/agent.toml", 1024); err != nil {
		t.Fatal(err)
	}
	if got := logs.String(); got != "" {
		t.Fatalf("successful requests produced logs: %s", got)
	}
}

func TestRemoteTemplateOversizedErrorBodyIsOmitted(t *testing.T) {
	store := NewRemoteStore("https://example.com", "known-credential")
	for _, input := range []string{
		`{"error":"` + strings.Repeat("x", remoteLogBodyLimit) + `","token":"secret"}`,
		strings.Repeat("x", remoteLogBodyLimit-5) + "known-credential",
		strings.Repeat("x", 4*1024*1024),
	} {
		if got := store.remoteLogBody([]byte(input)); got != remoteLogOversizedBody {
			t.Fatalf("oversized body was processed: %q", got)
		}
		if got := store.remoteLogText(input); got != remoteLogOversizedBody {
			t.Fatalf("oversized text was processed: %q", got)
		}
	}
	if got := store.remoteLogBody([]byte(`{"error":"template missing"}`)); !strings.Contains(got, "template missing") {
		t.Fatalf("small error body lost: %s", got)
	}
}
