package api

import (
	"net/http"

	runtimenotifier "csgclaw/internal/runtime/notifier"
)

// notifyWebhookDeps wires the HTTP API handler into notifier webhook delivery. It stays in this
// package (not in internal/runtime/notifier) to avoid an import cycle: deps need *Handler.svc / im.
func notifyWebhookDeps(h *Handler) runtimenotifier.WebhookHTTPDeps {
	var reload func() error
	var lookup func(string) (map[string]any, string, string, string, bool)
	if h.svc != nil {
		reload = h.svc.Reload
		lookup = func(id string) (map[string]any, string, string, string, bool) {
			a, ok := h.svc.Agent(id)
			if !ok {
				return nil, "", "", "", false
			}
			return a.RuntimeOptions, a.Role, a.RuntimeKind, a.Status, true
		}
	}
	return runtimenotifier.WebhookHTTPDeps{
		Reload:              reload,
		LookupNotifierAgent: lookup,
		Fanout:              h.notifierFanoutBridge(),
	}
}

func (h *Handler) handleNotifyHTTP(w http.ResponseWriter, r *http.Request) {
	runtimenotifier.ServeNotifyHTTP(w, r, notifyWebhookDeps(h))
}

func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	h.registerCoreRoutes(mux)
	h.registerNotifierRoutes(mux)
	h.registerChannelRoutes(mux)
	h.registerBotCompatibilityRoutes(mux)
	return mux
}

func (h *Handler) registerCoreRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/api/v1/version", h.handleVersion)
	mux.HandleFunc("/api/v1/upgrade/status", h.handleUpgradeStatus)
	mux.HandleFunc("/api/v1/upgrade/apply", h.handleUpgradeApply)
	mux.HandleFunc("/api/v1/bots", h.handleBots)
	mux.HandleFunc("/api/v1/bots/", h.handleBotByID)
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
	mux.HandleFunc("/api/v1/agents/", h.handleAgentByID)
	mux.HandleFunc("/api/v1/hub/templates", h.handleHubTemplates)
	mux.HandleFunc("/api/v1/hub/templates/", h.handleHubTemplateByID)
	mux.HandleFunc("/api/v1/cliproxy/auth/status", h.handleCLIProxyAuthStatus)
	mux.HandleFunc("/api/v1/cliproxy/auth/login", h.handleCLIProxyAuthLogin)
	mux.HandleFunc("/api/v1/agent-profiles/models", h.handleAgentProfileModels)
	mux.HandleFunc("/api/v1/agent-profile-defaults", h.handleAgentProfileDefaults)
	mux.HandleFunc("/api/v1/config/bootstrap", h.handleBootstrapConfig)
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

func (h *Handler) registerNotifierRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/notify/", h.handleNotifyHTTP)
}

func (h *Handler) registerChannelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/channels/feishu/config", h.handleFeishuConfig)
	mux.HandleFunc("/api/v1/channels/feishu/bots/", h.handleFeishuBotByID)
	mux.HandleFunc("/api/v1/channels/feishu/users", h.handleFeishuUsers)
	mux.HandleFunc("/api/v1/channels/feishu/users/", h.handleFeishuUserByID)
	mux.HandleFunc("/api/v1/channels/feishu/rooms", h.handleFeishuRooms)
	mux.HandleFunc("/api/v1/channels/feishu/rooms/", h.handleFeishuRoomByID)
	mux.HandleFunc("/api/v1/channels/feishu/messages", h.handleFeishuMessages)
}
