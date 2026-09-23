package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/participant"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type appPlatformToolHandler func(context.Context, *mcp.CallToolRequest, map[string]any) (any, error)

func (h *Handler) addAppPlatformTool(agentID, name, description string, properties map[string]any, required []string, readOnly bool, handler appPlatformToolHandler) {
	if properties == nil {
		properties = map[string]any{}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	raw, _ := json.Marshal(schema)
	var shape jsonschema.Schema
	_ = json.Unmarshal(raw, &shape)
	resolved, schemaErr := shape.Resolve(nil)
	tool := &mcp.Tool{Name: name, Description: description, InputSchema: schema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly}}
	h.apps.RegisterTool(agentID, tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if !readOnly && h.connectorReadOnlyAgent(agentID) {
			return platformToolFailure(fmt.Errorf("read-only Agent cannot perform this operation")), nil
		}
		if _, ok := h.svc.Agent(agentID); !ok {
			return platformToolFailure(fmt.Errorf("Agent is unavailable")), nil
		}
		args := map[string]any{}
		if request.Params == nil {
			return platformToolFailure(fmt.Errorf("invalid tool request")), nil
		}
		if len(request.Params.Arguments) > 0 && json.Unmarshal(request.Params.Arguments, &args) != nil {
			return platformToolFailure(fmt.Errorf("invalid tool arguments")), nil
		}
		if schemaErr != nil || resolved.Validate(args) != nil {
			return platformToolFailure(fmt.Errorf("invalid tool arguments")), nil
		}
		result, err := handler(ctx, request, args)
		if err != nil {
			return platformToolFailure(err), nil
		}
		if value, ok := result.(*mcp.CallToolResult); ok {
			return value, nil
		}
		body, err := json.Marshal(result)
		if err != nil {
			return platformToolFailure(fmt.Errorf("cannot encode tool result")), nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}, StructuredContent: map[string]any{"result": result}}, nil
	})
}

func platformToolFailure(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}
}

func platformTextFields(names ...string) map[string]any {
	fields := map[string]any{}
	for _, name := range names {
		fields[name] = map[string]any{"type": "string"}
	}
	return fields
}

func platformText(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func (h *Handler) requirePlatformManager(agentID string) error {
	if !h.agentPlatformManager(agentID) {
		return fmt.Errorf("only the Manager can perform this operation")
	}
	return nil
}

func (h *Handler) requirePlatformRoom(agentID, roomID string) error {
	if !h.agentPlatformRoom(agentID, roomID) {
		return fmt.Errorf("Agent is not a member of this room")
	}
	return nil
}

type platformResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *platformResponse) Header() http.Header { return w.header }
func (w *platformResponse) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}
func (w *platformResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

// Named tools call the existing HTTP application workflows in-process, retaining
// their dispatch, projection and notification effects without a second HTTP hop.
func (h *Handler) invokeAppPlatformHandler(ctx context.Context, agentID, method string, pathParams map[string]string, query url.Values, payload any, handler http.HandlerFunc) (any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid platform request")
	}
	r, err := http.NewRequestWithContext(context.WithValue(ctx, appAgentContextKey{}, agentID), method, "http://csgclaw.internal/platform?"+query.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+h.serverAccessToken)
	r.Header.Set("X-CSGClaw-Caller-Agent", h.agentPlatformParticipant(agentID))
	for key, value := range pathParams {
		r.SetPathValue(key, value)
	}
	w := &platformResponse{header: make(http.Header)}
	handler(w, r)
	if w.status >= 400 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(w.body.String()))
	}
	if w.body.Len() == 0 {
		return map[string]any{"ok": true}, nil
	}
	var result any
	if err := json.Unmarshal(w.body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("invalid platform response")
	}
	return result, nil
}

func (h *Handler) registerAppPlatformTools(agentID string) {
	h.appPlatformMu.Lock()
	defer h.appPlatformMu.Unlock()
	if h.appPlatformAgents[agentID] != "" {
		return
	}
	a, ok := h.svc.Agent(agentID)
	if !ok {
		return
	}
	h.appPlatformAgents[agentID] = a.Role
	h.registerAppIdentityTools(agentID)
	h.registerAppRoomTools(agentID)
	h.registerAppPlatformTaskTools(agentID)
	if a.RuntimeKind == agent.RuntimeKindCodex {
		h.registerAppFileTools(agentID)
	}
	if h.agentPlatformManager(agentID) {
		h.registerAppWorkerTools(agentID)
	}
}

