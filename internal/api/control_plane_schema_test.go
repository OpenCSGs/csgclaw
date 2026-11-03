package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"github.com/google/jsonschema-go/jsonschema"
)

type controlPlaneSchemaContent struct {
	Content map[string]struct {
		Schema json.RawMessage `json:"schema"`
	} `json:"content"`
	Required bool `json:"required"`
}

type controlPlaneSchemaDocument struct {
	Paths map[string]map[string]struct {
		RequestBody controlPlaneSchemaContent            `json:"requestBody"`
		Responses   map[string]controlPlaneSchemaContent `json:"responses"`
	} `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

func loadControlPlaneSchemas(t *testing.T) controlPlaneSchemaDocument {
	t.Helper()
	data, err := os.ReadFile("../../api/openapi/control-plane.json")
	if err != nil {
		t.Fatal(err)
	}
	var document controlPlaneSchemaDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func (document controlPlaneSchemaDocument) validate(t *testing.T, content controlPlaneSchemaContent, body []byte) {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(content.Content["application/json"].Schema, &schema); err != nil {
		t.Fatal(err)
	}
	// Move OpenAPI components into JSON Schema's standard definitions container.
	schema["$defs"] = document.Components.Schemas
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("#/components/schemas/"), []byte("#/$defs/"))
	var shape jsonschema.Schema
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatal(err)
	}
	resolved, err := shape.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(value); err != nil {
		t.Fatalf("HTTP payload does not match OpenAPI: %v\n%s", err, body)
	}
}

func TestControlPlaneSchemaModelProviderCheck(t *testing.T) {
	document := loadControlPlaneSchemas(t)
	operation := document.Paths["/model-providers/{id}/check"]["post"]
	if !operation.RequestBody.Required {
		t.Fatal("model provider check requires a JSON request body")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected model check request: %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]string{{"id": "chat"}}})
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, path)
	h := newModelProviderTestHandler(t, path, nil)
	for _, test := range []struct {
		name, body, status string
	}{
		{"default request", `{}`, "failed"},
		{"connection override", `{"base_url":"` + upstream.URL + `/v1","api_key":"test-key"}`, "connected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document.validate(t, operation.RequestBody, []byte(test.body))
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers/draft/check", strings.NewReader(test.body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
			}
			document.validate(t, operation.Responses["200"], rec.Body.Bytes())
			var response struct{ Status string }
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response.Status != test.status {
				t.Fatalf("status=%s, want %s; error=%v", response.Status, test.status, err)
			}
		})
	}
}

func TestControlPlaneSchemaModelProviderCatalog(t *testing.T) {
	document := loadControlPlaneSchemas(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, path)
	h := newModelProviderTestHandler(t, path, nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/model-providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	document.validate(t, document.Paths["/model-providers"]["get"].Responses["200"], rec.Body.Bytes())
}

func TestControlPlaneSchemaAgentMCPServers(t *testing.T) {
	document := loadControlPlaneSchemas(t)
	for _, test := range []struct {
		name    string
		servers map[string]any
	}{
		{"unmanaged", nil},
		{"empty", map[string]any{}},
		{"configured", map[string]any{"search": map[string]any{"command": "search"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-schema", Name: "schema", RuntimeKind: agent.RuntimeKindCodex, RuntimeName: agent.RuntimeNameCodex, Role: agent.RoleWorker, MCPServers: test.servers}}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
			h := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-schema/mcp-servers", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
			}
			document.validate(t, document.Paths["/agents/{id}/mcp-servers"]["get"].Responses["200"], rec.Body.Bytes())
		})
	}
}
