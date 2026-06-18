package im

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/slashcommand"
)

type User = apitypes.User

type Message = apitypes.Message

type Mention = apitypes.Mention

type EventPayload = apitypes.EventPayload

type MessageRelation = apitypes.MessageRelation

type ThreadSummary = apitypes.ThreadSummary

type ThreadContextSummary = apitypes.ThreadContextSummary

type ThreadState = apitypes.ThreadState

type ThreadView = apitypes.ThreadView

type ThreadListResponse = apitypes.ThreadListResponse

type ThreadRelationsResponse = apitypes.ThreadRelationsResponse

type Room = apitypes.Room

type Conversation = Room

type Bootstrap struct {
	CurrentUserID      string   `json:"current_user_id"`
	Users              []User   `json:"users"`
	Rooms              []Room   `json:"rooms,omitempty"`
	InviteDraftUserIDs []string `json:"invite_draft_user_ids,omitempty"`
}

type CreateMessageRequest = apitypes.CreateMessageRequest

type StartThreadRequest = apitypes.StartThreadRequest

const (
	RelationTypeThread = "m.thread"
)

const maxThreadContextSnapshotBytes = 32 * 1024

const (
	ThreadListIncludeAll          = "all"
	ThreadListIncludeParticipated = "participated"
)

type ListMessagesOptions struct {
	IncludeThreadReplies bool
	Locale               string
}

type ThreadListOptions struct {
	Include string
	Limit   int
	From    int
}

type DeliverMessageRequest struct {
	RoomID       string `json:"room_id"`
	SenderID     string `json:"sender_id,omitempty"`
	MentionID    string `json:"mention_id,omitempty"`
	Content      string `json:"text"`
	MessageID    string `json:"message_id,omitempty"`
	ThreadRootID string `json:"thread_root_id,omitempty"`
}

type DeliverEventRequest struct {
	RoomID    string        `json:"room_id"`
	SenderID  string        `json:"sender_id,omitempty"`
	MentionID string        `json:"mention_id,omitempty"`
	Content   string        `json:"text"`
	MessageID string        `json:"message_id,omitempty"`
	Event     *EventPayload `json:"event,omitempty"`
}

type CreateRoomRequest = apitypes.CreateRoomRequest

type CreateConversationRequest = CreateRoomRequest

type AddRoomMembersRequest = apitypes.AddRoomMembersRequest

type AddConversationMembersRequest = AddRoomMembersRequest

type EnsureAgentUserRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Handle      string `json:"handle"`
	Role        string `json:"role"`
	Avatar      string `json:"avatar,omitempty"`
}

type EnsureWorkerUserRequest = EnsureAgentUserRequest

type UpdateAgentUserRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Role        string `json:"role,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

type UpdateUserRequest struct {
	ID          string
	Name        *string
	Description *string
	Handle      *string
	Role        *string
	Avatar      *string
}

type AddAgentToConversationRequest struct {
	AgentID   string `json:"agent_id"`
	RoomID    string `json:"room_id,omitempty"`
	InviterID string `json:"inviter_id"`
	Locale    string `json:"locale"`
}

type Service struct {
	mu            sync.RWMutex
	bus           *Bus
	statePath     string
	currentUserID string
	users         map[string]User
	byHandle      map[string]string
	rooms         map[string]*Room
}

var mentionPattern = regexp.MustCompile(`(^|[^\w])@([a-zA-Z0-9._-]+)`)
var mentionTagPattern = regexp.MustCompile(`<at\s+user_id="([^"]+)">[^<]*</at>`)

func MentionTagUserIDs(content string) []string {
	if content == "" {
		return nil
	}
	ids := make([]string, 0, 1)
	for _, match := range mentionTagPattern.FindAllStringSubmatch(content, -1) {
		if len(match) <= 1 {
			continue
		}
		id := strings.TrimSpace(match[1])
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func HasMentionTagForUser(content, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	for _, id := range MentionTagUserIDs(content) {
		if id == userID {
			return true
		}
	}
	return false
}

const (
	sessionsDirName          = "sessions"
	AdminUserID              = "admin"
	DefaultAdminDescription  = "Human operator. Agents can @admin to ask clarifying questions, request confirmation, and double-check important decisions before continuing."
	adminUserID              = AdminUserID
	legacyAdminUserID        = "u-admin"
	managerParticipantUserID = "manager"
	legacyManagerUserID      = "u-manager"
)

type persistedBootstrap struct {
	CurrentUserID      string          `json:"current_user_id"`
	Users              []User          `json:"users"`
	Rooms              []persistedRoom `json:"rooms,omitempty"`
	InviteDraftUserIDs []string        `json:"invite_draft_user_ids,omitempty"`
}

type persistedRoom struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Subtitle    string        `json:"subtitle"`
	Description string        `json:"description,omitempty"`
	IsDirect    bool          `json:"is_direct,omitempty"`
	Members     []string      `json:"members"`
	Messages    string        `json:"messages"`
	Threads     []ThreadState `json:"threads,omitempty"`
}

func (r *persistedRoom) UnmarshalJSON(data []byte) error {
	type persistedRoomAlias persistedRoom
	type persistedRoomJSON struct {
		persistedRoomAlias
		Participants []string `json:"participants"`
	}

	var decoded persistedRoomJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = persistedRoom(decoded.persistedRoomAlias)
	if len(r.Members) == 0 && len(decoded.Participants) > 0 {
		r.Members = append([]string(nil), decoded.Participants...)
	}
	return nil
}

func NewService() *Service {
	return NewServiceFromBootstrap(DefaultBootstrap())
}

func NewServiceWithBus(bus *Bus) *Service {
	return NewServiceFromBootstrapWithBus(DefaultBootstrap(), bus)
}

func NewServiceFromPath(path string) (*Service, error) {
	return NewServiceFromPathWithBus(path, nil)
}

func NewServiceFromPathWithBus(path string, bus *Bus) (*Service, error) {
	state, err := LoadBootstrap(path)
	if err != nil {
		return nil, err
	}
	svc := NewServiceFromBootstrapWithBus(state, bus)
	svc.statePath = path
	return svc, nil
}

func NewServiceFromBootstrap(state Bootstrap) *Service {
	return NewServiceFromBootstrapWithBus(state, nil)
}

func NewServiceFromBootstrapWithBus(state Bootstrap, bus *Bus) *Service {
	svc := &Service{
		bus: bus,
	}
	svc.replaceState(normalizeBootstrap(state))
	return svc
}

func DefaultBootstrap() Bootstrap {
	return Bootstrap{
		CurrentUserID: adminUserID,
		Users:         nil,
		Rooms:         nil,
	}
}

func LoadBootstrap(path string) (Bootstrap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultBootstrap(), nil
		}
		return Bootstrap{}, fmt.Errorf("read im bootstrap: %w", err)
	}

	var persisted persistedBootstrap
	if err := json.Unmarshal(data, &persisted); err != nil {
		return Bootstrap{}, fmt.Errorf("decode im bootstrap: %w", err)
	}
	state, err := loadPersistedBootstrap(path, persisted)
	if err != nil {
		return Bootstrap{}, err
	}
	return normalizeBootstrap(state), nil
}

func (s *Service) Reload() error {
	if s == nil || strings.TrimSpace(s.statePath) == "" {
		return nil
	}

	state, err := LoadBootstrap(s.statePath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceStateLocked(state)
	return nil
}

func SaveBootstrap(path string, state Bootstrap) error {
	state = normalizeBootstrap(state)

	if err := ensureBootstrapDirs(path); err != nil {
		return err
	}

	persisted, err := savePersistedBootstrap(path, state)
	if err != nil {
		return err
	}
	return writePersistedBootstrap(path, persisted)
}

func ensureBootstrapDirs(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create im state dir: %w", err)
	}

	sessionsDir := filepath.Join(filepath.Dir(path), sessionsDirName)
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return fmt.Errorf("create im sessions dir: %w", err)
	}
	return nil
}

func writePersistedBootstrap(path string, persisted persistedBootstrap) error {
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode im bootstrap: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write im bootstrap: %w", err)
	}
	return nil
}

func loadPersistedBootstrap(path string, persisted persistedBootstrap) (Bootstrap, error) {
	state := Bootstrap{
		CurrentUserID:      persisted.CurrentUserID,
		Users:              append([]User(nil), persisted.Users...),
		InviteDraftUserIDs: append([]string(nil), persisted.InviteDraftUserIDs...),
	}

	rooms, err := loadPersistedRooms(path, persisted.Rooms)
	if err != nil {
		return Bootstrap{}, err
	}
	state.Rooms = cloneRooms(rooms)
	return state, nil
}

func loadPersistedRooms(statePath string, rooms []persistedRoom) ([]Room, error) {
	if len(rooms) == 0 {
		return nil, nil
	}

	loaded := make([]Room, 0, len(rooms))
	for _, room := range rooms {
		messages, err := loadRoomMessages(statePath, room.ID, room.Messages)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, Room{
			ID:          room.ID,
			Title:       room.Title,
			Subtitle:    room.Subtitle,
			Description: room.Description,
			IsDirect:    room.IsDirect,
			Members:     append([]string(nil), room.Members...),
			Messages:    messages,
			Threads:     cloneThreadStates(room.Threads),
		})
	}
	return loaded, nil
}

func loadRoomMessages(statePath, roomID, relativePath string) ([]Message, error) {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return nil, nil
	}
	if filepath.Ext(relativePath) != ".jsonl" {
		return nil, fmt.Errorf("decode room %s messages: expected jsonl session path", roomID)
	}
	sessionPath := filepath.Join(filepath.Dir(statePath), filepath.FromSlash(relativePath))
	return loadMessagesJSONL(sessionPath, roomID)
}

