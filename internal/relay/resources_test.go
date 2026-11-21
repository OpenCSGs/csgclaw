package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/knowledgebase"
)

func TestCloudEndpointIsBoundToTrustedGatewayAndSandbox(t *testing.T) {
	for _, tc := range []struct {
		endpoint, gateway, sandbox string
		valid                      bool
	}{
		{"https://opencsg.com/aigateway/v1/sandboxes/instance", "https://ai.space.opencsg.com/v1", "instance", true},
		{"https://ai.space.opencsg.com/v1/sandboxes/instance", "https://opencsg.com/aigateway/v1/", "instance", true},
		{"https://opencsg.com/aigateway/v1/sandboxes/other", "https://ai.space.opencsg.com/v1", "instance", false},
		{"https://opencsg.com/v1/sandboxes/instance", "https://ai.space.opencsg.com/v1", "instance", false},
		{"http://opencsg.com/aigateway/v1/sandboxes/instance", "https://ai.space.opencsg.com/v1", "instance", false},
		{"https://opencsg.com.evil.test/aigateway/v1/sandboxes/instance", "https://ai.space.opencsg.com/v1", "instance", false},
		{"https://opencsg.com/aigateway/v1/sandboxes/instance", "https://gateway.test/v1", "instance", false},
		{"https://opencsg.com/aigateway/v1/sandboxes/instance?token=x", "https://ai.space.opencsg.com/v1", "instance", false},
		{"https://gateway.test/v1/sandboxes/instance", "https://gateway.test/v1", "instance", true},
		{"https://gateway.test/v1/sandboxes/allocation", "https://gateway.test/v1", "", true},
		{"https://other.test/v1/sandboxes/instance", "https://gateway.test/v1", "instance", false},
		{"https://gateway.test/v1/sandboxes/other", "https://gateway.test/v1", "instance", false},
		{"https://gateway.test/v1/sandboxes/instance/api", "https://gateway.test/v1", "instance", false},
		{"https://gateway.test/v1/sandboxes/instance?token=x", "https://gateway.test/v1", "instance", false},
		{"https://user:secret@gateway.test/v1/sandboxes/instance", "https://gateway.test/v1", "instance", false},
		{"https://gateway.test/v1/sandboxes/%2e%2e", "https://gateway.test/v1", "", false},
		{"https://gateway.test/v1/sandboxes/..", "https://gateway.test/v1", "", false},
		{"http://gateway.test/v1/sandboxes/instance", "https://gateway.test/v1", "instance", false},
	} {
		endpoint, err := validatedCloudEndpoint(tc.endpoint, tc.gateway, tc.sandbox)
		if (err == nil) != tc.valid {
			t.Errorf("endpoint %q valid=%v err=%v", tc.endpoint, tc.valid, err)
		}
		if tc.valid && endpoint != tc.endpoint {
			t.Errorf("endpoint = %q, want original URL %q", endpoint, tc.endpoint)
		}
	}
}

