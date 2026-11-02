package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/config"
	hub "csgclaw/internal/template"
)

func TestHubVersionDeletion(t *testing.T) {
	service, err := hub.NewService(config.HubConfig{
		DefaultRegistry: "local", DefaultPublishRegistry: "local",
		Registries: []config.HubRegistryConfig{{Name: "local", Kind: hub.RegistryKindLocal, Path: t.TempDir(), Enabled: true}},
	}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatal(err)
	}
	spec := hub.PublishSpec{Name: "VersionBot", RuntimeKind: "codex", SourceAgentID: "agent-a", WorkspaceRef: hub.WorkspaceRef{Kind: hub.WorkspaceKindDir, Path: t.TempDir()}}
	for range 2 {
		if _, err := service.SaveAgentTemplate(context.Background(), spec); err != nil {
			t.Fatal(err)
		}
	}
	handler := &Handler{svc: mustNewService(t)}
	handler.SetHubService(service)
	for _, tc := range []struct {
		revision string
		status   int
	}{
		{"0", http.StatusBadRequest}, {"2", http.StatusBadRequest}, {"9", http.StatusNotFound}, {"1", http.StatusOK}, {"1", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/hub/templates/local.VersionBot/versions/"+tc.revision, nil))
		if response.Code != tc.status {
			t.Fatalf("delete V%s: status=%d body=%s", tc.revision, response.Code, response.Body.String())
		}
	}
	item, err := service.Get(context.Background(), "local.VersionBot")
	if err != nil || item.VersionCount != 1 || item.LatestRevision != 2 {
		t.Fatalf("item=%+v err=%v", item, err)
	}
	versions, err := service.Versions(context.Background(), "local.VersionBot")
	if err != nil || len(versions) != 1 || versions[0].Revision != 2 {
		t.Fatalf("versions=%+v err=%v", versions, err)
	}
}
