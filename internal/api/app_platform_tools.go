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
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	runtimedsh "csgclaw/internal/runtime/dsh"
	"github.com/go-chi/chi/v5"
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
		if len(request.Params.Arguments) > 0 && (json.Unmarshal(request.Params.Arguments, &args) != nil || args == nil) {
			return platformToolFailure(fmt.Errorf("invalid tool arguments")), nil
		}
		if err := h.scopeAppPlatformRoomTool(agentID, name, request, args); err != nil {
			return platformToolFailure(err), nil
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

// Use the native turn's persisted conversation binding, never an Agent's latest
// room: one Agent may coordinate several rooms concurrently.
func (h *Handler) scopeAppPlatformRoomTool(agentID, name string, request *mcp.CallToolRequest, args map[string]any) error {
	autonomousTask := strings.HasPrefix(name, "autonomous_task")
	autonomousMessage := name == "message_send" || name == "messages_read" || name == "messages_ack"
	if !autonomousTask && !autonomousMessage && name != "agent_task_create" && name != "messages_list" && !strings.HasPrefix(name, "room_task") && !strings.HasPrefix(name, "browser_") {
		return nil
	}
	if (autonomousTask || autonomousMessage || name == "messages_list") && request.Extra != nil {
		if token := request.Extra.Header.Get(runtimedsh.AppMCPSessionHeader); token != "" {
			return h.scopeDSHAppPlatformRoomTool(agentID, name, token, args)
		}
	}
	if request.Params.Meta["x-codex-turn-metadata"] == nil {
		if autonomousTask {
			return h.scopeUnboundAutonomous(agentID, args, true, name)
		}
		switch name {
		case "message_send", "messages_read", "messages_ack":
			return h.scopeUnboundAutonomousMessage(agentID, name, args)
		}
		return nil
	}
	thread, turn, err := appNativeTurnIDs(request)
	if err != nil {
		return err
	}
	rt := h.connectorCodexRuntime()
	if rt == nil {
		return fmt.Errorf("Codex runtime unavailable")
	}
	turnCtx, conversation, err := rt.AgentTurnContext(agentID, thread, turn)
	if err != nil {
		return err
	}
	source, _ := channel.TurnContextFrom(turnCtx)
	return h.scopeAppPlatformRoomConversation(agentID, name, conversation, source.SourceMessageID, args)
}

func (h *Handler) scopeDSHAppPlatformRoomTool(agentID, name, token string, args map[string]any) error {
	owner, ok := h.svc.(interface {
		Runtime(string) (agentruntime.Runtime, error)
	})
	if !ok {
		return fmt.Errorf("DSH runtime unavailable")
	}
	rt, err := owner.Runtime(agent.RuntimeKindDSH)
	if err != nil {
		return fmt.Errorf("DSH runtime unavailable")
	}
	dsh, ok := rt.(*runtimedsh.Runtime)
	if !ok {
		return fmt.Errorf("DSH runtime unavailable")
	}
	ctx, conversation, active := dsh.AgentAppMCPTurnContext(agentID, token)
	if !active {
		return fmt.Errorf("DSH MCP session has no active turn for this Agent")
	}
	source, _ := channel.TurnContextFrom(ctx)
	if !source.Autonomous && !strings.HasPrefix(name, "autonomous_task") {
		return nil
	}
	return h.scopeAppPlatformRoomConversation(agentID, name, conversation, source.SourceMessageID, args)
}

func (h *Handler) scopeAppPlatformRoomConversation(agentID, name, conversation, sourceID string, args map[string]any) error {
	autonomousTask := strings.HasPrefix(name, "autonomous_task")
	if !strings.HasPrefix(conversation, "csgclaw-im:") {
		if autonomousTask {
			return fmt.Errorf("this tool requires an active autonomous room turn")
		}
		return nil
	}
	_, scope, ok := strings.Cut(conversation, ":room:")
	if !ok {
		if autonomousTask {
			return fmt.Errorf("this tool requires an active autonomous room turn")
		}
		return nil
	}
	scope, tail, _ := strings.Cut(scope, ":")
	roomID, err := url.QueryUnescape(scope)
	if err != nil {
		return fmt.Errorf("invalid room conversation binding")
	}
	if strings.HasPrefix(name, "browser_") {
		args["room_id"] = roomID
		if name == "browser_screenshot" {
			args["thread_root_id"] = ""
			if root, ok := strings.CutPrefix(tail, "thread:"); ok {
				root, _, _ = strings.Cut(root, ":")
				thread, err := url.QueryUnescape(root)
				if err != nil {
					return fmt.Errorf("invalid browser thread binding")
				}
				args["thread_root_id"] = thread
			}
		}
		return nil
	}
	if h.im != nil {
		if room, exists := h.im.Room(roomID); exists && room.IsAutonomous() {
			boundThread := ""
			if encoded, ok := strings.CutPrefix(tail, "thread:"); ok {
				boundThread, err = url.QueryUnescape(encoded)
				if err != nil {
					return fmt.Errorf("invalid thread conversation binding")
				}
			}
			if autonomousTask {
				return bindAutonomousMessageTarget(args, roomID, boundThread)
			}
			switch name {
			case "message_send", "messages_read", "messages_ack":
				return h.bindAutonomousRoomMessage(args, name, roomID, boundThread, sourceID)
			case "messages_list":
				if platformText(args, "room_id") == roomID && platformText(args, "thread_root_id") == "" && boundThread != "" {
					args["thread_root_id"] = boundThread
				}
			}
		}
	}
	if autonomousTask {
		return fmt.Errorf("this tool requires an active autonomous room turn")
	}
	roster, ok := h.roomSchedulingContext(roomID)
	if !ok {
		return nil // Direct and free rooms retain their existing tools.
	}
	if strings.HasPrefix(name, "room_task") {
		args["room_id"] = roomID
		return nil
	}
	caller := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
	if !caller.matches(roster.ManagerID) {
		return nil
	}
	if name == "message_send" && platformText(args, "room_id") == roomID {
		target := h.participantBridgeTargetForRoomMember(platformText(args, "mention_id"))
		for _, worker := range roster.WorkerIDs {
			if target.matches(worker) {
				return fmt.Errorf("ordinary mentions do not dispatch Workers in this on-demand room; use room_task_create, room_task_plan and room_task_dispatch, or room_task_message for an existing task")
			}
		}
	}
	if name == "agent_task_create" {
		return fmt.Errorf("this turn belongs to on-demand room %s; use room_task_create, room_task_plan and room_task_dispatch so work and replies stay in this room; agent_task_create delivers to a private chat", roomID)
	}
	return nil
}

func bindAutonomousMessageTarget(args map[string]any, roomID, threadRootID string) error {
	if requested := platformText(args, "room_id"); requested != "" && requested != roomID {
		return fmt.Errorf("message operation must stay in the current collaboration room")
	}
	if requested := platformText(args, "thread_root_id"); requested != "" && requested != threadRootID {
		return fmt.Errorf("message operation must stay in the current conversation thread")
	}
	args["room_id"], args["thread_root_id"] = roomID, threadRootID
	return nil
}

func (h *Handler) bindAutonomousRoomMessage(args map[string]any, name, roomID, threadRootID, sourceID string) error {
	if threadRootID == "" {
		taskThread := h.im.AutonomousTaskThread(roomID, sourceID)
		requested := platformText(args, "thread_root_id")
		if requested != "" && (requested == taskThread || h.im.AutonomousTaskThread(roomID, requested) == requested) {
			// A main-chat turn can address a confirmed request's topic even
			// when a different request led the coalesced notification.
			threadRootID = requested
		} else if name == "message_send" && requested == "" {
			threadRootID = taskThread
		}
	}
	return bindAutonomousMessageTarget(args, roomID, threadRootID)
}

// CLI and MCP calls without native turn metadata have an Agent identity only.
// During autonomous work, the active conversation must be unambiguous before
// sending a message or reserving and acknowledging its unread page.
func (h *Handler) scopeUnboundAutonomousMessage(agentID, name string, args map[string]any) error {
	return h.scopeUnboundAutonomous(agentID, args, false, name)
}

func (h *Handler) scopeUnboundAutonomous(agentID string, args map[string]any, requireRoom bool, name string) error {
	reader, ok := h.participantWork.(interface {
		ActiveWorkForParticipant(string) []apitypes.ParticipantWorkUpdate
	})
	if !ok || h.im == nil {
		if requireRoom {
			return fmt.Errorf("this tool requires an active autonomous room turn")
		}
		return nil
	}
	type scope struct{ roomID, threadRootID string }
	scopes := make(map[scope]struct{})
	var current scope
	var sourceID string
	for _, work := range reader.ActiveWorkForParticipant(h.agentPlatformParticipant(agentID)) {
		if work.Kind != apitypes.ParticipantWorkKindAgentTurn {
			continue
		}
		target := scope{work.RoomID, work.ThreadRootID}
		scopes[target] = struct{}{}
		if room, ok := h.im.Room(work.RoomID); ok && room.IsAutonomous() {
			current = target
			sourceID = work.RequestID
		}
	}
	if current.roomID == "" {
		if requireRoom {
			return fmt.Errorf("this tool requires an active autonomous room turn")
		}
		return nil
	}
	if len(scopes) != 1 {
		return fmt.Errorf("multiple active conversations; use a message tool bound to the current turn")
	}
	if !requireRoom {
		return h.bindAutonomousRoomMessage(args, name, current.roomID, current.threadRootID, sourceID)
	}
	return bindAutonomousMessageTarget(args, current.roomID, current.threadRootID)
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
	// Internal requests must not inherit /agents/{id}/mcp route parameters.
	ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
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
	h.registerBrowserTools(agentID)
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
	h.addAppPlatformTool(agentID, "messages_list", "Preview public history without consuming unread messages; use thread_root_id for a specific thread.", platformTextFields("room_id", "thread_root_id"), []string{"room_id"}, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		id := platformText(args, "room_id")
		if err := h.requirePlatformRoom(agentID, id); err != nil {
			return nil, err
		}
		return h.invokeAppPlatformHandler(ctx, agentID, http.MethodGet, nil, url.Values{"room_id": {id}, "thread_root_id": {platformText(args, "thread_root_id")}}, nil, h.handleMessages)
	})
	readFields := platformTextFields("room_id", "thread_root_id")
	readFields["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 100}
	readFields["peek"] = map[string]any{"type": "boolean"}
	h.addAppPlatformTool(agentID, "messages_read", "Read one page of your unread public messages. Then call messages_ack with ack_token only after receiving the complete page. Repeat while has_more; peek does not consume. Main chat and threads have independent cursors.", readFields, []string{"room_id"}, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		roomID := platformText(args, "room_id")
		if err := h.requirePlatformRoom(agentID, roomID); err != nil {
			return nil, err
		}
		limit := 50
		if n, ok := args["limit"].(float64); ok {
			limit = int(n)
		}
		peek, _ := args["peek"].(bool)
		return h.im.PrepareUnread(roomID, h.agentPlatformParticipant(agentID), platformText(args, "thread_root_id"), limit, peek)
	})
	h.addAppPlatformTool(agentID, "messages_ack", "Confirm successful receipt of your previously read page. Never acknowledge an unread or incomplete page; this does not mean the task is completed.", platformTextFields("room_id", "thread_root_id", "ack_token"), []string{"room_id", "ack_token"}, true, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		roomID := platformText(args, "room_id")
		if err := h.requirePlatformRoom(agentID, roomID); err != nil {
			return nil, err
		}
		err := h.im.AcknowledgeUnread(apitypes.UnreadAck{RoomID: roomID, ParticipantID: h.agentPlatformParticipant(agentID), ThreadRootID: platformText(args, "thread_root_id"), Token: platformText(args, "ack_token")})
		return map[string]bool{"acknowledged": err == nil}, err
	})
	h.addAppPlatformTool(agentID, "message_send", "Send a message as yourself to a room you belong to. Mentions follow the room's routing policy. In an on-demand room, message mentions do not dispatch work; use room_task_dispatch to start a Worker and room_task_message for task-scoped communication.", platformTextFields("room_id", "content", "mention_id", "client_message_id", "thread_root_id"), []string{"room_id", "content"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
		if err := h.requirePlatformRoom(agentID, platformText(args, "room_id")); err != nil {
			return nil, err
		}
		payload := map[string]any{}
		for key, value := range args {
			payload[key] = value
		}
		payload["sender_id"] = h.agentPlatformParticipant(agentID)
		if thread := platformText(args, "thread_root_id"); thread != "" {
			payload["relates_to"] = map[string]string{"rel_type": "m.thread", "event_id": thread}
		}
		delete(payload, "thread_root_id")
		return h.invokeAppPlatformHandler(ctx, agentID, http.MethodPost, nil, nil, payload, h.handleCreateMessage)
	})
	if !h.agentPlatformManager(agentID) {
		return
	}
	fields := platformTextFields("title", "source_room_id", "source_message_id", "type")
	fields["type"] = map[string]any{
		"type":        "string",
		"enum":        []string{"", string(apitypes.RoomTypeOnDemand), string(apitypes.RoomTypeAutonomous)},
		"default":     string(apitypes.RoomTypeOnDemand),
		"description": "Room mode: on_demand (moderated collaboration, default) or autonomous (roundtable collaboration). Omitted or empty means on_demand. Legacy free rooms cannot be created.",
	}
	fields["member_ids"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	h.addAppPlatformTool(agentID, "room_create", "Create an explicitly requested CSGClaw collaboration space (group conversation), using moderated collaboration (on_demand) by default. Set type to autonomous for roundtable collaboration. This creates a group even with one invited Agent; it does not create a direct chat. The creator is resolved from a stored human request, never supplied as an identity argument.", fields, []string{"title", "source_room_id", "source_message_id", "member_ids"}, false, func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (any, error) {
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
		roomType := apitypes.RoomType(platformText(args, "type"))
		if roomType == "" {
			roomType = apitypes.RoomTypeOnDemand
		}
		payload := map[string]any{"title": args["title"], "creator_id": creator, "member_ids": members, "manager_id": h.agentPlatformParticipant(agentID), "type": roomType, "is_direct": false}
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