func savePersistedBootstrap(statePath string, state Bootstrap) (persistedBootstrap, error) {
	persisted := persistedBootstrapFromState(state)

	rooms, err := savePersistedRooms(statePath, state.Rooms)
	if err != nil {
		return persistedBootstrap{}, err
	}
	persisted.Rooms = rooms

	if err := cleanupSessionFiles(statePath, rooms); err != nil {
		return persistedBootstrap{}, err
	}
	return persisted, nil
}

func persistedBootstrapFromState(state Bootstrap) persistedBootstrap {
	persisted := persistedBootstrap{
		CurrentUserID:      state.CurrentUserID,
		Users:              append([]User(nil), state.Users...),
		InviteDraftUserIDs: append([]string(nil), state.InviteDraftUserIDs...),
	}
	if len(state.Rooms) > 0 {
		persisted.Rooms = make([]persistedRoom, 0, len(state.Rooms))
		for _, room := range state.Rooms {
			persisted.Rooms = append(persisted.Rooms, persistedRoomFromRoom(room))
		}
	}
	return persisted
}

func savePersistedRooms(statePath string, rooms []Room) ([]persistedRoom, error) {
	if len(rooms) == 0 {
		return nil, nil
	}

	persisted := make([]persistedRoom, 0, len(rooms))
	for _, room := range rooms {
		if err := saveRoomMessagesForState(statePath, room); err != nil {
			return nil, err
		}
		persisted = append(persisted, persistedRoomFromRoom(room))
	}
	return persisted, nil
}

func saveRoomMessagesForState(statePath string, room Room) error {
	relativePath := sessionRelativePath(room.ID)
	return saveMessagesJSONL(filepath.Join(filepath.Dir(statePath), filepath.FromSlash(relativePath)), room.ID, room.Messages)
}

func persistedRoomFromRoom(room Room) persistedRoom {
	return persistedRoom{
		ID:          room.ID,
		Title:       room.Title,
		Subtitle:    room.Subtitle,
		Description: room.Description,
		IsDirect:    room.IsDirect,
		Members:     append([]string(nil), room.Members...),
		Messages:    sessionRelativePath(room.ID),
		Threads:     cloneThreadStates(room.Threads),
	}
}

func cleanupSessionFiles(statePath string, rooms []persistedRoom) error {
	sessionsDir := filepath.Join(filepath.Dir(statePath), sessionsDirName)
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read im sessions dir: %w", err)
	}

	keep := make(map[string]struct{}, len(rooms))
	for _, room := range rooms {
		keep[filepath.Base(sessionRelativePath(room.ID))] = struct{}{}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		roomID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if err := os.Remove(filepath.Join(sessionsDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale im session: %w", err)
		}
		if err := removeRoomSessionBlobs(sessionsDir, roomID); err != nil {
			return err
		}
	}
	return cleanupStaleSessionBlobRooms(sessionsDir, rooms)
}

func cleanupStaleSessionBlobRooms(sessionsDir string, rooms []persistedRoom) error {
	blobsRoot := filepath.Join(sessionsDir, sessionBlobsDirName)
	entries, err := os.ReadDir(blobsRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read im session blobs dir: %w", err)
	}

	keepRooms := make(map[string]struct{}, len(rooms))
	for _, room := range rooms {
		keepRooms[room.ID] = struct{}{}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := keepRooms[entry.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(blobsRoot, entry.Name())); err != nil {
			return fmt.Errorf("remove stale im session blobs dir: %w", err)
		}
	}
	return nil
}

func sessionRelativePath(roomID string) string {
	return filepath.ToSlash(filepath.Join(sessionsDirName, roomID+".jsonl"))
}

func EnsureBootstrapState(path string) error {
	state, err := LoadBootstrap(path)
	if err != nil {
		return err
	}
	state = normalizeBootstrap(state)
	state.Rooms = ensureAdminManagerRoom(state.Rooms)
	state.InviteDraftUserIDs = nil
	return SaveBootstrap(path, state)
}

func normalizeBootstrap(state Bootstrap) Bootstrap {
	if state.CurrentUserID == "" {
		state.CurrentUserID = DefaultBootstrap().CurrentUserID
	}
	adminAliases := adminUserAliases(state.Users)
	managerAliases := managerUserAliases(state.Users)
	state.Users = ensureUsers(state.Users)
	state.Rooms = migrateLegacyAdminRoomRefs(cloneRooms(state.Rooms), adminAliases)
	state.Rooms = migrateLegacyManagerRoomRefs(state.Rooms, managerAliases)
	if !containsUserID(state.Users, state.CurrentUserID) {
		state.CurrentUserID = migrateLegacyAdminID(state.CurrentUserID, adminAliases)
		state.CurrentUserID = migrateLegacyManagerID(state.CurrentUserID, managerAliases)
		if !containsUserID(state.Users, state.CurrentUserID) {
			state.CurrentUserID = defaultCurrentUserID(state.Users)
		}
	}
	return state
}

func ensureUsers(users []User) []User {
	result := append([]User(nil), users...)
	for i := range result {
		result[i] = normalizeUser(result[i])
	}
	if !hasUserHandle(result, "admin") {
		result = append(result, User{
			ID:          adminUserID,
			Name:        "admin",
			Description: DefaultAdminDescription,
			Handle:      "admin",
			Role:        "admin",
			Avatar:      "AD",
			IsOnline:    true,
			AccentHex:   "#dc2626",
		})
	} else {
		for i := range result {
			if strings.EqualFold(strings.TrimSpace(result[i].Handle), "admin") {
				result[i].ID = adminUserID
				result[i].Name = "admin"
				result[i].Role = "admin"
				if strings.TrimSpace(result[i].Description) == "" {
					result[i].Description = DefaultAdminDescription
				}
			}
		}
	}
	if !hasUserHandle(result, "manager") {
		result = append(result, User{
			ID:        managerParticipantUserID,
			Name:      "manager",
			Handle:    "manager",
			Role:      "manager",
			Avatar:    "MG",
			IsOnline:  true,
			AccentHex: "#0f766e",
		})
	} else {
		for i := range result {
			if strings.EqualFold(strings.TrimSpace(result[i].Handle), "manager") {
				result[i].ID = managerParticipantUserID
				result[i].Name = "manager"
				result[i].Role = "manager"
			}
		}
	}
	result = dropLegacyAdminUserDuplicates(result)
	result = dropLegacyManagerUserDuplicates(result)
	return result
}

func dropLegacyAdminUserDuplicates(users []User) []User {
	out := make([]User, 0, len(users))
	seen := make(map[string]struct{}, len(users))
	for _, user := range users {
		id := strings.TrimSpace(user.ID)
		if id == "" || id == legacyAdminUserID {
			if strings.EqualFold(strings.TrimSpace(user.Handle), "admin") ||
				strings.EqualFold(strings.TrimSpace(user.Name), "admin") ||
				strings.EqualFold(strings.TrimSpace(user.Role), "admin") {
				id = adminUserID
				user.ID = adminUserID
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, user)
	}
	return out
}

func dropLegacyManagerUserDuplicates(users []User) []User {
	out := make([]User, 0, len(users))
	seen := make(map[string]struct{}, len(users))
	for _, user := range users {
		id := strings.TrimSpace(user.ID)
		if id == "" || id == legacyManagerUserID {
			if strings.EqualFold(strings.TrimSpace(user.Handle), "manager") ||
				strings.EqualFold(strings.TrimSpace(user.Name), "manager") ||
				strings.EqualFold(strings.TrimSpace(user.Role), "manager") {
				id = managerParticipantUserID
				user.ID = managerParticipantUserID
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, user)
	}
	return out
}

func normalizeUser(user User) User {
	user.Name = strings.ToLower(strings.TrimSpace(user.Name))
	user.Description = strings.TrimSpace(user.Description)
	user.Handle = strings.ToLower(strings.TrimSpace(user.Handle))
	user.Role = strings.ToLower(strings.TrimSpace(user.Role))
	user.Participants = nil
	return user
}

func hasUserHandle(users []User, handle string) bool {
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.Handle), handle) {
			return true
		}
	}
	return false
}

func containsUserID(users []User, userID string) bool {
	for _, user := range users {
		if user.ID == userID {
			return true
		}
	}
	return false
}

func defaultCurrentUserID(users []User) string {
	for _, preferred := range []string{adminUserID, managerParticipantUserID} {
		if containsUserID(users, preferred) {
			return preferred
		}
	}
	if len(users) > 0 {
		return users[0].ID
	}
	return ""
}

func adminUserAliases(users []User) map[string]struct{} {
	aliases := map[string]struct{}{
		legacyAdminUserID: {},
		adminUserID:       {},
	}
	for _, user := range users {
		id := strings.TrimSpace(user.ID)
		if id == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(user.Handle), "admin") ||
			strings.EqualFold(strings.TrimSpace(user.Name), "admin") ||
			strings.EqualFold(strings.TrimSpace(user.Role), "admin") {
			aliases[id] = struct{}{}
		}
	}
	return aliases
}

func managerUserAliases(users []User) map[string]struct{} {
	aliases := map[string]struct{}{
		legacyManagerUserID:      {},
		managerParticipantUserID: {},
	}
	for _, user := range users {
		id := strings.TrimSpace(user.ID)
		if id == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(user.Handle), "manager") ||
			strings.EqualFold(strings.TrimSpace(user.Name), "manager") ||
			strings.EqualFold(strings.TrimSpace(user.Role), "manager") {
			aliases[id] = struct{}{}
		}
	}
	return aliases
}

