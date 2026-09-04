package api

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/participant"
	"csgclaw/internal/participant/feishubind"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func participantCreateStatus(err error) int {
	if err == nil {
		return http.StatusCreated
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(message, "already exists") {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func (h *Handler) handleParticipants(w http.ResponseWriter, r *http.Request) {
	if h.participant == nil {
		http.Error(w, "participant service is not configured", http.StatusServiceUnavailable)
		return
	}
	channelName := pathValue(r, "channel")
	if channelName == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		items := h.participant.List(participant.ListOptions{
			Channel: channelName,
			Type:    r.URL.Query().Get("type"),
			AgentID: r.URL.Query().Get("agent_id"),
		})
		writeJSON(w, http.StatusOK, h.presentParticipants(items))
	case http.MethodPost:
		var req participant.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		req.Channel = channelName
		hubSvc, err := h.hubServiceForRequest(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("resolve hub service: %v", err), http.StatusInternalServerError)
			return
		}
		req.AgentHubService = hubSvc
		var created apitypes.Participant
		create := func(ctx context.Context) error {
			var err error
			created, err = h.participant.Create(ctx, req)
			return err
		}
		if channelName == participant.ChannelFeishu && req.Type == participant.TypeAgent {
			err = h.mutateFeishuBot(r.Context(), req.AgentBinding.AgentID, channelAppConfigString(req.ChannelAppConfig, "app_id"), create)
		} else {
			err = create(r.Context())
		}
		if err != nil {
			if errors.Is(err, feishubind.ErrBotAppIDConflict) {
				h.writeFeishuBotAppInfoError(w, err)
				return
			}
			billingURL := ""
			if req.AgentBinding.Agent != nil {
				billingURL = llm.OpenCSGBillingURL(req.AgentBinding.Agent.AgentProfile)
			}
			writeAgentOperationErrorWithBillingURL(w, err, participantCreateStatus(err), billingURL)
			return
		}
		presented := h.presentParticipant(created)
		h.publishParticipantEvent(im.EventTypeParticipantCreated, presented)
		writeJSON(w, http.StatusCreated, presented)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleParticipantByIDPath(w http.ResponseWriter, r *http.Request) {
	if h.participant == nil {
		http.Error(w, "participant service is not configured", http.StatusServiceUnavailable)
		return
	}
	channelName := pathValue(r, "channel")
	id := pathValue(r, "id")
	if channelName == "" || id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, ok := h.participant.Get(channelName, id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, h.presentParticipant(item))
	case http.MethodPatch:
		var req participant.UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		var updated apitypes.Participant
		var ok bool
		update := func(ctx context.Context) error {
			var err error
			updated, ok, err = h.participant.Update(ctx, channelName, id, req)
			return err
		}
		current, found := h.participant.Get(channelName, id)
		var err error
		if found && current.Channel == participant.ChannelFeishu && current.Type == participant.TypeAgent {
			if req.AgentID != nil && agent.CanonicalID(*req.AgentID) != agent.CanonicalID(current.AgentID) {
				http.Error(w, "Disconnect the Bot before binding it to another Agent", http.StatusConflict)
				return
			}
			appID := channelAppConfigString(current.ChannelAppConfig, "app_id")
			if req.ChannelAppConfig != nil {
				appID = channelAppConfigString(req.ChannelAppConfig, "app_id")
			}
			err = h.mutateFeishuBot(r.Context(), current.AgentID, appID, update)
		} else {
			err = update(r.Context())
		}
		if err != nil {
			if errors.Is(err, feishubind.ErrBotAppIDConflict) {
				h.writeFeishuBotAppInfoError(w, err)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		presented := h.presentParticipant(updated)
		h.publishParticipantEvent(im.EventTypeParticipantUpdated, presented)
		writeJSON(w, http.StatusOK, presented)
	case http.MethodDelete:
		current, exists := h.participant.Get(channelName, id)
		if !exists {
			http.NotFound(w, r)
			return
		}
		var deleted apitypes.Participant
		var removed bool
		remove := func(ctx context.Context) error {
			var err error
			deleted, removed, err = h.participant.Delete(ctx, channelName, id, participant.DeleteOptions{DeleteAgent: r.URL.Query().Get("delete_agent")})
			if err != nil || !removed {
				return err
			}
			// Publish the fact even when Runtime cleanup fails. Source tokens are
			// already invalid and the UI must not keep showing a connected Bot.
			h.publishParticipantEvent(im.EventTypeParticipantDeleted, h.presentParticipant(deleted))
			return h.deactivateFeishuAgentAfterDisconnect(ctx, deleted, r.URL.Query().Get("delete_agent"))
		}
		var err error
		if current.Channel == participant.ChannelFeishu && current.Type == participant.TypeAgent && current.AgentID != "" {
			if h.agentRuntime == nil {
				http.Error(w, "Agent lifecycle coordinator is unavailable", http.StatusServiceUnavailable)
				return
			}
			err = h.agentRuntime.WithAgentLifecycle(r.Context(), current.AgentID, remove)
		} else {
			err = remove(r.Context())
		}
		if err != nil {
			if removed {
				writeJSON(w, http.StatusConflict, map[string]string{"status": "partial", "code": "feishu_cleanup_pending", "error": "Feishu is disconnected. Runtime tool cleanup is incomplete; retry cleanup."})
			} else {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Both UI registration and manual Participant/CLI writes share the same Agent
// lease. A changed source token becomes invalid before any new Runtime runs.
func (h *Handler) mutateFeishuBot(ctx context.Context, agentID, appID string, write func(context.Context) error) error {
	if strings.TrimSpace(agentID) == "" {
		return feishubind.WithExclusiveBotAppID(h.participant, agentID, appID, func() error { return write(ctx) })
	}
	if h.agentRuntime == nil {
		return errors.New("An existing Agent and its lifecycle coordinator are required")
	}
	return h.agentRuntime.WithAgentLifecycle(ctx, agentID, func(ctx context.Context) error {
		if err := feishubind.WithExclusiveBotAppID(h.participant, agentID, appID, func() error { return write(ctx) }); err != nil {
			return err
		}
		if strings.TrimSpace(appID) == "" {
			return nil
		}
		target, ok := h.svc.Agent(agentID)
		if !ok {
			return fmt.Errorf("agent %q not found", agentID)
		}
		if target.RuntimeKind == agent.RuntimeKindCodex && appID != "" {
			// Optional tool errors are exposed in Engine status, not as failure
			// of the successfully saved Channel credentials.
			_, _ = h.configureAgentLarkCLILocked(ctx, target)
		}
		_, _, err := h.refreshAgentChannel(ctx, target, participant.ChannelFeishu)
		return err
	})
}

func (h *Handler) deactivateFeishuAgentAfterDisconnect(ctx context.Context, deleted apitypes.Participant, deleteAgentMode string) error {
	if !strings.EqualFold(strings.TrimSpace(deleted.Channel), participant.ChannelFeishu) {
		return nil
	}
	if strings.TrimSpace(deleteAgentMode) != "" {
		return nil
	}
	if strings.TrimSpace(deleted.Type) != participant.TypeAgent {
		return nil
	}
	agentID := strings.TrimSpace(deleted.AgentID)
	if agentID == "" {
		return nil
	}
	if h == nil || h.svc == nil {
		return fmt.Errorf("agent service is required to disconnect feishu participant %q", deleted.ID)
	}
	if h.participant != nil {
		for _, item := range h.participant.List(participant.ListOptions{
			Channel: participant.ChannelFeishu,
			Type:    participant.TypeAgent,
			AgentID: agentID,
		}) {
			if strings.TrimSpace(item.ID) == strings.TrimSpace(deleted.ID) {
				continue
			}
			if strings.TrimSpace(item.ChannelUserKind) != participant.ChannelUserKindAppID {
				continue
			}
			if _, _, err := h.participant.Delete(ctx, participant.ChannelFeishu, item.ID, participant.DeleteOptions{}); err != nil {
				return fmt.Errorf("delete feishu participant %q for agent %q: %w", item.ID, agentID, err)
			}
		}
	}
	var errs []error
	if target, ok := h.svc.Agent(agentID); ok {
		if _, _, err := h.refreshAgentChannel(ctx, target, participant.ChannelFeishu); err != nil {
			errs = append(errs, fmt.Errorf("reconcile feishu binding for agent %q after disconnecting participant %q: %w", agentID, deleted.ID, err))
		}
	}
	if err := h.clearAgentLarkCLIState(ctx, agentID); err != nil {
		errs = append(errs, fmt.Errorf("clear lark-cli state for agent %q after disconnecting participant %q: %w", agentID, deleted.ID, err))
	}
	return errors.Join(errs...)
}

func (h *Handler) handleParticipantEvents(w http.ResponseWriter, r *http.Request) {
	channelName := participantChannelName(pathValue(r, "channel"))
	id := pathValue(r, "id")
	if channelName == "" || id == "" {
		http.NotFound(w, r)
		return
	}

	switch channelName {
	case "csgclaw":
		participantID, ok := h.requireParticipantBridgeID(w, r, h.resolveParticipantBridgeID(channelName, id))
		if !ok {
			return
		}
		h.handleParticipantEventsStream(w, r, participantID)
	case "feishu":
		h.handleFeishuParticipantEvents(w, r, id, h.resolveFeishuParticipantTargetID(id))
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleParticipantMessage(w http.ResponseWriter, r *http.Request) {
	channelName := participantChannelName(pathValue(r, "channel"))
	id := pathValue(r, "id")
	if channelName == "" || id == "" {
		http.NotFound(w, r)
		return
	}
	if channelName != "csgclaw" {
		http.NotFound(w, r)
		return
	}
	participantID := h.resolveParticipantChannelUserID(channelName, id)
	participantID, ok := h.requireParticipantBridgeID(w, r, participantID)
	if !ok {
		return
	}
	h.handleParticipantSendMessage(w, r, participantID)
}

func (h *Handler) resolveParticipantChannelUserID(channelName, id string) string {
	id = strings.TrimSpace(id)
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(channelName, id); ok {
			return participantChannelLocalIdentity(item)
		}
		if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
			for _, item := range h.participant.List(participant.ListOptions{Channel: channelName}) {
				if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, id) {
					continue
				}
				return participantChannelLocalIdentity(item)
			}
		}
	}
	if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
		return csgclawParticipantIDFromAny(id)
	}
	return id
}

func (h *Handler) resolveParticipantBridgeID(channelName, id string) string {
	id = strings.TrimSpace(id)
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(channelName, id); ok && isCSGClawAgentParticipant(item) {
			return strings.TrimSpace(item.ID)
		}
		if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
			for _, item := range h.participant.List(participant.ListOptions{Channel: channelName}) {
				if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, id) {
					continue
				}
				return strings.TrimSpace(item.ID)
			}
		}
	}
	if id == agent.ManagerUserID {
		return agent.ManagerParticipantID
	}
	if strings.EqualFold(channelName, participant.ChannelCSGClaw) {
		return csgclawParticipantIDFromAny(id)
	}
	return id
}

func (h *Handler) resolveFeishuParticipantTargetID(id string) string {
	id = strings.TrimSpace(id)
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelFeishu, id); ok {
			return participantChannelUserOrID(item)
		}
		for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelFeishu}) {
			if !participantMatchesIdentity(item, id) {
				continue
			}
			return participantChannelUserOrID(item)
		}
	}
	return id
}

