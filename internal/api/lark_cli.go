package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	feishularkcli "csgclaw/internal/channel/feishu/larkcli"
	"csgclaw/internal/config"
	"csgclaw/internal/participant"
	"csgclaw/internal/participant/feishubind"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	larkCLISourceTokenPrefix   = "larkcli-src-v2"
	larkCLISourceTokenPurpose  = "feishu_app_info"
	feishuBotNotConfiguredCode = "feishu_bot_not_configured"
	feishuBotAppIDConflictCode = "feishu_bot_app_id_conflict"
	larkCLIStatusUnbound       = "unbound"
	larkCLIStatusBound         = "bound"
	larkCLIStatusMismatch      = "mismatch"
	larkCLIStatusUnavailable   = "unavailable"
)

var (
	errFeishuBotNotConfigured = errors.New("feishu bot is not configured")
	errFeishuBotAppIDConflict = feishubind.ErrBotAppIDConflict

	larkCLILookPath   = exec.LookPath
	larkCLICurrentExe = os.Executable
)

type feishuBotAppInfo struct {
	AgentID       string
	ParticipantID string
	AppID         string
	AppSecret     string
}

type larkCLISourceTokenPayload struct {
	Version            string `json:"version"`
	Purpose            string `json:"purpose"`
	AgentID            string `json:"agent_id"`
	ParticipantID      string `json:"participant_id"`
	CredentialRevision string `json:"credential_revision"`
}

type larkCLIConfigureError struct {
	status int
	code   string
	err    error
}

func (e *larkCLIConfigureError) Error() string {
	if e == nil || e.err == nil {
		return "lark-cli configuration failed"
	}
	return e.err.Error()
}

