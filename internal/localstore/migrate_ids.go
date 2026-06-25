package localstore

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type MigrateOptions struct {
	Root string
	Now  func() time.Time
}

type MigrateResult struct {
	BackupPath string
}

type modelRef struct {
	ModelProviderID string `json:"model_provider_id"`
	ModelID         string `json:"model_id"`
}

type rootState struct {
	Version        int            `json:"version"`
	ModelProviders map[string]any `json:"model_providers"`
	Agents         map[string]any `json:"agents"`
	Participants   map[string]any `json:"participants"`
	Auth           map[string]any `json:"auth,omitempty"`
}

type typedIDMigrator struct {
	root       string
	ids        migrationIDs
	agentNames map[string]string
}

type migrationIDs struct {
	agents       *idTable
	participants *idTable
	users        *idTable
	rooms        *idTable
	teams        *idTable
	messages     *idTable
	tasks        *idTable
	approvals    *idTable
	runtimes     *idTable
}

type idTable struct {
	prefix  string
	ids     map[string]string
	reverse map[string]string
}

var slugCharPattern = regexp.MustCompile(`[^a-z0-9._-]+`)

const (
	legacyRootAuthFileName      = "auth.json"
	legacyAuthDirName           = "auth"
	cliproxyAuthDirName         = "cliproxy-auth"
	legacyCSGHubAuthFileName    = "csghub.json"
	rootAuthSectionName         = "auth"
	rootOpenCSGAuthKey          = "opencsg"
	rootCSGHubAuthKey           = "csghub"
	aiGatewayBuiltinAPIKeyField = "ai_gateway_builtin_api_key"
)

func MigrateTypedIDs(opts MigrateOptions) (MigrateResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return MigrateResult{}, fmt.Errorf("root is required")
	}
	root = filepath.Clean(root)
	if _, err := os.Stat(root); err != nil {
		return MigrateResult{}, fmt.Errorf("stat store root: %w", err)
	}
	if !NeedsTypedIDMigration(root) {
		return MigrateResult{}, nil
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	backupPath, err := nextBackupPath(root, now())
	if err != nil {
		return MigrateResult{}, err
	}
	if err := copyDir(root, backupPath); err != nil {
		return MigrateResult{}, fmt.Errorf("backup store: %w", err)
	}
	m := newTypedIDMigrator(root)
	if err := m.migrate(); err != nil {
		return MigrateResult{}, err
	}
	return MigrateResult{BackupPath: backupPath}, nil
}

func NeedsTypedIDMigration(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return false
	}
	return needsTypedIDMigration(root) || needsOpenCSGAuthMigration(root) || needsCLIProxyAuthDirMigration(root)
}

func needsTypedIDMigration(root string) bool {
	for _, rel := range []string{
		"models.json",
		filepath.Join("agents", "state.json"),
		filepath.Join("im", "participants.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			return true
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "im", "state.json"))
	if err != nil {
		return false
	}
	var state struct {
		Rooms []struct {
			Members  []string          `json:"members"`
			Threads  []json.RawMessage `json:"threads"`
			Messages []struct {
				SenderID string `json:"sender_id"`
			} `json:"messages"`
		} `json:"rooms"`
	}
	if json.Unmarshal(data, &state) != nil {
		return false
	}
	for _, room := range state.Rooms {
		if len(room.Threads) > 0 {
			return true
		}
		for _, member := range room.Members {
			if !strings.HasPrefix(strings.TrimSpace(member), "pt-") {
				return true
			}
		}
		for _, message := range room.Messages {
			if senderID := strings.TrimSpace(message.SenderID); senderID != "" && !strings.HasPrefix(senderID, "pt-") {
				return true
			}
		}
	}
	return false
}

func needsOpenCSGAuthMigration(root string) bool {
	for _, rel := range []string{
		legacyRootAuthFileName,
		filepath.Join(legacyAuthDirName, legacyCSGHubAuthFileName),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			return true
		}
	}
	state, err := readJSONMapIfExists(filepath.Join(root, RootStateFileName))
	if err != nil {
		return false
	}
	authState, _ := state[rootAuthSectionName].(map[string]any)
	_, hasCSGHub := authState[rootCSGHubAuthKey]
	return hasCSGHub
}

func needsCLIProxyAuthDirMigration(root string) bool {
	info, err := os.Stat(filepath.Join(root, legacyAuthDirName))
	return err == nil && info.IsDir()
}

func newTypedIDMigrator(root string) *typedIDMigrator {
	return &typedIDMigrator{
		root:       root,
		agentNames: make(map[string]string),
		ids: migrationIDs{
			agents:       newIDTable("agent"),
			participants: newIDTable("pt"),
			users:        newIDTable("user"),
			rooms:        newIDTable("room"),
			teams:        newIDTable("team"),
			messages:     newIDTable("msg"),
			tasks:        newIDTable("task"),
			approvals:    newIDTable("approval"),
			runtimes:     newIDTable("rt"),
		},
	}
}

func newIDTable(prefix string) *idTable {
	return &idTable{
		prefix:  prefix,
		ids:     make(map[string]string),
		reverse: make(map[string]string),
	}
}

