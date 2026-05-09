package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/feishu"
)

func (h *Handler) handleFeishuConfig(w http.ResponseWriter, r *http.Request) {
	if h.feishu == nil {
		http.Error(w, "feishu channel is not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleFeishuConfigGet(w, r)
	case http.MethodPut:
		h.handleFeishuConfigPut(w, r)
	case http.MethodPost:
		h.handleFeishuConfigReload(w)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFeishuConfigGet(w http.ResponseWriter, r *http.Request) {
	view, err := h.feishu.GetConfig(r.URL.Query().Get("bot_id"))
	if err != nil {
		writeFeishuConfigError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, presentFeishuConfig(view, false))
}

func (h *Handler) handleFeishuConfigPut(w http.ResponseWriter, r *http.Request) {
	var req apitypes.FeishuConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid feishu channel config request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.BotID) == "" {
		req.BotID = r.URL.Query().Get("bot_id")
	}
	reload := true
	if req.Reload != nil {
		reload = *req.Reload
	}

	view, err := h.feishu.UpdateConfig(feishu.Update{
		BotID:       req.BotID,
		AppID:       req.AppID,
		AppSecret:   req.AppSecret,
		AdminOpenID: req.AdminOpenID,
	})
	if err != nil {
		writeFeishuConfigError(w, err)
		return
	}
	if reload {
		if _, err := h.feishu.ReloadConfig(); err != nil {
			writeFeishuConfigError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, presentFeishuConfig(view, reload))
}

func (h *Handler) handleFeishuConfigReload(w http.ResponseWriter) {
	botIDs, err := h.feishu.ReloadConfig()
	if err != nil {
		writeFeishuConfigError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, apitypes.FeishuConfigReloadResponse{
		Status:     "reloaded",
		FeishuBots: botIDs,
	})
}

func writeFeishuConfigError(w http.ResponseWriter, err error) {
	if feishu.IsValidationError(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func presentFeishuConfig(view feishu.Entry, reloaded bool) apitypes.FeishuConfigResponse {
	secretStatus := "missing"
	if view.HasSecret {
		secretStatus = "present"
	}
	return apitypes.FeishuConfigResponse{
		BotID:       view.BotID,
		Configured:  view.Configured,
		AppID:       view.AppID,
		AppSecret:   secretStatus,
		AdminOpenID: view.AdminOpenID,
		Reloaded:    reloaded,
	}
}
