package im

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/apitypes"
)

type User = apitypes.User

type Message = apitypes.Message

type EventPayload = apitypes.EventPayload

type Room = apitypes.Room

type Conversation = Room

type Bootstrap struct {
	CurrentUserID      string   `json:"current_user_id"`
	Users              []User   `json:"users"`
	Rooms              []Room   `json:"rooms,omitempty"`
	InviteDraftUserIDs []string `json:"invite_draft_user_ids,omitempty"`
}

type CreateMessageRequest = apitypes.CreateMessageRequest

type DeliverMessageRequest struct {
	RoomID    string `json:"room_id"`
	SenderID  string `json:"sender_id,omitempty"`
	Content   string `json:"text"`
	MessageID string `json:"message_id,omitempty"`
}

type CreateRoomRequest = apitypes.CreateRoomRequest

type CreateConversationRequest = CreateRoomRequest

type AddRoomMembersRequest = apitypes.AddRoomMembersRequest

type AddConversationMembersRequest = AddRoomMembersRequest

type EnsureAgentUserRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Handle string `json:"handle"`
	Role   string `json:"role"`
}

type EnsureWorkerUserRequest = EnsureAgentUserRequest

type AddAgentToConversationRequest struct {
	AgentID   string `json:"agent_id"`
	RoomID    string `json:"room_id,omitempty"`
	InviterID string `json:"inviter_id"`
	Locale    string `json:"locale"`
}

type Service struct {
	mu            sync.RWMutex
	statePath     string
	currentUserID string
	users         map[string]User
	byHandle      map[string]string
	rooms         map[string]*Room
}

var mentionPattern = regexp.MustCompile(`(^|[^\w])@([a-zA-Z0-9._-]+)`)

const sessionsDirName = "sessions"

type persistedBootstrap struct {
	CurrentUserID      string          `json:"current_user_id"`
	Users              []User          `json:"users"`
	Rooms              []persistedRoom `json:"rooms,omitempty"`
	InviteDraftUserIDs []string        `json:"invite_draft_user_ids,omitempty"`
}

type persistedRoom struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle"`
	Description  string   `json:"description,omitempty"`
	Participants []string `json:"participants"`
	Messages     string   `json:"messages"`
}

func NewService() *Service {
	return NewServiceFromBootstrap(DefaultBootstrap())
}

func NewServiceFromPath(path string) (*Service, error) {
	state, err := LoadBootstrap(path)
	if err != nil {
		return nil, err
	}
	svc := NewServiceFromBootstrap(state)
	svc.statePath = path
	return svc, nil
}

func NewServiceFromBootstrap(state Bootstrap) *Service {
	state = normalizeBootstrap(state)

	users := state.Users
	rooms := state.Rooms
	svc := &Service{
		currentUserID: state.CurrentUserID,
		users:         make(map[string]User, len(users)),
		byHandle:      make(map[string]string, len(users)),
		rooms:         make(map[string]*Room, len(rooms)),
	}
	for _, user := range users {
		svc.users[user.ID] = user
		svc.byHandle[strings.ToLower(user.Handle)] = user.ID
	}
	for i := range rooms {
		room := rooms[i]
		svc.rooms[room.ID] = &room
	}
	return svc
}

func DefaultBootstrap() Bootstrap {
	return Bootstrap{
		CurrentUserID: "u-admin",
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

func SaveBootstrap(path string, state Bootstrap) error {
	state = normalizeBootstrap(state)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create im state dir: %w", err)
	}

	sessionsDir := filepath.Join(filepath.Dir(path), sessionsDirName)
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return fmt.Errorf("create im sessions dir: %w", err)
	}

	persisted, err := savePersistedBootstrap(path, state)
	if err != nil {
		return err
	}

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
			ID:           room.ID,
			Title:        room.Title,
			Subtitle:     room.Subtitle,
			Description:  room.Description,
			Participants: append([]string(nil), room.Participants...),
			Messages:     messages,
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
	return loadMessagesJSONL(filepath.Join(filepath.Dir(statePath), filepath.FromSlash(relativePath)))
}

func loadMessagesJSONL(path string) ([]Message, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open im session: %w", err)
	}
	defer file.Close()

	var messages []Message
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var message Message
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			return nil, fmt.Errorf("decode im session line: %w", err)
		}
		messages = append(messages, message)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read im session: %w", err)
	}
	return messages, nil
}

