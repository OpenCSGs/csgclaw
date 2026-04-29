package api

import "net/http"

func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	h.registerCoreRoutes(mux)
	h.registerChannelRoutes(mux)
	h.registerPicoClawRoutes(mux)
	return mux
}

func (h *Handler) registerCoreRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/api/v1/bots", h.handleBots)
	mux.HandleFunc("/api/v1/bots/", h.handleBotByID)
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
	mux.HandleFunc("/api/v1/agents/", h.handleAgentByID)
	mux.HandleFunc("/api/v1/cliproxy/auth/status", h.handleCLIProxyAuthStatus)
	mux.HandleFunc("/api/v1/cliproxy/auth/login", h.handleCLIProxyAuthLogin)
	mux.HandleFunc("/api/v1/agent-profiles/models", h.handleAgentProfileModels)
	mux.HandleFunc("/api/v1/agent-profile-defaults", h.handleAgentProfileDefaults)
	mux.HandleFunc("/api/v1/bootstrap", h.handleIMBootstrap)
	mux.HandleFunc("/api/v1/events", h.handleIMEvents)
	mux.HandleFunc("/api/v1/rooms", h.handleRooms)
	mux.HandleFunc("/api/v1/rooms/", h.handleRoomByID)
	mux.HandleFunc("/api/v1/rooms/invite", h.handleIMRoomMembers)
	mux.HandleFunc("/api/v1/users", h.handleUsers)
	mux.HandleFunc("/api/v1/users/", h.handleUserByID)
	mux.HandleFunc("/api/v1/messages", h.handleMessages)
	mux.HandleFunc("/api/v1/im/agents/join", h.handleIMAgentJoin)
	mux.HandleFunc("/api/v1/im/bootstrap", h.handleIMBootstrap)
	mux.HandleFunc("/api/v1/im/events", h.handleIMEvents)
	mux.HandleFunc("/api/v1/im/messages", h.handleIMMessages)
	mux.HandleFunc("/api/v1/im/conversations", h.handleIMRooms)
	mux.HandleFunc("/api/v1/im/conversations/members", h.handleIMRoomMembers)
	mux.HandleFunc("/api/v1/im/rooms", h.handleIMRooms)
	mux.HandleFunc("/api/v1/im/rooms/invite", h.handleIMRoomMembers)
}

func (h *Handler) registerChannelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/channels/feishu/bots/", h.handleFeishuBotByID)
	mux.HandleFunc("/api/v1/channels/feishu/users", h.handleFeishuUsers)
	mux.HandleFunc("/api/v1/channels/feishu/users/", h.handleFeishuUserByID)
	mux.HandleFunc("/api/v1/channels/feishu/rooms", h.handleFeishuRooms)
	mux.HandleFunc("/api/v1/channels/feishu/rooms/", h.handleFeishuRoomByID)
	mux.HandleFunc("/api/v1/channels/feishu/messages", h.handleFeishuMessages)
}
