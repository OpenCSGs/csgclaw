package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/apps"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
)

// EnableApps wires the same installation service into HTTP administration and
// the Agent-scoped MCP endpoint. Call before accepting requests.
func (h *Handler) EnableApps(statePath string) error {
	service, err := apps.NewService(statePath, apps.Options{
		ReadOnly: h.appReadOnlyAgent,
		ResolveFeishu: func(ctx context.Context, agentID string) (apps.FeishuCredentials, error) {
			if err := ctx.Err(); err != nil {
				return apps.FeishuCredentials{}, err
			}
			info, err := h.feishuBotAppInfoForAgent(agentID)
			if err != nil {
				return apps.FeishuCredentials{}, fmt.Errorf("Feishu channel credentials are unavailable")
			}
			return apps.FeishuCredentials{AppID: info.AppID, AppSecret: info.AppSecret}, nil
		},
		OnCatalogChanged: func(agentID string, revision uint64) {
			// Never make an app-server RPC while answering its MCP request.
			// Every prompt also checks the revision source before admission.
			go func() {
				if rt := h.appCodexRuntime(); rt != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					if err := rt.RefreshAgentMCP(ctx, agentID, revision); err != nil {
						slog.Warn("refresh Agent App tools", "agent_id", agentID, "error", err)
					}
				}
			}()
		},
	})
	if err != nil {
		return err
	}
	h.apps = service
	h.appPlatformAgents = make(map[string]string)
	if rt := h.appCodexRuntime(); rt != nil {
		rt.SetAgentMCPRevisionSource(service.Revision)
	}
	if owner, ok := h.svc.(interface {
		SetAgentResourceCleanup(func(context.Context, string) error)
	}); ok {
		owner.SetAgentResourceCleanup(func(ctx context.Context, agentID string) error {
			if err := service.DeleteAgent(ctx, agentID); err != nil {
				return err
			}
			h.appPlatformMu.Lock()
			delete(h.appPlatformAgents, agentID)
			h.appPlatformMu.Unlock()
			return nil
		})
	}
	return nil
}

func (h *Handler) appReadOnlyAgent(agentID string) bool {
	if h.svc == nil {
		return false
	}
	a, ok := h.svc.Agent(agentID)
	if !ok || a.RuntimeKind != agent.RuntimeKindCodex {
		return false
	}
	options, err := runtimecodex.DecodeRuntimeOptions(a.RuntimeOptions)
	return err != nil || options.ExecutionMode == runtimecodex.ExecutionModeReadOnly
}

func (h *Handler) appCodexRuntime() *runtimecodex.Runtime {
	owner, ok := h.svc.(interface {
		Runtime(string) (agentruntime.Runtime, error)
	})
	if !ok {
		return nil
	}
	rt, err := owner.Runtime(agent.RuntimeKindCodex)
	if err != nil {
		return nil
	}
	result, _ := rt.(*runtimecodex.Runtime)
	return result
}

func (h *Handler) RestoreApps(ctx context.Context) {
	if h.apps == nil {
		return
	}
	if err := h.apps.RestoreAll(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("restore Agent Apps", "error", err)
	}
}

func (h *Handler) CloseApps() error {
	if h.apps == nil {
		return nil
	}
	if rt := h.appCodexRuntime(); rt != nil {
		rt.SetAgentMCPRevisionSource(nil)
	}
	return h.apps.Close()
}

func (h *Handler) refreshAppChannelCredentials(item apitypes.Participant) {
	if h.apps == nil || item.Channel != participant.ChannelFeishu || item.AgentID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.apps.RefreshCredentials(ctx, item.AgentID); err != nil {
			slog.Warn("refresh App channel credentials", "agent_id", item.AgentID)
		}
	}()
}

func (h *Handler) requireAppAgent(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.apps == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "apps_unavailable", "App service is unavailable")
		return "", false
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if h.svc == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "apps_unavailable", "Agent service is unavailable")
		return "", false
	}
	a, ok := h.svc.Agent(id)
	if !ok {
		writeCodedAPIError(w, http.StatusNotFound, "agent_not_found", "Agent not found")
		return "", false
	}
	return a.ID, true
}

