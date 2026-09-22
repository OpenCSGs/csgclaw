package apps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type platformTool struct {
	tool    *mcp.Tool
	handler mcp.ToolHandler
}

func (s *Service) isReadOnly(agentID string) bool {
	return s.options.ReadOnly != nil && s.options.ReadOnly(agentID)
}
func toolReadOnly(tool *mcp.Tool) bool {
	return tool != nil && tool.Annotations != nil && tool.Annotations.ReadOnlyHint
}

func (s *Service) gatewayLocked(agentID string) *gateway {
	if g := s.gateways[agentID]; g != nil {
		return g
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "csgclaw", Version: "1.0.0"}, &mcp.ServerOptions{Logger: quietLogger})
	g := &gateway{server: server, revision: uint64(time.Now().UnixNano()), readOnly: s.isReadOnly(agentID), platformTools: map[string]platformTool{}}
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true, SessionTimeout: 30 * time.Minute, Logger: quietLogger})
	g.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.refreshReadOnly(agentID); transport.ServeHTTP(w, r) })
	s.gateways[agentID] = g
	return g
}

// Handler is called only after the API has authorized the request for agentID.
// The SDK handler owns a separate session map for each Agent, preventing a
// session initialized under one path from being reused under another Agent.
func (s *Service) Handler(agentID string) http.Handler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gatewayLocked(agentID).handler
}
func (s *Service) Revision(agentID string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gatewayLocked(agentID).revision
}

func (s *Service) RegisterTool(agentID string, tool *mcp.Tool, handler mcp.ToolHandler) {
	s.mu.Lock()
	g := s.gatewayLocked(agentID)
	wrapped := func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.isReadOnly(agentID) && !toolReadOnly(tool) {
			return nil, fmt.Errorf("Agent is read-only")
		}
		return handler(ctx, request)
	}
	g.platformTools[tool.Name] = platformTool{tool: tool, handler: wrapped}
	if !g.readOnly || toolReadOnly(tool) {
		g.server.AddTool(tool, wrapped)
	}
	g.revision++
	s.mu.Unlock()
}

func (s *Service) refreshReadOnly(agentID string) {
	readOnly := s.isReadOnly(agentID)
	s.mu.Lock()
	g := s.gatewayLocked(agentID)
	if g.readOnly == readOnly {
		s.mu.Unlock()
		return
	}
	g.readOnly = readOnly
	for name, platform := range g.platformTools {
		g.server.RemoveTools(name)
		if !readOnly || toolReadOnly(platform.tool) {
			g.server.AddTool(platform.tool, platform.handler)
		}
	}
	for _, e := range s.entries {
		if e.record.AgentID == agentID && e.connection != nil {
			g.server.RemoveTools(e.connection.toolNames...)
			e.connection.toolNames = nil
			if e.record.active() && !e.record.Disconnected {
				s.attachLocked(e)
			}
		}
	}
	g.revision++
	revision := g.revision
	s.mu.Unlock()
	// A profile change may be observed during the runtime's own MCP reload.
	// Do not make that initialization wait on another reload request.
	go s.notify(agentID, revision)
}

func (s *Service) notify(agentID string, revision uint64) {
	if revision != 0 && s.options.OnCatalogChanged != nil {
		s.options.OnCatalogChanged(agentID, revision)
	}
}

func toolName(id, name string) string {
	slug := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, name)
	if len(slug) > 10 {
		slug = slug[:10]
	}
	hash := sha256.Sum256([]byte(id + "\x00" + name))
	return "app_" + id + "_" + slug + "_" + hex.EncodeToString(hash[:8])
}

func (s *Service) detachLocked(e *entry) uint64 {
	if e.pendingCancel != nil {
		e.pendingCancel()
		e.pendingCancel = nil
	}
	conn := e.connection
	e.connection = nil
	if conn == nil || len(conn.toolNames) == 0 {
		return 0
	}
	g := s.gatewayLocked(e.record.AgentID)
	g.server.RemoveTools(conn.toolNames...)
	g.revision++
	return g.revision
}

func (s *Service) attachLocked(e *entry) uint64 {
	conn := e.connection
	if conn == nil {
		return 0
	}
	g := s.gatewayLocked(e.record.AgentID)
	conn.toolNames = nil
	for _, upstream := range conn.tools {
		if g.readOnly && !toolReadOnly(upstream) {
			continue
		}
		tool := *upstream
		tool.Name = toolName(e.record.InstallationID, upstream.Name)
		tool.Description = strings.TrimSpace(upstream.Description + fmt.Sprintf("\n\nApp instance: %q; service: %s; installation ID: %s.", e.record.Name, e.record.AppID, e.record.InstallationID))
		if conn.tokens != nil {
			tool.Description += " Uses Feishu application identity (tenant_access_token); user identity and user OAuth are unavailable in this mode."
		}
		id, agentID, name, generation := e.record.InstallationID, e.record.AgentID, upstream.Name, e.generation
		g.server.AddTool(&tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.call(ctx, agentID, id, generation, name, req)
		})
		conn.toolNames = append(conn.toolNames, tool.Name)
	}
	g.revision++
	return g.revision
}