func participantChannelUserOrID(item apitypes.Participant) string {
	if ref := strings.TrimSpace(item.ChannelUserRef); ref != "" {
		return ref
	}
	return strings.TrimSpace(item.ID)
}

func participantChannelLocalIdentity(item apitypes.Participant) string {
	if strings.EqualFold(strings.TrimSpace(item.Channel), participant.ChannelCSGClaw) {
		if id := strings.TrimSpace(item.ID); id != "" {
			return id
		}
	}
	return participantChannelUserOrID(item)
}

func presentParticipants(items []apitypes.Participant) []apitypes.Participant {
	out := make([]apitypes.Participant, 0, len(items))
	for _, item := range items {
		out = append(out, presentParticipant(item))
	}
	return out
}

func presentParticipant(item apitypes.Participant) apitypes.Participant {
	if len(item.ChannelAppConfig) == 0 {
		return item
	}
	item.ChannelAppConfig = participant.RedactChannelAppConfig(item.ChannelAppConfig)
	return item
}

func (h *Handler) presentParticipants(items []apitypes.Participant) []apitypes.Participant {
	out := make([]apitypes.Participant, 0, len(items))
	for _, item := range items {
		out = append(out, h.presentParticipant(item))
	}
	return out
}

func (h *Handler) presentParticipant(item apitypes.Participant) apitypes.Participant {
	item = presentParticipant(item)
	if h == nil {
		return item
	}
	if strings.TrimSpace(item.AgentID) != "" {
		item.AgentID = agent.CanonicalID(item.AgentID)
	}
	if h.svc != nil {
		if name, ok := h.svc.AgentDisplayName(item.AgentID); ok {
			item.AgentName = name
		}
	}
	if item.UserID == "" {
		item.UserID = strings.TrimSpace(item.ChannelUserRef)
	}
	if h.im != nil && strings.TrimSpace(item.UserID) != "" {
		if user, ok := h.im.User(item.UserID); ok {
			item.UserName = strings.TrimSpace(user.Name)
		}
	}
	return item
}

func (h *Handler) requireParticipantBridgeID(w http.ResponseWriter, r *http.Request, id string) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return "", false
	}
	if h.participantBridge == nil {
		http.Error(w, "picoclaw integration is not configured", http.StatusServiceUnavailable)
		return "", false
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return id, true
}

func participantChannelName(channel string) string {
	return strings.TrimSpace(channel)
}