func platformAgentSummary(a agent.Agent) map[string]any {
	return map[string]any{"id": a.ID, "name": a.Name, "description": a.Description, "role": a.Role, "status": a.Status, "runtime_kind": a.RuntimeKind, "participant_id": agent.ParticipantIDForAgent(a.Name, a.ID)}
}

func (h *Handler) platformVisibleAgent(caller, target string) bool {
	if caller == target || h.agentPlatformManager(caller) {
		return true
	}
	if h.im == nil {
		return false
	}
	actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(target))
	for _, room := range h.im.ListRooms() {
		if !h.agentPlatformRoom(caller, room.ID) {
			continue
		}
		for _, member := range room.Members {
			if actor.matches(member) {
				return true
			}
		}
	}
	if h.teamSvc != nil {
		for _, team := range h.teamSvc.ListTeams() {
			hasCaller, hasTarget := false, false
			for _, id := range append(team.MemberAgentIDs, team.LeadAgentID) {
				hasCaller = hasCaller || agent.CanonicalID(id) == caller
				hasTarget = hasTarget || agent.CanonicalID(id) == target
			}
			if hasCaller && hasTarget {
				return true
			}
		}
	}
	return false
}

func (h *Handler) registerAppIdentityTools(agentID string) {
	h.addAppPlatformTool(agentID, "self_get", "Read your CSGClaw Agent identity and participant ID.", nil, nil, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		a, _ := h.svc.Agent(agentID)
		return platformAgentSummary(a), nil
	})
	h.addAppPlatformTool(agentID, "agents_list", "List Agents you are allowed to discover, without credentials.", nil, nil, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		items, err := h.agentEngine.Agents().List(ctx, agentengine.AgentListOptions{})
		if err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, item := range items {
			if h.platformVisibleAgent(agentID, item.ID) {
				out = append(out, platformAgentSummary(serviceAgentFromEngine(item)))
			}
		}
		return out, nil
	})
	h.addAppPlatformTool(agentID, "agents_get", "Read public identity and status of an accessible Agent.", platformTextFields("agent_id"), []string{"agent_id"}, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		a, ok := h.svc.Agent(platformText(args, "agent_id"))
		if !ok || !h.platformVisibleAgent(agentID, a.ID) {
			return nil, fmt.Errorf("Agent not found or inaccessible")
		}
		return platformAgentSummary(a), nil
	})
	h.addAppPlatformTool(agentID, "participants_list", "List public participant identities visible to your Agent.", nil, nil, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		out := []map[string]any{}
		if h.participant == nil {
			return out, nil
		}
		for _, p := range h.participant.List(participant.ListOptions{}) {
			visible := h.agentPlatformManager(agentID) || p.AgentID == agentID
			if !visible && h.im != nil {
				for _, room := range h.im.ListRooms() {
					if !h.agentPlatformRoom(agentID, room.ID) {
						continue
					}
					target := h.participantBridgeTargetForRoomMember(p.ID)
					for _, member := range room.Members {
						if target.matches(member) {
							visible = true
						}
					}
				}
			}
			if visible {
				out = append(out, map[string]any{"id": p.ID, "name": p.Name, "channel": p.Channel, "type": p.Type, "agent_id": p.AgentID})
			}
		}
		return out, nil
	})
	h.addAppPlatformTool(agentID, "apps_list", "Get the current App instances and available counts by service. This fresh inventory replaces previous lists; removed Apps are absent. Use the only available matching App directly; ask for a choice only when multiple current matches remain ambiguous. Credentials and tool schemas are not returned.", nil, nil, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		items, err := h.apps.List(ctx, agentID)
		if err != nil {
			return nil, err
		}
		summaries := make([]map[string]any, 0, len(items))
		counts := map[string]int{}
		for _, item := range items {
			available := item.Enabled && !item.Disconnected && item.Status == "connected" && len(item.Tools) > 0
			if _, ok := counts[item.AppID]; !ok {
				counts[item.AppID] = 0
			}
			if available {
				counts[item.AppID]++
			}
			summaries = append(summaries, map[string]any{"installation_id": item.InstallationID, "app_id": item.AppID, "name": item.Name, "status": item.Status, "available": available})
		}
		return map[string]any{"apps": summaries, "available_app_counts": counts}, nil
	})
	h.addAppPlatformTool(agentID, "app_setup", "Open your App configuration page to add or reconnect GitLab, Feishu, or a knowledge base. The user enters credentials in the UI.", platformTextFields("app_id", "installation_id"), nil, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		q := url.Values{"tab": {"connectors"}}
		if id := platformText(args, "installation_id"); id != "" {
			if _, err := h.apps.Get(ctx, agentID, id); err != nil {
				return nil, err
			}
			q.Set("connector", id)
		}
		if id := platformText(args, "app_id"); id != "" {
			if _, err := h.apps.Definition(id); err != nil {
				return nil, err
			}
			q.Set("add_connector", id)
		}
		return map[string]any{"url": strings.TrimRight(h.advertiseBaseURL, "/") + "/#/agents/" + url.PathEscape(agentID) + "?" + q.Encode(), "message": "Open App settings and enter credentials there."}, nil
	})
	if !h.agentPlatformManager(agentID) {
		return
	}
	h.addAppPlatformTool(agentID, "templates_list", "List available Agent templates for explicit worker creation.", nil, nil, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if h.hub == nil {
			return nil, fmt.Errorf("template service unavailable")
		}
		return h.hub.List(ctx)
	})
	h.addAppPlatformTool(agentID, "templates_get", "Inspect an Agent template.", platformTextFields("template_id"), []string{"template_id"}, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if h.hub == nil {
			return nil, fmt.Errorf("template service unavailable")
		}
		return h.hub.Get(ctx, platformText(args, "template_id"))
	})
}

