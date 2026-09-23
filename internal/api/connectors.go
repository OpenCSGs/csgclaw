package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apps"
	"csgclaw/internal/connectors"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
	runtimedsh "csgclaw/internal/runtime/dsh"
)

// EnableConnectors wires the same installation service into HTTP administration and
// the Agent-scoped MCP endpoint. Call before accepting requests.
func (h *Handler) EnableConnectors(statePath string) error {
	service, err := apps.NewService(statePath, apps.Options{
		ReadOnly: h.connectorReadOnlyAgent,
		ResolveConnectorHTTP: func(ctx context.Context, agentID string, appID string, cfg apps.Config) (apps.ConnectorHTTPConfig, error) {
			if appID != "gitlab" || h.connectors == nil || strings.TrimSpace(cfg.ConnectorID) != connectors.ProviderGitLab {
				return apps.ConnectorHTTPConfig{}, apps.ErrUnsupportedOAuth
			}
			credential, err := h.connectors.CredentialForAgent(ctx, agentID, connectors.ProviderGitLab)
			if err != nil {
				return apps.ConnectorHTTPConfig{}, err
			}
			if credential.TokenType != "private-token" {
				return apps.ConnectorHTTPConfig{}, fmt.Errorf("GitLab Connector requires a personal access token")
			}
			if !sameNormalizedURL(cfg.GitLabBaseURL, credential.BaseURL) {
				return apps.ConnectorHTTPConfig{}, fmt.Errorf("GitLab App instance does not match the connected GitLab Connector")
			}
			return apps.ConnectorHTTPConfig{
				Endpoint: cfg.URL, Token: credential.AccessToken, TokenHeader: "PRIVATE-TOKEN",
				Headers: http.Header{"X-GitLab-Base-URL": []string{credential.BaseURL}},
			}, nil
		},
		OnCatalogChanged: func(agentID string, revision uint64) {
			// Never refresh a runtime while answering its MCP request.
			// Every prompt also checks the revision source before admission.
			go func() {
				for _, rt := range h.connectorAgentMCPRuntimes() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					if err := rt.RefreshAgentMCP(ctx, agentID, revision); err != nil {
						slog.Warn("refresh Agent App tools", "agent_id", agentID, "error", err)
					}
					cancel()
				}
			}()
		},
	})
	if err != nil {
		return err
	}
	h.apps = service
	h.appPlatformAgents = make(map[string]string)
	for _, rt := range h.connectorAgentMCPRuntimes() {
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

func (h *Handler) connectorReadOnlyAgent(agentID string) bool {
	if h.svc == nil {
		return false
	}
	a, ok := h.svc.Agent(agentID)
	if !ok {
		return false
	}
	return readOnlyRuntime(a.RuntimeKind, a.RuntimeOptions)
}

func readOnlyRuntime(runtimeKind string, options map[string]any) bool {
	switch strings.TrimSpace(runtimeKind) {
	case agent.RuntimeKindCodex:
		decoded, err := runtimecodex.DecodeRuntimeOptions(options)
		return err != nil || decoded.ExecutionMode == runtimecodex.ExecutionModeReadOnly
	case agent.RuntimeKindDSH:
		decoded, err := runtimedsh.DecodeRuntimeOptions(options)
		return err != nil || decoded.PermissionMode == runtimedsh.PermissionModeReadOnly
	default:
		return false
	}
}

func (h *Handler) connectorCodexRuntime() *runtimecodex.Runtime {
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

type connectorAgentMCPRuntime interface {
	SetAgentMCPRevisionSource(func(string) uint64)
	RefreshAgentMCP(context.Context, string, uint64) error
}

func (h *Handler) connectorAgentMCPRuntimes() []connectorAgentMCPRuntime {
	owner, ok := h.svc.(interface {
		Runtime(string) (agentruntime.Runtime, error)
	})
	if !ok {
		return nil
	}
	var runtimes []connectorAgentMCPRuntime
	for _, kind := range []string{agent.RuntimeKindCodex, agent.RuntimeKindDSH} {
		rt, err := owner.Runtime(kind)
		if err == nil {
			if adapter, ok := rt.(connectorAgentMCPRuntime); ok {
				runtimes = append(runtimes, adapter)
			}
		}
	}
	return runtimes
}

func (h *Handler) RestoreConnectors(ctx context.Context) {
	if h.apps == nil {
		return
	}
	if err := h.apps.RestoreAll(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("restore Agent Apps", "error", err)
	}
}

func (h *Handler) CloseConnectors() error {
	if h.apps == nil {
		return nil
	}
	for _, rt := range h.connectorAgentMCPRuntimes() {
		rt.SetAgentMCPRevisionSource(nil)
	}
	return h.apps.Close()
}

func (h *Handler) requireConnectorAgent(w http.ResponseWriter, r *http.Request) (string, bool) {
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

func (h *Handler) handleConnectorCatalog(w http.ResponseWriter, r *http.Request) {
	if h.apps == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "apps_unavailable", "App service is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if id := pathValue(r, "app_id"); id != "" {
		item, err := h.apps.Definition(id)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": h.apps.Catalog()})
}

func (h *Handler) handleAgentConnectors(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireConnectorAgent(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		items, err := h.apps.List(r.Context(), agentID)
		if err != nil {
			writeConnectorError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	var req apps.BindRequest
	if !decodeConnectorRequest(w, r, &req) {
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
		item, err = h.apps.Bind(ctx, agentID, req)
		return err
	}
	var err error
	if h.agentRuntime != nil {
		err = h.agentRuntime.WithAgentLifecycle(r.Context(), agentID, create)
	} else {
		err = create(r.Context())
	}
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) handleAgentConnectorBinding(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireConnectorAgent(w, r)
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
		var req apps.BindingUpdateRequest
		if !decodeConnectorRequest(w, r, &req) {
			return
		}
		item, err = h.apps.Update(r.Context(), agentID, id, apps.UpdateRequest{Enabled: req.Enabled})
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
		writeConnectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) lockGitLabConnectorSettings(appID string, config apps.Config, credentials *apps.Credentials) func() {
	if appID != "gitlab" || (config.AuthMode != "connector" && config.AuthMode != "oauth2") {
		return func() {}
	}
	h.gitLabConnectorMu.Lock()
	return h.gitLabConnectorMu.Unlock
}

func (h *Handler) saveGitLabConnectorSettings(ctx context.Context, agentID, appID string, config apps.Config, credentials *apps.Credentials) (func(), error) {
	noop := func() {}
	if appID != "gitlab" || (config.AuthMode != "connector" && config.AuthMode != "oauth2") || credentials == nil || strings.TrimSpace(credentials.Token) == "" {
		return noop, nil
	}
	if h.connectors == nil {
		return noop, fmt.Errorf("GitLab Connector service is unavailable")
	}
	previous, existed, err := h.connectors.SnapshotGitLab()
	if err != nil {
		return noop, err
	}
	if _, err := h.connectors.SaveGitLabConfig(ctx, connectors.Config{
		BaseURL: config.GitLabBaseURL, AccessToken: credentials.Token,
	}); err != nil {
		return noop, err
	}
	if h.apps != nil {
		if err := h.apps.RefreshConnector(ctx, connectors.ProviderGitLab); err != nil {
			slog.Warn("refresh GitLab Apps after Connector update", "agent_id", agentID, "error", err)
		}
	}
	credentials.Token = ""
	return func() {
		if err := h.connectors.RestoreGitLab(previous, existed); err != nil {
			slog.Error("rollback GitLab Connector after App save failure", "agent_id", agentID, "error", err)
			return
		}
		if h.apps != nil {
			if err := h.apps.RefreshConnector(context.Background(), connectors.ProviderGitLab); err != nil {
				slog.Warn("refresh GitLab Apps after Connector rollback", "agent_id", agentID, "error", err)
			}
		}
	}, nil
}

func (h *Handler) handleAgentConnectorsProbe(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireConnectorAgent(w, r)
	if !ok {
		return
	}
	var req apps.ProbeRequest
	if !decodeConnectorRequest(w, r, &req) {
		return
	}
	item, err := h.probeConnectorDraft(r.Context(), agentID, req)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleConnectorOAuthStart(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireConnectorAgent(w, r)
	if !ok {
		return
	}
	if _, err := h.apps.Get(r.Context(), agentID, pathValue(r, "installation_id")); err != nil {
		writeConnectorError(w, err)
		return
	}
	writeConnectorError(w, apps.ErrUnsupportedOAuth)
}

func sameNormalizedURL(left, right string) bool {
	normalize := func(raw string) string {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Scheme == "" || u.Host == "" {
			return ""
		}
		u.RawQuery = ""
		u.Fragment = ""
		u.Path = strings.TrimRight(u.Path, "/")
		return strings.TrimRight(u.String(), "/")
	}
	return normalize(left) != "" && normalize(left) == normalize(right)
}

func (h *Handler) handleAgentConnectorMCP(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireConnectorAgent(w, r)
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

func decodeConnectorRequest(w http.ResponseWriter, r *http.Request, target any) bool {
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

func writeConnectorError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusBadGateway, "app_connection_failed", "App connection failed; check its settings and credentials"
	var detail *apps.ConnectionError
	switch {
	case errors.As(err, &detail):
		code, message = detail.Code, detail.Message
		if errors.Is(err, apps.ErrInvalid) {
			status = http.StatusBadRequest
		}
	case errors.Is(err, apps.ErrNotFound):
		status, code, message = http.StatusNotFound, "app_not_found", "App not found"
	case errors.Is(err, apps.ErrChanged):
		status, code, message = http.StatusConflict, "app_configuration_changed", "Connector settings changed during validation. Reload and retry."
	case errors.Is(err, apps.ErrConflict):
		status, code, message = http.StatusConflict, "app_name_conflict", "App name already exists"
	case errors.Is(err, apps.ErrUnsupportedOAuth):
		status, code, message = http.StatusNotImplemented, "app_oauth_unsupported", "OAuth2 is not supported in this version"
	case errors.Is(err, apps.ErrInvalid):
		status, code, message = http.StatusBadRequest, "app_invalid_configuration", "Invalid App configuration"
	}
	writeCodedAPIError(w, status, code, message)
}

func (h *Handler) refreshConnectorPlatformAuthentication() {
	if h.apps == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		_ = h.apps.RefreshPlatformCredentials(ctx)
	}()
}

// Global App resources own configuration and secrets. Agent endpoints only bind
// them and control per-Agent execution state.
func (h *Handler) handleConnectorResources(w http.ResponseWriter, r *http.Request) {
	if h.apps == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "apps_unavailable", "App service is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	id := pathValue(r, "resource_id")
	var result any
	var err error
	status := http.StatusOK
	switch r.Method {
	case http.MethodGet:
		if id == "" {
			var items []apps.Installation
			items, err = h.apps.List(r.Context(), "")
			for i := range items {
				items[i] = h.connectorResourceView(items[i])
			}
			result = map[string]any{"items": items}
		} else {
			result, err = h.apps.Get(r.Context(), "", id)
		}
	case http.MethodPost:
		var req apps.CreateRequest
		if !decodeConnectorRequest(w, r, &req) {
			return
		}
		req.Connect = false
		unlockConnector := h.lockGitLabConnectorSettings(req.AppID, req.Config, &req.Credentials)
		defer unlockConnector()
		if _, probeErr := h.apps.Probe(r.Context(), "", apps.ProbeRequest{AppID: req.AppID, Config: req.Config, Credentials: req.Credentials}); probeErr != nil {
			writeConnectorError(w, probeErr)
			return
		}
		rollbackConnector, saveErr := h.saveGitLabConnectorSettings(r.Context(), "", req.AppID, req.Config, &req.Credentials)
		if saveErr != nil {
			writeConnectorError(w, saveErr)
			return
		}
		result, err = h.apps.Create(r.Context(), "", req)
		if err != nil {
			rollbackConnector()
		}
		status = http.StatusCreated
	case http.MethodPatch:
		var req apps.UpdateRequest
		if !decodeConnectorRequest(w, r, &req) {
			return
		}
		current, getErr := h.apps.Get(r.Context(), "", id)
		if getErr != nil {
			writeConnectorError(w, getErr)
			return
		}
		config := current.Config
		if req.Config != nil {
			config = *req.Config
		}
		unlockConnector := h.lockGitLabConnectorSettings(current.AppID, config, req.Credentials)
		defer unlockConnector()
		if req.Config != nil || req.Credentials != nil {
			if probeErr := h.apps.ProbeResourceUpdate(r.Context(), id, current.UpdatedAt, req); probeErr != nil {
				writeConnectorError(w, probeErr)
				return
			}
		}
		rollbackConnector, saveErr := h.saveGitLabConnectorSettings(r.Context(), "", current.AppID, config, req.Credentials)
		if saveErr != nil {
			writeConnectorError(w, saveErr)
			return
		}
		result, err = h.apps.UpdateResourceAt(r.Context(), id, req, current.UpdatedAt)
		if err != nil {
			rollbackConnector()
		}
	case http.MethodDelete:
		err = h.apps.Delete(r.Context(), "", id)
		if err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	if resource, ok := result.(apps.Installation); ok {
		result = h.connectorResourceView(resource)
	}
	writeJSON(w, status, result)
}

// probeConnectorDraft validates supplied GitLab credentials before MCP discovery, which
// can succeed even when the upstream server has not authenticated a business call.
func (h *Handler) probeConnectorDraft(ctx context.Context, agentID string, req apps.ProbeRequest) (apps.ProbeResult, error) {
	if req.InstallationID != "" {
		current, err := h.apps.Get(ctx, agentID, req.InstallationID)
		if err != nil {
			return apps.ProbeResult{}, err
		}
		req.AppID = current.AppID
	}
	if req.AppID == "gitlab" && (req.Config.AuthMode == "connector" || req.Config.AuthMode == "oauth2") && req.Credentials.Token != "" {
		if h.connectors == nil {
			return apps.ProbeResult{}, fmt.Errorf("GitLab Connector service is unavailable")
		}
		if err := h.connectors.ValidateGitLabConfig(ctx, connectors.Config{BaseURL: req.Config.GitLabBaseURL, AccessToken: req.Credentials.Token}); err != nil {
			if errors.Is(err, connectors.ErrGitLabAuthentication) {
				return apps.ProbeResult{}, &apps.ConnectionError{Code: "app_gitlab_authentication_failed", Message: "GitLab rejected this PAT. Check the token, expiry and permissions."}
			}
			return apps.ProbeResult{}, &apps.ConnectionError{Code: "app_gitlab_validation_failed", Message: "Could not verify the GitLab account. Check the instance URL and network connection."}
		}
	}
	return h.apps.Probe(ctx, agentID, req)
}

func (h *Handler) handleConnectorResourceProbe(w http.ResponseWriter, r *http.Request) {
	if h.apps == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "apps_unavailable", "App service is unavailable")
		return
	}
	var req apps.ProbeRequest
	if !decodeConnectorRequest(w, r, &req) {
		return
	}

	result, err := h.probeConnectorDraft(r.Context(), "", req)
	if err != nil {
		writeConnectorError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) connectorResourceView(item apps.Installation) apps.Installation {
	// GitLab PATs belong to the shared connector owner, not the resource record.
	// Expose only presence for the matching instance so settings can show a mask.
	if item.AppID == "gitlab" && item.Config.AuthMode == "connector" && h.connectors != nil {
		if status, err := h.connectors.GitLabStatus(); err == nil {
			if item.CredentialsSet == nil {
				item.CredentialsSet = make(map[string]bool)
			}
			item.CredentialsSet["token"] = status.AccessTokenSet && sameNormalizedURL(item.Config.GitLabBaseURL, status.BaseURL)
		}
	}
	if h.svc == nil {
		return item
	}
	for i, binding := range item.Bindings {
		if a, ok := h.svc.Agent(binding.AgentID); ok {
			item.Bindings[i].AgentName = a.Name
		}
	}
	return item
}