func (s *Service) call(ctx context.Context, agentID, id string, generation uint64, name string, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	readOnly := s.isReadOnly(agentID)
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil || e.generation != generation || !e.record.active() || e.record.Disconnected || e.connection == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("App is no longer connected or enabled")
	}
	conn := e.connection
	if readOnly {
		allowed := false
		for _, tool := range conn.tools {
			if tool.Name == name && toolReadOnly(tool) {
				allowed = true
				break
			}
		}
		if !allowed {
			s.mu.Unlock()
			return nil, fmt.Errorf("Agent is read-only")
		}
	}
	config := e.record.Config
	s.mu.Unlock()
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(config.ToolTimeoutSec)*time.Second)
	defer cancel()
	params := req.Params
	result, err := callAppTool(callCtx, conn, &mcp.CallToolParams{Meta: params.Meta, Name: name, Arguments: params.Arguments, InputResponses: params.InputResponses, RequestState: params.RequestState})
	if conn.tokens != nil && (feishuTokenRejected(result) || feishuRPCTokenRejected(err)) {
		s.failConnectionDetail(agentID, id, conn, connectionError("app_feishu_token_rejected", "Feishu rejected the refreshed application token. Check application credentials and reconnect.", 0, true))
	}
	if err != nil {
		var detail *ConnectionError
		if errors.As(err, &detail) {
			s.failConnectionDetail(agentID, id, conn, detail)
			return nil, detail
		}
		if errors.Is(err, errAuthentication) {
			s.failConnection(agentID, id, conn, "authorization_required", "App authorization is no longer valid; reconnect the App")
			return nil, errAuthentication
		}
		var rpcErr *jsonrpc.Error
		if errors.As(err, &rpcErr) {
			return nil, &jsonrpc.Error{Code: rpcErr.Code, Message: "Upstream App rejected the tool call"}
		}
		if detail := conn.httpAuth.latestFailure(); detail != nil {
			s.failConnectionDetail(agentID, id, conn, detail)
			return nil, detail
		}
		if ctx.Err() == nil && callCtx.Err() == nil {
			s.failConnection(agentID, id, conn, "error", "MCP connection failed; reconnect the App")
		}
		return nil, fmt.Errorf("App tool call failed; verify the service connection and permissions")
	}
	return result, nil
}

func (s *Service) failConnectionDetail(agentID, id string, conn *connection, err error) {
	item := Installation{}
	setConnectionError(&item, err)
	s.failConnection(agentID, id, conn, item.Status, item.LastError, err)
}

func (s *Service) failConnection(agentID, id string, conn *connection, status, message string, cause ...error) {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil || e.connection != conn {
		s.mu.Unlock()
		return
	}
	revision := s.detachLocked(e)
	e.generation++
	e.record.Status = status
	e.record.LastError = message
	e.record.LastErrorCode = ""
	e.record.LastErrorHTTPStatus = 0
	if len(cause) > 0 {
		setConnectionError(&e.record.Installation, cause[0])
	}
	e.record.UpdatedAt = time.Now().UTC()
	_ = s.persistLocked(id, &e.record)
	s.mu.Unlock()
	conn.close()
	s.notify(agentID, revision)
}

func (s *Service) watch(agentID, id string, conn *connection) {
	waitErr := conn.session.Wait()
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil || e.connection != conn || s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	revision := s.detachLocked(e)
	e.generation++
	e.record.Status = "error"
	e.record.LastError = "MCP connection closed; reconnect the App"
	e.record.LastErrorCode = ""
	e.record.LastErrorHTTPStatus = 0
	var detail *ConnectionError
	if !errors.As(waitErr, &detail) {
		detail = conn.httpAuth.latestFailure()
	}
	if detail != nil {
		setConnectionError(&e.record.Installation, detail)
	}
	_ = s.persistLocked(id, &e.record)
	s.mu.Unlock()
	conn.cancel()
	s.notify(agentID, revision)
}

func (s *Service) refreshTools(agentID, id string, generation uint64) {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil || e.generation != generation || e.connection == nil {
		s.mu.Unlock()
		return
	}
	conn := e.connection
	timeout := e.record.Config.ToolTimeoutSec
	s.mu.Unlock()
	select {
	case conn.refreshMu <- struct{}{}:
		defer func() { <-conn.refreshMu }()
	default:
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	tools, err := listTools(ctx, conn.session)
	s.mu.Lock()
	e, found := s.findLocked(agentID, id)
	if found != nil || e.generation != generation || e.connection != conn {
		s.mu.Unlock()
		return
	}
	if err != nil {
		revision := s.detachLocked(e)
		e.generation++
		e.record.Status = "error"
		e.record.LastError = "MCP tool discovery failed; reconnect the App"
		_ = s.persistLocked(id, &e.record)
		s.mu.Unlock()
		conn.close()
		s.notify(agentID, revision)
		return
	}
	g := s.gatewayLocked(agentID)
	g.server.RemoveTools(conn.toolNames...)
	conn.tools = tools
	conn.toolNames = nil
	var revision uint64
	if e.record.active() && !e.record.Disconnected {
		revision = s.attachLocked(e)
	}
	s.mu.Unlock()
	s.notify(agentID, revision)
}
