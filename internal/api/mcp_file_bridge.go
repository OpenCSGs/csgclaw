package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	internalmcp "csgclaw/internal/mcp"
	"csgclaw/internal/opencsgmcp"

	"github.com/go-chi/chi/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var callMCPFileBridgeTool = internalmcp.CallAdvertisedToolWithoutRedirects
var listMCPFileBridgeTools = internalmcp.ListToolDefinitionsWithoutRedirects

type mcpFileBridgeRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (h *Handler) handleMCPFileBridge(w http.ResponseWriter, r *http.Request) {
	agentID := strings.TrimSpace(chi.URLParam(r, "id"))
	serverName := strings.TrimSpace(chi.URLParam(r, "server"))
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if agentID == "" || serverName == "" || !opencsgmcp.ValidFileBridgeToken(token, h.serverAccessToken, agentID, serverName) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agentRecord, ok := h.svc.Agent(agentID)
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	serverConfig, ok := agentRecord.MCPServers[serverName].(map[string]any)
	if !ok || !opencsgmcp.IsGatewayServer(serverConfig) {
		http.Error(w, "managed MCP server not found", http.StatusNotFound)
		return
	}
	bindings, err := opencsgmcp.FileBindings(serverConfig)
	if err != nil {
		http.Error(w, "invalid MCP file bindings", http.StatusUnprocessableEntity)
		return
	}
	if len(bindings) == 0 {
		http.Error(w, "MCP file bindings are not configured", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read MCP request", http.StatusBadRequest)
		return
	}
	var request mcpFileBridgeRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid MCP request", http.StatusBadRequest)
		return
	}
	switch request.Method {
	case "initialize":
		h.writeMCPFileBridgeResult(w, request.ID, map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "csgclaw-file-bridge", "version": "1.0.0"},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "tools/list":
		h.handleMCPFileBridgeList(w, r.Context(), request.ID, serverName, serverConfig, bindings)
	case "tools/call":
		h.handleMCPFileBridgeCall(w, r.Context(), request.ID, agentID, serverName, serverConfig, bindings, request.Params)
	default:
		h.writeMCPFileBridgeError(w, request.ID, -32601, "method not found")
	}
}

func (h *Handler) handleMCPFileBridgeList(w http.ResponseWriter, ctx context.Context, id json.RawMessage, serverName string, serverConfig, bindings map[string]any) {
	connection, err := loadKnowledgeBaseConnection(ctx)
	if err != nil {
		h.writeMCPFileBridgeError(w, id, -32603, err.Error())
		return
	}
	tools, err := listMCPFileBridgeTools(ctx, serverName, mcpFileBridgeUpstreamConfig(connection, serverConfig))
	if err != nil {
		h.writeMCPFileBridgeError(w, id, -32603, err.Error())
		return
	}
	h.writeMCPFileBridgeResult(w, id, map[string]any{"tools": mcpFileBridgeToolDefinitions(tools, bindings)})
}

func mcpFileBridgeToolDefinitions(upstream []*mcpsdk.Tool, bindings map[string]any) []map[string]any {
	tools := make([]map[string]any, 0, len(upstream))
	for _, tool := range upstream {
		if tool == nil {
			continue
		}
		encoded, err := json.Marshal(tool)
		if err != nil {
			continue
		}
		var definition map[string]any
		if err := json.Unmarshal(encoded, &definition); err != nil {
			continue
		}
		if _, bound := bindings[tool.Name]; bound {
			definition["description"] = "Process a Runtime workspace file through the configured MCP service. Pass only a workspace-relative path; CSGClaw transfers the bytes outside model context. " + tool.Description
			definition["inputSchema"] = map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"path":         map[string]any{"type": "string", "description": "Runtime workspace-relative file path."},
					"content_type": map[string]any{"type": "string", "description": "Optional MIME type."},
				},
				"required": []string{"path"},
			}
		}
		tools = append(tools, definition)
	}
	return tools
}