func savePersistedBootstrap(statePath string, state Bootstrap) (persistedBootstrap, error) {
	persisted := persistedBootstrap{
		CurrentUserID:      state.CurrentUserID,
		Users:              append([]User(nil), state.Users...),
		InviteDraftUserIDs: append([]string(nil), state.InviteDraftUserIDs...),
	}

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

func savePersistedRooms(statePath string, rooms []Room) ([]persistedRoom, error) {
	if len(rooms) == 0 {
		return nil, nil
	}

	persisted := make([]persistedRoom, 0, len(rooms))
	for _, room := range rooms {
		relativePath := sessionRelativePath(room.ID)
		if err := saveMessagesJSONL(filepath.Join(filepath.Dir(statePath), filepath.FromSlash(relativePath)), room.Messages); err != nil {
			return nil, err
		}
		persisted = append(persisted, persistedRoom{
			ID:           room.ID,
			Title:        room.Title,
			Subtitle:     room.Subtitle,
			Description:  room.Description,
			Participants: append([]string(nil), room.Participants...),
			Messages:     relativePath,
		})
	}
	return persisted, nil
}

func saveMessagesJSONL(path string, messages []Message) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create im session dir: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create im session: %w", err)
	}
	defer file.Close()

	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("encode im session message: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("write im session: %w", err)
		}
		if _, err := io.WriteString(file, "\n"); err != nil {
			return fmt.Errorf("write im session newline: %w", err)
		}
	}
	return nil
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
		if err := os.Remove(filepath.Join(sessionsDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale im session: %w", err)
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
	state.Users = ensureUsers(state.Users)
	state.Rooms = cloneRooms(state.Rooms)
	if !containsUserID(state.Users, state.CurrentUserID) {
		state.CurrentUserID = defaultCurrentUserID(state.Users)
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
			ID:        "u-admin",
			Name:      "admin",
			Handle:    "admin",
			Role:      "admin",
			Avatar:    "AD",
			IsOnline:  true,
			AccentHex: "#dc2626",
		})
	} else {
		for i := range result {
			if strings.EqualFold(strings.TrimSpace(result[i].Handle), "admin") {
				result[i].Name = "admin"
				result[i].Role = "admin"
			}
		}
	}
	if !hasUserHandle(result, "manager") {
		result = append(result, User{
			ID:        "u-manager",
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
				result[i].Name = "manager"
				result[i].Role = "manager"
			}
		}
	}
	return result
}

func normalizeUser(user User) User {
	user.Name = strings.ToLower(strings.TrimSpace(user.Name))
	user.Handle = strings.ToLower(strings.TrimSpace(user.Handle))
	user.Role = strings.ToLower(strings.TrimSpace(user.Role))
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
	for _, preferred := range []string{"u-admin", "u-manager"} {
		if containsUserID(users, preferred) {
			return preferred
		}
	}
	if len(users) > 0 {
		return users[0].ID
	}
	return ""
}

func cloneRooms(rooms []Room) []Room {
	cloned := make([]Room, 0, len(rooms))
	for _, room := range rooms {
		cloned = append(cloned, cloneRoom(room))
	}
	return cloned
}

