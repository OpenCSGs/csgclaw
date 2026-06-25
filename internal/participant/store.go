package participant

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/localstore"
)

type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]apitypes.Participant
}

type persistedState struct {
	Participants []apitypes.Participant `json:"participants"`
}

type rootParticipantsState struct {
	Items        []apitypes.Participant `json:"items"`
	Participants []apitypes.Participant `json:"participants,omitempty"`
}

type legacyBotState struct {
	Bots []apitypes.LegacyBot `json:"bots"`
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:  path,
		items: make(map[string]apitypes.Participant),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func NewMemoryStore(items []apitypes.Participant) *Store {
	s := &Store{items: make(map[string]apitypes.Participant)}
	for _, item := range items {
		item = normalizeStoredParticipant(item)
		if item.Channel == "" || item.ID == "" {
			continue
		}
		s.items[storeKey(item.Channel, item.ID)] = item
	}
	return s
}

func (s *Store) List(opts ListOptions) []apitypes.Participant {
	if s == nil {
		return nil
	}
	channel := strings.TrimSpace(opts.Channel)
	typ := strings.TrimSpace(opts.Type)
	agentID := strings.TrimSpace(opts.AgentID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]apitypes.Participant, 0, len(s.items))
	for _, item := range s.items {
		if channel != "" && item.Channel != channel {
			continue
		}
		if typ != "" && item.Type != typ {
			continue
		}
		if agentID != "" && item.AgentID != agentID {
			continue
		}
		out = append(out, cloneParticipant(item))
	}
	sortParticipants(out)
	return out
}