func (m *typedIDMigrator) migrate() error {
	existingRootState, err := readJSONMapIfExists(filepath.Join(m.root, RootStateFileName))
	if err != nil {
		return err
	}
	typedIDsNeeded := needsTypedIDMigration(m.root)
	agentsState := map[string]any{}
	modelsState := map[string]any{}
	participantsState := map[string]any{}
	imState := map[string]any{}
	if typedIDsNeeded {
		agentsState, err = readJSONMapIfExists(filepath.Join(m.root, "agents", "state.json"))
		if err != nil {
			return err
		}
		modelsState, err = readJSONMapIfExists(filepath.Join(m.root, "models.json"))
		if err != nil {
			return err
		}
		participantsState, err = readJSONMapIfExists(filepath.Join(m.root, "im", "participants.json"))
		if err != nil {
			return err
		}
		imState, err = readJSONMapIfExists(filepath.Join(m.root, "im", "state.json"))
		if err != nil {
			return err
		}

		if err := m.buildIDMaps(agentsState, participantsState, imState); err != nil {
			return err
		}
		if err := m.buildSessionIDMaps(imState); err != nil {
			return err
		}
		if err := m.buildTeamIDMaps(); err != nil {
			return err
		}
	}
	authState, err := m.migrateOpenCSGAuthSection(existingRootState)
	if err != nil {
		return err
	}
	state := rootState{
		Version:        1,
		ModelProviders: rootMapSection(existingRootState, "model_providers", map[string]any{"items": map[string]any{}}),
		Agents:         rootMapSection(existingRootState, "agents", map[string]any{"items": []any{}}),
		Participants:   rootMapSection(existingRootState, "participants", map[string]any{"items": []any{}}),
		Auth:           authState,
	}
	if len(modelsState) > 0 {
		state.ModelProviders = m.migrateModelProviders(modelsState)
	}
	if len(agentsState) > 0 {
		state.Agents = m.migrateAgents(agentsState)
	}
	if len(participantsState) > 0 {
		state.Participants = m.migrateParticipants(participantsState)
	}
	if err := writeJSONFile(filepath.Join(m.root, "state.json"), state); err != nil {
		return err
	}
	if typedIDsNeeded {
		if err := m.migrateIMState(imState); err != nil {
			return err
		}
		if err := m.migrateAgentDirs(); err != nil {
			return err
		}
		if err := m.migrateTeams(); err != nil {
			return err
		}
		if err := removeIfExists(filepath.Join(m.root, "agents", "state.json")); err != nil {
			return err
		}
		if err := removeIfExists(filepath.Join(m.root, "models.json")); err != nil {
			return err
		}
		if err := removeIfExists(filepath.Join(m.root, "im", "participants.json")); err != nil {
			return err
		}
	}
	if err := removeIfExists(filepath.Join(m.root, legacyRootAuthFileName)); err != nil {
		return err
	}
	if err := removeIfExists(filepath.Join(m.root, legacyAuthDirName, legacyCSGHubAuthFileName)); err != nil {
		return err
	}
	if err := migrateCLIProxyAuthDir(m.root); err != nil {
		return err
	}
	return nil
}

func (m *typedIDMigrator) migrateOpenCSGAuthSection(existingRootState map[string]any) (map[string]any, error) {
	authState := copyMap(stringAnyMap(existingRootState[rootAuthSectionName]))
	openCSG := copyMap(stringAnyMap(authState[rootOpenCSGAuthKey]))
	if csgHubState := stringAnyMap(authState[rootCSGHubAuthKey]); len(csgHubState) > 0 {
		mergeAIGatewayAPIKey(openCSG, csgHubState)
		delete(authState, rootCSGHubAuthKey)
	}
	legacyRootAuth, err := readJSONMapIfExists(filepath.Join(m.root, legacyRootAuthFileName))
	if err != nil {
		return nil, err
	}
	if len(legacyRootAuth) > 0 {
		mergeMissingValues(openCSG, legacyRootAuth)
	}
	legacyCSGHubAuth, err := readJSONMapIfExists(filepath.Join(m.root, legacyAuthDirName, legacyCSGHubAuthFileName))
	if err != nil {
		return nil, err
	}
	mergeAIGatewayAPIKey(openCSG, legacyCSGHubAuth)
	if len(openCSG) > 0 {
		authState[rootOpenCSGAuthKey] = openCSG
	}
	if len(authState) == 0 {
		return nil, nil
	}
	return authState, nil
}

func rootMapSection(state map[string]any, key string, fallback map[string]any) map[string]any {
	if section := stringAnyMap(state[key]); len(section) > 0 {
		return section
	}
	return fallback
}

func stringAnyMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]json.RawMessage:
		out := make(map[string]any, len(typed))
		for key, raw := range typed {
			var value any
			if err := json.Unmarshal(raw, &value); err == nil {
				out[key] = value
			}
		}
		return out
	default:
		return nil
	}
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		if nested := stringAnyMap(value); nested != nil {
			out[key] = copyMap(nested)
			continue
		}
		out[key] = value
	}
	return out
}

func mergeMissingValues(dst, src map[string]any) {
	if dst == nil || src == nil {
		return
	}
	for key, value := range src {
		if nestedSrc := stringAnyMap(value); len(nestedSrc) > 0 {
			if nestedDst := stringAnyMap(dst[key]); len(nestedDst) > 0 {
				mergeMissingValues(nestedDst, nestedSrc)
				dst[key] = nestedDst
				continue
			}
		}
		if missingValue(dst[key]) {
			dst[key] = value
		}
	}
}

func mergeAIGatewayAPIKey(openCSG, source map[string]any) {
	if openCSG == nil || source == nil {
		return
	}
	value := strings.TrimSpace(stringField(source, aiGatewayBuiltinAPIKeyField))
	if value == "" || !missingValue(openCSG[aiGatewayBuiltinAPIKeyField]) {
		return
	}
	openCSG[aiGatewayBuiltinAPIKeyField] = value
}

func missingValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}

func (m *typedIDMigrator) buildIDMaps(agentsState, participantsState, imState map[string]any) error {
	for _, item := range arrayOfMaps(agentsState["agents"]) {
		oldID := stringField(item, "id")
		if oldID == "" {
			continue
		}
		newID := m.agentID(oldID)
		m.agentNames[newID] = stringField(item, "name")
		if runtimeID := stringField(item, "runtime_id"); runtimeID != "" {
			m.ids.runtimes.mapRuntime(runtimeID, "rt-"+newID)
		}
	}
	for _, item := range arrayOfMaps(participantsState["participants"]) {
		if oldID := stringField(item, "id"); oldID != "" {
			m.participantID(oldID)
		}
		if oldID := stringField(item, "agent_id"); oldID != "" {
			m.agentID(oldID)
		}
		if oldID := stringField(item, "channel_user_ref"); oldID != "" {
			m.userID(oldID)
		}
	}
	for _, item := range arrayOfMaps(imState["users"]) {
		if oldID := stringField(item, "id"); oldID != "" {
			m.userID(oldID)
		}
	}
	if current := stringField(imState, "current_user_id"); current != "" {
		m.userID(current)
	}
	for _, room := range arrayOfMaps(imState["rooms"]) {
		if oldID := stringField(room, "id"); oldID != "" {
			m.roomID(oldID)
		}
		for _, member := range stringArray(room["members"]) {
			m.participantID(member)
		}
		for _, thread := range arrayOfMaps(room["threads"]) {
			if rootID := stringField(thread, "root_message_id"); rootID != "" {
				m.messageID(rootID)
			}
			for _, message := range arrayOfMaps(thread["context"]) {
				m.collectMessageIDs(message)
			}
		}
	}
	return nil
}