func cloneRooms(rooms []Room) []Room {
	cloned := make([]Room, 0, len(rooms))
	for _, room := range rooms {
		cloned = append(cloned, cloneRoom(room))
	}
	return cloned
}

func migrateLegacyAdminRoomRefs(rooms []Room, adminAliases map[string]struct{}) []Room {
	for i := range rooms {
		rooms[i].Members = migrateLegacyAdminIDs(rooms[i].Members, adminAliases)
		for j := range rooms[i].Messages {
			rooms[i].Messages[j].SenderID = migrateLegacyAdminID(rooms[i].Messages[j].SenderID, adminAliases)
			rooms[i].Messages[j].Content = migrateLegacyAdminMentionTags(rooms[i].Messages[j].Content, adminAliases)
			if rooms[i].Messages[j].Event != nil {
				rooms[i].Messages[j].Event.ActorID = migrateLegacyAdminID(rooms[i].Messages[j].Event.ActorID, adminAliases)
				rooms[i].Messages[j].Event.TargetIDs = migrateLegacyAdminIDs(rooms[i].Messages[j].Event.TargetIDs, adminAliases)
			}
			for k := range rooms[i].Messages[j].Mentions {
				rooms[i].Messages[j].Mentions[k].ID = migrateLegacyAdminID(rooms[i].Messages[j].Mentions[k].ID, adminAliases)
			}
		}
	}
	return rooms
}

func migrateLegacyManagerRoomRefs(rooms []Room, managerAliases map[string]struct{}) []Room {
	for i := range rooms {
		rooms[i].Members = migrateLegacyManagerIDs(rooms[i].Members, managerAliases)
		for j := range rooms[i].Messages {
			rooms[i].Messages[j].SenderID = migrateLegacyManagerID(rooms[i].Messages[j].SenderID, managerAliases)
			rooms[i].Messages[j].Content = migrateLegacyManagerMentionTags(rooms[i].Messages[j].Content, managerAliases)
			if rooms[i].Messages[j].Event != nil {
				rooms[i].Messages[j].Event.ActorID = migrateLegacyManagerID(rooms[i].Messages[j].Event.ActorID, managerAliases)
				rooms[i].Messages[j].Event.TargetIDs = migrateLegacyManagerIDs(rooms[i].Messages[j].Event.TargetIDs, managerAliases)
			}
			for k := range rooms[i].Messages[j].Mentions {
				rooms[i].Messages[j].Mentions[k].ID = migrateLegacyManagerID(rooms[i].Messages[j].Mentions[k].ID, managerAliases)
			}
		}
	}
	return rooms
}

func migrateLegacyAdminIDs(ids []string, adminAliases map[string]struct{}) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = migrateLegacyAdminID(id, adminAliases)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func migrateLegacyManagerIDs(ids []string, managerAliases map[string]struct{}) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = migrateLegacyManagerID(id, managerAliases)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func migrateLegacyAdminID(id string, adminAliases map[string]struct{}) string {
	id = strings.TrimSpace(id)
	if _, ok := adminAliases[id]; ok {
		return adminUserID
	}
	return id
}

func migrateLegacyManagerID(id string, managerAliases map[string]struct{}) string {
	id = strings.TrimSpace(id)
	if _, ok := managerAliases[id]; ok {
		return managerParticipantUserID
	}
	return id
}

func migrateLegacyAdminMentionTags(content string, adminAliases map[string]struct{}) string {
	for id := range adminAliases {
		if id == adminUserID {
			continue
		}
		content = strings.ReplaceAll(content, `user_id="`+id+`"`, `user_id="`+adminUserID+`"`)
	}
	return content
}

func migrateLegacyManagerMentionTags(content string, managerAliases map[string]struct{}) string {
	for id := range managerAliases {
		if id == managerParticipantUserID {
			continue
		}
		content = strings.ReplaceAll(content, `user_id="`+id+`"`, `user_id="`+managerParticipantUserID+`"`)
	}
	return content
}

func ensureAdminManagerRoom(rooms []Room) []Room {
	for _, room := range rooms {
		if room.IsDirect && len(room.Members) == 2 && containsUserIDInRoom(room, adminUserID) && containsUserIDInRoom(room, managerParticipantUserID) {
			normalized := room
			if normalized.Title == "Admin & Manager" {
				normalized.Title = "admin & manager"
			}
			if normalized.Description == "Bootstrap room for Admin and Manager." {
				normalized.Description = "Bootstrap room for admin and manager."
			}
			if len(normalized.Messages) > 0 && normalized.Messages[0].Content == "Bootstrap room created for Admin and Manager." {
				normalized.Messages[0].Content = "Bootstrap room created for admin and manager."
			}
			normalized.IsDirect = true
			updated := append([]Room(nil), rooms...)
			for i := range updated {
				if updated[i].ID == normalized.ID {
					updated[i] = normalized
					return updated
				}
			}
			return rooms
		}
	}

	now := time.Now().UTC()
	room := Room{
		ID:          fmt.Sprintf("room-%d", now.UnixNano()),
		Title:       "admin & manager",
		Subtitle:    formatConversationSubtitle(2),
		Description: "Bootstrap room for admin and manager.",
		IsDirect:    true,
		Members:     []string{adminUserID, managerParticipantUserID},
		Messages: []Message{
			{
				ID:        fmt.Sprintf("msg-%d", now.UnixNano()+1),
				SenderID:  managerParticipantUserID,
				Content:   "Bootstrap room created for admin and manager.",
				CreatedAt: now,
			},
		},
	}
	return append(cloneRooms(rooms), room)
}

func containsUserIDInRoom(room Room, userID string) bool {
	for _, memberID := range room.Members {
		if memberID == userID {
			return true
		}
	}
	return false
}

func containsMentionID(mentions []Mention, userID string) bool {
	for _, mention := range mentions {
		if mention.ID == userID {
			return true
		}
	}
	return false
}

func containsUserIDInConversation(conv Conversation, userID string) bool {
	return containsUserIDInRoom(conv, userID)
}

func (s *Service) Bootstrap() Bootstrap {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })

	rooms := make([]Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, s.presentRoomLocked(*room, ""))
	}
	slices.SortFunc(rooms, func(a, b Room) int {
		return latestRoomMessageAt(b).Compare(latestRoomMessageAt(a))
	})

	return Bootstrap{
		CurrentUserID: s.currentUserID,
		Users:         users,
		Rooms:         rooms,
	}
}

func (s *Service) ListRooms() []Room {
	return s.ListRoomsWithOptions(ListMessagesOptions{})
}

func (s *Service) ListRoomsWithOptions(opts ListMessagesOptions) []Room {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]Room, 0, len(s.rooms))
	locale := messagePresentationLocale(opts.Locale)
	for _, room := range s.rooms {
		presented := s.presentRoomLocked(*room, locale)
		if opts.IncludeThreadReplies {
			presented.Messages = s.presentMessagesLocked(*room, true, locale)
		}
		rooms = append(rooms, presented)
	}
	slices.SortFunc(rooms, func(a, b Room) int {
		return latestRoomMessageAt(b).Compare(latestRoomMessageAt(a))
	})
	return rooms
}

func (s *Service) ListUsers() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })
	return users
}

func (s *Service) ListMembers(roomID string) ([]User, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, fmt.Errorf("room_id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return nil, fmt.Errorf("room not found")
	}

	users := make([]User, 0, len(room.Members))
	for _, userID := range room.Members {
		user, ok := s.users[userID]
		if !ok {
			return nil, fmt.Errorf("member user not found: %s", userID)
		}
		users = append(users, user)
	}
	return users, nil
}

