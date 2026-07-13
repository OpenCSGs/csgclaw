package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/mcp"
)

func TestNewHandlerWiresMCPService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	handler := newHandler(Options{MCP: mcp.NewService()})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers", nil)
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
