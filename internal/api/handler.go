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
	imProvisioner     *im.Provisioner
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

type agentResponse struct {
	ID               string                         `json:"id"`
	Name             string                         `json:"name"`
	Description      string                         `json:"description,omitempty"`
	Image            string                         `json:"image,omitempty"`
	BoxID            string                         `json:"box_id,omitempty"`
	Role             string                         `json:"role"`
	Status           string                         `json:"status"`
	CreatedAt        time.Time                      `json:"created_at"`
	Profile          string                         `json:"profile,omitempty"`
	Provider         string                         `json:"provider,omitempty"`
	ModelID          string                         `json:"model_id,omitempty"`
	ReasoningEffort  string                         `json:"reasoning_effort,omitempty"`
	AgentProfile     agent.AgentProfileView         `json:"agent_profile,omitempty"`
	ProfileComplete  bool                           `json:"profile_complete"`
	DetectionResults []agent.ProfileDetectionResult `json:"detection_results,omitempty"`
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
		botSvc.SetIMBus(imBus)
	}
	return &Handler{
		svc:               svc,
		botSvc:            botSvc,
		im:                imSvc,
		imBus:             imBus,
		imProvisioner:     im.NewProvisioner(imSvc, imBus),
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

func (h *Handler) handleBots(w http.ResponseWriter, r *http.Request) {
	if h.botSvc == nil {
		http.Error(w, "bot service is not configured", http.StatusServiceUnavailable)
		return
	}
	h.botSvc.SetIMBus(h.imBus)

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
		writeJSON(w, http.StatusOK, presentAgents(h.svc.List()))
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
	if id, ok := strings.CutSuffix(path, "/start"); ok {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.handleAgentStart(w, r, id)
		return
	}
	if id, ok := strings.CutSuffix(path, "/stop"); ok {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.handleAgentStop(w, r, id)
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
	if id, ok := strings.CutSuffix(path, "/profile"); ok {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.handleAgentProfile(w, r, id)
		return
	}
	if id, ok := strings.CutSuffix(path, "/recreate"); ok {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		h.handleAgentRecreate(w, r, id)
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
		writeJSON(w, http.StatusOK, presentAgent(a))
	case http.MethodPatch:
		var req agent.UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		updated, err := h.svc.Update(r.Context(), id, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, presentAgent(updated))
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

func (h *Handler) handleAgentProfile(w http.ResponseWriter, r *http.Request, id string) {
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
		profile, err := h.svc.AgentProfileView(id)
		if err != nil {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case http.MethodPut:
		var req agent.AgentProfile
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		profile, err := h.svc.UpdateAgentProfile(id, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAgentRecreate(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	recreated, err := h.svc.Recreate(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, presentAgent(recreated))
}

func (h *Handler) handleAgentProfileModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	var req agent.ProfileModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	models, err := h.svc.ListModelsForRequest(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, agent.ProfileModelsResponse{
		Provider: req.Provider,
		Models:   models,
	})
}

func (h *Handler) handleAgentProfileDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, h.svc.ProfileDefaultsView())
}

func (h *Handler) handleAgentStart(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.svc.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	started, err := h.svc.Start(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, presentAgent(started))
}

func (h *Handler) handleAgentStop(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.svc.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stopped, err := h.svc.Stop(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, presentAgent(stopped))
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
	var req apitypes.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	created, err := h.svc.Create(r.Context(), agentCreateRequestFromAPI(req))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if provisioner := h.workerIMProvisioner(); provisioner != nil {
		if _, err := provisioner.EnsureAgentUser(r.Context(), im.AgentIdentity{
			ID:          created.ID,
			Name:        created.Name,
			Description: created.Description,
			Handle:      deriveAgentHandle(created),
			Role:        displayRole(created.Role),
		}); err != nil {
			http.Error(w, fmt.Sprintf("ensure agent im user: %v", err), http.StatusBadGateway)
			return
		}
	}

	writeJSON(w, http.StatusCreated, presentAgent(created))
}

func agentCreateRequestFromAPI(req apitypes.CreateAgentRequest) agent.CreateRequest {
	return agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:           req.ID,
			Name:         req.Name,
			Description:  req.Description,
			Image:        req.Image,
			Role:         req.Role,
			Status:       req.Status,
			CreatedAt:    req.CreatedAt,
			Profile:      req.Profile,
			ModelID:      req.ModelID,
			AgentProfile: agentProfileFromAPI(req.AgentProfile),
		},
		Replace:   req.Replace,
		FieldMask: req.FieldMask,
	}
}

func agentProfileFromAPI(req apitypes.CreateAgentProfile) agent.AgentProfile {
	return agent.AgentProfile{
		Name:            req.Name,
		Description:     req.Description,
		Provider:        req.Provider,
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		Headers:         req.Headers,
		ModelID:         req.ModelID,
		ReasoningEffort: req.ReasoningEffort,
		EnableFastMode:  req.EnableFastMode,
		RequestOptions:  req.RequestOptions,
		Env:             req.Env,
		ProfileComplete: req.ProfileComplete,
	}
}

func (h *Handler) workerIMProvisioner() *im.Provisioner {
	if h == nil || h.im == nil {
		return nil
	}
	if h.imProvisioner == nil {
		h.imProvisioner = im.NewProvisioner(h.im, h.imBus)
	}
	return h.imProvisioner
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
	if err := h.reloadIM(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, h.im.ListUsers())
	case http.MethodPost:
		h.handleCreateUser(w, r)
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
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		if err := h.reloadIM(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		if err := h.im.DeleteUser(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "cannot delete current user") {
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

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	provisioner := h.workerIMProvisioner()
	if provisioner == nil {
		http.Error(w, "im provisioner is not configured", http.StatusServiceUnavailable)
		return
	}

	var req apitypes.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	handle := strings.TrimSpace(req.Handle)
	role := strings.TrimSpace(req.Role)

	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if handle == "" {
		handle = name
	}

	if h.botSvc != nil && h.svc != nil && shouldCreateWorkerForUser(id, role) {
		h.botSvc.SetDependencies(h.svc, h.im, h.feishu)
		h.botSvc.SetIMBus(h.imBus)
		created, err := h.botSvc.Create(r.Context(), apitypes.CreateBotRequest{
			ID:      id,
			Name:    name,
			Role:    string(bot.RoleWorker),
			Channel: string(bot.ChannelCSGClaw),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if user, ok := h.im.User(created.UserID); ok {
			writeJSON(w, http.StatusCreated, user)
			return
		}
		http.Error(w, "created worker user not found", http.StatusInternalServerError)
		return
	}

	result, err := provisioner.EnsureAgentUser(r.Context(), im.AgentIdentity{
		ID:     id,
		Name:   name,
		Handle: handle,
		Role:   role,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, result.User)
}

func shouldCreateWorkerForUser(id, role string) bool {
	id = strings.TrimSpace(id)
	switch strings.ToLower(id) {
	case "", "u-admin", agent.ManagerUserID:
		return false
	}

	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", agent.RoleWorker, agent.RoleAgent:
		return true
	case agent.RoleManager, "admin":
		return false
	default:
		return true
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

func presentAgents(items []agent.Agent) []agentResponse {
	out := make([]agentResponse, 0, len(items))
	for _, item := range items {
		out = append(out, presentAgent(item))
	}
	return out
}

func presentAgent(item agent.Agent) agentResponse {
	return agentResponse{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		Image:            item.Image,
		BoxID:            item.BoxID,
		Role:             item.Role,
		Status:           item.Status,
		CreatedAt:        item.CreatedAt,
		Profile:          item.Profile,
		Provider:         item.Provider,
		ModelID:          item.ModelID,
		ReasoningEffort:  item.ReasoningEffort,
		AgentProfile:     agent.RedactedProfileView(item.AgentProfile, item.DetectionResults),
		ProfileComplete:  item.ProfileComplete,
		DetectionResults: append([]agent.ProfileDetectionResult(nil), item.DetectionResults...),
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

func (h *Handler) reloadIM() error {
	if h == nil || h.im == nil {
		return nil
	}
	return h.im.Reload()
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