func (e *larkCLIConfigureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (h *Handler) initAgentLarkCLI(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	if h.participant == nil {
		http.Error(w, "participant service is not configured", http.StatusServiceUnavailable)
		return
	}
	agentID := strings.TrimSpace(pathValue(r, "id"))
	if agentID == "" {
		http.NotFound(w, r)
		return
	}
	target, ok := h.svc.Agent(agentID)
	if !ok {
		writeAgentOperationError(w, fmt.Errorf("agent %q not found", agent.CanonicalID(agentID)), http.StatusNotFound)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(target.RuntimeKind), agent.RuntimeKindCodex) {
		writeCodedAPIError(w, http.StatusBadRequest, "unsupported_runtime", "lark-cli init is supported only for Codex workers")
		return
	}

	result, err := h.configureAgentLarkCLI(r.Context(), target)
	if err != nil {
		h.writeLarkCLIConfigureError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) configureAgentLarkCLI(ctx context.Context, target agent.Agent) (result apitypes.AgentLarkCLIInitResponse, err error) {
	if h.agentRuntime == nil {
		return result, errors.New("Agent lifecycle coordinator is unavailable")
	}
	err = h.agentRuntime.WithAgentLifecycle(ctx, target.ID, func(ctx context.Context) error {
		var applyErr error
		result, applyErr = h.configureAgentLarkCLILocked(ctx, target)
		return applyErr
	})
	return result, err
}
func (h *Handler) configureAgentLarkCLILocked(ctx context.Context, target agent.Agent) (apitypes.AgentLarkCLIInitResponse, error) {
	appInfo, err := h.feishuBotAppInfoForAgent(target.ID)
	if err != nil {
		return apitypes.AgentLarkCLIInitResponse{}, err
	}
	if err := feishubind.ValidateBotAppIDExclusive(h.participant, target.ID, appInfo.AppID); err != nil {
		return apitypes.AgentLarkCLIInitResponse{}, err
	}
	if h.agentEngine == nil {
		return apitypes.AgentLarkCLIInitResponse{}, newLarkCLIConfigureError(http.StatusServiceUnavailable, "runtime_extension_unavailable", errors.New("agent engine is unavailable"))
	}
	h.configureRuntimeExtensionSources()
	extension, err := h.agentEngine.RuntimeExtensions(target.ID).Apply(ctx, agentengine.RuntimeExtensionApplyRequest{Spec: agentengine.RuntimeExtensionSpec{
		Name: larkextension.Name,
		Kind: larkextension.Kind,
		Source: agentengine.RuntimeExtensionSourceRef{
			Provider: larkextension.SourceProvider,
			Ref:      appInfo.ParticipantID,
		},
		FailurePolicy: agentengine.RuntimeExtensionOptional,
	}})
	if err != nil {
		status, code := classifyLarkCLIConfigureError(extension.Status.Reason)
		message := extension.Status.Message
		if message == "" {
			message = "Runtime extension update failed; retry."
		}
		return apitypes.AgentLarkCLIInitResponse{}, newLarkCLIConfigureError(status, code, errors.New(message))
	}
	if extension.Status.State == agentengine.RuntimeExtensionUnavailable {
		return apitypes.AgentLarkCLIInitResponse{}, newLarkCLIConfigureError(http.StatusServiceUnavailable, "lark_cli_unavailable", errors.New(extension.Status.Message))
	}
	if extension.Status.State != agentengine.RuntimeExtensionConfigured {
		return apitypes.AgentLarkCLIInitResponse{}, newLarkCLIConfigureError(http.StatusBadGateway, "lark_cli_bind_failed", errors.New(extension.Status.Message))
	}
	restartStatus := "restart_skipped"
	restartError := ""
	if extension.Status.Reason == "restart_failed" {
		restartStatus = "restart_failed"
		restartError = extension.Status.Message
	} else if extension.Status.RuntimeLoaded {
		restartStatus = "runtime_loaded"
	}
	warning := ""
	if extension.Status.Reason != "" && extension.Status.Reason != "configured" {
		warning = extension.Status.Message
	}
	return apitypes.AgentLarkCLIInitResponse{
		Status:        "configured",
		Generation:    extension.Status.Generation,
		RuntimeLoaded: extension.Status.RuntimeLoaded,
		Warning:       warning,
		AgentID:       target.ID,
		ParticipantID: appInfo.ParticipantID,
		AppID:         appInfo.AppID,
		RestartStatus: restartStatus,
		RestartError:  restartError,
	}, nil
}

func classifyLarkCLIConfigureError(reason string) (int, string) {
	switch reason {
	case "source_unavailable", "invalid_source":
		return http.StatusServiceUnavailable, "lark_cli_source_unavailable"
	case "extension_unsupported":
		return http.StatusBadRequest, "unsupported_runtime"
	case "bind_failed", "bind_invalid":
		return http.StatusBadGateway, "lark_cli_bind_failed"
	default:
		return http.StatusInternalServerError, "lark_cli_config_failed"
	}
}

func newLarkCLIConfigureError(status int, code string, err error) error {
	return &larkCLIConfigureError{status: status, code: strings.TrimSpace(code), err: err}
}

func (h *Handler) writeLarkCLIConfigureError(w http.ResponseWriter, err error) {
	if errors.Is(err, errFeishuBotNotConfigured) || errors.Is(err, errFeishuBotAppIDConflict) {
		h.writeFeishuBotAppInfoError(w, err)
		return
	}
	var configureErr *larkCLIConfigureError
	if errors.As(err, &configureErr) {
		writeCodedAPIError(w, configureErr.status, configureErr.code, configureErr.Error())
		return
	}
	writeAgentOperationError(w, err, http.StatusInternalServerError)
}

func (h *Handler) clearAgentLarkCLIState(ctx context.Context, agentID string) error {
	if h == nil {
		return nil
	}
	if h.svc != nil {
		if target, ok := h.svc.Agent(agentID); !ok || !strings.EqualFold(strings.TrimSpace(target.RuntimeKind), agent.RuntimeKindCodex) {
			return nil
		}
	}
	h.configureRuntimeExtensionSources()
	if h.agentEngine == nil {
		return nil
	}
	err := h.agentEngine.RuntimeExtensions(agentID).Delete(ctx, larkextension.Name)
	if agentengine.ErrorCodeOf(err) == agentengine.ErrorRuntimeExtensionNotFound {
		return nil
	}
	return err
}

// This fixed product action cannot remove a newly connected Bot or accept
// arbitrary Extension payloads. Deletion is retryable after the Participant is gone.
func (h *Handler) cleanupAgentLarkCLI(w http.ResponseWriter, r *http.Request) {
	if h.agentRuntime == nil || h.agentEngine == nil {
		http.Error(w, "Agent lifecycle coordinator is unavailable", http.StatusServiceUnavailable)
		return
	}
	agentID := pathValue(r, "id")
	err := h.agentRuntime.WithAgentLifecycle(r.Context(), agentID, func(ctx context.Context) error {
		if h.participant != nil {
			for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelFeishu, Type: participant.TypeAgent, AgentID: agentID}) {
				if item.ChannelUserKind == participant.ChannelUserKindAppID {
					return errFeishuBotAppIDConflict
				}
			}
		}
		return h.clearAgentLarkCLIState(ctx, agentID)
	})
	if err != nil {
		writeCodedAPIError(w, http.StatusConflict, "feishu_cleanup_pending", "Cleanup could not complete. Ensure Feishu is disconnected and retry.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) agentLarkCLIStatus(target agent.Agent) *apitypes.AgentLarkCLIStatus {
	if h == nil {
		return nil
	}
	h.configureRuntimeExtensionSources()
	if h.agentEngine == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(target.RuntimeKind), agent.RuntimeKindCodex) {
		return nil
	}
	extension, err := h.agentEngine.RuntimeExtensions(target.ID).Get(context.Background(), larkextension.Name)
	if agentengine.ErrorCodeOf(err) == agentengine.ErrorRuntimeExtensionNotFound {
		return &apitypes.AgentLarkCLIStatus{State: larkCLIStatusUnbound}
	}
	status := &apitypes.AgentLarkCLIStatus{AppID: h.feishuBotAppIDForExistingAgent(target), Generation: extension.Status.Generation, ObservedGeneration: extension.Status.ObservedGeneration, RuntimeLoaded: extension.Status.RuntimeLoaded, Reason: extension.Status.Reason}
	status.CleanupPending = extension.Status.Reason == "deleting" || extension.Status.Reason == "delete_failed"
	if !extension.Status.CheckedAt.IsZero() {
		checked := extension.Status.CheckedAt
		status.CheckedAt = &checked
	}
	if err != nil {
		status.State = larkCLIStatusMismatch
		status.Error = err.Error()
		return status
	}
	switch extension.Status.State {
	case agentengine.RuntimeExtensionConfigured:
		status.State = larkCLIStatusBound
		status.Bound = true
		status.Available = true
	case agentengine.RuntimeExtensionUnavailable:
		status.State = larkCLIStatusUnavailable
		status.Error = extension.Status.Message
	default:
		status.State = larkCLIStatusMismatch
		status.Available = extension.Status.Reason != "executable_unavailable"
		status.Error = extension.Status.Message
	}
	if !extension.Status.AppliedAt.IsZero() {
		appliedAt := extension.Status.AppliedAt
		status.BoundAt = &appliedAt
	}
	return status
}

func (h *Handler) getAgentFeishuAppInfo(w http.ResponseWriter, r *http.Request) {
	agentID := pathValue(r, "id")
	appInfo, authorized := h.authorizeLarkCLISource(r.Header.Get("Authorization"), agentID)
	if !authorized {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, apitypes.FeishuBotAppInfo{
		AgentID:       appInfo.AgentID,
		ParticipantID: appInfo.ParticipantID,
		AppID:         appInfo.AppID,
		AppSecret:     appInfo.AppSecret,
	})
}

func (h *Handler) feishuBotAppInfoForAgent(agentID string) (feishuBotAppInfo, error) {
	if h.svc == nil {
		return feishuBotAppInfo{}, fmt.Errorf("agent service is required")
	}
	if h.participant == nil {
		return feishuBotAppInfo{}, fmt.Errorf("participant service is required")
	}
	agentID = strings.TrimSpace(agentID)
	target, ok := h.svc.Agent(agentID)
	if !ok {
		return feishuBotAppInfo{}, fmt.Errorf("agent %q not found", agent.CanonicalID(agentID))
	}

	canonicalParticipantID := agent.ParticipantIDForAgent(target.Name, target.ID)
	if item, ok := h.participant.Get(participant.ChannelFeishu, canonicalParticipantID); ok {
		if info, ok := feishuBotAppInfoFromParticipant(target.ID, item); ok {
			return info, nil
		}
	}
	for _, item := range h.participant.List(participant.ListOptions{
		Channel: participant.ChannelFeishu,
		Type:    participant.TypeAgent,
		AgentID: target.ID,
	}) {
		if info, ok := feishuBotAppInfoFromParticipant(target.ID, item); ok {
			return info, nil
		}
	}
	return feishuBotAppInfo{}, fmt.Errorf("%w for agent %q", errFeishuBotNotConfigured, target.ID)
}

func feishuBotAppInfoFromParticipant(agentID string, item apitypes.Participant) (feishuBotAppInfo, bool) {
	appID, ok := feishuBotAppIDFromParticipant(agentID, item)
	if !ok {
		return feishuBotAppInfo{}, false
	}
	appSecret := channelAppConfigString(item.ChannelAppConfig, participant.ChannelAppConfigAppSecretKey)
	if appSecret == "" || appSecret == participant.RedactedSecretValue {
		return feishuBotAppInfo{}, false
	}
	return feishuBotAppInfo{
		AgentID:       strings.TrimSpace(agentID),
		ParticipantID: strings.TrimSpace(item.ID),
		AppID:         appID,
		AppSecret:     appSecret,
	}, true
}

func (h *Handler) feishuBotAppIDForExistingAgent(target agent.Agent) string {
	if h == nil || h.participant == nil {
		return ""
	}
	canonicalParticipantID := agent.ParticipantIDForAgent(target.Name, target.ID)
	if item, ok := h.participant.Get(participant.ChannelFeishu, canonicalParticipantID); ok {
		if appID, ok := feishuBotAppIDFromParticipant(target.ID, item); ok {
			return appID
		}
	}
	for _, item := range h.participant.List(participant.ListOptions{
		Channel: participant.ChannelFeishu,
		Type:    participant.TypeAgent,
		AgentID: target.ID,
	}) {
		if appID, ok := feishuBotAppIDFromParticipant(target.ID, item); ok {
			return appID
		}
	}
	return ""
}

func feishuBotAppIDFromParticipant(agentID string, item apitypes.Participant) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(item.Channel), participant.ChannelFeishu) ||
		strings.TrimSpace(item.Type) != participant.TypeAgent ||
		strings.TrimSpace(item.AgentID) != strings.TrimSpace(agentID) {
		return "", false
	}
	appID := channelAppConfigString(item.ChannelAppConfig, "app_id")
	return appID, appID != ""
}