func TestCloudReadUsesRuntimeAndResourceChatStillUsesRelay(t *testing.T) {
	const id = "agt_123456789012345678901234"
	var server *httptest.Server
	var sourceCalls, relayCalls int
	capability := true
	status, instanceType, endpointOverride := "online", "csgclaw", ""
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing caller authentication")
		}
		switch r.URL.Path {
		case "/api/v1/agent/instances":
			endpoint := server.URL + "/v1/sandboxes/instance"
			if endpointOverride != "" {
				endpoint = endpointOverride
				if endpointOverride == "missing" {
					endpoint = ""
				}
			}
			writeTestJSON(w, 200, map[string]any{"data": []CommunityInstance{{ID: 1, Source: "platform", Type: instanceType, Kind: "runtime", ContentID: "instance", Endpoint: endpoint, Relay: &RelayRegistration{AgentID: id}}}})
		case "/api/v1/relay/v1/agents/" + id:
			capabilities := []string{}
			if capability {
				capabilities = append(capabilities, CapabilityResourceRefs)
			}
			writeTestJSON(w, 200, Agent{ID: id, SourceAgentID: "worker", Status: status, Capabilities: capabilities})
		case "/v1/sandboxes/instance/api/v1/agents/worker":
			sourceCalls++
			if r.Method != http.MethodGet {
				t.Error("profile must be read-only")
			}
			writeTestJSON(w, 200, map[string]any{"id": "worker", "name": "runtime-name", "description": "runtime-description", "instructions": "runtime-instructions", "runtime": map[string]any{"name": "codex"}, "model_config": map[string]any{"model_id": "model", "model_provider_id": "opencsg", "api_key": "private-secret", "env": map[string]string{"TOKEN": "private-secret"}}, "mcpServers": map[string]any{"private": "private-secret"}})
		case "/v1/sandboxes/instance/api/v1/agents/worker/resources":
			sourceCalls++
			if r.Method != http.MethodGet {
				t.Error("resource discovery must be read-only")
			}
			writeTestJSON(w, 200, ResourcePage{Items: []apitypes.AgentResourceSummary{{ResourceSnapshot: apitypes.ResourceSnapshot{ResourceReference: apitypes.ResourceReference{Source: "skill", ID: "review"}, Type: "skill", Name: "runtime-review"}, Enabled: true, Available: true}}})
		case "/api/v1/relay/v1/responses":
			relayCalls++
			var body CallRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.ResourceRefs) != 1 || body.ResourceRefs[0].ID != "review" || body.PreviousResponseID != "resp_previous" {
				t.Errorf("resource chat lost selection or parent: %+v", body)
			}
			writeTestJSON(w, 200, Response{ID: "resp_done", Model: id, Status: "completed"})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cred := SiteCredentials{Site: server.URL, Token: "fixture-token", User: "caller"}
	credentials := func() (SiteCredentials, error) { return cred, nil }
	m := &Manager{credentials: credentials, community: HubCommunity{Credentials: credentials}, manifest: relayManifest{ConnectionID: "connection"}, cloudConnection: func(context.Context) (knowledgebase.Connection, error) {
		return knowledgebase.Connection{CSGHubBaseURL: server.URL, CSGHubAccessToken: cred.Token, AIGatewayBaseURL: server.URL + "/v1"}, nil
	}}
	connection := m.connectionReferenceFor(cred)
	profile, err := m.CloudProfile(t.Context(), id, connection)
	if err != nil || profile.Name != "runtime-name" || profile.Model != "model" || profile.Provider != "opencsg" {
		t.Fatalf("profile=%+v err=%v", profile, err)
	}
	data, _ := json.Marshal(profile)
	if strings.Contains(string(data), "private-secret") {
		t.Fatal("read profile exposed credentials")
	}
	page, err := m.CloudResources(t.Context(), id, connection, url.Values{"type": {"skill"}, "pagination": {"cursor"}})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("resources=%+v err=%v", page, err)
	}
	_, selection, err := m.ResolveCloudResources(t.Context(), id, connection, "", []apitypes.ResourceReference{{Source: "skill", ID: "review"}})
	if err != nil || selection.Refs[0].Name != "runtime-review" {
		t.Fatalf("selection=%+v err=%v", selection, err)
	}
	_, err = m.ChatWithResources(t.Context(), id, connection, "key", "", "resp_previous", []apitypes.ResourceReference{{Source: "skill", ID: "review"}})
	if err != nil || relayCalls != 1 || sourceCalls != 3 {
		t.Fatalf("source=%d relay=%d err=%v", sourceCalls, relayCalls, err)
	}
	capability = false
	if err = m.CheckResourceSupport(t.Context(), id, connection); !errors.Is(err, ErrResourcesUnsupported) {
		t.Fatalf("legacy executor accepted resources: %v", err)
	}
	if _, err = m.CloudProfile(t.Context(), id, "other-login"); !errors.Is(err, ErrChatReference) {
		t.Fatalf("wrong account accepted: %v", err)
	}
	for _, tc := range []struct {
		typ, endpoint string
		readable      bool
	}{
		{"csgclaw", "", true},
		{"csgclaw", "missing", false},
		{"csgclaw-workspace", "missing", false},
		{"csgclaw", "https://untrusted.invalid/v1/sandboxes/instance", false},
	} {
		instanceType, endpointOverride = tc.typ, tc.endpoint
		entries, err := m.DiscoverCatalog(t.Context(), []string{})
		if err != nil || len(entries) != 1 || entries[0].CloudReadAvailable != tc.readable {
			t.Fatalf("read capability for %+v: entries=%+v err=%v", tc, entries, err)
		}
	}
	instanceType, endpointOverride, status = "csgclaw", "", "offline"
	before := sourceCalls
	if _, err := m.CloudProfile(t.Context(), id, connection); !errors.Is(err, ErrCloudAgentOffline) {
		t.Fatalf("offline profile must not call sandbox: %v", err)
	}
	if _, err := m.CloudResources(t.Context(), id, connection, url.Values{"type": {"skill"}}); !errors.Is(err, ErrCloudAgentOffline) {
		t.Fatalf("offline resources must not call sandbox: %v", err)
	}
	if sourceCalls != before {
		t.Fatal("offline read reached the sandbox auto-resume route")
	}
}

func TestLocalExecutorPreservesOnlyAllowlistedResourceErrors(t *testing.T) {
	for _, code := range []string{"resource_not_attached", "resource_disabled", "resource_unavailable", "invalid_resource_reference", "unexpected_secret_code"} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeTestJSON(w, 400, map[string]any{"error": map[string]string{"code": code, "message": "private-upstream-diagnostic"}})
			}))
			defer server.Close()
			executor := LocalExecutor{Client: apiclient.New(server.URL, "fixture-token", server.Client()), AgentID: "worker"}
			result := executor.Execute(t.Context(), Frame{ResponseID: "resp", SessionID: "session", Input: json.RawMessage(`"inspect"`), ResourceRefs: []apitypes.ResourceReference{{Source: "mcp", ID: "search"}}})
			want := code
			if code == "unexpected_secret_code" {
				want = "execution_failed"
			}
			if result.Type != "response.failed" || result.Error == nil || result.Error.Code != want || strings.Contains(result.Error.Message, "private-upstream-diagnostic") {
				t.Fatalf("unsafe or missing error code: %+v", result)
			}
		})
	}
}

func TestLocalExecutorForwardsResourcesToSessionOnly(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/responses") {
			t.Errorf("unexpected execution path %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Input string                       `json:"input"`
			Refs  []apitypes.ResourceReference `json:"resource_refs"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Input != "" || len(body.Refs) != 1 || body.Refs[0].ID != "review" {
			t.Errorf("selection not forwarded %+v", body)
		}
		writeTestJSON(w, 200, Response{Status: "completed"})
	}))
	defer server.Close()
	executor := LocalExecutor{Client: apiclient.New(server.URL, "fixture-token", server.Client()), AgentID: "worker"}
	result := executor.Execute(t.Context(), Frame{ResponseID: "resp", SessionID: "session", Input: json.RawMessage(`""`), ResourceRefs: []apitypes.ResourceReference{{Source: "skill", ID: "review"}}})
	if calls != 1 || result.Type != "response.completed" {
		t.Fatalf("execution failed %+v", result)
	}
}