func (s *Store) Get(channel, id string) (apitypes.Participant, bool) {
	if s == nil {
		return apitypes.Participant{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[storeKey(channel, id)]
	if !ok {
		return apitypes.Participant{}, false
	}
	return cloneParticipant(item), true
}

func (s *Store) Save(item apitypes.Participant) error {
	if s == nil {
		return fmt.Errorf("participant store is required")
	}
	item = normalizeStoredParticipant(item)
	if item.Channel == "" {
		return fmt.Errorf("channel is required")
	}
	if item.ID == "" {
		return fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[storeKey(item.Channel, item.ID)] = item
	return s.saveLocked()
}

func (s *Store) Delete(channel, id string) (apitypes.Participant, bool, error) {
	if s == nil {
		return apitypes.Participant{}, false, fmt.Errorf("participant store is required")
	}
	channel = strings.TrimSpace(channel)
	id = strings.TrimSpace(id)
	if channel == "" || id == "" {
		return apitypes.Participant{}, false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(channel, id)
	item, ok := s.items[key]
	if !ok {
		return apitypes.Participant{}, false, nil
	}
	delete(s.items, key)
	if err := s.saveLocked(); err != nil {
		s.items[key] = item
		return apitypes.Participant{}, false, err
	}
	return cloneParticipant(item), true, nil
}

func (s *Store) load() error {
	items, err := s.readState()
	if err != nil {
		return err
	}
	legacyPath, legacyExists, err := mergeLegacyBotState(s.path, items)
	if err != nil {
		return err
	}
	repairedLegacyAdmin := migrateLegacyCSGClawAdminParticipant(items)
	repairedLegacyIDs := migrateLegacyCSGClawAgentParticipantIDs(items)
	s.items = items
	if legacyExists || repairedLegacyAdmin || repairedLegacyIDs {
		if err := s.saveLocked(); err != nil {
			return fmt.Errorf("write migrated participant state: %w", err)
		}
	}
	if legacyExists {
		if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete legacy bot state after participant migration: %w", err)
		}
	}
	return nil
}

func (s *Store) readState() (map[string]apitypes.Participant, error) {
	items := make(map[string]apitypes.Participant)
	if s.path == "" {
		return items, nil
	}
	if root, ok, err := s.readRootParticipantsState(); err != nil {
		return nil, err
	} else if ok {
		for _, item := range rootParticipants(root) {
			item = normalizeStoredParticipant(item)
			if item.Channel == "" || item.ID == "" {
				return nil, fmt.Errorf("decode participant state: channel and id are required")
			}
			items[storeKey(item.Channel, item.ID)] = item
		}
		return items, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, fmt.Errorf("read participant state: %w", err)
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode participant state: %w", err)
	}
	for _, item := range state.Participants {
		item = normalizeStoredParticipant(item)
		if item.Channel == "" || item.ID == "" {
			return nil, fmt.Errorf("decode participant state: channel and id are required")
		}
		items[storeKey(item.Channel, item.ID)] = item
	}
	return items, nil
}

func (s *Store) readRootParticipantsState() (rootParticipantsState, bool, error) {
	if !s.pathLooksLikeRootState() {
		return rootParticipantsState{}, false, nil
	}
	var root rootParticipantsState
	ok, err := localstore.ReadSection(s.path, "participants", &root)
	if err != nil {
		return rootParticipantsState{}, false, err
	}
	return root, ok, nil
}

func rootParticipants(root rootParticipantsState) []apitypes.Participant {
	if len(root.Items) > 0 {
		return root.Items
	}
	return root.Participants
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	items := make([]apitypes.Participant, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, cloneParticipant(item))
	}
	sortParticipants(items)
	if s.shouldWriteRootParticipantsState() {
		return localstore.WriteSection(s.path, "participants", rootParticipantsState{Items: items})
	}
	data, err := json.MarshalIndent(persistedState{Participants: items}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode participant state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create participant state dir: %w", err)
	}
	if err := os.WriteFile(s.path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write participant state: %w", err)
	}
	return nil
}

func (s *Store) shouldWriteRootParticipantsState() bool {
	if strings.TrimSpace(s.path) == "" {
		return false
	}
	if s.pathIsDefaultRootStatePath() {
		return true
	}
	return participantRootSectionExists(s.path)
}

func (s *Store) pathLooksLikeRootState() bool {
	if strings.TrimSpace(s.path) == "" {
		return false
	}
	if participantRootSectionExists(s.path) {
		return true
	}
	return s.pathIsDefaultRootStatePath()
}

func (s *Store) pathIsDefaultRootStatePath() bool {
	path := strings.TrimSpace(s.path)
	return filepath.Base(path) == config.StateFileName && filepath.Base(filepath.Dir(path)) == config.AppDirName
}

func participantRootSectionExists(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	value, ok := raw["participants"]
	if !ok || len(value) == 0 {
		return false
	}
	var probe map[string]any
	return json.Unmarshal(value, &probe) == nil
}

func normalizeStoredParticipant(item apitypes.Participant) apitypes.Participant {
	item.ID = strings.TrimSpace(item.ID)
	item.Channel = strings.TrimSpace(item.Channel)
	item.Type = strings.TrimSpace(item.Type)
	item.Name = strings.TrimSpace(item.Name)
	item.ChannelUserRef = strings.TrimSpace(item.ChannelUserRef)
	item.ChannelUserKind = strings.TrimSpace(item.ChannelUserKind)
	item.ChannelAppRef = strings.TrimSpace(item.ChannelAppRef)
	item.AgentID = strings.TrimSpace(item.AgentID)
	item.LifecycleStatus = strings.TrimSpace(item.LifecycleStatus)
	item.Presence = strings.TrimSpace(item.Presence)
	return item
}

func sortParticipants(items []apitypes.Participant) {
	slices.SortFunc(items, func(a, b apitypes.Participant) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			if a.Channel != b.Channel {
				if a.Channel < b.Channel {
					return -1
				}
				return 1
			}
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
}

func cloneParticipant(item apitypes.Participant) apitypes.Participant {
	if item.Metadata != nil {
		cloned := make(map[string]any, len(item.Metadata))
		for key, value := range item.Metadata {
			cloned[key] = value
		}
		item.Metadata = cloned
	}
	if item.ChannelAppConfig != nil {
		cloned := make(map[string]any, len(item.ChannelAppConfig))
		for key, value := range item.ChannelAppConfig {
			cloned[key] = value
		}
		item.ChannelAppConfig = cloned
	}
	return item
}

func storeKey(channel, id string) string {
	return strings.TrimSpace(channel) + "\x00" + strings.TrimSpace(id)
}

func migrateLegacyCSGClawAdminParticipant(items map[string]apitypes.Participant) bool {
	if len(items) == 0 {
		return false
	}

	changed := false
	adminKey := storeKey(ChannelCSGClaw, bootstrapAdminParticipantID)
	legacyKey := storeKey(ChannelCSGClaw, legacyAdminParticipantID)
	legacyBareKey := storeKey(ChannelCSGClaw, legacyBareAdminParticipantID)
	if legacy, ok := items[legacyKey]; ok && isLegacyStoredAdminParticipant(legacy) {
		next := repairStoredAdminParticipant(legacy)
		if existing, exists := items[adminKey]; exists {
			next = mergeAdminParticipant(existing, next)
		}
		items[adminKey] = next
		delete(items, legacyKey)
		changed = true
	}
	if legacy, ok := items[legacyBareKey]; ok && isLegacyStoredAdminParticipant(legacy) {
		next := repairStoredAdminParticipant(legacy)
		if existing, exists := items[adminKey]; exists {
			next = mergeAdminParticipant(existing, next)
		}
		items[adminKey] = next
		delete(items, legacyBareKey)
		changed = true
	}

	if existing, ok := items[adminKey]; ok && adminParticipantNeedsRepair(existing) {
		items[adminKey] = repairStoredAdminParticipant(existing)
		changed = true
	}
	return changed
}

func isLegacyStoredAdminParticipant(item apitypes.Participant) bool {
	item = normalizeStoredParticipant(item)
	return item.Channel == ChannelCSGClaw && (item.ID == legacyAdminParticipantID || item.ID == legacyBareAdminParticipantID)
}

func adminParticipantNeedsRepair(item apitypes.Participant) bool {
	item = normalizeStoredParticipant(item)
	if item.Channel != ChannelCSGClaw || item.ID != bootstrapAdminParticipantID {
		return false
	}
	return item.Type != TypeHuman ||
		item.Name == "" ||
		item.ChannelUserRef != im.AdminUserID ||
		item.ChannelUserKind != ChannelUserKindLocalUserID ||
		item.AgentID != "" ||
		item.LifecycleStatus == "" ||
		!item.Mentionable
}

func repairStoredAdminParticipant(item apitypes.Participant) apitypes.Participant {
	item = normalizeStoredParticipant(item)
	item.ID = bootstrapAdminParticipantID
	item.Channel = ChannelCSGClaw
	item.Type = TypeHuman
	if item.Name == "" {
		item.Name = "admin"
	}
	item.ChannelUserRef = im.AdminUserID
	item.ChannelUserKind = ChannelUserKindLocalUserID
	item.AgentID = ""
	if item.LifecycleStatus == "" {
		item.LifecycleStatus = LifecycleStatusActive
	}
	item.Mentionable = true
	return item
}

func mergeAdminParticipant(existing, legacy apitypes.Participant) apitypes.Participant {
	merged := repairStoredAdminParticipant(existing)
	legacy = repairStoredAdminParticipant(legacy)
	if merged.Name == "" {
		merged.Name = legacy.Name
	}
	if merged.Avatar == "" {
		merged.Avatar = legacy.Avatar
	}
	if merged.ChannelUserKind == "" {
		merged.ChannelUserKind = legacy.ChannelUserKind
	}
	if merged.LifecycleStatus == "" {
		merged.LifecycleStatus = legacy.LifecycleStatus
	}
	if merged.Presence == "" {
		merged.Presence = legacy.Presence
	}
	merged.Mentionable = true
	if merged.Metadata == nil {
		merged.Metadata = cloneParticipant(legacy).Metadata
	} else {
		for key, value := range legacy.Metadata {
			if _, ok := merged.Metadata[key]; !ok {
				merged.Metadata[key] = value
			}
		}
	}
	if merged.CreatedAt.IsZero() || (!legacy.CreatedAt.IsZero() && legacy.CreatedAt.Before(merged.CreatedAt)) {
		merged.CreatedAt = legacy.CreatedAt
	}
	if merged.UpdatedAt.IsZero() || legacy.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = legacy.UpdatedAt
	}
	return merged
}

func migrateLegacyCSGClawAgentParticipantIDs(items map[string]apitypes.Participant) bool {
	if len(items) == 0 {
		return false
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	changed := false
	for _, key := range keys {
		item, ok := items[key]
		if !ok {
			continue
		}
		nextID, ok := legacyCSGClawAgentParticipantID(item)
		if !ok {
			continue
		}
		next := item
		legacyID := strings.TrimSpace(next.ID)
		next.ID = nextID
		suffix := strings.TrimPrefix(nextID, "pt-")
		if strings.TrimSpace(next.ChannelUserRef) == "" || strings.TrimSpace(next.ChannelUserRef) == legacyID {
			next.ChannelUserRef = "user-" + suffix
		}
		if strings.TrimSpace(next.AgentID) == "" || strings.TrimSpace(next.AgentID) == legacyID {
			next.AgentID = "agent-" + suffix
		}
		next = normalizeStoredParticipant(next)

		nextKey := storeKey(next.Channel, next.ID)
		if nextKey == key {
			continue
		}
		if existing, exists := items[nextKey]; exists {
			if sameAgentParticipantBinding(existing, next) {
				items[nextKey] = mergeAgentParticipant(existing, next)
				delete(items, key)
				changed = true
			}
			continue
		}
		items[nextKey] = next
		delete(items, key)
		changed = true
	}
	return changed
}

func legacyCSGClawAgentParticipantID(item apitypes.Participant) (string, bool) {
	item = normalizeStoredParticipant(item)
	if item.Channel != ChannelCSGClaw || item.Type != TypeAgent {
		return "", false
	}
	if item.ID == "" || item.ID == agent.ManagerUserID || !strings.HasPrefix(item.ID, "u-") {
		return "", false
	}
	if item.AgentID != "" && item.AgentID != item.ID {
		return "", false
	}
	id := strings.TrimPrefix(item.ID, "u-")
	if id == "" || id == item.ID {
		return "", false
	}
	return "pt-" + id, true
}

func sameAgentParticipantBinding(a, b apitypes.Participant) bool {
	a = normalizeStoredParticipant(a)
	b = normalizeStoredParticipant(b)
	if a.Channel != b.Channel || a.Type != b.Type || a.Type != TypeAgent {
		return false
	}
	if a.AgentID != "" && b.AgentID != "" && a.AgentID == b.AgentID {
		return true
	}
	return a.ChannelUserRef != "" && b.ChannelUserRef != "" && a.ChannelUserRef == b.ChannelUserRef
}

func mergeAgentParticipant(existing, legacy apitypes.Participant) apitypes.Participant {
	merged := normalizeStoredParticipant(existing)
	legacy = normalizeStoredParticipant(legacy)
	if merged.Name == "" {
		merged.Name = legacy.Name
	}
	if merged.Avatar == "" {
		merged.Avatar = legacy.Avatar
	}
	if merged.ChannelUserRef == "" {
		merged.ChannelUserRef = legacy.ChannelUserRef
	}
	if merged.ChannelUserKind == "" {
		merged.ChannelUserKind = legacy.ChannelUserKind
	}
	if merged.AgentID == "" {
		merged.AgentID = legacy.AgentID
	}
	if merged.LifecycleStatus == "" {
		merged.LifecycleStatus = legacy.LifecycleStatus
	}
	if merged.Presence == "" {
		merged.Presence = legacy.Presence
	}
	merged.Mentionable = merged.Mentionable || legacy.Mentionable
	if merged.Metadata == nil {
		merged.Metadata = cloneParticipant(legacy).Metadata
	} else {
		for key, value := range legacy.Metadata {
			if _, ok := merged.Metadata[key]; !ok {
				merged.Metadata[key] = value
			}
		}
	}
	if merged.CreatedAt.IsZero() || (!legacy.CreatedAt.IsZero() && legacy.CreatedAt.Before(merged.CreatedAt)) {
		merged.CreatedAt = legacy.CreatedAt
	}
	if merged.UpdatedAt.IsZero() || legacy.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = legacy.UpdatedAt
	}
	return merged
}

func mergeLegacyBotState(participantPath string, items map[string]apitypes.Participant) (string, bool, error) {
	if strings.TrimSpace(participantPath) == "" {
		return "", false, nil
	}
	legacyPath := filepath.Join(filepath.Dir(participantPath), "bots.json")
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			if rootLegacyPath := legacyBotPathForRootParticipantState(participantPath); rootLegacyPath != "" && rootLegacyPath != legacyPath {
				legacyPath = rootLegacyPath
				data, err = os.ReadFile(legacyPath)
			}
		}
		if err != nil {
			if os.IsNotExist(err) {
				return "", false, nil
			}
			return legacyPath, true, fmt.Errorf("read legacy bot state: %w", err)
		}
	}

	var state legacyBotState
	if err := json.Unmarshal(data, &state); err != nil {
		return legacyPath, true, fmt.Errorf("decode legacy bot state: %w", err)
	}
	now := time.Now().UTC()
	for _, b := range state.Bots {
		item, err := participantFromLegacyBot(b, now)
		if err != nil {
			return legacyPath, true, fmt.Errorf("decode legacy bot state: %w", err)
		}
		key := storeKey(item.Channel, item.ID)
		if _, exists := items[key]; exists {
			continue
		}
		items[key] = item
	}
	return legacyPath, true, nil
}