func (h *Handler) handleMCPFileBridgeCall(w http.ResponseWriter, ctx context.Context, id json.RawMessage, agentID, serverName string, serverConfig, bindings map[string]any, rawParams json.RawMessage) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		h.writeMCPFileBridgeError(w, id, -32602, "invalid tool arguments")
		return
	}
	connection, err := loadKnowledgeBaseConnection(ctx)
	if err != nil {
		h.writeMCPFileBridgeToolFailure(w, id, err.Error())
		return
	}
	upstreamConfig := mcpFileBridgeUpstreamConfig(connection, serverConfig)
	binding, _ := bindings[strings.TrimSpace(params.Name)].(map[string]any)
	if binding == nil {
		result, callErr := callMCPFileBridgeTool(ctx, serverName, upstreamConfig, params.Name, params.Arguments)
		h.writeMCPFileBridgeCallResult(w, id, result, callErr)
		return
	}
	path, _ := params.Arguments["path"].(string)
	contentType, _ := params.Arguments["content_type"].(string)
	filename, content, err := h.readMCPBridgeWorkspaceFile(agentID, path, binding)
	if err != nil {
		h.writeMCPFileBridgeToolFailure(w, id, err.Error())
		return
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	} else if parsed, _, parseErr := mime.ParseMediaType(contentType); parseErr != nil || parsed == "" {
		h.writeMCPFileBridgeToolFailure(w, id, "content_type is invalid")
		return
	}
	arguments := map[string]any{
		mcpBindingString(binding, "filename_argument"): filename,
		mcpBindingString(binding, "content_argument"):  base64.StdEncoding.EncodeToString(content),
	}
	if key := mcpBindingString(binding, "content_type_argument"); key != "" {
		arguments[key] = contentType
	}
	result, err := callMCPFileBridgeTool(ctx, serverName, upstreamConfig, params.Name, arguments)
	h.writeMCPFileBridgeCallResult(w, id, result, err)
}

func mcpFileBridgeUpstreamConfig(connection knowledgeBaseConnection, serverConfig map[string]any) map[string]any {
	config := map[string]any{
		"url":     strings.TrimRight(connection.AIGatewayBaseURL, "/") + "/gateway/mcp",
		"headers": map[string]any{"Authorization": "Bearer " + connection.CSGHubAccessToken},
	}
	for _, key := range []string{"startup_timeout_sec", "tool_timeout_sec"} {
		if value, ok := serverConfig[key]; ok {
			config[key] = value
		}
	}
	return config
}

func (h *Handler) writeMCPFileBridgeCallResult(w http.ResponseWriter, id json.RawMessage, result *mcpsdk.CallToolResult, err error) {
	if err != nil {
		h.writeMCPFileBridgeToolFailure(w, id, err.Error())
		return
	}
	h.writeMCPFileBridgeResult(w, id, result)
}

func (h *Handler) readMCPBridgeWorkspaceFile(agentID, path string, binding map[string]any) (string, []byte, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("path must stay within the Runtime workspace")
	}
	rootPath, err := h.workspace.WorkspaceRootByID(agentID)
	if err != nil {
		return "", nil, fmt.Errorf("resolve Runtime workspace: %w", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return "", nil, fmt.Errorf("open Runtime workspace: %w", err)
	}
	defer root.Close()
	file, err := root.Open(cleaned)
	if err != nil {
		return "", nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("path must reference a regular file")
	}
	limit := opencsgmcp.DefaultMaxFileBytes
	if declared := mcpBindingMaxBytes(binding); declared > 0 && declared < limit {
		limit = declared
	}
	if info.Size() > limit {
		return "", nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", nil, fmt.Errorf("read file: %w", err)
	}
	if int64(len(content)) > limit {
		return "", nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return filepath.Base(cleaned), content, nil
}

func mcpBindingString(binding map[string]any, key string) string {
	value, _ := binding[key].(string)
	return strings.TrimSpace(value)
}

func mcpBindingMaxBytes(binding map[string]any) int64 {
	switch value := binding["max_bytes"].(type) {
	case float64:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	default:
		return 0
	}
}

func (h *Handler) writeMCPFileBridgeResult(w http.ResponseWriter, id json.RawMessage, result any) {
	h.writeMCPFileBridgeEnvelope(w, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func (h *Handler) writeMCPFileBridgeError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	h.writeMCPFileBridgeEnvelope(w, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "error": map[string]any{"code": code, "message": message}})
}

func (h *Handler) writeMCPFileBridgeToolFailure(w http.ResponseWriter, id json.RawMessage, message string) {
	h.writeMCPFileBridgeResult(w, id, map[string]any{"content": []map[string]any{{"type": "text", "text": message}}, "isError": true})
}

func (h *Handler) writeMCPFileBridgeEnvelope(w http.ResponseWriter, envelope any) {
	encoded, _ := json.Marshal(envelope)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", encoded)
}