func (m *typedIDMigrator) buildSessionIDMaps(imState map[string]any) error {
	for _, room := range arrayOfMaps(imState["rooms"]) {
		oldRoomID := stringField(room, "id")
		rel := sessionPathForRoom(room, oldRoomID)
		if rel == "" {
			continue
		}
		path := filepath.Join(m.root, "im", filepath.FromSlash(rel))
		lines, err := readJSONLinesIfExists(path)
		if err != nil {
			return err
		}
		for _, line := range lines {
			m.collectMessageIDs(line)
		}
		if err := m.collectExistingThreadIDs(oldRoomID); err != nil {
			return err
		}
	}
	return nil
}

func (m *typedIDMigrator) collectExistingThreadIDs(oldRoomID string) error {
	oldRoomID = strings.TrimSpace(oldRoomID)
	if oldRoomID == "" {
		return nil
	}
	threadsDir := filepath.Join(m.root, "im", "threads", oldRoomID)
	entries, err := os.ReadDir(threadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		thread, err := readJSONMapIfExists(filepath.Join(threadsDir, entry.Name()))
		if err != nil {
			return err
		}
		rootID := firstNonEmpty(stringField(thread, "root_message_id"), strings.TrimSuffix(entry.Name(), ".json"))
		if rootID != "" {
			m.messageID(rootID)
		}
		for _, message := range arrayOfMaps(thread["context"]) {
			m.collectMessageIDs(message)
		}
	}
	return nil
}

func (m *typedIDMigrator) buildTeamIDMaps() error {
	teamsRoot := filepath.Join(m.root, "teams")
	entries, err := os.ReadDir(teamsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		teamDir := filepath.Join(teamsRoot, entry.Name())
		meta, err := readJSONMapIfExists(filepath.Join(teamDir, "team.json"))
		if err != nil {
			return err
		}
		if len(meta) == 0 {
			continue
		}
		if oldID := firstNonEmpty(stringField(meta, "id"), entry.Name()); oldID != "" {
			m.teamID(oldID)
		}
		if roomID := stringField(meta, "room_id"); roomID != "" {
			m.roomID(roomID)
		}
		if agentID := stringField(meta, "lead_agent_id"); agentID != "" {
			m.agentID(agentID)
		}
		if err := m.collectTaskIDs(filepath.Join(teamDir, "tasks.json")); err != nil {
			return err
		}
		if err := m.collectApprovalIDs(filepath.Join(teamDir, "approvals.json")); err != nil {
			return err
		}
		if err := m.collectTeamEventIDs(filepath.Join(teamDir, "events.jsonl")); err != nil {
			return err
		}
	}
	return nil
}

func (m *typedIDMigrator) collectTaskIDs(path string) error {
	items, err := readJSONArrayIfExists(path)
	if err != nil {
		return err
	}
	for _, item := range arrayOfMaps(items) {
		if oldID := stringField(item, "id"); oldID != "" {
			m.taskID(oldID)
		}
		if parentID := stringField(item, "parent_id"); parentID != "" {
			m.taskID(parentID)
		}
		for _, id := range stringArray(item["depends_on"]) {
			m.taskID(id)
		}
	}
	return nil
}

func (m *typedIDMigrator) collectApprovalIDs(path string) error {
	items, err := readJSONArrayIfExists(path)
	if err != nil {
		return err
	}
	for _, item := range arrayOfMaps(items) {
		if oldID := stringField(item, "id"); oldID != "" {
			m.approvalID(oldID)
		}
		if taskID := stringField(item, "task_id"); taskID != "" {
			m.taskID(taskID)
		}
	}
	return nil
}

func (m *typedIDMigrator) collectTeamEventIDs(path string) error {
	lines, err := readJSONLinesIfExists(path)
	if err != nil {
		return err
	}
	for _, item := range lines {
		if taskID := stringField(item, "task_id"); taskID != "" {
			m.taskID(taskID)
		}
	}
	return nil
}

func (m *typedIDMigrator) collectMessageIDs(item map[string]any) {
	if id := stringField(item, "id"); id != "" {
		m.messageID(id)
	}
	if senderID := stringField(item, "sender_id"); senderID != "" {
		m.participantID(senderID)
	}
	for _, mention := range arrayOfMaps(item["mentions"]) {
		if id := stringField(mention, "id"); id != "" {
			m.participantID(id)
		}
	}
	if relatesTo, ok := item["relates_to"].(map[string]any); ok {
		if eventID := stringField(relatesTo, "event_id"); eventID != "" {
			m.messageID(eventID)
		}
	}
	if event, ok := item["event"].(map[string]any); ok {
		if actorID := stringField(event, "actor_id"); actorID != "" {
			m.participantID(actorID)
		}
		for _, id := range stringArray(event["target_ids"]) {
			m.participantID(id)
		}
	}
}