func ensureAdminManagerRoom(rooms []Room) []Room {
	for _, room := range rooms {
		if containsUserIDInRoom(room, "u-admin") && containsUserIDInRoom(room, "u-manager") {
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
		ID:           fmt.Sprintf("room-%d", now.UnixNano()),
		Title:        "admin & manager",
		Subtitle:     formatConversationSubtitle(2),
		Description:  "Bootstrap room for admin and manager.",
		Participants: []string{"u-admin", "u-manager"},
		Messages: []Message{
			{
				ID:        fmt.Sprintf("msg-%d", now.UnixNano()+1),
				SenderID:  "u-manager",
				Content:   "Bootstrap room created for admin and manager.",
				CreatedAt: now,
			},
		},
	}
	return append(cloneRooms(rooms), room)
}

func containsUserIDInRoom(room Room, userID string) bool {
	for _, participantID := range room.Participants {
		if participantID == userID {
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
		rooms = append(rooms, s.presentRoomLocked(*room))
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, s.presentRoomLocked(*room))
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

func (s *Service) ListMessages(roomID string) ([]Message, error) {
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
	return append([]Message(nil), room.Messages...), nil
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

func (s *Service) KickUser(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return fmt.Errorf("user not found")
	}
	if userID == s.currentUserID {
		return fmt.Errorf("cannot kick current user")
	}

	delete(s.users, userID)
	delete(s.byHandle, strings.ToLower(user.Handle))

	for id, room := range s.rooms {
		participants := make([]string, 0, len(room.Participants))
		for _, participantID := range room.Participants {
			if participantID != userID {
				participants = append(participants, participantID)
			}
		}

		messages := make([]Message, 0, len(room.Messages))
		for _, message := range room.Messages {
			if message.SenderID != userID {
				messages = append(messages, message)
			}
		}

		if len(participants) < 2 {
			delete(s.rooms, id)
			continue
		}

		room.Participants = participants
		room.Messages = messages
		room.Subtitle = formatRoomSubtitle(len(participants))
	}

	return s.saveLocked()
}

func (s *Service) EnsureAgentUser(req EnsureAgentUserRequest) (User, *Room, error) {
	id := strings.TrimSpace(req.ID)
	name := strings.ToLower(strings.TrimSpace(req.Name))
	handle := strings.ToLower(strings.TrimSpace(req.Handle))
	role := strings.ToLower(strings.TrimSpace(req.Role))
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
		ID:        id,
		Name:      name,
		Handle:    handle,
		Role:      role,
		Avatar:    initials(name),
		IsOnline:  true,
		AccentHex: accentHexForID(id),
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

func (s *Service) CreateMessage(req CreateMessageRequest) (Message, error) {
	content := strings.TrimSpace(req.Content)
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return Message{}, fmt.Errorf("room_id is required")
	}
	if req.SenderID == "" {
		return Message{}, fmt.Errorf("sender_id is required")
	}
	if content == "" {
		return Message{}, fmt.Errorf("content is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[req.SenderID]; !ok {
		return Message{}, fmt.Errorf("sender not found")
	}

	room, ok := s.rooms[roomID]
	if !ok {
		return Message{}, fmt.Errorf("room not found")
	}

	message := s.newMessage("", req.SenderID, MessageKindMessage, content)
	room.Messages = append(room.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) DeliverMessage(req DeliverMessageRequest) (Message, error) {
	roomID := strings.TrimSpace(req.RoomID)
	senderID := strings.TrimSpace(req.SenderID)
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
	room, ok := s.rooms[roomID]
	if !ok {
		return Message{}, fmt.Errorf("room not found")
	}

	message := s.newMessage(req.MessageID, senderID, MessageKindMessage, content)
	room.Messages = append(room.Messages, message)
	if err := s.saveLocked(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) CreateRoom(req CreateRoomRequest) (Room, error) {
	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)
	if title == "" {
		return Room{}, fmt.Errorf("title is required")
	}
	if req.CreatorID == "" {
		return Room{}, fmt.Errorf("creator_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[req.CreatorID]; !ok {
		return Room{}, fmt.Errorf("creator not found")
	}

	participants, err := s.normalizeParticipants(req.CreatorID, req.ParticipantIDs)
	if err != nil {
		return Room{}, err
	}

	room := Room{
		ID:           fmt.Sprintf("room-%d", time.Now().UnixNano()),
		Title:        title,
		Subtitle:     formatRoomSubtitle(len(participants)),
		Description:  description,
		Participants: participants,
		Messages: []Message{
			{
				ID:       fmt.Sprintf("msg-%d", time.Now().UnixNano()),
				SenderID: req.CreatorID,
				Kind:     MessageKindEvent,
				Event: &EventPayload{
					Key:     "room_created",
					ActorID: req.CreatorID,
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
	return s.presentRoomLocked(room), nil
}

func (s *Service) CreateConversation(req CreateConversationRequest) (Conversation, error) {
	return s.CreateRoom(req)
}

func (s *Service) AddRoomMembers(req AddRoomMembersRequest) (Room, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return Room{}, fmt.Errorf("room_id is required")
	}
	if req.InviterID == "" {
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
	if _, ok := s.users[req.InviterID]; !ok {
		return Room{}, fmt.Errorf("inviter not found")
	}
	if !slices.Contains(room.Participants, req.InviterID) {
		return Room{}, fmt.Errorf("inviter is not a room member")
	}

	existing := make(map[string]struct{}, len(room.Participants))
	for _, id := range room.Participants {
		existing[id] = struct{}{}
	}

	addedIDs := make([]string, 0, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		userID = strings.TrimSpace(userID)
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
		room.Participants = append(room.Participants, userID)
		addedIDs = append(addedIDs, userID)
	}
	if len(addedIDs) == 0 {
		return Room{}, fmt.Errorf("no new users to invite")
	}

	room.Subtitle = formatRoomSubtitle(len(room.Participants))
	room.Messages = append(room.Messages, Message{
		ID:       fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SenderID: req.InviterID,
		Kind:     MessageKindEvent,
		Event: &EventPayload{
			Key:       "room_members_added",
			ActorID:   req.InviterID,
			TargetIDs: append([]string(nil), addedIDs...),
		},
		CreatedAt: time.Now().UTC(),
		Mentions:  append([]string(nil), addedIDs...),
	})
	if err := s.saveLocked(); err != nil {
		return Room{}, err
	}

	return s.presentRoomLocked(*room), nil
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
	return s.presentRoomLocked(*room), true
}

func (s *Service) Conversation(conversationID string) (Conversation, bool) {
	return s.Room(conversationID)
}

func (s *Service) User(userID string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	return user, ok
}

func (s *Service) extractMentions(content string) []string {
	matches := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	mentions := make([]string, 0, len(matches))
	for _, match := range matches {
		handle := strings.ToLower(match[2])
		if userID, ok := s.byHandle[handle]; ok {
			if _, exists := seen[userID]; exists {
				continue
			}
			seen[userID] = struct{}{}
			mentions = append(mentions, userID)
		}
	}
	return mentions
}

func (s *Service) normalizeParticipants(creatorID string, participantIDs []string) ([]string, error) {
	seen := map[string]struct{}{creatorID: {}}
	participants := []string{creatorID}
	for _, userID := range participantIDs {
		userID = strings.TrimSpace(userID)
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
		participants = append(participants, userID)
	}
	return participants, nil
}

const (
	MessageKindMessage = "message"
	MessageKindEvent   = "event"
)

func (s *Service) localizeSystemText(locale, key, actorID, title string, userIDs []string) string {
	actor := s.userDisplayName(actorID)
	targets := s.userDisplayNames(userIDs)
	switch normalizeLocale(locale) {
	case "en":
		switch key {
		case "room_created":
			return fmt.Sprintf("%s created the room \"%s\"", actor, title)
		case "room_members_added":
			return fmt.Sprintf("%s invited %s to join the room", actor, strings.Join(targets, ", "))
		}
	default:
		switch key {
		case "room_created":
			return fmt.Sprintf("%s 创建了房间“%s”", actor, title)
		case "room_members_added":
			return fmt.Sprintf("%s 邀请 %s 加入了房间", actor, strings.Join(targets, "、"))
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

func (s *Service) presentRoomLocked(room Room) Room {
	cloned := cloneRoom(room)
	if len(cloned.Participants) != 2 {
		return cloned
	}

	otherID := cloned.Participants[0]
	if otherID == s.currentUserID {
		otherID = cloned.Participants[1]
	}
	if user, ok := s.users[otherID]; ok && strings.TrimSpace(user.Name) != "" {
		cloned.Title = user.Name
	}
	return cloned
}

func (s *Service) presentConversationLocked(conv Conversation) Conversation {
	return s.presentRoomLocked(conv)
}

func cloneRoom(room Room) Room {
	cloned := room
	cloned.Participants = append([]string(nil), room.Participants...)
	cloned.Messages = append([]Message(nil), room.Messages...)
	return cloned
}

func cloneConversation(conv Conversation) Conversation {
	return cloneRoom(conv)
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

func (s *Service) saveLocked() error {
	if s.statePath == "" {
		return nil
	}
	return SaveBootstrap(s.statePath, s.bootstrapLocked())
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
		if len(room.Participants) != 2 {
			continue
		}
		if containsUserIDInRoom(*room, "u-admin") && containsUserIDInRoom(*room, agentID) {
			presented := s.presentRoomLocked(*room)
			return &presented, false
		}
	}

	now := time.Now().UTC()
	room := Room{
		ID:           fmt.Sprintf("room-%d", now.UnixNano()),
		Title:        agentName,
		Subtitle:     formatRoomSubtitle(2),
		Description:  fmt.Sprintf("Bootstrap room for admin and %s.", agentName),
		Participants: []string{"u-admin", agentID},
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
	presented := s.presentRoomLocked(room)
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
