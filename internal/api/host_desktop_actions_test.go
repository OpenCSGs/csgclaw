package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestDisabledHostDesktopActionsRejectLoopbackManagementRequests(t *testing.T) {
	h := &Handler{serverAccessToken: "test-token"}
	h.SetHostDesktopActionsDisabled(true)
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workspace/open", strings.NewReader(`{"path":".","target":"system"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	if targets := h.workspaceOpenTargets(req, ".", true); len(targets) != 0 {
		t.Fatalf("host actions advertised: %v", targets)
	}
	rec := httptest.NewRecorder()
	h.handleWorkspaceOpen(rec, req, func(string) (workspaceLocation, error) {
		t.Fatal("disabled host action reached filesystem resolution")
		return workspaceLocation{}, nil
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want forbidden", rec.Code)
	}
}

func TestDisabledHostDesktopActionsRejectDirectoryPicker(t *testing.T) {
	h := &Handler{
		serverAccessToken: "test-token",
		localDirectoryPicker: func(context.Context) (string, error) {
			t.Fatal("disabled host action invoked the native directory picker")
			return "", nil
		},
	}
	h.SetHostDesktopActionsDisabled(true)
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/local/directory-picker", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want forbidden; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostDesktopActionsControlDirectoryPickerAvailability(t *testing.T) {
	h := &Handler{}
	for _, disabled := range []bool{false, true} {
		h.SetHostDesktopActionsDisabled(disabled)
		got := h.bootstrapConfigView(context.Background(), config.Config{})
		want := !disabled && localDirectoryPickerAvailable()
		if got.DirectoryPickerAvailable != want {
			t.Fatalf("disabled=%t: directory picker available=%t, want %t", disabled, got.DirectoryPickerAvailable, want)
		}
	}
}
