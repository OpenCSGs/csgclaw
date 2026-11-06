package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/config"
	"csgclaw/internal/relay"
)

// EnableRelay ties outbound executors to the HTTP server lifetime.
func (h *Handler) EnableRelay(ctx context.Context) error {
	root, err := config.DefaultDomainDir("relay")
	if err != nil {
		return err
	}
	// The actual local listener identifies an installation slot: restart stable,
	// while two servers on different ports never share publication state or IDs.
	root = relay.InstanceRoot(root, h.internalBaseURL)
	h.relayContext, h.relayCancel = context.WithCancel(ctx)
	h.relayManager, err = relay.NewManager(h.relayContext, root, apiclient.New(h.internalBaseURL, h.serverAccessToken, nil), relay.WithCommunity(relay.HubCommunity{Credentials: relay.LoadSiteCredentials}, relay.LoadSiteCredentials))
	if err == nil {
		if h.im != nil {
			for _, room := range h.im.Bootstrap().Rooms {
				if room.CloudAgent != nil {
					h.startCloudTask(room.ID)
				}
			}
		}
	}
	if err != nil {
		h.relayCancel()
	}
	return err
}
func (h *Handler) CloseRelay() {
	if h.relayCancel != nil {
		h.relayCancel()
	}
	if h.relayManager != nil {
		h.relayManager.Close()
	}
	h.cloudTaskWG.Wait()
}
func (h *Handler) handleAgentRelay(w http.ResponseWriter, r *http.Request) {
	if h.relayManager == nil {
		writeAgentSessionError(w, 503, "relay_unavailable", "relay service is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	id := pathValue(r, "id")
	if r.Method == http.MethodGet {
		writeJSON(w, 200, h.relayManager.Status(ctx, id))
		return
	}
	if r.Method == http.MethodDelete {
		if err := h.relayManager.Unpublish(ctx, id); err != nil {
			writeAgentSessionError(w, 502, "relay_unpublish_failed", err.Error(), nil)
			return
		}
		writeJSON(w, 200, h.relayManager.Status(ctx, id))
		return
	}
	var body struct {
		Endpoint    string `json:"endpoint"`
		Token       string `json:"token"`
		AutoPublish bool   `json:"auto_publish"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if err := d.Decode(&body); err != nil {
		writeAgentSessionError(w, 400, "invalid_request", "invalid relay publication request", nil)
		return
	}
	if d.Decode(new(any)) != io.EOF {
		writeAgentSessionError(w, 400, "invalid_request", "expected one JSON value", nil)
		return
	}
	if body.AutoPublish {
		if body.Endpoint != "" || body.Token != "" {
			writeAgentSessionError(w, 400, "invalid_request", "automatic publication uses the current community credentials", nil)
			return
		}
		if err := h.queueAgentPublication(id); err != nil {
			if errors.Is(err, relay.ErrLocalAgentDeleted) {
				writeAgentSessionError(w, http.StatusConflict, "agent_identity_deleted", err.Error(), nil)
				return
			}
			writeAgentSessionError(w, 409, "relay_publish_failed", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusAccepted, h.relayManager.Status(ctx, id))
		return
	}
	selected, err := h.resolveSessionAgent(id)
	if err != nil {
		writeAgentSessionError(w, 409, "agent_unavailable", err.Error(), nil)
		return
	}
	if selected.Spec.Runtime.Sandboxed || (selected.Spec.Runtime.Adapter != "codex" && selected.Spec.Runtime.Adapter != "dsh") {
		writeAgentSessionError(w, 400, "relay_runtime_unsupported", "relay requires a host codex or dsh agent", nil)
		return
	}

	if err = h.relayManager.Configure(body.Endpoint, body.Token); err != nil {
		writeAgentSessionError(w, 400, "relay_configuration_failed", err.Error(), nil)
		return
	}
	a, ok := h.svc.Agent(string(selected.ID))
	if !ok {
		writeAgentSessionError(w, 404, "agent_not_found", "agent not found", nil)
		return
	}
	_, err = h.relayManager.Publish(ctx, relay.PublishRequest{Name: a.Name, Description: a.Description, Visibility: "public", Executor: "csgclaw", SourceAgentID: string(selected.ID)}, a.Avatar)
	if err != nil {
		if errors.Is(err, relay.ErrLocalAgentDeleted) {
			writeAgentSessionError(w, http.StatusConflict, "agent_identity_deleted", err.Error(), nil)
			return
		}
		writeAgentSessionError(w, 502, "relay_publish_failed", err.Error(), nil)
		return
	}
	writeJSON(w, 200, h.relayManager.Status(ctx, string(selected.ID)))
}

// handleRelayCatalog serves browser-safe community discovery records.
func (h *Handler) handleRelayCatalog(w http.ResponseWriter, r *http.Request) {
	if h.relayManager == nil {
		writeAgentSessionError(w, 503, "relay_unavailable", "relay service is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	entries, err := h.relayManager.Discover(ctx)
	if err != nil {
		if errors.Is(err, relay.ErrSiteLoginRequired) {
			writeAgentSessionError(w, http.StatusUnauthorized, "relay_login_required", err.Error(), nil)
			return
		}
		writeAgentSessionError(w, 502, "relay_catalog_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
}

func (h *Handler) queueAgentPublication(id string) error {
	if h.relayManager == nil {
		return fmt.Errorf("relay service is unavailable")
	}
	a, ok := h.svc.Agent(id)
	if !ok {
		return fmt.Errorf("agent not found")
	}
	runtime := a.RuntimeConfig()
	if runtime.Sandboxed || (runtime.Name != "codex" && runtime.Name != "dsh") {
		return fmt.Errorf("relay requires a host codex or dsh agent")
	}
	target, err := relay.PlatformTargetFromEnvironment()
	if err != nil {
		return err
	}
	return h.relayManager.QueuePublication(relay.PublishRequest{Name: a.Name, Description: a.Description, SourceAgentID: id, Executor: "csgclaw", Visibility: "public"}, target)
}