func (h *Handler) writeFeishuBotAppInfoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errFeishuBotNotConfigured):
		writeCodedAPIError(w, http.StatusConflict, feishuBotNotConfiguredCode, "This worker has not configured a Feishu bot app yet.")
	case errors.Is(err, errFeishuBotAppIDConflict):
		writeCodedAPIError(w, http.StatusConflict, feishuBotAppIDConflictCode, err.Error())
	default:
		writeAgentOperationError(w, err, http.StatusBadRequest)
	}
}

func writeCodedAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    strings.TrimSpace(code),
			"message": strings.TrimSpace(message),
		},
	})
}

func channelAppConfigString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	for k, value := range values {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func larkCLISourceHelperPath() (string, error) {
	path, err := larkCLICurrentExe()
	if err != nil {
		return "", fmt.Errorf("resolve csgclaw executable for lark-cli source: %w", err)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("csgclaw executable path is empty")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve csgclaw executable absolute path: %w", err)
		}
		path = abs
	}
	return path, nil
}

func (h *Handler) sourceAccessToken(agentID string) (string, error) {
	return h.larkCLISourceAccessToken(agentID)
}

func (h *Handler) larkCLISourceAccessToken(agentID string) (string, error) {
	info, err := h.feishuBotAppInfoForAgent(agentID)
	if err != nil {
		return "", err
	}
	return h.larkCLIRevisionAccessToken(info.AgentID, info.ParticipantID, feishularkcli.CredentialRevision(info.ParticipantID, info.AppID, info.AppSecret))
}
func (h *Handler) larkCLIRevisionAccessToken(agentID, participantID, revision string) (string, error) {
	secrets := h.larkCLISourceSigningSecrets()
	if h.larkCLISigningErr != nil {
		return "", fmt.Errorf("initialize source capability: %w", h.larkCLISigningErr)
	}
	if len(secrets) == 0 {
		return "", fmt.Errorf("CSGClaw API token is required for the lark-cli source command")
	}
	payload := larkCLISourceTokenPayload{Version: larkCLISourceTokenPrefix, Purpose: larkCLISourceTokenPurpose, AgentID: agent.CanonicalID(agentID), ParticipantID: participantID, CredentialRevision: revision}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	return larkCLISourceTokenPrefix + "." + encoded + "." + signLarkCLISourceToken(encoded, secrets[0]), nil
}