// RoomIDsForMember returns sorted room IDs where the given user is a member (including direct rooms).
func (s *Service) RoomIDsForMember(userID string) []string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	for id, room := range s.rooms {
		if slices.Contains(room.Members, userID) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func (s *Service) ListMessages(roomID string) ([]Message, error) {
	return s.ListMessagesWithOptions(roomID, ListMessagesOptions{})
}

func (s *Service) ListMessagesWithOptions(roomID string, opts ListMessagesOptions) ([]Message, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, fmt.Errorf("room_id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return nil, fmt.Errorf("room not found")
	}
	return s.presentMessagesLocked(*room, opts.IncludeThreadReplies, opts.Locale), nil
}

func (s *Service) StartThread(req StartThreadRequest) (ThreadView, bool, error) {
	roomID := strings.TrimSpace(req.RoomID)
	rootID := strings.TrimSpace(req.RootMessageID)
	if roomID == "" {
		return ThreadView{}, false, fmt.Errorf("room_id is required")
	}
	if rootID == "" {
		return ThreadView{}, false, fmt.Errorf("root_message_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return ThreadView{}, false, fmt.Errorf("room not found")
	}
	rootIndex := messageIndexByID(room.Messages, rootID)
	if rootIndex < 0 {
		return ThreadView{}, false, fmt.Errorf("root message not found")
	}
	if isThreadReply(room.Messages[rootIndex]) {
		return ThreadView{}, false, fmt.Errorf("cannot start a thread from a thread reply")
	}

	created := false
	if threadIndexByRoot(room.Threads, rootID) < 0 {
		room.Threads = append(room.Threads, s.newThreadStateLocked(*room, rootIndex))
		created = true
		if err := s.saveLocked(); err != nil {
			return ThreadView{}, false, err
		}
	}

	view, err := s.threadViewLocked(*room, rootID, true)
	if err != nil {
		return ThreadView{}, false, err
	}
	return view, created, nil
}

func (s *Service) ListThreads(roomID string, opts ThreadListOptions) (ThreadListResponse, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return ThreadListResponse{}, fmt.Errorf("room_id is required")
	}
	include := strings.TrimSpace(strings.ToLower(opts.Include))
	if include == "" {
		include = ThreadListIncludeAll
	}
	if include != ThreadListIncludeAll && include != ThreadListIncludeParticipated {
		return ThreadListResponse{}, fmt.Errorf("invalid include")
	}
	if opts.From < 0 {
		return ThreadListResponse{}, fmt.Errorf("invalid from")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return ThreadListResponse{}, fmt.Errorf("room not found")
	}

	views := make([]ThreadView, 0, len(room.Threads))
	for _, state := range room.Threads {
		rootID := strings.TrimSpace(state.RootMessageID)
		if rootID == "" {
			continue
		}
		view, err := s.threadViewLocked(*room, rootID, false)
		if err != nil {
			continue
		}
		if include == ThreadListIncludeParticipated && !view.Summary.CurrentUserParticipated {
			continue
		}
		views = append(views, view)
	}
	slices.SortFunc(views, func(a, b ThreadView) int {
		return threadLatestAt(b).Compare(threadLatestAt(a))
	})

	start := opts.From
	if start > len(views) {
		start = len(views)
	}
	end := len(views)
	if opts.Limit > 0 && start+opts.Limit < end {
		end = start + opts.Limit
	}
	resp := ThreadListResponse{
		Threads: append([]ThreadView(nil), views[start:end]...),
	}
	if end < len(views) {
		resp.NextFrom = fmt.Sprintf("%d", end)
	}
	return resp, nil
}

func (s *Service) GetThread(roomID, rootMessageID string) (ThreadView, error) {
	roomID = strings.TrimSpace(roomID)
	rootMessageID = strings.TrimSpace(rootMessageID)
	if roomID == "" {
		return ThreadView{}, fmt.Errorf("room_id is required")
	}
	if rootMessageID == "" {
		return ThreadView{}, fmt.Errorf("root_message_id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return ThreadView{}, fmt.Errorf("room not found")
	}
	return s.threadViewLocked(*room, rootMessageID, true)
}

func (s *Service) ListThreadRelations(roomID, rootMessageID string) (ThreadRelationsResponse, error) {
	view, err := s.GetThread(roomID, rootMessageID)
	if err != nil {
		return ThreadRelationsResponse{}, err
	}
	return ThreadRelationsResponse{Chunk: view.Replies}, nil
}

func (s *Service) DeleteRoom(roomID string) error {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return fmt.Errorf("room_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rooms[roomID]; !ok {
		return fmt.Errorf("room not found")
	}
	delete(s.rooms, roomID)
	return s.saveLocked()
}

func (s *Service) DeleteConversation(conversationID string) error {
	return s.DeleteRoom(conversationID)
}

func (s *Service) ClearRoomMessages(roomID string) (Room, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return Room{}, fmt.Errorf("room_id is required")
	}

	s.mu.Lock()
	room, ok := s.rooms[roomID]
	if !ok {
		s.mu.Unlock()
		return Room{}, fmt.Errorf("room not found")
	}
	room.Messages = []Message{}
	room.Threads = []ThreadState{}
	if err := s.saveRoomLocked(*room); err != nil {
		s.mu.Unlock()
		return Room{}, err
	}
	presented := s.presentRoomLocked(*room, "")
	bus := s.bus
	s.mu.Unlock()

	if bus != nil {
		roomCopy := presented
		bus.Publish(Event{
			Type:   EventTypeRoomMessagesCleared,
			RoomID: presented.ID,
			Room:   &roomCopy,
		})
	}
	return presented, nil
}

func (s *Service) DeleteUser(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}

	s.mu.Lock()
	userID = s.resolveUserIDLocked(userID)
	user, ok := s.users[userID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("user not found")
	}
	if userID == s.currentUserID {
		s.mu.Unlock()
		return fmt.Errorf("cannot delete current user")
	}

	delete(s.users, userID)
	delete(s.byHandle, strings.ToLower(user.Handle))

	for id, room := range s.rooms {
		members := make([]string, 0, len(room.Members))
		for _, memberID := range room.Members {
			if memberID != userID {
				members = append(members, memberID)
			}
		}

		messages := make([]Message, 0, len(room.Messages))
		for _, message := range room.Messages {
			if message.SenderID != userID {
				messages = append(messages, message)
			}
		}

		if len(members) < 2 {
			delete(s.rooms, id)
			continue
		}

		room.Members = members
		room.Messages = messages
		s.rebuildThreadStatesLocked(room)
		room.Subtitle = formatRoomSubtitle(len(members))
	}

	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	bus := s.bus
	s.mu.Unlock()

	if bus != nil {
		userCopy := user
		bus.Publish(Event{
			Type: EventTypeUserDeleted,
			User: &userCopy,
		})
	}
	return nil
}

func (s *Service) EnsureAgentUser(req EnsureAgentUserRequest) (User, *Room, error) {
	id := strings.TrimSpace(req.ID)
	name := strings.ToLower(strings.TrimSpace(req.Name))
	description := strings.TrimSpace(req.Description)
	handle := strings.ToLower(strings.TrimSpace(req.Handle))
	role := strings.ToLower(strings.TrimSpace(req.Role))
	avatar := strings.TrimSpace(req.Avatar)
	switch {
	case id == "":
		return User{}, nil, fmt.Errorf("id is required")
	case name == "":
		return User{}, nil, fmt.Errorf("name is required")
	case handle == "":
		return User{}, nil, fmt.Errorf("handle is required")
	}
	if role == "" {
		role = "worker"
	}
	if avatar == "" {
		avatar = initials(name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.users[id]; ok {
		room, _ := s.ensureAdminAgentRoomLocked(id, existing.Name)
		if err := s.saveLocked(); err != nil {
			return User{}, nil, err
		}
		return existing, room, nil
	}
	if existingID, ok := s.byHandle[strings.ToLower(handle)]; ok && existingID != id {
		return User{}, nil, fmt.Errorf("handle %q already exists", handle)
	}

	user := User{
		ID:          id,
		Name:        name,
		Description: description,
		Handle:      handle,
		Role:        role,
		Avatar:      avatar,
		IsOnline:    true,
		AccentHex:   accentHexForID(id),
		CreatedAt:   time.Now().UTC(),
	}
	s.users[id] = user
	s.byHandle[strings.ToLower(handle)] = id
	room, roomCreated := s.ensureAdminAgentRoomLocked(id, name)
	if err := s.saveLocked(); err != nil {
		delete(s.users, id)
		delete(s.byHandle, strings.ToLower(handle))
		if roomCreated && room != nil {
			delete(s.rooms, room.ID)
		}
		return User{}, nil, err
	}
	return user, room, nil
}

func (s *Service) EnsureWorkerUser(req EnsureWorkerUserRequest) (User, *Room, error) {
	return s.EnsureAgentUser(req)
}

func (s *Service) UpdateAgentUser(req UpdateAgentUserRequest) (User, bool, error) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return User{}, false, fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = strings.TrimSpace(strings.TrimPrefix(id, "u-"))
			if name == "" {
				name = id
			}
		}
		handle := strings.ToLower(strings.TrimSpace(name))
		handle = strings.ReplaceAll(handle, " ", "-")
		role := strings.ToLower(strings.TrimSpace(req.Role))
		if role == "" {
			role = "worker"
		}
		user = User{
			ID:          id,
			Name:        name,
			Description: strings.TrimSpace(req.Description),
			Handle:      handle,
			Role:        role,
			Avatar:      strings.TrimSpace(req.Avatar),
			IsOnline:    true,
			AccentHex:   accentHexForID(id),
			CreatedAt:   time.Now().UTC(),
		}
		ok = true
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		user.Name = name
	}
	if description := strings.TrimSpace(req.Description); description != "" {
		user.Description = description
	}
	if role := strings.TrimSpace(req.Role); role != "" {
		user.Role = role
	}
	if avatar := strings.TrimSpace(req.Avatar); avatar != "" {
		user.Avatar = avatar
	}
	user = normalizeUser(user)
	if strings.TrimSpace(user.Avatar) == "" {
		user.Avatar = initials(user.Name)
	}
	s.users[id] = user
	if err := s.saveLocked(); err != nil {
		return User{}, false, err
	}
	return user, true, nil
}

func (s *Service) UpdateUser(req UpdateUserRequest) (User, bool, error) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return User{}, false, fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		return User{}, false, nil
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return User{}, false, fmt.Errorf("name is required")
		}
		user.Name = name
	}
	if req.Description != nil {
		user.Description = strings.TrimSpace(*req.Description)
		if user.ID == adminUserID && user.Description == "" {
			user.Description = DefaultAdminDescription
		}
	}
	if req.Handle != nil {
		handle := strings.ToLower(strings.TrimSpace(*req.Handle))
		if handle == "" {
			return User{}, false, fmt.Errorf("handle is required")
		}
		if existingID, ok := s.byHandle[handle]; ok && existingID != id {
			return User{}, false, fmt.Errorf("handle %q already exists", handle)
		}
		delete(s.byHandle, strings.ToLower(strings.TrimSpace(user.Handle)))
		user.Handle = handle
		s.byHandle[handle] = id
	}
	if req.Role != nil {
		role := strings.ToLower(strings.TrimSpace(*req.Role))
		if role != "" {
			user.Role = role
		}
	}
	if req.Avatar != nil {
		user.Avatar = strings.TrimSpace(*req.Avatar)
	}
	user = normalizeUser(user)
	if user.ID == adminUserID && strings.TrimSpace(user.Description) == "" {
		user.Description = DefaultAdminDescription
	}
	if strings.TrimSpace(user.Avatar) == "" {
		user.Avatar = initials(user.Name)
	}
	s.users[id] = user
	if err := s.saveLocked(); err != nil {
		return User{}, false, err
	}
	return user, true, nil
}

