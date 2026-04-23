package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
)

type Handler struct {
	svc               *agent.Service
	botSvc            *bot.Service
	im                *im.Service
	imBus             *im.Bus
	picoclaw          *im.PicoClawBridge
	feishu            *channel.FeishuService
	llm               *llm.Service
	serverAccessToken string
	serverNoAuth      bool
}

type imBootstrapResponse struct {
	CurrentUserID      string    `json:"current_user_id"`
	Users              []im.User `json:"users"`
	Rooms              []im.Room `json:"rooms"`
	InviteDraftUserIDs []string  `json:"invite_draft_user_ids,omitempty"`
}

type imEventResponse struct {
	Type    string      `json:"type"`
	RoomID  string      `json:"room_id,omitempty"`
	Room    *im.Room    `json:"room,omitempty"`
	User    *im.User    `json:"user,omitempty"`
	Message *im.Message `json:"message,omitempty"`
	Sender  *im.User    `json:"sender,omitempty"`
}

type imAgentJoinResponse struct {
	Message string `json:"message"`
	RoomID  string `json:"room_id,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
}

type createMessageRequest struct {
	RoomID    string `json:"room_id"`
	SenderID  string `json:"sender_id"`
	Content   string `json:"content"`
	MentionID string `json:"mention_id,omitempty"`
}

type addRoomMembersRequest struct {
	RoomID    string   `json:"room_id"`
	InviterID string   `json:"inviter_id"`
	UserIDs   []string `json:"user_ids"`
	Locale    string   `json:"locale"`
}

func NewHandler(svc *agent.Service, imSvc *im.Service, imBus *im.Bus, picoclaw *im.PicoClawBridge, feishu *channel.FeishuService, llmSvc *llm.Service) *Handler {
	return NewHandlerWithBotAndAccessToken(svc, nil, imSvc, imBus, picoclaw, feishu, llmSvc, "")
}

func NewHandlerWithBot(svc *agent.Service, botSvc *bot.Service, imSvc *im.Service, imBus *im.Bus, picoclaw *im.PicoClawBridge, feishu *channel.FeishuService, llmSvc *llm.Service) *Handler {
	return NewHandlerWithBotAndAccessToken(svc, botSvc, imSvc, imBus, picoclaw, feishu, llmSvc, "")
}

func NewHandlerWithBotAndAccessToken(svc *agent.Service, botSvc *bot.Service, imSvc *im.Service, imBus *im.Bus, picoclaw *im.PicoClawBridge, feishu *channel.FeishuService, llmSvc *llm.Service, serverAccessToken string) *Handler {
	return NewHandlerWithBotAndAuth(svc, botSvc, imSvc, imBus, picoclaw, feishu, llmSvc, serverAccessToken, false)
}

func NewHandlerWithBotAndAuth(svc *agent.Service, botSvc *bot.Service, imSvc *im.Service, imBus *im.Bus, picoclaw *im.PicoClawBridge, feishu *channel.FeishuService, llmSvc *llm.Service, serverAccessToken string, serverNoAuth bool) *Handler {
	if botSvc != nil {
		botSvc.SetDependencies(svc, imSvc, feishu)
	}
	return &Handler{
		svc:               svc,
		botSvc:            botSvc,
		im:                imSvc,
		imBus:             imBus,
		picoclaw:          picoclaw,
		feishu:            feishu,
		llm:               llmSvc,
		serverAccessToken: serverAccessToken,
		serverNoAuth:      serverNoAuth,
	}
}

func (h *Handler) validateServerAccessToken(authHeader string) bool {
	if h.serverNoAuth {
		return true
	}
	token := strings.TrimSpace(h.serverAccessToken)
	return authHeader == "Bearer "+token
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) handleWorkers(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.svc.ListWorkers())
	case http.MethodPost:
		h.handleCreateWorkerAlias(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleBots(w http.ResponseWriter, r *http.Request) {
	if h.botSvc == nil {
		http.Error(w, "bot service is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		bots, err := h.botSvc.List(r.URL.Query().Get("channel"), r.URL.Query().Get("role"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, bots)
	case http.MethodPost:
		var req apitypes.CreateBotRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		created, err := h.botSvc.Create(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleBotByID(w http.ResponseWriter, r *http.Request) {
	if h.botSvc == nil {
		http.Error(w, "bot service is not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/bots/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := h.botSvc.Delete(r.Context(), r.URL.Query().Get("channel"), id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAgents(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := h.svc.Reload(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, h.svc.List())
	case http.MethodPost:
		h.handleCreateAgentWorker(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/agents/"))
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if id, ok := strings.CutSuffix(path, "/logs"); ok {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.handleAgentLogs(w, r, id)
		return
	}

	id := path
	if strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if err := h.svc.Reload(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a, ok := h.svc.Agent(id)
		if !ok {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, a)
	case http.MethodDelete:
		if err := h.svc.Delete(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "agent not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAgentLogs(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.svc.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lines := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("lines")); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &lines); err != nil || lines <= 0 {
			http.Error(w, "invalid lines value", http.StatusBadRequest)
			return
		}
	}
	follow := parseBoolQuery(r.URL.Query().Get("follow"))

	logWriter := io.Writer(w)
	if follow {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming is not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		logWriter = flushWriter{ResponseWriter: w, flusher: flusher}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := h.svc.StreamLogs(r.Context(), id, follow, lines, logWriter); err != nil {
		if !parseBoolQuery(r.URL.Query().Get("follow")) {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if _, writeErr := io.WriteString(w, err.Error()+"\n"); writeErr != nil {
			return
		}
	}
}

type flushWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		w.flusher.Flush()
	}
	return n, err
}

func parseBoolQuery(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (h *Handler) handleCreateAgentWorker(w http.ResponseWriter, r *http.Request) {
	var req agent.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	req.Role = agent.RoleWorker

	created, err := h.svc.CreateWorker(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.ensureWorkerIMState(created); err != nil {
		http.Error(w, fmt.Sprintf("agent created but failed to ensure im user: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) handleCreateWorkerAlias(w http.ResponseWriter, r *http.Request) {
	if h.botSvc == nil {
		http.Error(w, "bot service is not configured", http.StatusServiceUnavailable)
		return
	}

	var req agent.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	createdBot, err := h.botSvc.Create(r.Context(), bot.CreateRequest{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Role:        string(bot.RoleWorker),
		Channel:     string(bot.ChannelCSGClaw),
		ModelID:     req.ModelID,
	})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "failed to ensure im user") {
			status = http.StatusBadGateway
		}
		http.Error(w, err.Error(), status)
		return
	}

	created, ok := h.svc.Agent(createdBot.AgentID)
	if !ok {
		http.Error(w, "worker agent created through bot service but not found", http.StatusInternalServerError)
		return
	}

	if err := h.ensureWorkerIMState(created); err != nil {
		http.Error(w, fmt.Sprintf("agent created but failed to ensure im user: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) handleIMAgentJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	var req im.AddAgentToConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	joinedAgent, ok := h.svc.Agent(req.AgentID)
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	if _, _, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     joinedAgent.ID,
		Name:   joinedAgent.Name,
		Handle: deriveAgentHandle(joinedAgent),
		Role:   displayRole(joinedAgent.Role),
	}); err != nil {
		http.Error(w, fmt.Sprintf("ensure agent im user: %v", err), http.StatusBadGateway)
		return
	}

	if strings.TrimSpace(req.InviterID) == "" {
		req.InviterID = "u-admin"
	}
	room, err := h.im.AddAgentToRoom(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishRoomEvent(im.EventTypeRoomMembersAdded, room)
	writeJSON(w, http.StatusOK, imAgentJoinResponse{
		Message: "agent joined successfully",
		RoomID:  room.ID,
		AgentID: joinedAgent.ID,
	})
}

func (h *Handler) handleIMBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, presentBootstrap(h.im.Bootstrap()))
}

func (h *Handler) handleRooms(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.im.ListRooms())
	case http.MethodPost:
		h.handleCreateRoom(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleUsers(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.im.ListUsers())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		roomID, err := roomIDFromQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		messages, err := h.im.ListMessages(roomID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, messages)
	case http.MethodPost:
		h.handleCreateMessage(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleRoomByID(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	id, membersPath := parseRoomMembersPath(r.URL.Path)
	if id == "" {
		http.NotFound(w, r)
		return
	}

	if membersPath {
		h.handleRoomMembersByID(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := h.im.DeleteRoom(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "room not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleRoomMembersByID(w http.ResponseWriter, r *http.Request, roomID string) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		h.handleAddRoomMembers(w, r, roomID)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	members, err := h.im.ListMembers(roomID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *Handler) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/users/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := h.im.KickUser(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "cannot kick current user") {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	var req createMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	serviceReq, err := req.toServiceRequest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	message, err := h.im.CreateMessage(serviceReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishMessageCreated(serviceReq.RoomID, serviceReq.SenderID, message)
	writeJSON(w, http.StatusCreated, message)
}

func (h *Handler) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req apitypes.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	room, err := h.im.CreateRoom(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishRoomEvent(im.EventTypeRoomCreated, room)
	writeJSON(w, http.StatusCreated, room)
}

func (h *Handler) handleIMMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handleCreateMessage(w, r)
}

func (h *Handler) handleIMRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handleCreateRoom(w, r)
}

func (h *Handler) handleIMRoomMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handleAddRoomMembers(w, r, "")
}

func (h *Handler) handleAddRoomMembers(w http.ResponseWriter, r *http.Request, pathRoomID string) {
	var req addRoomMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	pathRoomID = strings.TrimSpace(pathRoomID)
	if pathRoomID != "" {
		bodyRoomID := strings.TrimSpace(req.RoomID)
		if bodyRoomID != "" && bodyRoomID != pathRoomID {
			http.Error(w, "room_id does not match path room id", http.StatusBadRequest)
			return
		}
		req.RoomID = pathRoomID
	}

	serviceReq, err := req.toServiceRequest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	room, err := h.im.AddRoomMembers(serviceReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishRoomEvent(im.EventTypeRoomMembersAdded, room)
	writeJSON(w, http.StatusOK, room)
}

func (h *Handler) handleIMEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.imBus == nil {
		http.Error(w, "im events are not configured", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	events, cancel := h.imBus.Subscribe()
	defer cancel()

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(presentEvent(evt))
			if err != nil {
				return
			}
			if _, err := io.Copy(w, bytes.NewReader([]byte("data: "))); err != nil {
				return
			}
			if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func deriveAgentHandle(a agent.Agent) string {
	if handle, ok := sanitizeHandle(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(a.Name), " ", "-"))); ok {
		return handle
	}
	if handle, ok := sanitizeHandle(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a.ID), "u-"))); ok {
		return handle
	}
	switch strings.ToLower(strings.TrimSpace(a.Role)) {
	case agent.RoleManager:
		return "manager"
	case agent.RoleWorker:
		return "worker"
	default:
		return "agent"
	}
}

func displayRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case agent.RoleManager:
		return "manager"
	case agent.RoleWorker:
		return "Worker"
	default:
		return "Agent"
	}
}

func sanitizeHandle(input string) (string, bool) {
	var b strings.Builder
	hasAlphaNum := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			hasAlphaNum = true
			b.WriteRune(r)
			continue
		}
		if r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || !hasAlphaNum {
		return "", false
	}
	return b.String(), true
}

func roomIDFromQuery(r *http.Request) (string, error) {
	roomID := strings.TrimSpace(r.URL.Query().Get("room_id"))
	if roomID == "" {
		return "", fmt.Errorf("room_id is required")
	}
	return roomID, nil
}

func presentBootstrap(state im.Bootstrap) imBootstrapResponse {
	return imBootstrapResponse{
		CurrentUserID:      state.CurrentUserID,
		Users:              state.Users,
		Rooms:              state.Rooms,
		InviteDraftUserIDs: state.InviteDraftUserIDs,
	}
}

func presentEvent(evt im.Event) imEventResponse {
	return imEventResponse{
		Type:    evt.Type,
		RoomID:  evt.RoomID,
		Room:    evt.Room,
		User:    evt.User,
		Message: evt.Message,
		Sender:  evt.Sender,
	}
}

func (r createMessageRequest) toServiceRequest() (im.CreateMessageRequest, error) {
	roomID := strings.TrimSpace(r.RoomID)
	if roomID == "" {
		return im.CreateMessageRequest{}, fmt.Errorf("room_id is required")
	}

	return im.CreateMessageRequest{
		RoomID:    roomID,
		SenderID:  r.SenderID,
		Content:   r.Content,
		MentionID: r.MentionID,
	}, nil
}

func (r addRoomMembersRequest) toServiceRequest() (im.AddRoomMembersRequest, error) {
	roomID := strings.TrimSpace(r.RoomID)
	if roomID == "" {
		return im.AddRoomMembersRequest{}, fmt.Errorf("room_id is required")
	}

	return im.AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: r.InviterID,
		UserIDs:   r.UserIDs,
		Locale:    r.Locale,
	}, nil
}

func parseRoomMembersPath(path string) (string, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/api/v1/rooms/"), "/")
	if rest == "" {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		return parts[0], false
	}
	if len(parts) == 2 && parts[1] == "members" {
		return parts[0], true
	}
	return "", false
}

func (h *Handler) ensureWorkerIMState(created agent.Agent) error {
	if h.im == nil {
		return nil
	}
	user, room, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID:     created.ID,
		Name:   created.Name,
		Handle: deriveAgentHandle(created),
		Role:   displayRole(created.Role),
	})
	if err != nil {
		return err
	}
	h.publishUserEvent(im.EventTypeUserCreated, user)
	if room != nil {
		h.publishRoomEvent(im.EventTypeRoomCreated, *room)
		imSvc := h.im
		roomID := room.ID
		name := created.Name
		description := created.Description
		go func() {
			time.Sleep(time.Second)
			message, err := imSvc.CreateMessage(im.CreateMessageRequest{
				RoomID:   roomID,
				SenderID: "u-admin",
				Content:  buildWorkerBootstrapMessage(name, description),
			})
			if err != nil {
				return
			}
			h.publishMessageCreated(roomID, message.SenderID, message)
		}()
	}
	return nil
}

func buildWorkerBootstrapMessage(name, description string) string {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	message := fmt.Sprintf("@%s Write this down in your memory: your name is %s.", name, name)
	if description == "" {
		return message
	}
	return fmt.Sprintf("%s Your responsibility is %s", message, description)
}

func (h *Handler) publishMessageCreated(conversationID, senderID string, message im.Message) {
	if h.imBus == nil {
		return
	}
	sender, ok := h.im.User(senderID)
	if !ok {
		return
	}
	messageCopy := message
	senderCopy := sender
	h.imBus.Publish(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  conversationID,
		Message: &messageCopy,
		Sender:  &senderCopy,
	})
}

func (h *Handler) publishRoomEvent(eventType string, room im.Room) {
	if h.imBus == nil {
		return
	}
	roomCopy := room
	h.imBus.Publish(im.Event{
		Type:   eventType,
		RoomID: room.ID,
		Room:   &roomCopy,
	})
}

func (h *Handler) publishUserEvent(eventType string, user im.User) {
	if h.imBus == nil {
		return
	}
	userCopy := user
	h.imBus.Publish(im.Event{
		Type: eventType,
		User: &userCopy,
	})
}
