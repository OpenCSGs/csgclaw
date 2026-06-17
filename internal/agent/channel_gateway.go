package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

// ChannelGatewayRequest describes a channel sidecar that is attached to an
// existing agent runtime. The sidecar owns channel IO and delegates model work
// back through CSGClaw's LLM bridge for the agent.
type ChannelGatewayRequest struct {
	Agent         Agent
	Channel       string
	ParticipantID string
	Fingerprint   string
}

type channelGatewayState struct {
	Channel     string    `json:"channel"`
	Name        string    `json:"name"`
	BoxID       string    `json:"box_id,omitempty"`
	RuntimeKind string    `json:"runtime_kind,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

func (s *Service) EnsureChannelGateway(ctx context.Context, req ChannelGatewayRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return fmt.Errorf("agent service is required")
	}
	a := req.Agent
	agentID := strings.TrimSpace(a.ID)
	agentName := strings.TrimSpace(a.Name)
	channel := normalizeChannelGatewayChannel(req.Channel)
	if agentID == "" {
		return fmt.Errorf("agent id is required")
	}
	if agentName == "" {
		return fmt.Errorf("agent name is required")
	}
	if channel == "" {
		return fmt.Errorf("channel is required")
	}
	participantID := strings.TrimSpace(req.ParticipantID)
	if participantID == "" {
		participantID = participantIDForAgent(agentName, agentID)
	}
	sidecarName := ChannelGatewayName(agentName, channel)
	runtimeKind := s.gatewayRuntimeKind()
	runtimeImpl, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return err
	}
	profile := s.runtimeProfileForAgent(a)
	gateway, err := s.gatewayProvisionRequest(runtimeKind, agentName, agentID)
	if err != nil {
		return err
	}
	gateway.DisableInternalCSGClawChannel = true
	if err := s.provisionRuntime(ctx, runtimeImpl, runtimeKind, agentruntime.ProvisionRequest{
		RuntimeID:     normalizeRuntimeID(a.RuntimeID, agentID),
		AgentID:       agentID,
		ParticipantID: participantID,
		AgentName:     agentName,
		Profile:       profile,
		Gateway:       gateway,
	}); err != nil {
		return fmt.Errorf("provision %s channel gateway: %w", channel, err)
	}

	statePath, err := channelGatewayStatePath(agentName, channel)
	if err != nil {
		return err
	}
	state, err := readChannelGatewayState(statePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	rt, runtimeHome, err := s.openChannelGatewayRuntime(agentName)
	if err != nil {
		return err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()

	existing, existingKey, found, err := s.lookupChannelGatewayBox(ctx, rt, state, sidecarName)
	if err != nil {
		return err
	}
	if found && !channelGatewayNeedsRecreate(state, channel, sidecarName, runtimeKind, req.Fingerprint) && existing.State == sandbox.StateRunning {
		if strings.TrimSpace(state.BoxID) == "" && strings.TrimSpace(existing.ID) != "" {
			state.BoxID = strings.TrimSpace(existing.ID)
			_ = writeChannelGatewayState(statePath, state)
		}
		return nil
	}
	if found {
		removeKey := strings.TrimSpace(existing.ID)
		if removeKey == "" {
			removeKey = strings.TrimSpace(existingKey)
		}
		if err := s.removeChannelGatewayBox(ctx, rt, channelGatewayState{BoxID: removeKey, Name: sidecarName}); err != nil {
			return err
		}
	}

	image := s.managerGatewayImage()
	if image == "" {
		return fmt.Errorf("gateway image is required")
	}
	box, info, err := s.createGatewayBox(ctx, rt, image, sidecarName, agentID, profileFromRuntimeProfile(profile, a.Name, a.Description))
	if err != nil {
		return fmt.Errorf("create %s channel gateway: %w", channel, err)
	}
	defer func() {
		_ = s.closeBox(box)
	}()
	if strings.TrimSpace(info.ID) == "" {
		info.ID = strings.TrimSpace(sidecarName)
	}
	if err := writeChannelGatewayState(statePath, channelGatewayState{
		Channel:     channel,
		Name:        sidecarName,
		BoxID:       strings.TrimSpace(info.ID),
		RuntimeKind: runtimeKind,
		Fingerprint: strings.TrimSpace(req.Fingerprint),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) StopChannelGateway(ctx context.Context, agentName, channel string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return nil
	}
	agentName = strings.TrimSpace(agentName)
	channel = normalizeChannelGatewayChannel(channel)
	if agentName == "" || channel == "" {
		return nil
	}
	statePath, err := channelGatewayStatePath(agentName, channel)
	if err != nil {
		return err
	}
	state, err := readChannelGatewayState(statePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.TrimSpace(state.Name) == "" {
		state.Name = ChannelGatewayName(agentName, channel)
	}
	rt, runtimeHome, err := s.openChannelGatewayRuntime(agentName)
	if err != nil {
		if sandbox.IsNotFound(err) {
			_ = os.Remove(statePath)
			return nil
		}
		return err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()
	if err := s.removeChannelGatewayBox(ctx, rt, state); err != nil {
		return err
	}
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove channel gateway state: %w", err)
	}
	return nil
}

func ChannelGatewayName(agentName, channel string) string {
	agentName = strings.TrimSpace(agentName)
	channel = normalizeChannelGatewayChannel(channel)
	if agentName == "" || channel == "" {
		return ""
	}
	return agentName + "-" + channel + "-gateway"
}

func normalizeChannelGatewayChannel(channel string) string {
	return strings.ToLower(strings.TrimSpace(channel))
}

func (s *Service) managerGatewayImage() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.managerImage)
}

func (s *Service) openChannelGatewayRuntime(agentName string) (sandbox.Runtime, string, error) {
	runtimeHome, err := s.sandboxRuntimeHome(agentName)
	if err != nil {
		return nil, "", err
	}
	rt, err := s.ensureRuntime(agentName)
	if err != nil {
		return nil, "", err
	}
	return rt, runtimeHome, nil
}

func (s *Service) lookupChannelGatewayBox(ctx context.Context, rt sandbox.Runtime, state channelGatewayState, sidecarName string) (sandbox.Info, string, bool, error) {
	for _, key := range uniqueNonEmpty(strings.TrimSpace(state.BoxID), strings.TrimSpace(sidecarName)) {
		box, err := s.getBox(ctx, rt, key)
		if err != nil {
			if sandbox.IsNotFound(err) {
				continue
			}
			return sandbox.Info{}, "", false, err
		}
		info, infoErr := s.boxInfo(ctx, box)
		_ = s.closeBox(box)
		if infoErr != nil {
			return sandbox.Info{}, "", false, infoErr
		}
		return info, key, true, nil
	}
	return sandbox.Info{}, "", false, nil
}

func (s *Service) removeChannelGatewayBox(ctx context.Context, rt sandbox.Runtime, state channelGatewayState) error {
	var lastErr error
	for _, key := range uniqueNonEmpty(strings.TrimSpace(state.BoxID), strings.TrimSpace(state.Name)) {
		err := s.forceRemoveBox(ctx, rt, key)
		if err == nil {
			return nil
		}
		if sandbox.IsNotFound(err) {
			lastErr = err
			continue
		}
		return fmt.Errorf("remove channel gateway %q: %w", key, err)
	}
	if lastErr != nil {
		return nil
	}
	return nil
}

func channelGatewayNeedsRecreate(state channelGatewayState, channel, name, runtimeKind, fingerprint string) bool {
	return !strings.EqualFold(strings.TrimSpace(state.Channel), strings.TrimSpace(channel)) ||
		strings.TrimSpace(state.Name) != strings.TrimSpace(name) ||
		strings.TrimSpace(state.RuntimeKind) != strings.TrimSpace(runtimeKind) ||
		strings.TrimSpace(state.Fingerprint) != strings.TrimSpace(fingerprint)
}

func channelGatewayStatePath(agentName, channel string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, "channel-gateways", normalizeChannelGatewayChannel(channel)+".json"), nil
}

func readChannelGatewayState(path string) (channelGatewayState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return channelGatewayState{}, err
	}
	var state channelGatewayState
	if err := json.Unmarshal(data, &state); err != nil {
		return channelGatewayState{}, fmt.Errorf("decode channel gateway state: %w", err)
	}
	return state, nil
}

func writeChannelGatewayState(path string, state channelGatewayState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create channel gateway state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode channel gateway state: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write channel gateway state: %w", err)
	}
	return nil
}

func uniqueNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func profileFromRuntimeProfile(profile agentruntime.Profile, name, description string) AgentProfile {
	return AgentProfile{
		Name:            strings.TrimSpace(name),
		Description:     strings.TrimSpace(description),
		Provider:        profile.Provider,
		BaseURL:         profile.BaseURL,
		APIKey:          profile.APIKey,
		ModelID:         profile.ModelID,
		ReasoningEffort: profile.ReasoningEffort,
		Env:             profile.Env,
		ProfileComplete: strings.TrimSpace(profile.ModelID) != "",
	}
}
