package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenCSGMCPGatewayProxyUsesCurrentUserToken(t *testing.T) {
	originalLoader := loadKnowledgeBaseConnection
	originalClient := openCSGMCPGatewayHTTPClient
	t.Cleanup(func() {
		loadKnowledgeBaseConnection = originalLoader
		openCSGMCPGatewayHTTPClient = originalClient
	})
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		return knowledgeBaseConnection{
			AIGatewayBaseURL:  "https://gateway.example.test/v1",
			CSGHubAccessToken: "current-user-token",
		}, nil
	}
	openCSGMCPGatewayHTTPClient = &http.Client{Transport: knowledgeBaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "https://gateway.example.test/v1/gateway/mcp"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if got, want := req.Header.Get("Authorization"), "Bearer current-user-token"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		if req.Header.Get("Cookie") != "" {
			t.Fatal("cookie was forwarded")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "Mcp-Session-Id": []string{"session-1"}},
			Body:       io.NopCloser(strings.NewReader("event: message\ndata: {}\n\n")),
		}, nil
	})}

	handler := &Handler{serverAccessToken: "internal-token"}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/opencsg-mcp-gateway/mcp", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer internal-token")
	request.Header.Set("Cookie", "session=browser")
	recorder := httptest.NewRecorder()
	handler.handleOpenCSGMCPGatewayProxy(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Mcp-Session-Id"); got != "session-1" {
		t.Fatalf("Mcp-Session-Id = %q", got)
	}
}

func TestOpenCSGMCPGatewayProxyRejectsInvalidInternalToken(t *testing.T) {
	handler := &Handler{serverAccessToken: "internal-token"}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/opencsg-mcp-gateway/mcp", nil)
	recorder := httptest.NewRecorder()
	handler.handleOpenCSGMCPGatewayProxy(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
