package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/opencsgmcp"
)

func TestMCPFileBridgeExposesNativeToolAndInjectsFileContent(t *testing.T) {
	item := completeWorkerAgent("agent-file", "file-worker")
	item.MCPServers = map[string]any{"file-parser": map[string]any{
		"url": "https://untrusted.example/mcp",
		opencsgmcp.ManagedMetaKey: map[string]any{
			opencsgmcp.ManagedMetaNamespace: map[string]any{
				"type": opencsgmcp.GatewayMCPType, "auth_type": opencsgmcp.CSGHubAuthType,
				"file_bindings": map[string]any{"parse_file_content": map[string]any{
					"encoding": "base64", "content_argument": "content_base64",
					"filename_argument": "filename", "content_type_argument": "content_type",
				}},
			},
		},
	}}
	svc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{item})
	workspace := svc.Workspace()
	root, err := workspace.WorkspaceRootByID(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "resume.txt"), []byte("PRIVATE RESUME"), 0o600); err != nil {
		t.Fatal(err)
	}

	originalCall := callMCPFileBridgeTool
	originalLoader := loadKnowledgeBaseConnection
	t.Cleanup(func() { callMCPFileBridgeTool = originalCall; loadKnowledgeBaseConnection = originalLoader })
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		return knowledgeBaseConnection{AIGatewayBaseURL: "https://gateway.example/v1", CSGHubAccessToken: "user-token"}, nil
	}
	callMCPFileBridgeTool = func(_ context.Context, server string, config map[string]any, tool string, arguments map[string]any) (any, error) {
		if server != "file-parser" || tool != "parse_file_content" {
			t.Fatalf("call = %s/%s", server, tool)
		}
		decoded, err := base64.StdEncoding.DecodeString(arguments["content_base64"].(string))
		if err != nil || string(decoded) != "PRIVATE RESUME" {
			t.Fatalf("content = %q, err=%v", decoded, err)
		}
		if arguments["filename"] != "resume.txt" || arguments["content_type"] != "text/plain" {
			t.Fatalf("arguments = %#v", arguments)
		}
		return map[string]any{"text": "parsed resume", "char_count": 13}, nil
	}

	handler := &Handler{svc: svc, workspace: workspace, serverAccessToken: "root-token"}
	token := opencsgmcp.FileBridgeToken("root-token", item.ID, "file-parser")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-file/mcp-file-bridge/file-parser", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"parse_file_content","arguments":{"path":"resume.txt","content_type":"text/plain"}}}`))
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "parsed resume") || strings.Contains(recorder.Body.String(), "PRIVATE RESUME") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestMCPFileBridgeToolSchemaUsesPathInsteadOfBase64(t *testing.T) {
	tools := mcpFileBridgeTools(map[string]any{"parse_file_content": map[string]any{
		"encoding": "base64", "content_argument": "content_base64", "filename_argument": "filename",
	}})
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"path"`) || strings.Contains(text, "content_base64") {
		t.Fatalf("schema = %s", text)
	}
}