func (h *Handler) registerAppRoomTools(agentID string) {
	h.addAppPlatformTool(agentID, "rooms_list", "List rooms you belong to.", nil, nil, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		out := []map[string]any{}
		if h.im == nil {
			return out, nil
		}
		for _, room := range h.im.ListRooms() {
			if h.agentPlatformRoom(agentID, room.ID) {
				out = append(out, map[string]any{"id": room.ID, "title": room.Title, "members": room.Members, "manager_id": room.ManagerID, "is_direct": room.IsDirect})
			}
		}
		return out, nil
	})
	h.addAppPlatformTool(agentID, "room_get", "Read a room and its members after checking membership.", platformTextFields("room_id"), []string{"room_id"}, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		id := platformText(args, "room_id")
		if err := h.requirePlatformRoom(agentID, id); err != nil {
			return nil, err
		}
		room, _ := h.im.Room(id)
		return map[string]any{"id": room.ID, "title": room.Title, "members": room.Members, "manager_id": room.ManagerID, "is_direct": room.IsDirect}, nil
	})
	h.addAppPlatformTool(agentID, "messages_list", "Read messages in a room you belong to.", platformTextFields("room_id"), []string{"room_id"}, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		id := platformText(args, "room_id")
		if err := h.requirePlatformRoom(agentID, id); err != nil {
			return nil, err
		}
		return h.invokeAppPlatformHandler(ctx, agentID, http.MethodGet, nil, url.Values{"room_id": {id}}, nil, h.handleMessages)
	})
	h.addAppPlatformTool(agentID, "message_send", "Send a message as yourself to a room you belong to. Use mention_id for an explicitly requested notification.", platformTextFields("room_id", "content", "mention_id", "client_message_id"), []string{"room_id", "content"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if err := h.requirePlatformRoom(agentID, platformText(args, "room_id")); err != nil {
			return nil, err
		}
		payload := map[string]any{}
		for key, value := range args {
			payload[key] = value
		}
		payload["sender_id"] = h.agentPlatformParticipant(agentID)
		return h.invokeAppPlatformHandler(ctx, agentID, http.MethodPost, nil, nil, payload, h.handleCreateMessage)
	})
	if !h.agentPlatformManager(agentID) {
		return
	}
	fields := platformTextFields("title", "source_room_id", "source_message_id", "type")
	fields["member_ids"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	h.addAppPlatformTool(agentID, "room_create", "Create an explicitly requested room. The creator is resolved from a stored human request, never supplied as an identity argument.", fields, []string{"title", "source_room_id", "source_message_id", "member_ids"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if err := h.requirePlatformManager(agentID); err != nil {
			return nil, err
		}
		source := platformText(args, "source_room_id")
		if err := h.requirePlatformRoom(agentID, source); err != nil {
			return nil, err
		}
		room, _ := h.im.Room(source)
		creator := ""
		for _, msg := range room.Messages {
			if msg.ID == platformText(args, "source_message_id") {
				if user, ok := h.im.User(msg.SenderID); ok && user.Role != "manager" && user.Role != "worker" && user.Role != "agent" {
					creator = user.ID
				}
				break
			}
		}
		if creator == "" {
			return nil, fmt.Errorf("a stored human request is required")
		}
		members := []string{creator, h.agentPlatformParticipant(agentID)}
		for _, v := range args["member_ids"].([]any) {
			members = append(members, v.(string))
		}
		payload := map[string]any{"title": args["title"], "creator_id": creator, "member_ids": members, "manager_id": h.agentPlatformParticipant(agentID), "type": platformText(args, "type")}
		return h.invokeAppPlatformHandler(ctx, agentID, http.MethodPost, nil, nil, payload, h.handleCreateRoom)
	})
	memberFields := platformTextFields("room_id")
	memberFields["member_ids"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	h.addAppPlatformTool(agentID, "room_members_add", "Add requested participants to a room you belong to; Manager only.", memberFields, []string{"room_id", "member_ids"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if err := h.requirePlatformManager(agentID); err != nil {
			return nil, err
		}
		id := platformText(args, "room_id")
		if err := h.requirePlatformRoom(agentID, id); err != nil {
			return nil, err
		}
		return h.invokeAppPlatformHandler(ctx, agentID, http.MethodPost, map[string]string{"id": id}, nil, map[string]any{"room_id": id, "user_ids": args["member_ids"], "inviter_id": h.agentPlatformParticipant(agentID)}, h.addRoomMembers)
	})
}

func appNativeTurnIDs(req *mcp.CallToolRequest) (string, string, error) {
	if req.Params == nil {
		return "", "", fmt.Errorf("active Codex turn metadata required")
	}
	meta, _ := req.Params.Meta["x-codex-turn-metadata"].(map[string]any)
	thread, _ := meta["thread_id"].(string)
	session, _ := meta["session_id"].(string)
	turn, _ := meta["turn_id"].(string)
	if thread == "" {
		thread = session
	}
	if thread == "" || turn == "" || (session != "" && thread != session) {
		return "", "", fmt.Errorf("active Codex turn metadata required")
	}
	return thread, turn, nil
}

func (h *Handler) registerAppFileTools(agentID string) {
	for _, name := range []string{"csgclaw_publish_file", "csgclaw_upload_file"} {
		props := platformTextFields("path", "name", "mimeType")
		required := []string{"path"}
		if name == "csgclaw_upload_file" {
			props = platformTextFields("path", "server", "installation_id", "uploadUri", "contentType")
			props["installation_id"] = map[string]any{"type": "string", "description": "App installation ID returned by apps_list. Use this for App uploads instead of server."}
			required = append(required, "uploadUri")
		}
		h.addAppPlatformTool(agentID, name, "Publish or upload a workspace-relative file for the exact active Codex thread and turn. For upload provide either an App installation_id or a manually configured MCP server.", props, required, name == "csgclaw_publish_file", func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
			thread, turn, err := appNativeTurnIDs(req)
			if err != nil {
				return nil, err
			}
			rt := h.connectorCodexRuntime()
			if rt == nil {
				return nil, fmt.Errorf("Codex runtime unavailable")
			}
			if name == "csgclaw_upload_file" {
				installation, server := platformText(args, "installation_id"), platformText(args, "server")
				if (installation == "") == (server == "") {
					return nil, fmt.Errorf("provide either installation_id or server")
				}
				if installation != "" {
					raw, err := rt.WithPlatformFile(ctx, agentID, thread, turn, platformText(args, "path"), func(fileCtx context.Context, filename string, size int64, body io.Reader) (json.RawMessage, error) {
						return h.apps.Upload(fileCtx, agentID, installation, platformText(args, "uploadUri"), filename, platformText(args, "contentType"), size, body)
					})
					if err != nil {
						return nil, err
					}
					var result any
					if err := json.Unmarshal(raw, &result); err != nil {
						return nil, err
					}
					return result, nil
				}
			}
			raw, err := rt.CallPlatformFileTool(ctx, agentID, name, thread, turn, req.Params.Arguments)
			if err != nil {
				return nil, err
			}
			var result mcp.CallToolResult
			if err := json.Unmarshal(raw, &result); err != nil {
				return nil, err
			}
			return &result, nil
		})
	}
}