func legacyBotPathForRootParticipantState(participantPath string) string {
	participantPath = strings.TrimSpace(participantPath)
	if filepath.Base(participantPath) != config.StateFileName {
		return ""
	}
	return filepath.Join(filepath.Dir(participantPath), "im", "bots.json")
}

func participantFromLegacyBot(b apitypes.LegacyBot, now time.Time) (apitypes.Participant, error) {
	legacyID := strings.TrimSpace(b.ID)
	if legacyID == "" {
		return apitypes.Participant{}, fmt.Errorf("id is required")
	}
	channel := normalizeChannel(b.Channel)
	if channel == "" {
		return apitypes.Participant{}, fmt.Errorf("channel must be one of %q or %q", ChannelCSGClaw, ChannelFeishu)
	}
	typ := TypeAgent
	if strings.EqualFold(strings.TrimSpace(b.Type), TypeNotification) {
		typ = TypeNotification
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		name = legacyID
	}
	channelUserRef := strings.TrimSpace(b.UserID)
	if channelUserRef == "" {
		channelUserRef = strings.TrimSpace(b.AgentID)
	}
	if channelUserRef == "" {
		channelUserRef = legacyID
	}
	channelUserKind := ChannelUserKindLocalUserID
	if channel == ChannelFeishu {
		channelUserKind = ChannelUserKindOpenID
	}
	agentID := strings.TrimSpace(b.AgentID)
	if typ == TypeAgent && agentID == "" && strings.HasPrefix(legacyID, "u-") {
		agentID = legacyID
	}
	if typ == TypeNotification {
		agentID = ""
	}
	id := legacyID
	if channel == ChannelCSGClaw && typ == TypeAgent && strings.HasPrefix(legacyID, "u-") {
		suffix := strings.TrimPrefix(legacyID, "u-")
		if suffix != "" {
			id = "pt-" + suffix
			channelUserRef = "user-" + suffix
			agentID = "agent-" + suffix
		}
	}
	if isLegacyCSGClawManagerBot(b, typ, channel, agentID) {
		id = agent.ManagerParticipantID
		channelUserRef = im.ManagerUserID
		if agentID == "" {
			agentID = agent.ManagerUserID
		}
	}
	createdAt := b.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now
	}

	return apitypes.Participant{
		ID:              id,
		Channel:         channel,
		Type:            typ,
		Name:            name,
		Avatar:          strings.TrimSpace(b.Avatar),
		ChannelUserRef:  channelUserRef,
		ChannelUserKind: channelUserKind,
		AgentID:         agentID,
		LifecycleStatus: LifecycleStatusActive,
		Mentionable:     true,
		Metadata:        legacyBotMetadata(b),
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}, nil
}

