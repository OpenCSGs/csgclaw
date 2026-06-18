package api

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/participant"
	"csgclaw/internal/participant/feishubind"
)

const (
	feishuRegistrationPath           = "/oauth/v1/app/registration"
	feishuRegistrationDefaultExpires = 10 * time.Minute
)

var (
	feishuRegistrationAccountsBaseURL = "https://accounts.feishu.cn"
	feishuRegistrationHTTPClient      = http.DefaultClient
	feishuRegistrationNow             = time.Now
)

type createFeishuRegistrationRequest struct {
	AgentID string `json:"agent_id"`
}

type feishuRegistrationState struct {
	RegistrationID  string    `json:"registration_id"`
	AgentID         string    `json:"agent_id"`
	ParticipantID   string    `json:"participant_id"`
	DeviceCode      string    `json:"device_code"`
	ConnectURL      string    `json:"connect_url"`
	UserCode        string    `json:"user_code,omitempty"`
	IntervalSeconds int       `json:"interval_seconds"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type feishuRegistrationResponse struct {
	RegistrationID  string    `json:"registration_id"`
	AgentID         string    `json:"agent_id"`
	ParticipantID   string    `json:"participant_id"`
	ConnectURL      string    `json:"connect_url,omitempty"`
	UserCode        string    `json:"user_code,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
	NextPollSeconds int       `json:"next_poll_seconds,omitempty"`
	Status          string    `json:"status,omitempty"`
}