func (h *Handler) registerAppWorkerTools(agentID string) {
	h.addAppPlatformTool(agentID, "agent_create_from_template", "Create a Worker from an existing template only when explicitly requested.", platformTextFields("name", "description", "template_id"), []string{"name", "template_id"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if err := h.requirePlatformManager(agentID); err != nil {
			return nil, err
		}
		payload := participant.CreateRequest{Type: participant.TypeAgent, Name: platformText(args, "name"), AgentBinding: participant.AgentBindingSpec{Mode: participant.BindingModeCreate, Agent: &agent.CreateAgentSpec{Name: platformText(args, "name"), Description: platformText(args, "description"), FromTemplate: platformText(args, "template_id"), Role: agent.RoleWorker}}}
		result, err := h.invokeAppPlatformHandler(ctx, agentID, http.MethodPost, map[string]string{"channel": participant.ChannelCSGClaw}, nil, payload, h.handleParticipants)
		if err != nil {
			return nil, err
		}
		created, _ := result.(map[string]any)
		id, _ := created["agent_id"].(string)
		if item, ok := h.svc.Agent(id); ok {
			return platformAgentSummary(item), nil
		}
		return map[string]any{"id": id, "participant_id": created["id"], "name": created["name"]}, nil
	})
	for _, op := range []struct {
		name    string
		handler http.HandlerFunc
		method  string
	}{{"agent_start", h.startAgent, http.MethodPost}, {"agent_stop", h.stopAgent, http.MethodPost}, {"agent_recreate", h.recreateAgent, http.MethodPost}, {"agent_delete", h.deleteAgent, http.MethodDelete}} {
		h.addAppPlatformTool(agentID, op.name, "Manage an existing Worker; Manager only.", platformTextFields("agent_id"), []string{"agent_id"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
			if err := h.requirePlatformManager(agentID); err != nil {
				return nil, err
			}
			target, ok := h.svc.Agent(platformText(args, "agent_id"))
			if !ok || target.Role != agent.RoleWorker {
				return nil, fmt.Errorf("target must be an existing Worker")
			}
			if _, err := h.invokeAppPlatformHandler(ctx, agentID, op.method, map[string]string{"id": target.ID}, nil, nil, op.handler); err != nil {
				return nil, err
			}
			if op.name == "agent_delete" {
				return map[string]any{"id": target.ID, "deleted": true}, nil
			}
			current, ok := h.svc.Agent(target.ID)
			if !ok {
				return nil, fmt.Errorf("Worker is unavailable")
			}
			return platformAgentSummary(current), nil
		})
	}
	h.addAppPlatformTool(agentID, "agent_update", "Update a Worker's name, description or instructions; Manager only.", platformTextFields("agent_id", "name", "description", "instructions"), []string{"agent_id"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if err := h.requirePlatformManager(agentID); err != nil {
			return nil, err
		}
		target, ok := h.svc.Agent(platformText(args, "agent_id"))
		if !ok || target.Role != agent.RoleWorker {
			return nil, fmt.Errorf("target must be an existing Worker")
		}
		payload := map[string]any{}
		for _, field := range []string{"name", "description", "instructions"} {
			if v, ok := args[field]; ok {
				payload[field] = v
			}
		}
		if _, err := h.invokeAppPlatformHandler(ctx, agentID, http.MethodPatch, map[string]string{"id": target.ID}, nil, payload, h.updateAgent); err != nil {
			return nil, err
		}
		current, ok := h.svc.Agent(target.ID)
		if !ok {
			return nil, fmt.Errorf("Worker is unavailable")
		}
		return platformAgentSummary(current), nil
	})
}
