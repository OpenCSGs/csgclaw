package api

import (
	"csgclaw/internal/apps"
	"encoding/json"
	"net/http"
	"testing"
)

func TestGlobalAppResourceAPIAndAgentBindings(t *testing.T) {
	h, alice, bob, aliceToken, _ := newAppPlatformAuthFixture(t)
	created := appAuthRequest(t, h, http.MethodPost, "/api/v1/app-resources", `{"app_id":"gitlab","name":"Shared","credentials":{"token":"global-only-secret"}}`, "", nil)
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
		response := appAuthRequest(t, h, http.MethodPost, "/api/v1/agents/"+id+"/apps", `{"resource_id":"`+resource.ResourceID+`"}`, "", nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("bind: %d %s", response.Code, response.Body)
		}
	}
	response := appAuthRequest(t, h, http.MethodGet, "/api/v1/app-resources", "", "", nil)
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
	for _, path := range []string{"/api/v1/app-resources", "/api/v1/app-resources/" + resource.ResourceID} {
		if response := appAuthRequest(t, h, http.MethodGet, path, "", aliceToken, nil); response.Code != http.StatusForbidden {
			t.Fatal("Agent enumerated global resource management")
		}
	}
	binding := inventory.Items[0].Bindings[0]
	response = appAuthRequest(t, h, http.MethodPatch, "/api/v1/agents/"+binding.AgentID+"/apps/"+binding.InstallationID, `{"credentials":{"token":"override"}}`, "", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatal("Agent binding accepted shared credential mutation")
	}
	response = appAuthRequest(t, h, http.MethodDelete, "/api/v1/app-resources/"+resource.ResourceID, "", "", nil)
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