func (h *Handler) createFeishuRegistration(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil || h.participant == nil {
		http.Error(w, "agent and participant services are required", http.StatusServiceUnavailable)
		return
	}
	var req createFeishuRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	target, err := feishubind.ResolveAgent(h.svc, req.AgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if existing, found, err := h.activeFeishuRegistrationForAgent(target.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if found {
		writeJSON(w, http.StatusConflict, existing.safeResponse("pending"))
		return
	}
	if err := feishuAccountsInit(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	begin, err := feishuAccountsBegin(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	now := feishuRegistrationNow().UTC()
	registrationID, err := newFeishuRegistrationID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	interval := intFromMap(begin, "interval", 5)
	expireSeconds := intFromMap(begin, "expire_in", int(feishuRegistrationDefaultExpires.Seconds()))
	if expireSeconds <= 0 {
		expireSeconds = int(feishuRegistrationDefaultExpires.Seconds())
	}
	state := feishuRegistrationState{
		RegistrationID:  registrationID,
		AgentID:         target.ID,
		ParticipantID:   feishubind.CanonicalParticipantID(target),
		DeviceCode:      stringFromMap(begin, "device_code"),
		ConnectURL:      appendFeishuLauncherParams(stringFromMap(begin, "verification_uri_complete")),
		UserCode:        stringFromMap(begin, "user_code"),
		IntervalSeconds: interval,
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Duration(expireSeconds) * time.Second),
	}
	if state.DeviceCode == "" {
		http.Error(w, "registration begin did not return device_code", http.StatusBadGateway)
		return
	}
	if state.ConnectURL == "" {
		http.Error(w, "registration begin did not return verification_uri_complete", http.StatusBadGateway)
		return
	}
	if err := h.saveFeishuRegistration(state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, state.safeResponse("started"))
}

func (h *Handler) getFeishuRegistration(w http.ResponseWriter, r *http.Request) {
	state, ok := h.loadFeishuRegistrationHTTP(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, state.safeResponse("started"))
}

func (h *Handler) finalizeFeishuRegistration(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil || h.participant == nil {
		http.Error(w, "agent and participant services are required", http.StatusServiceUnavailable)
		return
	}
	state, ok := h.loadFeishuRegistrationHTTP(w, r)
	if !ok {
		return
	}
	if !state.ExpiresAt.IsZero() && feishuRegistrationNow().UTC().After(state.ExpiresAt) {
		http.Error(w, "registration expired", http.StatusGone)
		return
	}
	poll, err := feishuAccountsPoll(r.Context(), state.DeviceCode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if pendingFeishuRegistration(poll) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":            "pending",
			"agent_id":          state.AgentID,
			"participant_id":    state.ParticipantID,
			"next_poll_seconds": state.IntervalSeconds,
		})
		return
	}
	if failure := stringFromMap(poll, "error"); failure != "" {
		status := http.StatusBadGateway
		if failure == "access_denied" {
			status = http.StatusBadRequest
		}
		if failure == "expired_token" {
			status = http.StatusGone
		}
		http.Error(w, "registration failed: "+failure, status)
		return
	}
	appID := stringFromMap(poll, "client_id")
	appSecret := stringFromMap(poll, "client_secret")
	if appID == "" || appSecret == "" {
		http.Error(w, "registration response did not include app credentials", http.StatusBadGateway)
		return
	}

	if state.AgentID == agent.ManagerUserID {
		if openID := openIDFromRegistrationResult(poll); openID != "" {
			if _, err := feishubind.BindAdminHuman(r.Context(), h.participant, openID, "admin"); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	result, err := feishubind.BindBot(r.Context(), h.svc, h.participant, state.AgentID, appID, appSecret, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = h.deleteFeishuRegistration(state.RegistrationID)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) loadFeishuRegistrationHTTP(w http.ResponseWriter, r *http.Request) (feishuRegistrationState, bool) {
	registrationID := pathValue(r, "registration_id")
	if strings.TrimSpace(registrationID) == "" {
		http.NotFound(w, r)
		return feishuRegistrationState{}, false
	}
	state, err := h.loadFeishuRegistration(registrationID)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return feishuRegistrationState{}, false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return feishuRegistrationState{}, false
	}
	return state, true
}

func (s feishuRegistrationState) safeResponse(status string) feishuRegistrationResponse {
	return feishuRegistrationResponse{
		RegistrationID:  s.RegistrationID,
		AgentID:         s.AgentID,
		ParticipantID:   s.ParticipantID,
		ConnectURL:      s.ConnectURL,
		UserCode:        s.UserCode,
		ExpiresAt:       s.ExpiresAt,
		NextPollSeconds: s.IntervalSeconds,
		Status:          status,
	}
}

func (h *Handler) feishuRegistrationDir() (string, error) {
	if h != nil && strings.TrimSpace(h.feishuRegistrationStateDir) != "" {
		return strings.TrimSpace(h.feishuRegistrationStateDir), nil
	}
	dir, err := config.DefaultChannelDir(participant.ChannelFeishu)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "registrations"), nil
}

func (h *Handler) saveFeishuRegistration(state feishuRegistrationState) error {
	dir, err := h.feishuRegistrationDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create feishu registration dir: %w", err)
	}
	_ = os.Chmod(dir, 0o700)
	path := filepath.Join(dir, safeFeishuRegistrationID(state.RegistrationID)+".json")
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode feishu registration state: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write feishu registration state: %w", err)
	}
	_ = os.Chmod(tmp, 0o600)
	return os.Rename(tmp, path)
}

func (h *Handler) loadFeishuRegistration(registrationID string) (feishuRegistrationState, error) {
	dir, err := h.feishuRegistrationDir()
	if err != nil {
		return feishuRegistrationState{}, err
	}
	path := filepath.Join(dir, safeFeishuRegistrationID(registrationID)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return feishuRegistrationState{}, err
	}
	var state feishuRegistrationState
	if err := json.Unmarshal(data, &state); err != nil {
		return feishuRegistrationState{}, fmt.Errorf("decode feishu registration state: %w", err)
	}
	if strings.TrimSpace(state.RegistrationID) == "" {
		state.RegistrationID = strings.TrimSpace(registrationID)
	}
	return state, nil
}