func (s *Service) CreateMessage(req CreateMessageRequest) (Message, error) {
	content := strings.TrimSpace(req.Content)
	roomID := strings.TrimSpace(req.RoomID)
	senderID := strings.TrimSpace(req.SenderID)
	if roomID == "" {
		return Message{}, fmt.Errorf("room_id is required")
	}
	if senderID == "" {
		return Message{}, fmt.Errorf("sender_id is required")
	}
	if content == "" {
		return Message{}, fmt.Errorf("content is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	senderID = s.resolveUserIDLocked(senderID)
	if _, ok := s.users[senderID]; !ok {
		return Message{}, fmt.Errorf("sender not found")
	}
	content, err := s.contentWithMentionPrefixLocked(content, req.MentionID)
	if err != nil {
		return Message{}, err
	}

	room, ok := s.rooms[roomID]
	if !ok {
		return Message{}, fmt.Errorf("room not found")
	}
	relatesTo, err := s.normalizeMessageRelationLocked(*room, req.RelatesTo)
	if err != nil {
		return Message{}, err
	}
	if relatesTo != nil && relatesTo.RelType == RelationTypeThread {
		s.ensureThreadStateLocked(room, relatesTo.EventID)
	}

	message := s.newMessage("", senderID, MessageKindMessage, content)
	message.RelatesTo = relatesTo
	room.Messages = append(room.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	return s.presentMessageLocked(*room, message, ""), nil
}

func (s *Service) DeliverMessage(req DeliverMessageRequest) (Message, error) {
	roomID := strings.TrimSpace(req.RoomID)
	senderID := strings.TrimSpace(req.SenderID)
	mentionID := strings.TrimSpace(req.MentionID)
	content := strings.TrimSpace(req.Content)
	if roomID == "" {
		return Message{}, fmt.Errorf("room_id is required")
	}
	if content == "" {
		return Message{}, fmt.Errorf("text is required")
	}
	if senderID == "" {
		senderID = s.currentUserID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[senderID]; !ok {
		return Message{}, fmt.Errorf("sender not found")
	}
	content, err := s.contentWithMentionPrefixLocked(content, mentionID)
	if err != nil {
		return Message{}, err
	}
	room, ok := s.rooms[roomID]
	if !ok {
		return Message{}, fmt.Errorf("room not found")
	}
	var relation *MessageRelation
	if rootID := strings.TrimSpace(req.ThreadRootID); rootID != "" {
		relation = &MessageRelation{RelType: RelationTypeThread, EventID: rootID}
	}
	relatesTo, err := s.normalizeMessageRelationLocked(*room, relation)
	if err != nil {
		return Message{}, err
	}
	if relatesTo != nil && relatesTo.RelType == RelationTypeThread {
		s.ensureThreadStateLocked(room, relatesTo.EventID)
	}

	message := s.newMessage(req.MessageID, senderID, MessageKindMessage, content)
	message.RelatesTo = relatesTo
	if strings.TrimSpace(req.MessageID) != "" {
		for idx := range room.Messages {
			if room.Messages[idx].ID != message.ID {
				continue
			}
			if room.Messages[idx].SenderID != senderID {
				return Message{}, fmt.Errorf("message id %q already exists for another sender", message.ID)
			}
			message.CreatedAt = room.Messages[idx].CreatedAt
			room.Messages[idx] = message
			s.rebuildThreadStatesLocked(room)
			if err := s.saveLocked(); err != nil {
				return Message{}, err
			}
			presented := s.presentMessageLocked(*room, message, "")
			s.publishMessageCreatedLocked(roomID, senderID, presented)
			return presented, nil
		}
	}
	room.Messages = append(room.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	presented := s.presentMessageLocked(*room, message, "")
	s.publishMessageCreatedLocked(roomID, senderID, presented)
	return presented, nil
}

func (s *Service) DeliverEvent(req DeliverEventRequest) (Message, error) {
	roomID := strings.TrimSpace(req.RoomID)
	senderID := strings.TrimSpace(req.SenderID)
	mentionID := strings.TrimSpace(req.MentionID)
	content := strings.TrimSpace(req.Content)
	if roomID == "" {
		return Message{}, fmt.Errorf("room_id is required")
	}
	if content == "" && req.Event == nil {
		return Message{}, fmt.Errorf("event content is required")
	}
	if senderID == "" {
		senderID = s.currentUserID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[senderID]; !ok {
		return Message{}, fmt.Errorf("sender not found")
	}
	room, ok := s.rooms[roomID]
	if !ok {
		return Message{}, fmt.Errorf("room not found")
	}
	mentions := s.extractMentions(content)
	if mentionID != "" {
		if _, ok := s.users[mentionID]; !ok {
			return Message{}, fmt.Errorf("mentioned user not found")
		}
		mentions = appendMissingMentions(mentions, s.mentionsForUserIDs([]string{mentionID}))
	}

	message := s.newMessage(req.MessageID, senderID, MessageKindEvent, content)
	message.Event = cloneEventPayload(req.Event)
	message.Mentions = mentions
	if strings.TrimSpace(req.MessageID) != "" {
		for idx := range room.Messages {
			if room.Messages[idx].ID != message.ID {
				continue
			}
			if room.Messages[idx].SenderID != senderID {
				return Message{}, fmt.Errorf("message id %q already exists for another sender", message.ID)
			}
			message.CreatedAt = room.Messages[idx].CreatedAt
			room.Messages[idx] = message
			if err := s.saveLocked(); err != nil {
				return Message{}, err
			}
			presented := s.presentMessageLocked(*room, message, "")
			s.publishMessageCreatedLocked(roomID, senderID, presented)
			return presented, nil
		}
	}
	room.Messages = append(room.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	presented := s.presentMessageLocked(*room, message, "")
	s.publishMessageCreatedLocked(roomID, senderID, presented)
	return presented, nil
}

func (s *Service) publishMessageCreatedLocked(roomID, senderID string, message Message) {
	if s.bus == nil {
		return
	}
	sender, ok := s.users[senderID]
	if !ok {
		return
	}
	messageCopy := message
	senderCopy := sender
	s.bus.Publish(Event{
		Type:    EventTypeMessageCreated,
		RoomID:  roomID,
		Message: &messageCopy,
		Sender:  &senderCopy,
	})
}

func (s *Service) CreateRoom(req CreateRoomRequest) (Room, error) {
	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	creatorID := strings.TrimSpace(req.CreatorID)
	if title == "" {
		return Room{}, fmt.Errorf("title is required")
	}
	if creatorID == "" {
		return Room{}, fmt.Errorf("creator_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	creatorID = s.resolveRoomUserIDLocked(creatorID)
	if _, ok := s.users[creatorID]; !ok {
		return Room{}, fmt.Errorf("creator not found")
	}

	members, err := s.normalizeMembers(creatorID, req.MemberIDs)
	if err != nil {
		return Room{}, err
	}

	room := Room{
		ID:          fmt.Sprintf("room-%d", time.Now().UnixNano()),
		Title:       title,
		Subtitle:    formatRoomSubtitle(len(members)),
		Description: description,
		IsDirect:    false,
		Members:     members,
		Messages: []Message{
			{
				ID:       fmt.Sprintf("msg-%d", time.Now().UnixNano()),
				SenderID: creatorID,
				Kind:     MessageKindEvent,
				Event: &EventPayload{
					Key:     "room_created",
					ActorID: creatorID,
					Title:   title,
				},
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	s.rooms[room.ID] = &room
	if err := s.saveLocked(); err != nil {
		return Room{}, err
	}
	return s.presentRoomLocked(room, messagePresentationLocale(req.Locale)), nil
}

func (s *Service) CreateConversation(req CreateConversationRequest) (Conversation, error) {
	return s.CreateRoom(req)
}

func (s *Service) AddRoomMembers(req AddRoomMembersRequest) (Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	inviterID := strings.TrimSpace(req.InviterID)
	if roomID == "" {
		return Room{}, fmt.Errorf("room_id is required")
	}
	if inviterID == "" {
		return Room{}, fmt.Errorf("inviter_id is required")
	}
	if len(req.UserIDs) == 0 {
		return Room{}, fmt.Errorf("user_ids is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return Room{}, fmt.Errorf("room not found")
	}
	inviterID = s.resolveRoomUserIDLocked(inviterID)
	if _, ok := s.users[inviterID]; !ok {
		return Room{}, fmt.Errorf("inviter not found")
	}
	if !slices.Contains(room.Members, inviterID) {
		return Room{}, fmt.Errorf("inviter is not a room member")
	}
	if room.IsDirect {
		return Room{}, fmt.Errorf("cannot add members to direct room")
	}

	existing := make(map[string]struct{}, len(room.Members))
	for _, id := range room.Members {
		existing[id] = struct{}{}
	}

	addedIDs := make([]string, 0, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		userID = s.resolveRoomUserIDLocked(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return Room{}, fmt.Errorf("user not found: %s", userID)
		}
		if _, ok := existing[userID]; ok {
			continue
		}
		existing[userID] = struct{}{}
		room.Members = append(room.Members, userID)
		addedIDs = append(addedIDs, userID)
	}
	if len(addedIDs) == 0 {
		return Room{}, fmt.Errorf("no new users to invite")
	}

	room.Subtitle = formatRoomSubtitle(len(room.Members))
	room.Messages = append(room.Messages, Message{
		ID:       fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SenderID: inviterID,
		Kind:     MessageKindEvent,
		Event: &EventPayload{
			Key:       "room_members_added",
			ActorID:   inviterID,
			TargetIDs: append([]string(nil), addedIDs...),
		},
		CreatedAt: time.Now().UTC(),
		Mentions:  s.mentionsForUserIDs(addedIDs),
	})
	if err := s.saveLocked(); err != nil {
		return Room{}, err
	}

	return s.presentRoomLocked(*room, messagePresentationLocale(req.Locale)), nil
}

func (s *Service) RemoveRoomMembers(req AddRoomMembersRequest) (Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	inviterID := strings.TrimSpace(req.InviterID)
	if roomID == "" {
		return Room{}, fmt.Errorf("room_id is required")
	}
	if inviterID == "" {
		return Room{}, fmt.Errorf("inviter_id is required")
	}
	if len(req.UserIDs) == 0 {
		return Room{}, fmt.Errorf("user_ids is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return Room{}, fmt.Errorf("room not found")
	}
	inviterID = s.resolveRoomUserIDLocked(inviterID)
	if _, ok := s.users[inviterID]; !ok {
		return Room{}, fmt.Errorf("inviter not found")
	}
	if !slices.Contains(room.Members, inviterID) {
		return Room{}, fmt.Errorf("inviter is not a room member")
	}
	if room.IsDirect {
		return Room{}, fmt.Errorf("cannot remove members from direct room")
	}

	removing := make(map[string]struct{}, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		userID = s.resolveRoomUserIDLocked(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return Room{}, fmt.Errorf("user not found: %s", userID)
		}
		if userID == inviterID {
			continue
		}
		removing[userID] = struct{}{}
	}
	if len(removing) == 0 {
		return Room{}, fmt.Errorf("no removable users")
	}

	remaining := make([]string, 0, len(room.Members))
	removedIDs := make([]string, 0, len(removing))
	for _, memberID := range room.Members {
		if _, ok := removing[memberID]; ok {
			removedIDs = append(removedIDs, memberID)
			continue
		}
		remaining = append(remaining, memberID)
	}
	if len(removedIDs) == 0 {
		return Room{}, fmt.Errorf("no matching users to remove")
	}

	room.Members = remaining
	room.Subtitle = formatRoomSubtitle(len(room.Members))
	room.Messages = append(room.Messages, Message{
		ID:       fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SenderID: inviterID,
		Kind:     MessageKindEvent,
		Event: &EventPayload{
			Key:       "room_members_removed",
			ActorID:   inviterID,
			TargetIDs: append([]string(nil), removedIDs...),
		},
		CreatedAt: time.Now().UTC(),
		Mentions:  s.mentionsForUserIDs(removedIDs),
	})
	if err := s.saveLocked(); err != nil {
		return Room{}, err
	}

	return s.presentRoomLocked(*room, messagePresentationLocale(req.Locale)), nil
}

func (s *Service) AddConversationMembers(req AddConversationMembersRequest) (Conversation, error) {
	return s.AddRoomMembers(req)
}

func (s *Service) AddAgentToRoom(req AddAgentToConversationRequest) (Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return Room{}, fmt.Errorf("room_id is required")
	}

	return s.AddRoomMembers(AddRoomMembersRequest{
		RoomID:    roomID,
		InviterID: strings.TrimSpace(req.InviterID),
		UserIDs:   []string{strings.TrimSpace(req.AgentID)},
		Locale:    req.Locale,
	})
}

func (s *Service) AddAgentToConversation(req AddAgentToConversationRequest) (Conversation, error) {
	return s.AddAgentToRoom(req)
}

func (s *Service) Room(roomID string) (Room, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return Room{}, false
	}
	return s.presentRoomLocked(*room, ""), true
}

func (s *Service) Conversation(conversationID string) (Conversation, bool) {
	return s.Room(conversationID)
}

func (s *Service) User(userID string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID = s.resolveUserIDLocked(userID)
	user, ok := s.users[userID]
	return user, ok
}

func (s *Service) extractMentions(content string) []Mention {
	tagMatches := mentionTagPattern.FindAllStringSubmatch(content, -1)
	handleMatches := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(tagMatches) == 0 && len(handleMatches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(tagMatches)+len(handleMatches))
	mentions := make([]Mention, 0, len(tagMatches)+len(handleMatches))
	for _, match := range tagMatches {
		userID := strings.TrimSpace(match[1])
		user, ok := s.users[userID]
		if !ok {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		mentions = append(mentions, Mention{
			ID:   userID,
			Name: s.userMentionName(user),
		})
	}
	for _, match := range handleMatches {
		handle := strings.ToLower(match[2])
		if userID, ok := s.byHandle[handle]; ok {
			if _, exists := seen[userID]; exists {
				continue
			}
			seen[userID] = struct{}{}
			mentions = append(mentions, Mention{
				ID:   userID,
				Name: s.userMentionName(s.users[userID]),
			})
		}
	}
	return mentions
}

func (s *Service) normalizeMembers(creatorID string, memberIDs []string) ([]string, error) {
	seen := map[string]struct{}{creatorID: {}}
	members := []string{creatorID}
	for _, userID := range memberIDs {
		userID = s.resolveRoomUserIDLocked(userID)
		if userID == "" {
			continue
		}
		if _, ok := s.users[userID]; !ok {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		members = append(members, userID)
	}
	return members, nil
}

const (
	MessageKindMessage = "message"
	MessageKindEvent   = "event"
)

func messagePresentationLocale(locale string) string {
	if strings.TrimSpace(locale) == "" {
		return "en"
	}
	switch normalizeLocale(locale) {
	case "zh":
		return "zh"
	default:
		return "en"
	}
}

func (s *Service) localizeEventMessageLocked(locale string, message Message) string {
	if message.Event == nil {
		return strings.TrimSpace(message.Content)
	}
	key := strings.TrimSpace(message.Event.Key)
	if key == "" {
		return strings.TrimSpace(message.Content)
	}
	if localized := strings.TrimSpace(s.localizeSystemText(messagePresentationLocale(locale), key, message.Event.ActorID, message.Event.Title, message.Event.TargetIDs)); localized != "" {
		return localized
	}
	return strings.TrimSpace(message.Content)
}

func (s *Service) localizeSystemText(locale, key, actorID, title string, userIDs []string) string {
	actor := s.userDisplayName(actorID)
	targets := s.userDisplayNames(userIDs)
	switch normalizeLocale(locale) {
	case "en":
		switch key {
		case "room_created":
			return fmt.Sprintf("%s created the room", actor)
		case "room_members_added":
			return fmt.Sprintf("%s invited %s to join the room", actor, strings.Join(targets, ", "))
		case "room_members_removed":
			return fmt.Sprintf("%s removed %s from the room", actor, strings.Join(targets, ", "))
		}
	default:
		switch key {
		case "room_created":
			return fmt.Sprintf("%s 创建了房间", actor)
		case "room_members_added":
			return fmt.Sprintf("%s 邀请 %s 加入了房间", actor, strings.Join(targets, "、"))
		case "room_members_removed":
			return fmt.Sprintf("%s 将 %s 移出了房间", actor, strings.Join(targets, "、"))
		}
	}
	return ""
}

func (s *Service) userDisplayName(userID string) string {
	if user, ok := s.users[userID]; ok {
		if strings.TrimSpace(user.Name) != "" {
			return user.Name
		}
		if strings.TrimSpace(user.Handle) != "" {
			return "@" + user.Handle
		}
	}
	return userID
}

func (s *Service) userMentionName(user User) string {
	if strings.TrimSpace(user.Name) != "" {
		return user.Name
	}
	if strings.TrimSpace(user.Handle) != "" {
		return user.Handle
	}
	return user.ID
}

func (s *Service) userDisplayNames(userIDs []string) []string {
	names := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		names = append(names, s.userDisplayName(userID))
	}
	return names
}

func normalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if strings.HasPrefix(locale, "en") {
		return "en"
	}
	return "zh"
}

func formatRoomSubtitle(count int) string {
	return ""
}

func formatConversationSubtitle(count int) string {
	return formatRoomSubtitle(count)
}

func (s *Service) presentRoomLocked(room Room, locale string) Room {
	cloned := cloneRoom(room)
	cloned.Messages = s.presentMessagesLocked(room, false, locale)
	if !cloned.IsDirect || len(cloned.Members) != 2 {
		return cloned
	}

	otherID := cloned.Members[0]
	if otherID == s.currentUserID {
		otherID = cloned.Members[1]
	}
	if user, ok := s.users[otherID]; ok && strings.TrimSpace(user.Name) != "" {
		cloned.Title = user.Name
	}
	return cloned
}

func (s *Service) presentConversationLocked(conv Conversation) Conversation {
	return s.presentRoomLocked(conv, "")
}

func cloneRoom(room Room) Room {
	cloned := room
	cloned.Members = append([]string(nil), room.Members...)
	cloned.Messages = cloneMessages(room.Messages)
	cloned.Threads = cloneThreadStates(room.Threads)
	return cloned
}

func cloneConversation(conv Conversation) Conversation {
	return cloneRoom(conv)
}

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	cloned := make([]Message, 0, len(messages))
	for _, message := range messages {
		cloned = append(cloned, cloneMessage(message))
	}
	return cloned
}

func cloneMessage(message Message) Message {
	cloned := message
	cloned.Mentions = append([]Mention(nil), message.Mentions...)
	cloned.Event = cloneEventPayload(message.Event)
	if message.RelatesTo != nil {
		rel := *message.RelatesTo
		cloned.RelatesTo = &rel
	}
	if message.Thread != nil {
		summary := cloneThreadSummary(*message.Thread)
		cloned.Thread = &summary
	}
	return cloned
}

func cloneEventPayload(event *EventPayload) *EventPayload {
	if event == nil {
		return nil
	}
	cloned := *event
	cloned.TargetIDs = append([]string(nil), event.TargetIDs...)
	return &cloned
}

func cloneThreadStates(states []ThreadState) []ThreadState {
	if len(states) == 0 {
		return nil
	}
	cloned := make([]ThreadState, 0, len(states))
	for _, state := range states {
		cloned = append(cloned, ThreadState{
			RootMessageID: strings.TrimSpace(state.RootMessageID),
			CreatedAt:     state.CreatedAt,
			Context:       cloneMessages(state.Context),
			Summary:       state.Summary,
		})
	}
	return cloned
}

func cloneThreadSummary(summary ThreadSummary) ThreadSummary {
	cloned := summary
	cloned.Participants = append([]Mention(nil), summary.Participants...)
	if summary.LatestReply != nil {
		latest := cloneMessage(*summary.LatestReply)
		latest.Thread = nil
		cloned.LatestReply = &latest
	}
	return cloned
}

func (s *Service) presentMessagesLocked(room Room, includeThreadReplies bool, locale string) []Message {
	messages := make([]Message, 0, len(room.Messages))
	for _, message := range room.Messages {
		if !includeThreadReplies && isThreadReply(message) {
			continue
		}
		messages = append(messages, s.presentMessageLocked(room, message, locale))
	}
	return messages
}

func (s *Service) presentMessageLocked(room Room, message Message, locale string) Message {
	cloned := cloneMessage(message)
	if cloned.Kind == MessageKindEvent && cloned.Event != nil {
		if localized := s.localizeEventMessageLocked(locale, cloned); localized != "" {
			cloned.Content = localized
		}
	}
	if isThreadReply(cloned) {
		cloned.Thread = nil
		return cloned
	}
	if threadIndexByRoot(room.Threads, cloned.ID) >= 0 {
		if summary, ok := s.threadSummaryLocked(room, cloned.ID); ok {
			cloned.Thread = &summary
		}
	}
	return cloned
}

func (s *Service) normalizeMessageRelationLocked(room Room, relation *MessageRelation) (*MessageRelation, error) {
	if relation == nil {
		return nil, nil
	}
	relType := strings.TrimSpace(relation.RelType)
	eventID := strings.TrimSpace(relation.EventID)
	if relType == "" && eventID == "" {
		return nil, nil
	}
	if relType != RelationTypeThread {
		return nil, fmt.Errorf("unsupported relation type")
	}
	if eventID == "" {
		return nil, fmt.Errorf("relation event_id is required")
	}
	rootIndex := messageIndexByID(room.Messages, eventID)
	if rootIndex < 0 {
		return nil, fmt.Errorf("thread root message not found")
	}
	if isThreadReply(room.Messages[rootIndex]) {
		return nil, fmt.Errorf("cannot reply to a thread reply")
	}
	return &MessageRelation{RelType: RelationTypeThread, EventID: eventID}, nil
}

func (s *Service) ensureThreadStateLocked(room *Room, rootMessageID string) {
	if room == nil {
		return
	}
	rootIndex := messageIndexByID(room.Messages, rootMessageID)
	if rootIndex < 0 || isThreadReply(room.Messages[rootIndex]) {
		return
	}
	if threadIndexByRoot(room.Threads, rootMessageID) >= 0 {
		return
	}
	room.Threads = append(room.Threads, s.newThreadStateLocked(*room, rootIndex))
}

func (s *Service) rebuildThreadStatesLocked(room *Room) {
	if room == nil || len(room.Threads) == 0 {
		return
	}
	rebuilt := make([]ThreadState, 0, len(room.Threads))
	for _, state := range room.Threads {
		rootIndex := messageIndexByID(room.Messages, state.RootMessageID)
		if rootIndex < 0 || isThreadReply(room.Messages[rootIndex]) {
			continue
		}
		next := s.newThreadStateLocked(*room, rootIndex)
		if !state.CreatedAt.IsZero() {
			next.CreatedAt = state.CreatedAt
		}
		rebuilt = append(rebuilt, next)
	}
	room.Threads = rebuilt
}

func (s *Service) newThreadStateLocked(room Room, rootIndex int) ThreadState {
	context, before, after := threadContextWindow(room.Messages, rootIndex)
	root := room.Messages[rootIndex]
	return ThreadState{
		RootMessageID: root.ID,
		CreatedAt:     time.Now().UTC(),
		Context:       context,
		Summary: ThreadContextSummary{
			RootExcerpt:  excerpt(root.Content, 160),
			MessageCount: len(context),
			BeforeCount:  before,
			AfterCount:   after,
		},
	}
}

func (s *Service) threadViewLocked(room Room, rootMessageID string, includeContext bool) (ThreadView, error) {
	rootIndex := messageIndexByID(room.Messages, rootMessageID)
	if rootIndex < 0 {
		return ThreadView{}, fmt.Errorf("thread root message not found")
	}
	if isThreadReply(room.Messages[rootIndex]) {
		return ThreadView{}, fmt.Errorf("thread root message is a thread reply")
	}
	if threadIndexByRoot(room.Threads, rootMessageID) < 0 {
		return ThreadView{}, fmt.Errorf("thread not found")
	}
	summary, _ := s.threadSummaryLocked(room, rootMessageID)
	root := s.presentMessageLocked(room, room.Messages[rootIndex], "")
	root.Thread = &summary
	view := ThreadView{
		RoomID:  room.ID,
		Root:    root,
		Replies: s.threadRepliesLocked(room, rootMessageID),
		Summary: summary,
	}
	if includeContext {
		if state, ok := threadStateByRoot(room.Threads, rootMessageID); ok {
			view.Context = cloneMessages(state.Context)
		}
	}
	return view, nil
}

func (s *Service) threadSummaryLocked(room Room, rootMessageID string) (ThreadSummary, bool) {
	state, ok := threadStateByRoot(room.Threads, rootMessageID)
	if !ok {
		return ThreadSummary{}, false
	}
	replies := s.threadRepliesLocked(room, rootMessageID)
	var latest *Message
	if len(replies) > 0 {
		msg := cloneMessage(replies[len(replies)-1])
		msg.Thread = nil
		latest = &msg
	}
	return ThreadSummary{
		RootID:                  rootMessageID,
		ReplyCount:              len(replies),
		LatestReply:             latest,
		Participants:            s.threadParticipantsLocked(room, rootMessageID, replies),
		CurrentUserParticipated: s.threadUserParticipatedLocked(room, rootMessageID, replies, s.currentUserID),
		Context:                 state.Summary,
	}, true
}

func (s *Service) threadRepliesLocked(room Room, rootMessageID string) []Message {
	replies := make([]Message, 0)
	for _, message := range room.Messages {
		if threadRootID(message) != rootMessageID {
			continue
		}
		reply := cloneMessage(message)
		reply.Thread = nil
		replies = append(replies, reply)
	}
	slices.SortFunc(replies, func(a, b Message) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	return replies
}

func (s *Service) threadParticipantsLocked(room Room, rootMessageID string, replies []Message) []Mention {
	seen := make(map[string]struct{}, len(replies)+1)
	participants := make([]Mention, 0, len(replies)+1)
	add := func(userID string) {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			return
		}
		if _, ok := seen[userID]; ok {
			return
		}
		seen[userID] = struct{}{}
		if user, ok := s.users[userID]; ok {
			participants = append(participants, Mention{ID: userID, Name: s.userMentionName(user)})
			return
		}
		participants = append(participants, Mention{ID: userID})
	}
	if idx := messageIndexByID(room.Messages, rootMessageID); idx >= 0 {
		add(room.Messages[idx].SenderID)
	}
	for _, reply := range replies {
		add(reply.SenderID)
	}
	return participants
}

func (s *Service) threadUserParticipatedLocked(room Room, rootMessageID string, replies []Message, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	if idx := messageIndexByID(room.Messages, rootMessageID); idx >= 0 && room.Messages[idx].SenderID == userID {
		return true
	}
	for _, reply := range replies {
		if reply.SenderID == userID {
			return true
		}
	}
	return false
}

func messageIndexByID(messages []Message, id string) int {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1
	}
	for idx, message := range messages {
		if message.ID == id {
			return idx
		}
	}
	return -1
}

func threadIndexByRoot(states []ThreadState, rootMessageID string) int {
	rootMessageID = strings.TrimSpace(rootMessageID)
	for idx, state := range states {
		if strings.TrimSpace(state.RootMessageID) == rootMessageID {
			return idx
		}
	}
	return -1
}

func threadStateByRoot(states []ThreadState, rootMessageID string) (ThreadState, bool) {
	if idx := threadIndexByRoot(states, rootMessageID); idx >= 0 {
		return states[idx], true
	}
	return ThreadState{}, false
}

func isThreadReply(message Message) bool {
	return threadRootID(message) != ""
}

func threadRootID(message Message) string {
	if message.RelatesTo == nil || strings.TrimSpace(message.RelatesTo.RelType) != RelationTypeThread {
		return ""
	}
	return strings.TrimSpace(message.RelatesTo.EventID)
}

func threadContextWindow(messages []Message, rootIndex int) ([]Message, int, int) {
	if rootIndex < 0 || rootIndex >= len(messages) {
		return nil, 0, 0
	}
	topLevel := make([]Message, 0, len(messages))
	rootTopLevelIndex := -1
	rootID := messages[rootIndex].ID
	for _, message := range messages {
		if isThreadReply(message) {
			continue
		}
		if message.ID == rootID {
			rootTopLevelIndex = len(topLevel)
		}
		topLevel = append(topLevel, message)
	}
	if rootTopLevelIndex < 0 {
		return nil, 0, 0
	}
	start := rootTopLevelIndex - 5
	if start < 0 {
		start = 0
	}
	end := rootTopLevelIndex + 3
	if end > len(topLevel) {
		end = len(topLevel)
	}
	context := cloneMessages(topLevel[start:end])
	before := rootTopLevelIndex - start
	after := end - rootTopLevelIndex - 1
	context, before, after = trimThreadContextToPayloadCap(context, before, after)
	return context, before, after
}

func trimThreadContextToPayloadCap(context []Message, before, after int) ([]Message, int, int) {
	for len(context) > 1 && threadContextPayloadSize(context) > maxThreadContextSnapshotBytes {
		switch {
		case before > after && before > 0:
			context = context[1:]
			before--
		case after > 0:
			context = context[:len(context)-1]
			after--
		case before > 0:
			context = context[1:]
			before--
		default:
			return context, before, after
		}
	}
	return context, before, after
}

func threadContextPayloadSize(context []Message) int {
	data, err := json.Marshal(context)
	if err != nil {
		return 0
	}
	return len(data)
}

func threadLatestAt(view ThreadView) time.Time {
	if view.Summary.LatestReply != nil && !view.Summary.LatestReply.CreatedAt.IsZero() {
		return view.Summary.LatestReply.CreatedAt
	}
	if !view.Root.CreatedAt.IsZero() {
		return view.Root.CreatedAt
	}
	return time.Time{}
}

func excerpt(content string, maxRunes int) string {
	content = previewText(content)
	if maxRunes <= 0 || len([]rune(content)) <= maxRunes {
		return content
	}
	runes := []rune(content)
	return string(runes[:maxRunes])
}

func previewText(content string) string {
	return strings.Join(strings.Fields(stripPreviewCodeFence(content)), " ")
}

func stripPreviewCodeFence(content string) string {
	text := strings.TrimSpace(content)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	text = strings.TrimLeft(strings.TrimPrefix(text, "```"), " \t")
	if idx := strings.IndexAny(text, "\r\n"); idx >= 0 {
		next := idx + 1
		if text[idx] == '\r' && len(text) > next && text[next] == '\n' {
			next++
		}
		text = text[next:]
	} else {
		fields := strings.Fields(text)
		if len(fields) > 1 && isPreviewFenceLanguage(fields[0]) {
			text = strings.Join(fields[1:], " ")
		}
	}
	text = strings.TrimSpace(text)
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSpace(strings.TrimSuffix(text, "```"))
	}
	return text
}

func isPreviewFenceLanguage(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text", "txt", "plain",
		"json", "jsonc", "yaml", "yml", "toml",
		"go", "ts", "tsx", "js", "jsx",
		"bash", "sh", "zsh", "fish",
		"python", "py", "html", "css", "sql",
		"md", "markdown", "diff", "xml",
		"dockerfile", "makefile", "ini", "env", "csv":
		return true
	default:
		return false
	}
}

func (s *Service) newMessage(messageID, senderID, kind, content string) Message {
	if strings.TrimSpace(messageID) == "" {
		messageID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(kind) == "" {
		kind = MessageKindMessage
	}
	return Message{
		ID:        messageID,
		SenderID:  senderID,
		Kind:      kind,
		Content:   content,
		CreatedAt: time.Now().UTC(),
		Mentions:  s.extractMentions(content),
	}
}

func (s *Service) contentWithMentionPrefixLocked(content, mentionID string) (string, error) {
	mentionID = s.resolveUserIDLocked(mentionID)
	if mentionID == "" {
		return content, nil
	}

	user, ok := s.users[mentionID]
	if !ok {
		return "", fmt.Errorf("mentioned user not found")
	}
	displayName := strings.TrimSpace(user.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Handle)
	}
	if displayName == "" {
		displayName = mentionID
	}

	prefix := fmt.Sprintf("<at user_id=\"%s\">%s</at>", mentionID, displayName)
	if content == prefix || strings.HasPrefix(content, prefix+" ") {
		return content, nil
	}
	cmd, isSlash, err := slashcommand.Parse(content)
	if err != nil {
		return "", err
	}
	if isSlash {
		if cmd.Body == prefix || strings.HasPrefix(cmd.Body, prefix+" ") {
			return slashcommand.Render(cmd)
		}
		cmd.Body = strings.TrimSpace(prefix + " " + cmd.Body)
		return slashcommand.Render(cmd)
	}
	return prefix + " " + strings.TrimSpace(content), nil
}

func (s *Service) mentionsForUserIDs(userIDs []string) []Mention {
	if len(userIDs) == 0 {
		return nil
	}
	mentions := make([]Mention, 0, len(userIDs))
	for _, userID := range userIDs {
		user, ok := s.users[userID]
		if !ok {
			continue
		}
		mentions = append(mentions, Mention{
			ID:   userID,
			Name: s.userMentionName(user),
		})
	}
	if len(mentions) == 0 {
		return nil
	}
	return mentions
}

func appendMissingMentions(base, extra []Mention) []Mention {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, mention := range base {
		if id := strings.TrimSpace(mention.ID); id != "" {
			seen[id] = struct{}{}
		}
	}
	out := append([]Mention(nil), base...)
	for _, mention := range extra {
		id := strings.TrimSpace(mention.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, mention)
	}
	return out
}

func (s *Service) saveLocked() error {
	if s.statePath == "" {
		return nil
	}
	return SaveBootstrap(s.statePath, s.bootstrapLocked())
}

func (s *Service) saveRoomLocked(room Room) error {
	if s.statePath == "" {
		return nil
	}
	if err := ensureBootstrapDirs(s.statePath); err != nil {
		return err
	}
	if err := saveRoomMessagesForState(s.statePath, room); err != nil {
		return err
	}
	return writePersistedBootstrap(s.statePath, persistedBootstrapFromState(s.bootstrapLocked()))
}

func (s *Service) replaceState(state Bootstrap) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceStateLocked(state)
}

func (s *Service) replaceStateLocked(state Bootstrap) {
	state = normalizeBootstrap(state)
	users := state.Users
	rooms := state.Rooms

	s.currentUserID = state.CurrentUserID
	s.users = make(map[string]User, len(users))
	s.byHandle = make(map[string]string, len(users))
	s.rooms = make(map[string]*Room, len(rooms))

	for _, user := range users {
		s.users[user.ID] = user
		s.byHandle[strings.ToLower(user.Handle)] = user.ID
	}
	for i := range rooms {
		room := rooms[i]
		s.rooms[room.ID] = &room
	}
}

func (s *Service) bootstrapLocked() Bootstrap {
	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })

	rooms := make([]Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, cloneRoom(*room))
	}
	slices.SortFunc(rooms, func(a, b Room) int {
		return latestRoomMessageAt(b).Compare(latestRoomMessageAt(a))
	})

	return Bootstrap{
		CurrentUserID: s.currentUserID,
		Users:         users,
		Rooms:         rooms,
	}
}

func (s *Service) ensureAdminAgentRoomLocked(agentID, agentName string) (*Room, bool) {
	for _, room := range s.rooms {
		if len(room.Members) != 2 {
			continue
		}
		if containsUserIDInRoom(*room, adminUserID) && containsUserIDInRoom(*room, agentID) {
			presented := s.presentRoomLocked(*room, "")
			return &presented, false
		}
	}

	now := time.Now().UTC()
	room := Room{
		ID:          fmt.Sprintf("room-%d", now.UnixNano()),
		Title:       agentName,
		Subtitle:    formatRoomSubtitle(2),
		Description: fmt.Sprintf("Bootstrap room for admin and %s.", agentName),
		IsDirect:    true,
		Members:     []string{adminUserID, agentID},
		Messages: []Message{
			{
				ID:        fmt.Sprintf("msg-%d", now.UnixNano()+1),
				SenderID:  agentID,
				Content:   fmt.Sprintf("Bootstrap room created for admin and %s.", agentName),
				CreatedAt: now,
			},
		},
	}
	s.rooms[room.ID] = &room
	presented := s.presentRoomLocked(room, "")
	return &presented, true
}

func initials(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) == 0 {
		return "WK"
	}
	var b strings.Builder
	for _, field := range fields {
		for _, r := range field {
			if r == '-' || r == '_' {
				continue
			}
			b.WriteRune(r)
			if b.Len() >= 2 {
				return strings.ToUpper(b.String())
			}
			break
		}
	}
	if b.Len() == 0 {
		return "WK"
	}
	return strings.ToUpper(b.String())
}

func accentHexForID(id string) string {
	palette := []string{
		"#2563eb",
		"#7c3aed",
		"#0891b2",
		"#059669",
		"#ea580c",
		"#db2777",
	}
	sum := 0
	for _, r := range id {
		sum += int(r)
	}
	return palette[sum%len(palette)]
}

func latestRoomMessageAt(room Room) time.Time {
	if len(room.Messages) == 0 {
		return time.Time{}
	}
	return room.Messages[len(room.Messages)-1].CreatedAt
}

func latestMessageAt(conv Conversation) time.Time {
	return latestRoomMessageAt(conv)
}

func seedTime(hour, minute int) time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
}
