package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAccessLogCapturesImplicitOK(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	handler := accessLog(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?ready=1", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("User-Agent", "test-agent")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, "level=INFO") {
		t.Fatalf("expected info log, got %q", logLine)
	}
	if !strings.Contains(logLine, "method=GET") {
		t.Fatalf("expected method in log, got %q", logLine)
	}
	if !strings.Contains(logLine, "uri=\"/api/v1/users?ready=1\"") {
		t.Fatalf("expected uri in log, got %q", logLine)
	}
	if !strings.Contains(logLine, "status=200") {
		t.Fatalf("expected status in log, got %q", logLine)
	}
	if !strings.Contains(logLine, "bytes=2") {
		t.Fatalf("expected bytes in log, got %q", logLine)
	}
}

func TestAccessLogCapturesExplicitStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	handler := accessLog(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/users/u-1", nil))

	logLine := buf.String()
	if !strings.Contains(logLine, "status=204") {
		t.Fatalf("expected explicit status in log, got %q", logLine)
	}
	if !strings.Contains(logLine, "bytes=0") {
		t.Fatalf("expected zero bytes in log, got %q", logLine)
	}
}

func TestAccessLogSkipsAgentPolling(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	handler := accessLog(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents?poll=1", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "[]" {
		t.Fatalf("response = %d %q, want 200 []", rec.Code, rec.Body.String())
	}
	if got := buf.String(); got != "" {
		t.Fatalf("polling request was logged: %q", got)
	}
}

func TestAccessLogSkipsHealthzPolling(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	handler := accessLog(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz?ready=1", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("response = %d %q, want 200 ok", rec.Code, rec.Body.String())
	}
	if got := buf.String(); got != "" {
		t.Fatalf("healthz request was logged: %q", got)
	}
}

func TestAccessLogPreservesFlusher(t *testing.T) {
	handler := accessLog(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Fatal("expected flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": connected\n\n"))
		w.(http.Flusher).Flush()
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/events", nil))

	if body := rec.Body.String(); body != ": connected\n\n" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestAccessLogColorsTerminalOutput(t *testing.T) {
	origStderr := accessLogStderr
	origWriter := accessLogWriter
	origIsTerminalFD := accessLogIsTerminalFD
	accessLogStderr = os.Stderr
	var buf bytes.Buffer
	accessLogWriter = &buf
	accessLogIsTerminalFD = func(int) bool { return true }
	defer func() {
		accessLogStderr = origStderr
		accessLogWriter = origWriter
		accessLogIsTerminalFD = origIsTerminalFD
	}()

	handler := accessLog(slog.New(slog.NewTextHandler(&buf, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("User-Agent", "demo-agent")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, "\x1b[36mGET\x1b[0m /missing \x1b[33m404\x1b[0m") {
		t.Fatalf("expected colored method in log, got %q", logLine)
	}
	if !strings.Contains(logLine, "\x1b[90mbytes\x1b[0m=0") {
		t.Fatalf("expected colored bytes label in log, got %q", logLine)
	}
	if !strings.Contains(logLine, "\x1b[90mremote\x1b[0m=127.0.0.1:1234") {
		t.Fatalf("expected remote address in log, got %q", logLine)
	}
}