func (h *Handler) activeFeishuRegistrationForAgent(agentID string) (feishuRegistrationState, bool, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return feishuRegistrationState{}, false, nil
	}
	dir, err := h.feishuRegistrationDir()
	if err != nil {
		return feishuRegistrationState{}, false, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return feishuRegistrationState{}, false, nil
		}
		return feishuRegistrationState{}, false, fmt.Errorf("read feishu registration dir: %w", err)
	}
	now := feishuRegistrationNow().UTC()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return feishuRegistrationState{}, false, fmt.Errorf("read feishu registration state: %w", err)
		}
		var state feishuRegistrationState
		if err := json.Unmarshal(data, &state); err != nil {
			return feishuRegistrationState{}, false, fmt.Errorf("decode feishu registration state: %w", err)
		}
		if strings.TrimSpace(state.AgentID) != agentID {
			continue
		}
		if !state.ExpiresAt.IsZero() && !now.Before(state.ExpiresAt) {
			continue
		}
		return state, true, nil
	}
	return feishuRegistrationState{}, false, nil
}

func (h *Handler) deleteFeishuRegistration(registrationID string) error {
	dir, err := h.feishuRegistrationDir()
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, safeFeishuRegistrationID(registrationID)+".json"))
}

func feishuAccountsInit(ctx context.Context) error {
	res, err := feishuAccountsPost(ctx, map[string]string{"action": "init"})
	if err != nil {
		return err
	}
	methods, _ := res["supported_auth_methods"].([]any)
	for _, method := range methods {
		if strings.TrimSpace(fmt.Sprint(method)) == "client_secret" {
			return nil
		}
	}
	return fmt.Errorf("Feishu registration does not support client_secret auth")
}

func feishuAccountsBegin(ctx context.Context) (map[string]any, error) {
	return feishuAccountsPost(ctx, map[string]string{
		"action":            "begin",
		"archetype":         "PersonalAgent",
		"auth_method":       "client_secret",
		"request_user_info": "open_id",
	})
}

func feishuAccountsPoll(ctx context.Context, deviceCode string) (map[string]any, error) {
	return feishuAccountsPost(ctx, map[string]string{
		"action":      "poll",
		"device_code": strings.TrimSpace(deviceCode),
		"tp":          "ob_app",
	})
}

func feishuAccountsPost(ctx context.Context, body map[string]string) (map[string]any, error) {
	endpoint := strings.TrimRight(feishuRegistrationAccountsBaseURL, "/") + feishuRegistrationPath
	values := url.Values{}
	for key, value := range body {
		values.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := feishuRegistrationHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feishu registration request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read feishu registration response: %w", err)
	}
	var decoded map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, fmt.Errorf("decode feishu registration response: %w", err)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(decoded) > 0 {
			return decoded, nil
		}
		return nil, fmt.Errorf("feishu registration status %d", resp.StatusCode)
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	return decoded, nil
}

func pendingFeishuRegistration(values map[string]any) bool {
	switch stringFromMap(values, "error") {
	case "authorization_pending", "slow_down", "temporary_network_error":
		return true
	default:
		return false
	}
}

func openIDFromRegistrationResult(values map[string]any) string {
	userInfo, ok := values["user_info"].(map[string]any)
	if !ok {
		return ""
	}
	return stringFromMap(userInfo, "open_id")
}

func appendFeishuLauncherParams(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURL
	}
	query := parsed.Query()
	if query.Get("from") == "" {
		query.Set("from", "csgclaw")
	}
	if query.Get("tp") == "" {
		query.Set("tp", "csgclaw")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func newFeishuRegistrationID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate registration id: %w", err)
	}
	return strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(data[:]), "=")), nil
}

func safeFeishuRegistrationID(registrationID string) string {
	var b strings.Builder
	for _, r := range registrationID {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stringFromMap(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func intFromMap(values map[string]any, key string, fallback int) int {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		n, err := typed.Int64()
		if err == nil {
			return int(n)
		}
	case string:
		var out int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &out); err == nil {
			return out
		}
	}
	return fallback
}