func (h *Handler) validateLarkCLISourceAccessToken(header, agentID string) bool {
	_, ok := h.authorizeLarkCLISource(header, agentID)
	return ok
}

func (h *Handler) authorizeLarkCLISource(header, agentID string) (feishuBotAppInfo, bool) {
	if h == nil || h.participant == nil || h.svc == nil {
		return feishuBotAppInfo{}, false
	}
	if _, exists := h.svc.Agent(agentID); !exists {
		return feishuBotAppInfo{}, false
	}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "Bearer ") {
		return feishuBotAppInfo{}, false
	}
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), ".")
	if len(parts) != 3 || parts[0] != larkCLISourceTokenPrefix {
		return feishuBotAppInfo{}, false
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return feishuBotAppInfo{}, false
	}
	var payload larkCLISourceTokenPayload
	if json.Unmarshal(data, &payload) != nil || payload.Version != larkCLISourceTokenPrefix || payload.Purpose != larkCLISourceTokenPurpose || payload.AgentID != agent.CanonicalID(agentID) || payload.ParticipantID == "" || payload.CredentialRevision == "" {
		return feishuBotAppInfo{}, false
	}
	valid := false
	for _, secret := range h.larkCLISourceSigningSecrets() {
		if hmac.Equal([]byte(parts[2]), []byte(signLarkCLISourceToken(parts[1], secret))) {
			valid = true
			break
		}
	}
	if !valid {
		return feishuBotAppInfo{}, false
	}
	current, err := h.feishuBotAppInfoForAgent(payload.AgentID)
	if err != nil || current.ParticipantID != payload.ParticipantID {
		return feishuBotAppInfo{}, false
	}
	item, exists := h.participant.Get(participant.ChannelFeishu, payload.ParticipantID)
	if !exists || item.Type != participant.TypeAgent || agent.CanonicalID(item.AgentID) != payload.AgentID {
		return feishuBotAppInfo{}, false
	}
	appID := channelAppConfigString(item.ChannelAppConfig, "app_id")
	secret := channelAppConfigString(item.ChannelAppConfig, participant.ChannelAppConfigAppSecretKey)
	if appID == "" || secret == "" || secret == participant.RedactedSecretValue || !hmac.Equal([]byte(payload.CredentialRevision), []byte(feishularkcli.CredentialRevision(item.ID, appID, secret))) {
		return feishuBotAppInfo{}, false
	}
	return feishuBotAppInfo{AgentID: payload.AgentID, ParticipantID: item.ID, AppID: appID, AppSecret: secret}, true
}

func (h *Handler) larkCLISourceSigningSecrets() []string {
	var secrets []string
	if h == nil {
		return nil
	}
	if token := strings.TrimSpace(h.serverAccessToken); token != "" {
		secrets = append(secrets, token)
	}
	if token := strings.TrimSpace(h.desktopSessionToken); token != "" {
		secrets = append(secrets, token)
	}
	if len(secrets) == 0 && h.serverNoAuth {
		h.larkCLISigningOnce.Do(func() {
			var key [32]byte
			if _, err := rand.Read(key[:]); err != nil {
				h.larkCLISigningErr = err
				return
			}
			h.larkCLISigningSecret = base64.RawURLEncoding.EncodeToString(key[:])
		})
		if h.larkCLISigningSecret != "" {
			secrets = append(secrets, h.larkCLISigningSecret)
		}
	}
	return secrets
}

func signLarkCLISourceToken(encodedPayload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (h *Handler) internalSourceBaseURL() string {
	if h != nil && strings.TrimSpace(h.internalBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(h.internalBaseURL), "/")
	}
	return strings.TrimRight(config.DefaultAPIBaseURL(), "/")
}
