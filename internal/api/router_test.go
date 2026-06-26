package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/im"
)

func TestCsgclawUserPatchRouteRegistered(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: im.AdminUserID,
			Users: []im.User{{
				ID:   im.AdminUserID,
				Name: "admin",
				Role: "admin",
			}},
		}),
	}

	rec := httptest.NewRecorder()
	routes := srv.Routes()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/channels/csgclaw/users/admin", strings.NewReader(`{"description":"patched"}`))
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