func (m *typedIDMigrator) migrateModelProviders(modelsState map[string]any) map[string]any {
	providers, _ := modelsState["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	defaultModel := structuredModelRef(stringField(modelsState, "default"))
	if defaultModel.ModelProviderID == "" && len(providers) == 1 {
		for providerID, provider := range providers {
			defaultModel.ModelProviderID = providerID
			if providerMap, ok := provider.(map[string]any); ok {
				models := stringArray(providerMap["models"])
				if len(models) > 0 {
					defaultModel.ModelID = models[0]
				}
			}
		}
	}
	return map[string]any{
		"default_model": defaultModel,
		"items":         providers,
	}
}

func (m *typedIDMigrator) migrateAgents(agentsState map[string]any) map[string]any {
	runtimes := make(map[string]map[string]any)
	for _, rt := range arrayOfMaps(agentsState["runtimes"]) {
		id := stringField(rt, "id")
		if id == "" {
			continue
		}
		if newID, ok := m.ids.runtimes.lookup(id); ok {
			rt["id"] = newID
		}
		if agentIDs := stringArray(rt["agent_ids"]); len(agentIDs) > 0 {
			next := make([]string, 0, len(agentIDs))
			for _, id := range agentIDs {
				next = append(next, m.agentID(id))
			}
			rt["agent_ids"] = next
		}
		runtimes[id] = rt
	}
	items := make([]map[string]any, 0)
	for _, agent := range arrayOfMaps(agentsState["agents"]) {
		oldID := stringField(agent, "id")
		newID := m.agentID(oldID)
		agent["id"] = newID
		oldRuntimeID := stringField(agent, "runtime_id")
		if oldRuntimeID != "" {
			newRuntimeID, _ := m.ids.runtimes.lookup(oldRuntimeID)
			if newRuntimeID == "" {
				newRuntimeID = "rt-" + newID
			}
			agent["runtime_id"] = newRuntimeID
			runtime := map[string]any{"id": newRuntimeID}
			if oldRuntime, ok := runtimes[oldRuntimeID]; ok {
				for key, value := range oldRuntime {
					runtime[key] = value
				}
				runtime["id"] = newRuntimeID
			}
			if kind := stringField(agent, "runtime_kind"); kind != "" {
				runtime["kind"] = kind
			}
			runtime["agent_ids"] = []string{newID}
			agent["runtime"] = runtime
		}
		if profile, ok := agent["agent_profile"].(map[string]any); ok {
			agent["agent_profile"] = migrateProfile(profile)
		}
		items = append(items, agent)
	}
	state := map[string]any{
		"items": items,
	}
	if profileDefaults, ok := agentsState["profile_defaults"].(map[string]any); ok {
		state["profile_defaults"] = migrateProfile(profileDefaults)
	}
	if detection := agentsState["detection_results"]; detection != nil {
		state["detection_results"] = detection
	}
	return state
}

func (m *typedIDMigrator) migrateParticipants(participantsState map[string]any) map[string]any {
	participants := arrayOfMaps(participantsState["participants"])
	preferredCounts := make(map[string]int, len(participants))
	for _, participant := range participants {
		preferredCounts[m.participantID(stringField(participant, "id"))]++
	}
	items := make([]map[string]any, 0, len(participants))
	used := make(map[string]struct{}, len(participants))
	for _, participant := range participants {
		preferredID := m.participantID(stringField(participant, "id"))
		participant["id"] = uniqueParticipantRecordID(participant, preferredID, preferredCounts[preferredID], used)
		if agentID := stringField(participant, "agent_id"); agentID != "" {
			participant["agent_id"] = m.agentID(agentID)
		}
		if userID := stringField(participant, "channel_user_ref"); userID != "" {
			participant["channel_user_ref"] = m.userID(userID)
		}
		items = append(items, participant)
	}
	return map[string]any{"items": items}
}

func uniqueParticipantRecordID(participant map[string]any, preferredID string, preferredCount int, used map[string]struct{}) string {
	preferredID = strings.TrimSpace(preferredID)
	if preferredID == "" {
		return ""
	}
	channel := strings.ToLower(strings.TrimSpace(stringField(participant, "channel")))
	if preferredCount <= 1 || (channel == "" || channel == "csgclaw") {
		if _, exists := used[preferredID]; !exists {
			used[preferredID] = struct{}{}
			return preferredID
		}
	}
	source := strings.Join([]string{
		stringField(participant, "id"),
		stringField(participant, "channel"),
		stringField(participant, "type"),
		stringField(participant, "agent_id"),
		stringField(participant, "channel_user_ref"),
	}, "|")
	candidate := preferredID + "-" + shortHash(source)
	for suffix := 2; ; suffix++ {
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s-%s-%d", preferredID, shortHash(source), suffix)
	}
}

func (m *typedIDMigrator) migrateIMState(imState map[string]any) error {
	if len(imState) == 0 {
		return nil
	}
	imState["current_user_id"] = m.userID(stringField(imState, "current_user_id"))
	users := make([]map[string]any, 0)
	for _, user := range arrayOfMaps(imState["users"]) {
		user["id"] = m.userID(stringField(user, "id"))
		users = append(users, user)
	}
	imState["users"] = users

	rooms := make([]map[string]any, 0)
	for _, room := range arrayOfMaps(imState["rooms"]) {
		oldRoomID := stringField(room, "id")
		newRoomID := m.roomID(oldRoomID)
		oldSessionRel := sessionPathForRoom(room, oldRoomID)
		room["id"] = newRoomID
		members := stringArray(room["members"])
		nextMembers := make([]string, 0, len(members))
		for _, id := range members {
			nextMembers = append(nextMembers, m.participantID(id))
		}
		room["members"] = nextMembers
		if err := m.migrateSession(oldSessionRel, newRoomID); err != nil {
			return err
		}
		room["messages"] = filepath.ToSlash(filepath.Join("sessions", newRoomID+".jsonl"))
		threadRefs, err := m.extractThreads(oldRoomID, newRoomID, room)
		if err != nil {
			return err
		}
		if len(threadRefs) > 0 {
			room["threads"] = threadRefs
		} else {
			delete(room, "threads")
		}
		rooms = append(rooms, room)
	}
	imState["rooms"] = rooms
	return writeJSONFile(filepath.Join(m.root, "im", "state.json"), imState)
}

func (m *typedIDMigrator) migrateSession(oldRel, newRoomID string) error {
	if oldRel == "" {
		return nil
	}
	oldPath := filepath.Join(m.root, "im", filepath.FromSlash(oldRel))
	lines, err := readJSONLinesIfExists(oldPath)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	newPath := filepath.Join(m.root, "im", "sessions", newRoomID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(newPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	for _, line := range lines {
		m.rewriteMessage(line)
		if err := enc.Encode(line); err != nil {
			file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	if filepath.Clean(oldPath) != filepath.Clean(newPath) {
		if err := removeIfExists(oldPath); err != nil {
			return err
		}
	}
	return nil
}

func (m *typedIDMigrator) extractThreads(oldRoomID, newRoomID string, room map[string]any) ([]map[string]any, error) {
	refs := make([]map[string]any, 0)
	seen := make(map[string]struct{})
	for _, thread := range arrayOfMaps(room["threads"]) {
		if rootID := stringField(thread, "root_message_id"); rootID != "" {
			thread["root_message_id"] = m.messageID(rootID)
		}
		context := make([]map[string]any, 0)
		for _, message := range arrayOfMaps(thread["context"]) {
			m.rewriteMessage(message)
			context = append(context, message)
		}
		thread["context"] = context
		rootID := stringField(thread, "root_message_id")
		if rootID == "" {
			continue
		}
		path := filepath.Join(m.root, "im", "threads", newRoomID, rootID+".json")
		if err := writeJSONFile(path, thread); err != nil {
			return nil, err
		}
		if _, ok := seen[rootID]; !ok {
			refs = append(refs, threadRef(thread))
			seen[rootID] = struct{}{}
		}
	}
	existingRefs, err := m.migrateExistingThreads(oldRoomID, newRoomID, seen)
	if err != nil {
		return nil, err
	}
	refs = append(refs, existingRefs...)
	return refs, nil
}

func (m *typedIDMigrator) migrateExistingThreads(oldRoomID, newRoomID string, seen map[string]struct{}) ([]map[string]any, error) {
	oldRoomID = strings.TrimSpace(oldRoomID)
	if oldRoomID == "" {
		return nil, nil
	}
	oldDir := filepath.Join(m.root, "im", "threads", oldRoomID)
	entries, err := os.ReadDir(oldDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	refs := make([]map[string]any, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		oldPath := filepath.Join(oldDir, entry.Name())
		thread, err := readJSONMapIfExists(oldPath)
		if err != nil {
			return nil, err
		}
		oldRootID := firstNonEmpty(stringField(thread, "root_message_id"), strings.TrimSuffix(entry.Name(), ".json"))
		newRootID := m.messageID(oldRootID)
		if newRootID == "" {
			continue
		}
		thread["root_message_id"] = newRootID
		context := make([]map[string]any, 0)
		for _, message := range arrayOfMaps(thread["context"]) {
			m.rewriteMessage(message)
			context = append(context, message)
		}
		thread["context"] = context
		newPath := filepath.Join(m.root, "im", "threads", newRoomID, newRootID+".json")
		if err := writeJSONFile(newPath, thread); err != nil {
			return nil, err
		}
		if filepath.Clean(oldPath) != filepath.Clean(newPath) {
			if err := removeIfExists(oldPath); err != nil {
				return nil, err
			}
		}
		if _, ok := seen[newRootID]; !ok {
			refs = append(refs, threadRef(thread))
			seen[newRootID] = struct{}{}
		}
	}
	if filepath.Clean(oldDir) != filepath.Clean(filepath.Join(m.root, "im", "threads", newRoomID)) {
		if err := removeEmptyDirIfExists(oldDir); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func threadRef(thread map[string]any) map[string]any {
	ref := make(map[string]any)
	for _, key := range []string{"root_message_id", "created_at", "summary"} {
		if value, ok := thread[key]; ok {
			ref[key] = value
		}
	}
	return ref
}

func (m *typedIDMigrator) rewriteMessage(message map[string]any) {
	if id := stringField(message, "id"); id != "" {
		message["id"] = m.messageID(id)
	}
	if senderID := stringField(message, "sender_id"); senderID != "" {
		message["sender_id"] = m.participantID(senderID)
	}
	for _, mention := range arrayOfMaps(message["mentions"]) {
		if id := stringField(mention, "id"); id != "" {
			mention["id"] = m.participantID(id)
		}
	}
	if relatesTo, ok := message["relates_to"].(map[string]any); ok {
		if eventID := stringField(relatesTo, "event_id"); eventID != "" {
			relatesTo["event_id"] = m.messageID(eventID)
		}
	}
	if event, ok := message["event"].(map[string]any); ok {
		if actorID := stringField(event, "actor_id"); actorID != "" {
			event["actor_id"] = m.participantID(actorID)
		}
		targets := stringArray(event["target_ids"])
		if len(targets) > 0 {
			next := make([]string, 0, len(targets))
			for _, id := range targets {
				next = append(next, m.participantID(id))
			}
			event["target_ids"] = next
		}
	}
}

func (m *typedIDMigrator) migrateAgentDirs() error {
	agentsRoot := filepath.Join(m.root, "agents")
	entries, err := os.ReadDir(agentsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		oldDirName := entry.Name()
		newID := m.agentDirID(oldDirName)
		if newID == "" || newID == oldDirName {
			continue
		}
		if err := renameDirNoOverwrite(filepath.Join(agentsRoot, oldDirName), filepath.Join(agentsRoot, newID)); err != nil {
			return err
		}
	}
	return filepath.WalkDir(agentsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		var value any
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return nil
		}
		changed := m.rewriteRuntimeJSON(value)
		if !changed {
			return nil
		}
		return writeJSONFile(path, value)
	})
}

func (m *typedIDMigrator) agentDirID(dirName string) string {
	dirName = strings.TrimSpace(dirName)
	if dirName == "" {
		return ""
	}
	if id, ok := m.ids.agents.lookup(dirName); ok {
		return id
	}
	if id := m.agentIDForNameDir(dirName); id != "" {
		return id
	}
	if id, ok := m.ids.agents.lookup("u-" + dirName); ok {
		return id
	}
	return m.agentID(dirName)
}

func (m *typedIDMigrator) agentIDForNameDir(dirName string) string {
	candidates := legacyAgentDirNameCandidates(dirName)
	if len(candidates) == 0 {
		return ""
	}
	names := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		names[candidate] = struct{}{}
	}
	ids := make([]string, 0, len(m.agentNames))
	for agentID, name := range m.agentNames {
		if _, ok := names[strings.TrimSpace(name)]; ok {
			ids = append(ids, agentID)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func legacyAgentDirNameCandidates(dirName string) []string {
	dirName = strings.TrimSpace(dirName)
	if dirName == "" {
		return nil
	}
	var candidates []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range candidates {
			if existing == value {
				return
			}
		}
		candidates = append(candidates, value)
	}
	add(dirName)
	for _, prefix := range []string{"agent-", "u-agent-", "u-"} {
		if strings.HasPrefix(dirName, prefix) {
			add(strings.TrimPrefix(dirName, prefix))
		}
	}
	return candidates
}

// ReconcileTypedAgentDirs folds post-migration name-based agent directories into
// their canonical ID directories. It is intentionally narrow and idempotent so
// serve startup can repair stores that were migrated before this check existed.
func ReconcileTypedAgentDirs(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("root is required")
	}
	agentIDs, agentNames, err := readRootAgentDirIndex(root)
	if err != nil {
		return err
	}
	if len(agentIDs) == 0 {
		return nil
	}
	agentsRoot := filepath.Join(root, "agents")
	entries, err := os.ReadDir(agentsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		oldDirName := entry.Name()
		newID := canonicalAgentDirTarget(oldDirName, agentIDs, agentNames)
		if newID == "" || newID == oldDirName {
			continue
		}
		if err := renameDirNoOverwrite(filepath.Join(agentsRoot, oldDirName), filepath.Join(agentsRoot, newID)); err != nil {
			return err
		}
	}
	return nil
}

func readRootAgentDirIndex(root string) (map[string]struct{}, map[string][]string, error) {
	state, err := readJSONMapIfExists(filepath.Join(root, "state.json"))
	if err != nil {
		return nil, nil, err
	}
	agentsSection, _ := state["agents"].(map[string]any)
	items := arrayOfMaps(agentsSection["items"])
	agentIDs := make(map[string]struct{}, len(items))
	agentNames := make(map[string][]string)
	for _, item := range items {
		id := strings.TrimSpace(stringField(item, "id"))
		if id == "" {
			continue
		}
		agentIDs[id] = struct{}{}
		name := strings.TrimSpace(stringField(item, "name"))
		if name != "" {
			agentNames[name] = append(agentNames[name], id)
		}
	}
	for name := range agentNames {
		sort.Strings(agentNames[name])
	}
	return agentIDs, agentNames, nil
}

func canonicalAgentDirTarget(dirName string, agentIDs map[string]struct{}, agentNames map[string][]string) string {
	dirName = strings.TrimSpace(dirName)
	if dirName == "" {
		return ""
	}
	if _, ok := agentIDs[dirName]; ok {
		return dirName
	}
	for _, candidate := range legacyAgentDirNameCandidates(dirName) {
		if ids := agentNames[candidate]; len(ids) > 0 {
			return ids[0]
		}
	}
	return ""
}

func (m *typedIDMigrator) rewriteRuntimeJSON(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		changed := false
		for key, item := range v {
			switch key {
			case "agent_id":
				if s, ok := item.(string); ok {
					v[key] = m.agentID(s)
					changed = changed || v[key] != s
				}
			case "participant_id":
				if s, ok := item.(string); ok {
					v[key] = m.participantID(s)
					changed = changed || v[key] != s
				}
			case "user_id", "channel_user_ref":
				if s, ok := item.(string); ok {
					v[key] = m.userID(s)
					changed = changed || v[key] != s
				}
			case "runtime_id":
				if s, ok := item.(string); ok {
					if next, ok := m.ids.runtimes.lookup(s); ok {
						v[key] = next
						changed = true
					}
				}
			default:
				if m.rewriteRuntimeJSON(item) {
					changed = true
				}
			}
		}
		return changed
	case []any:
		changed := false
		for _, item := range v {
			if m.rewriteRuntimeJSON(item) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func (m *typedIDMigrator) migrateTeams() error {
	teamsRoot := filepath.Join(m.root, "teams")
	entries, err := os.ReadDir(teamsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		oldDir := filepath.Join(teamsRoot, entry.Name())
		meta, err := readJSONMapIfExists(filepath.Join(oldDir, "team.json"))
		if err != nil {
			return err
		}
		if len(meta) == 0 {
			continue
		}
		oldTeamID := firstNonEmpty(stringField(meta, "id"), entry.Name())
		newTeamID := m.teamID(oldTeamID)
		newDir := filepath.Join(teamsRoot, newTeamID)
		if err := renameDirNoOverwrite(oldDir, newDir); err != nil {
			return err
		}
		if err := m.migrateTeamDir(newDir, oldTeamID); err != nil {
			return err
		}
	}
	return m.writeTeamIndex()
}

func (m *typedIDMigrator) migrateTeamDir(teamDir, oldTeamID string) error {
	metaPath := filepath.Join(teamDir, "team.json")
	meta, err := readJSONMapIfExists(metaPath)
	if err != nil {
		return err
	}
	meta["id"] = m.teamID(oldTeamID)
	if roomID := stringField(meta, "room_id"); roomID != "" {
		meta["room_id"] = m.roomID(roomID)
	}
	if leadID := stringField(meta, "lead_agent_id"); leadID != "" {
		meta["lead_agent_id"] = m.agentID(leadID)
	}
	if err := writeJSONFile(metaPath, meta); err != nil {
		return err
	}
	if err := m.rewriteTeamTasks(filepath.Join(teamDir, "tasks.json")); err != nil {
		return err
	}
	if err := m.rewriteTeamApprovals(filepath.Join(teamDir, "approvals.json")); err != nil {
		return err
	}
	if err := m.rewriteTeamPresence(filepath.Join(teamDir, "presence.json")); err != nil {
		return err
	}
	return m.rewriteTeamEvents(filepath.Join(teamDir, "events.jsonl"))
}

func (m *typedIDMigrator) rewriteTeamTasks(path string) error {
	items, err := readJSONArrayIfExists(path)
	if err != nil || len(items) == 0 {
		return err
	}
	for _, item := range arrayOfMaps(items) {
		item["id"] = m.taskID(stringField(item, "id"))
		if teamID := stringField(item, "team_id"); teamID != "" {
			item["team_id"] = m.teamID(teamID)
		}
		if roomID := stringField(item, "room_id"); roomID != "" {
			item["room_id"] = m.roomID(roomID)
		}
		if parentID := stringField(item, "parent_id"); parentID != "" {
			item["parent_id"] = m.taskID(parentID)
		}
		if createdBy := stringField(item, "created_by"); createdBy != "" {
			item["created_by"] = m.participantID(createdBy)
		}
		if assignedTo := stringField(item, "assigned_to"); assignedTo != "" {
			item["assigned_to"] = m.participantID(assignedTo)
		}
		if claimedBy := stringField(item, "claimed_by"); claimedBy != "" {
			item["claimed_by"] = m.participantID(claimedBy)
		}
		deps := stringArray(item["depends_on"])
		if len(deps) > 0 {
			next := make([]string, 0, len(deps))
			for _, id := range deps {
				next = append(next, m.taskID(id))
			}
			item["depends_on"] = next
		}
	}
	return writeJSONFile(path, items)
}

func (m *typedIDMigrator) rewriteTeamApprovals(path string) error {
	items, err := readJSONArrayIfExists(path)
	if err != nil || len(items) == 0 {
		return err
	}
	for _, item := range arrayOfMaps(items) {
		item["id"] = m.approvalID(stringField(item, "id"))
		if teamID := stringField(item, "team_id"); teamID != "" {
			item["team_id"] = m.teamID(teamID)
		}
		if roomID := stringField(item, "room_id"); roomID != "" {
			item["room_id"] = m.roomID(roomID)
		}
		if taskID := stringField(item, "task_id"); taskID != "" {
			item["task_id"] = m.taskID(taskID)
		}
		if requestedBy := stringField(item, "requested_by"); requestedBy != "" {
			item["requested_by"] = m.participantID(requestedBy)
		}
		if approverID := stringField(item, "approver_id"); approverID != "" {
			item["approver_id"] = m.participantID(approverID)
		}
	}
	return writeJSONFile(path, items)
}

func (m *typedIDMigrator) rewriteTeamPresence(path string) error {
	items, err := readJSONArrayIfExists(path)
	if err != nil || len(items) == 0 {
		return err
	}
	for _, item := range arrayOfMaps(items) {
		if teamID := stringField(item, "team_id"); teamID != "" {
			item["team_id"] = m.teamID(teamID)
		}
		if participantID := stringField(item, "participant_id"); participantID != "" {
			item["participant_id"] = m.participantID(participantID)
		}
		if userID := stringField(item, "user_id"); userID != "" {
			item["user_id"] = m.userID(userID)
		}
		if agentID := stringField(item, "agent_id"); agentID != "" {
			item["agent_id"] = m.agentID(agentID)
		}
		if taskID := stringField(item, "current_task_id"); taskID != "" {
			item["current_task_id"] = m.taskID(taskID)
		}
	}
	return writeJSONFile(path, items)
}

func (m *typedIDMigrator) rewriteTeamEvents(path string) error {
	lines, err := readJSONLinesIfExists(path)
	if err != nil || len(lines) == 0 {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	for _, item := range lines {
		if teamID := stringField(item, "team_id"); teamID != "" {
			item["team_id"] = m.teamID(teamID)
		}
		if roomID := stringField(item, "room_id"); roomID != "" {
			item["room_id"] = m.roomID(roomID)
		}
		if actorID := stringField(item, "actor_id"); actorID != "" {
			item["actor_id"] = m.participantID(actorID)
		}
		if taskID := stringField(item, "task_id"); taskID != "" {
			item["task_id"] = m.taskID(taskID)
		}
		if targetID := stringField(item, "target_id"); targetID != "" {
			item["target_id"] = m.participantID(targetID)
		}
		if err := enc.Encode(item); err != nil {
			file.Close()
			return err
		}
	}
	return file.Close()
}

func (m *typedIDMigrator) writeTeamIndex() error {
	teamsRoot := filepath.Join(m.root, "teams")
	entries, err := os.ReadDir(teamsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	index := make([]map[string]any, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := readJSONMapIfExists(filepath.Join(teamsRoot, entry.Name(), "team.json"))
		if err != nil {
			return err
		}
		if len(meta) == 0 {
			continue
		}
		index = append(index, map[string]any{
			"id":            stringField(meta, "id"),
			"channel":       stringField(meta, "channel"),
			"room_id":       stringField(meta, "room_id"),
			"title":         stringField(meta, "title"),
			"lead_agent_id": stringField(meta, "lead_agent_id"),
			"status":        stringField(meta, "status"),
		})
	}
	sort.Slice(index, func(i, j int) bool {
		return stringField(index[i], "id") < stringField(index[j], "id")
	})
	if len(index) == 0 {
		return nil
	}
	return writeJSONFile(filepath.Join(teamsRoot, "index.json"), index)
}

func (m *typedIDMigrator) agentID(old string) string {
	preferred := reservedAgentID(old)
	if preferred == "" {
		preferred = typedLocalIdentityID("agent", old)
	}
	return m.ids.agents.mapID(old, preferred)
}

func (m *typedIDMigrator) participantID(old string) string {
	preferred := reservedParticipantID(old)
	if preferred == "" {
		preferred = typedLocalIdentityID("pt", old)
	}
	return m.ids.participants.mapID(old, preferred)
}

func (m *typedIDMigrator) userID(old string) string {
	preferred := reservedUserID(old)
	if preferred == "" {
		preferred = typedLocalIdentityID("user", old)
	}
	return m.ids.users.mapID(old, preferred)
}

func (m *typedIDMigrator) roomID(old string) string {
	return m.ids.rooms.mapID(old, "")
}

func (m *typedIDMigrator) teamID(old string) string {
	return m.ids.teams.mapID(old, "")
}

func (m *typedIDMigrator) messageID(old string) string {
	return m.ids.messages.mapID(old, "")
}

func (m *typedIDMigrator) taskID(old string) string {
	return m.ids.tasks.mapID(old, "")
}

func (m *typedIDMigrator) approvalID(old string) string {
	return m.ids.approvals.mapID(old, "")
}

func (t *idTable) lookup(old string) (string, bool) {
	old = strings.TrimSpace(old)
	v, ok := t.ids[old]
	return v, ok
}

func (t *idTable) mapRuntime(old, forced string) {
	old = strings.TrimSpace(old)
	forced = strings.TrimSpace(forced)
	if old == "" || forced == "" {
		return
	}
	t.ids[old] = forced
	t.reverse[forced] = old
}

func (t *idTable) mapID(old, preferred string) string {
	old = strings.TrimSpace(old)
	if old == "" {
		return ""
	}
	if next, ok := t.ids[old]; ok {
		return next
	}
	candidate := strings.TrimSpace(preferred)
	if candidate == "" {
		candidate = typedID(t.prefix, old)
	}
	if prior, exists := t.reverse[candidate]; exists && prior != old {
		if localIdentityAlias(t.prefix, prior, old) {
			t.ids[old] = candidate
			return candidate
		}
		candidate = candidate + "-" + shortHash(old)
	}
	t.ids[old] = candidate
	t.reverse[candidate] = old
	return candidate
}

func typedLocalIdentityID(prefix, old string) string {
	old = strings.TrimSpace(old)
	if old == "" {
		return ""
	}
	base := canonicalLocalIdentityBase(old)
	if base == "" {
		base = old
	}
	slug := slugify(base)
	if slug == "" {
		slug = "x" + shortHash(old)
	}
	return prefix + "-" + slug
}

func typedID(prefix, old string) string {
	old = strings.TrimSpace(old)
	if strings.HasPrefix(old, prefix+"-") {
		return old
	}
	base := old
	if strings.HasPrefix(base, "u-") {
		base = strings.TrimPrefix(base, "u-")
	}
	slug := slugify(base)
	if slug == "" {
		slug = "x" + shortHash(old)
	}
	return prefix + "-" + slug
}

func localIdentityAlias(prefix, a, b string) bool {
	switch prefix {
	case "agent", "pt", "user":
	default:
		return false
	}
	return localIdentityBase(a) != "" && localIdentityBase(a) == localIdentityBase(b)
}

func localIdentityBase(value string) string {
	return strings.ToLower(canonicalLocalIdentityBase(value))
}

func canonicalLocalIdentityBase(value string) string {
	value = strings.TrimSpace(value)
	for {
		next := trimLocalIdentityPrefixes(value)
		if trimmed, ok := trimStableHashSuffix(next); ok {
			next = trimLocalIdentityPrefixes(trimmed)
		}
		if next == value {
			break
		}
		value = next
	}
	return strings.TrimSpace(value)
}

func trimLocalIdentityPrefixes(value string) string {
	value = strings.TrimSpace(value)
	for {
		next := value
		for _, prefix := range []string{"user-", "agent-", "pt-", "u-"} {
			if strings.HasPrefix(next, prefix) {
				next = strings.TrimPrefix(next, prefix)
				break
			}
		}
		if next == value {
			break
		}
		value = next
	}
	return strings.TrimSpace(value)
}

func trimStableHashSuffix(value string) (string, bool) {
	value = strings.TrimSpace(value)
	idx := strings.LastIndex(value, "-")
	if idx <= 0 || len(value)-idx-1 != 8 {
		return "", false
	}
	for _, r := range value[idx+1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return "", false
		}
	}
	return value[:idx], true
}

func stripKnownPrefix(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"u-", "agent-", "pt-", "user-", "room-", "team-", "msg-", "task-", "approval-"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = slugCharPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return value
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func reservedAgentID(old string) string {
	switch stripKnownPrefix(strings.ToLower(strings.TrimSpace(old))) {
	case "manager":
		return "agent-manager"
	default:
		return ""
	}
}

func reservedParticipantID(old string) string {
	switch stripKnownPrefix(strings.ToLower(strings.TrimSpace(old))) {
	case "admin":
		return "pt-admin"
	case "manager":
		return "pt-manager"
	default:
		return ""
	}
}

func reservedUserID(old string) string {
	switch stripKnownPrefix(strings.ToLower(strings.TrimSpace(old))) {
	case "admin":
		return "user-admin"
	case "manager":
		return "user-manager"
	default:
		return ""
	}
}

func migrateProfile(profile map[string]any) map[string]any {
	provider := strings.TrimSpace(stringField(profile, "model_provider_id"))
	if provider == "" {
		provider = providerToModelProviderID(stringField(profile, "provider"))
	}
	if provider != "" {
		profile["model_provider_id"] = provider
	}
	delete(profile, "provider")
	return profile
}

func providerToModelProviderID(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "api", "llm-api", "openai", "openai_compatible":
		return "openai"
	case "csghub_lite", "csghub-lite":
		return "csghub-lite"
	case "csghub":
		return "csghub"
	case "codex":
		return "codex"
	case "claude_code", "claude-code":
		return "claude"
	default:
		return strings.TrimSpace(provider)
	}
}

func structuredModelRef(selector string) modelRef {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return modelRef{}
	}
	best := -1
	for _, sep := range []string{".", ":", "/"} {
		if idx := strings.Index(selector, sep); idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	if best >= 0 {
		provider := strings.TrimSpace(selector[:best])
		modelID := strings.TrimSpace(selector[best+1:])
		if provider != "" && modelID != "" {
			return modelRef{ModelProviderID: provider, ModelID: modelID}
		}
	}
	return modelRef{ModelProviderID: selector}
}

func nextBackupPath(root string, now time.Time) (string, error) {
	date := now.Format("20060102")
	parent := filepath.Dir(root)
	base := filepath.Base(root)
	for i := 1; i < 1000; i++ {
		path := filepath.Join(parent, fmt.Sprintf("%s_backup_%s_%03d", base, date, i))
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no backup index available for %s", date)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func readJSONMapIfExists(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func readJSONArrayIfExists(path string) ([]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return out, nil
}

func readJSONLinesIfExists(path string) ([]map[string]any, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []map[string]any
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		out = append(out, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeEmptyDirIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func migrateCLIProxyAuthDir(root string) error {
	src := filepath.Join(root, legacyAuthDirName)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := removeIfExists(filepath.Join(src, legacyCSGHubAuthFileName)); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return removeEmptyDirIfExists(src)
	}
	dst := filepath.Join(root, cliproxyAuthDirName)
	return moveDirContentsWithStableCollisions(src, dst)
}

func moveDirContentsWithStableCollisions(src, dst string) error {
	entries, err := os.ReadDir(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := moveDirEntryWithStableCollisions(from, to); err != nil {
				return err
			}
			continue
		}
		if err := moveFileWithStableCollision(from, to); err != nil {
			return err
		}
	}
	return os.Remove(src)
}

func moveDirEntryWithStableCollisions(src, dst string) error {
	info, err := os.Stat(dst)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.Rename(src, dst)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		candidate, err := collisionPathForFile(src, dst)
		if err != nil {
			return err
		}
		return os.Rename(src, candidate)
	}
	return moveDirContentsWithStableCollisions(src, dst)
}

func moveFileWithStableCollision(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(dst); errors.Is(err, os.ErrNotExist) {
		return os.Rename(src, dst)
	} else if err != nil {
		return err
	} else if info.IsDir() {
		candidate, err := collisionPathForFile(src, dst)
		if err != nil {
			return err
		}
		return os.Rename(src, candidate)
	}
	same, err := sameFileContent(src, dst)
	if err != nil {
		return err
	}
	if same {
		return os.Remove(src)
	}
	candidate, err := collisionPathForFile(src, dst)
	if err != nil {
		return err
	}
	for {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return os.Rename(src, candidate)
		} else if err != nil {
			return err
		}
		same, err := sameFileContent(src, candidate)
		if err != nil {
			return err
		}
		if same {
			return os.Remove(src)
		}
		candidate = nextCollisionPath(candidate)
	}
}

func collisionPathForFile(src, dst string) (string, error) {
	hash, err := shortFileHash(src)
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(dst)
	base := strings.TrimSuffix(filepath.Base(dst), ext)
	return filepath.Join(filepath.Dir(dst), fmt.Sprintf("%s.legacy-%s%s", base, hash, ext)), nil
}

func nextCollisionPath(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func shortFileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])[:8], nil
}

func sameFileContent(a, b string) (bool, error) {
	left, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	right, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(left, right), nil
}

func renameDirNoOverwrite(oldPath, newPath string) error {
	if filepath.Clean(oldPath) == filepath.Clean(newPath) {
		return nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return mergeDirs(oldPath, newPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func mergeDirs(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := mergeDirs(from, to); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Stat(to); errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(from, to); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := os.Remove(from); err != nil {
			return err
		}
	}
	return os.Remove(src)
}

func arrayOfMaps(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]any); ok {
			out = append(out, mapped)
		}
	}
	return out
}

func stringArray(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func stringField(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	value, _ := item[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sessionPathForRoom(room map[string]any, oldRoomID string) string {
	if path := stringField(room, "messages"); path != "" {
		return path
	}
	if oldRoomID == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join("sessions", oldRoomID+".jsonl"))
}
