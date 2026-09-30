package api

import (
	"context"
	"csgclaw/internal/apps"
	"csgclaw/internal/connectors"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGlobalAppResourceAPIAndAgentBindings(t *testing.T) {
	upstream := appSaveMCPServer(t)
	h, alice, bob, aliceToken, _ := newAppPlatformAuthFixture(t)
	created := appAuthRequest(t, h, http.MethodPost, "/api/v1/connectors/resources", `{"app_id":"gitlab","name":"Shared","config":{"url":"`+upstream.URL+`","auth_mode":"none"},"credentials":{"token":"global-only-secret"}}`, "", nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create global: %d %s", created.Code, created.Body)
	}
	var resource apps.Installation
	if err := json.Unmarshal(created.Body.Bytes(), &resource); err != nil {
		t.Fatal(err)
	}
	if resource.AgentID != "" || resource.ResourceID == "" {
		t.Fatal("global resource has an Agent owner")
	}
	for _, id := range []string{alice.ID, bob.ID} {
		response := appAuthRequest(t, h, http.MethodPost, "/api/v1/agents/"+id+"/connectors", `{"resource_id":"`+resource.ResourceID+`"}`, "", nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("bind: %d %s", response.Code, response.Body)
		}
	}
	response := appAuthRequest(t, h, http.MethodGet, "/api/v1/connectors/resources", "", "", nil)
	if response.Code != 200 {
		t.Fatal(response.Code)
	}
	var inventory struct {
		Items []apps.Installation `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Items) != 1 || len(inventory.Items[0].Bindings) != 2 {
		t.Fatal("global usage list missing")
	}
	for _, path := range []string{"/api/v1/connectors/resources", "/api/v1/connectors/resources/" + resource.ResourceID} {
		if response := appAuthRequest(t, h, http.MethodGet, path, "", aliceToken, nil); response.Code != http.StatusForbidden {
			t.Fatal("Agent enumerated global resource management")
		}
	}
	binding := inventory.Items[0].Bindings[0]
	response = appAuthRequest(t, h, http.MethodPatch, "/api/v1/agents/"+binding.AgentID+"/connectors/"+binding.InstallationID, `{"credentials":{"token":"override"}}`, "", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatal("Agent binding accepted shared credential mutation")
	}
	response = appAuthRequest(t, h, http.MethodDelete, "/api/v1/connectors/resources/"+resource.ResourceID, "", "", nil)
	if response.Code != http.StatusNoContent {
		t.Fatal("global deletion failed")
	}
	for _, id := range []string{alice.ID, bob.ID} {
		items, _ := h.apps.List(t.Context(), id)
		if len(items) != 0 {
			t.Fatal("global deletion left binding")
		}
	}
}

func appSaveMCPServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "save-validation", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "inspect", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	upstream := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	t.Cleanup(upstream.Close)
	return upstream
}

func TestConnectionValidationPrecedesResourcePersistence(t *testing.T) {
	ctx := t.Context()
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "validated", Version: "1"}, nil)
	mcpServer.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-fixture" {
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(upstream.Close)
	h, alice, _, _, _ := newAppPlatformAuthFixture(t)
	body := func(token string) string {
		return `{"app_id":"llm-wiki","name":"Validated","config":{"url":"` + upstream.URL + `","auth_mode":"bearer"},"credentials":{"token":"` + token + `"}}`
	}
	failed := appAuthRequest(t, h, http.MethodPost, "/api/v1/connectors/resources", body("invalid-fixture"), "", nil)
	if failed.Code < 400 {
		t.Fatal("invalid connection saved")
	}
	items, _ := h.apps.List(ctx, "")
	if len(items) != 0 {
		t.Fatal("failed validation persisted a resource")
	}
	good := appAuthRequest(t, h, http.MethodPost, "/api/v1/connectors/resources", body("valid-fixture"), "", nil)
	if good.Code != 201 {
		t.Fatalf("valid save failed: %d %s", good.Code, good.Body)
	}
	var resource apps.Installation
	if err := json.Unmarshal(good.Body.Bytes(), &resource); err != nil {
		t.Fatal(err)
	}
	binding, err := h.apps.Bind(ctx, alice.ID, apps.BindRequest{ResourceID: resource.ResourceID, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	failed = appAuthRequest(t, h, http.MethodPatch, "/api/v1/connectors/resources/"+resource.ResourceID, `{"credentials":{"token":"invalid-fixture"}}`, "", nil)
	if failed.Code < 400 {
		t.Fatal("invalid credential update saved")
	}
	after, _ := h.apps.Get(ctx, "", resource.ResourceID)
	if !after.UpdatedAt.Equal(resource.UpdatedAt) {
		t.Fatal("failed save changed the resource")
	}
	current, _ := h.apps.Get(ctx, alice.ID, binding.InstallationID)
	if current.Status != "connected" || len(current.Tools) != 1 {
		t.Fatal("failed save revoked existing connection")
	}
	good = appAuthRequest(t, h, http.MethodPatch, "/api/v1/connectors/resources/"+resource.ResourceID, `{"config":{"url":"`+upstream.URL+`","auth_mode":"bearer"},"credentials":{}}`, "", nil)
	if good.Code != 200 {
		t.Fatal("saved credentials were not reused during validation")
	}
	upstream.CloseClientConnections()
	disabled := appAuthRequest(t, h, http.MethodPatch, "/api/v1/connectors/resources/"+resource.ResourceID, `{"enabled":false}`, "", nil)
	if disabled.Code != 200 {
		t.Fatal("disabling a resource required successful network validation")
	}
}

func TestGitLabDraftProbeValidatesPATWithoutSaving(t *testing.T) {
	upstream := appSaveMCPServer(t)
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" || r.Header.Get("PRIVATE-TOKEN") != "valid-draft" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": 1, "username": "draft-user"})
	}))
	t.Cleanup(gitlab.Close)
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	h.SetConnectorService(connectors.NewService(connectors.NewStore(filepath.Join(t.TempDir(), "state.json"))))
	before, existed, err := h.connectors.SnapshotGitLab()
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"invalid-draft", "valid-draft"} {
		body, _ := json.Marshal(apps.ProbeRequest{AppID: "gitlab", Config: apps.Config{URL: upstream.URL, AuthMode: "connector", GitLabBaseURL: gitlab.URL}, Credentials: apps.Credentials{Token: token}})
		response := appAuthRequest(t, h, http.MethodPost, "/api/v1/connectors/resources:probe", string(body), "", nil)
		if token == "valid-draft" {
			if response.Code != http.StatusOK {
				t.Fatalf("valid draft: %d %s", response.Code, response.Body)
			}
		} else if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "app_gitlab_authentication_failed") {
			t.Fatalf("invalid PAT accepted: %d %s", response.Code, response.Body)
		}
		after, exists, err := h.connectors.SnapshotGitLab()
		if err != nil || exists != existed || !reflect.DeepEqual(before, after) {
			t.Fatal("test persisted draft credentials")
		}
		resources, err := h.apps.List(t.Context(), "")
		if err != nil || len(resources) != 0 {
			t.Fatal("test persisted a resource")
		}
	}
}

func TestResourceViewReportsManagedGitLabTokenPresence(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	store := connectors.NewStore(filepath.Join(t.TempDir(), "state.json"))
	h.SetConnectorService(connectors.NewService(store))
	if err := store.SaveGitLab(connectors.State{Config: connectors.Config{BaseURL: "https://gitlab.example.com", AccessToken: "saved-secret"}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		base string
		want bool
	}{{"https://gitlab.example.com/", true}, {"https://other.example.com", false}} {
		item := h.connectorResourceView(apps.Installation{AppID: "gitlab", Config: apps.Config{AuthMode: "connector", GitLabBaseURL: tc.base}})
		if item.CredentialsSet["token"] != tc.want {
			t.Fatalf("token presence for %s = %v", tc.base, item.CredentialsSet["token"])
		}
		data, err := json.Marshal(item)
		if err != nil || strings.Contains(string(data), "saved-secret") {
			t.Fatal("resource response exposed the token")
		}
	}
	if _, err := h.connectors.DisconnectGitLab(); err != nil {
		t.Fatal(err)
	}
	item := h.connectorResourceView(apps.Installation{AppID: "gitlab", Config: apps.Config{AuthMode: "connector", GitLabBaseURL: "https://gitlab.example.com"}})
	if item.CredentialsSet["token"] {
		t.Fatal("removed credential still marked as saved")
	}
}

func TestGlobalConnectorNamesAreUniqueAcrossApplicationTypes(t *testing.T) {
	upstream := appSaveMCPServer(t)
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	create := func(appID, name string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(apps.CreateRequest{AppID: appID, Name: name, Config: apps.Config{URL: upstream.URL, AuthMode: "none"}})
		if err != nil {
			t.Fatal(err)
		}
		return appAuthRequest(t, h, http.MethodPost, "/api/v1/connectors/resources", string(body), "", nil)
	}
	first := create("gitlab", "Company workspace")
	if first.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", first.Code, first.Body)
	}
	for _, appID := range []string{"gitlab", "feishu"} {
		for _, name := range []string{"Company workspace", "  Company workspace  "} {
			result := create(appID, name)
			if result.Code != http.StatusConflict || !strings.Contains(result.Body.String(), `"code":"app_name_conflict"`) {
				t.Fatalf("duplicate %s %q: %d %s", appID, name, result.Code, result.Body)
			}
		}
	}
	second := create("feishu", "Personal workspace")
	if second.Code != http.StatusCreated {
		t.Fatalf("create second: %d %s", second.Code, second.Body)
	}
	var resource apps.Installation
	if err := json.Unmarshal(second.Body.Bytes(), &resource); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/connectors/resources/" + resource.InstallationID
	result := appAuthRequest(t, h, http.MethodPatch, path, `{"name":" Company workspace "}`, "", nil)
	if result.Code != http.StatusConflict || !strings.Contains(result.Body.String(), `"code":"app_name_conflict"`) {
		t.Fatalf("duplicate rename: %d %s", result.Code, result.Body)
	}
	current, err := h.apps.Get(t.Context(), "", resource.InstallationID)
	if err != nil || current.Name != "Personal workspace" {
		t.Fatalf("failed rename changed resource: %+v %v", current, err)
	}
	result = appAuthRequest(t, h, http.MethodPatch, path, `{"name":" Personal workspace "}`, "", nil)
	if result.Code != http.StatusOK {
		t.Fatalf("unchanged name: %d %s", result.Code, result.Body)
	}
}

func TestGlobalResourceAPIReportsActualConnectionState(t *testing.T) {
	upstream := appSaveMCPServer(t)
	h, alice, _, _, _ := newAppPlatformAuthFixture(t)
	created := appAuthRequest(t, h, http.MethodPost, "/api/v1/connectors/resources", `{"app_id":"gitlab","name":"Shared","config":{"url":"`+upstream.URL+`","auth_mode":"none"}}`, "", nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var resource apps.Installation
	if err := json.Unmarshal(created.Body.Bytes(), &resource); err != nil {
		t.Fatal(err)
	}
	if resource.Status != "not_connected" {
		t.Fatalf("configuration alone claimed a connection: %s", resource.Status)
	}
	bound := appAuthRequest(t, h, http.MethodPost, "/api/v1/agents/"+alice.ID+"/connectors", `{"resource_id":"`+resource.InstallationID+`","connect":true}`, "", nil)
	if bound.Code != http.StatusCreated {
		t.Fatalf("bind: %d %s", bound.Code, bound.Body)
	}
	var binding apps.Installation
	if err := json.Unmarshal(bound.Body.Bytes(), &binding); err != nil {
		t.Fatal(err)
	}
	check := func(status string) {
		t.Helper()
		response := appAuthRequest(t, h, http.MethodGet, "/api/v1/connectors/resources", "", "", nil)
		var inventory struct {
			Items []apps.Installation `json:"items"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &inventory) != nil || len(inventory.Items) != 1 || inventory.Items[0].Status != status {
			t.Fatalf("global status want %s: %d %s", status, response.Code, response.Body)
		}
	}
	check("connected")
	response := appAuthRequest(t, h, http.MethodPost, "/api/v1/agents/"+alice.ID+"/connectors/"+binding.InstallationID+"/disconnect", "", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("disconnect: %d %s", response.Code, response.Body)
	}
	check("not_connected")
}