func isLegacyCSGClawManagerBot(b apitypes.LegacyBot, typ, channel, agentID string) bool {
	if channel != ChannelCSGClaw || typ != TypeAgent {
		return false
	}
	if strings.TrimSpace(agentID) == agent.ManagerUserID {
		return true
	}
	if strings.TrimSpace(b.ID) == agent.ManagerUserID {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(b.Role), agent.RoleManager)
}

func legacyBotMetadata(b apitypes.LegacyBot) map[string]any {
	metadata := cloneAnyMap(b.RuntimeOptions)
	putMetadataString(metadata, "description", b.Description)
	putMetadataString(metadata, "legacy_bot_type", b.Type)
	putMetadataString(metadata, "legacy_role", b.Role)
	putMetadataString(metadata, "legacy_runtime_kind", b.RuntimeKind)
	putMetadataString(metadata, "legacy_image", b.Image)
	putMetadataString(metadata, "legacy_status", b.Status)
	putMetadataString(metadata, "legacy_provider", b.Provider)
	putMetadataString(metadata, "legacy_model_id", b.ModelID)
	if strings.TrimSpace(b.AgentID) != "" && strings.EqualFold(strings.TrimSpace(b.Type), TypeNotification) {
		putMetadataString(metadata, "legacy_agent_id", b.AgentID)
	}
	metadata["legacy_available"] = b.Available
	if b.ProfileComplete {
		metadata["legacy_profile_complete"] = true
	}
	if b.EnvRestartRequired {
		metadata["legacy_env_restart_required"] = true
	}
	if b.ImageUpgradeRequired {
		metadata["legacy_image_upgrade_required"] = true
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func putMetadataString(metadata map[string]any, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if _, exists := metadata[key]; exists {
		return
	}
	metadata[key] = value
}