func (h *Handler) handleApps(w http.ResponseWriter, r *http.Request) {
	if h.apps == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "apps_unavailable", "App service is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if id := pathValue(r, "app_id"); id != "" {
		item, err := h.apps.Definition(id)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": h.apps.Catalog()})
}

func (h *Handler) handleAgentApps(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAppAgent(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		items, err := h.apps.List(r.Context(), agentID)
		if err != nil {
			writeAppError(w, err)
			return
		}
		_, channelErr := h.feishuBotAppInfoForAgent(agentID)
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "feishu_channel_available": channelErr == nil})
		return
	}
	var req apps.CreateRequest
	if !decodeAppRequest(w, r, &req) {
		return
	}
	var item apps.Installation
	create := func(ctx context.Context) error {
		// App creation shares the Agent deletion lease and rechecks existence
		// after acquiring it, so deletion cannot leave a late installation.
		if _, exists := h.svc.Agent(agentID); !exists {
			return apps.ErrNotFound
		}
		var err error
		item, err = h.apps.Create(ctx, agentID, req)
		return err
	}
	var err error
	if h.agentRuntime != nil {
		err = h.agentRuntime.WithAgentLifecycle(r.Context(), agentID, create)
	} else {
		err = create(r.Context())
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) handleAgentApp(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAppAgent(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	id := pathValue(r, "installation_id")
	var item apps.Installation
	var err error
	switch r.Method {
	case http.MethodGet:
		item, err = h.apps.Get(r.Context(), agentID, id)
	case http.MethodPatch:
		var req apps.UpdateRequest
		if !decodeAppRequest(w, r, &req) {
			return
		}
		item, err = h.apps.Update(r.Context(), agentID, id, req)
	case http.MethodDelete:
		if err = h.apps.Delete(r.Context(), agentID, id); err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	case http.MethodPost:
		switch pathValue(r, "app_action") {
		case "connect":
			item, err = h.apps.Connect(r.Context(), agentID, id)
		case "disconnect":
			item, err = h.apps.Disconnect(r.Context(), agentID, id)
		default:
			err = apps.ErrInvalid
		}
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleAgentAppsProbe(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAppAgent(w, r)
	if !ok {
		return
	}
	var req apps.ProbeRequest
	if !decodeAppRequest(w, r, &req) {
		return
	}
	item, err := h.apps.Probe(r.Context(), agentID, req)
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleAppOAuthUnsupported(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAppAgent(w, r)
	if !ok {
		return
	}
	if _, err := h.apps.Get(r.Context(), agentID, pathValue(r, "installation_id")); err != nil {
		writeAppError(w, err)
		return
	}
	writeAppError(w, apps.ErrUnsupportedOAuth)
}

func (h *Handler) handleAgentAppMCP(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAppAgent(w, r)
	if !ok {
		return
	}
	if !h.authorizesAgentToken(agentID, r.Header.Get("Authorization")) {
		writeCodedAPIError(w, http.StatusUnauthorized, "unauthorized", "Agent credential required")
		return
	}
	h.registerAppPlatformTools(agentID)
	h.apps.Handler(agentID).ServeHTTP(w, r)
}

func decodeAppRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "app_invalid_configuration", "Invalid App request")
		return false
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "app_invalid_configuration", "Invalid App request")
		return false
	}
	return true
}

func writeAppError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusBadGateway, "app_connection_failed", "App connection failed; check its settings and credentials"
	switch {
	case errors.Is(err, apps.ErrNotFound):
		status, code, message = http.StatusNotFound, "app_not_found", "App not found"
	case errors.Is(err, apps.ErrConflict):
		status, code, message = http.StatusConflict, "app_name_conflict", "App name already exists"
	case errors.Is(err, apps.ErrUnsupportedOAuth):
		status, code, message = http.StatusNotImplemented, "app_oauth_unsupported", "OAuth2 is not supported in this version"
	case errors.Is(err, apps.ErrInvalid):
		status, code, message = http.StatusBadRequest, "app_invalid_configuration", "Invalid App configuration"
	}
	writeCodedAPIError(w, status, code, message)
}
