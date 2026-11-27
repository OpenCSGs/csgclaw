package im

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/identity"
)

var ErrAgentIdentityDeleted = errors.New("agent identity was deleted; use a new ID")

// HoldAgentUserName keeps the current name available for rollback while the
// participant and its worker are being renamed in their respective stores.
func (s *Service) HoldAgentUserName(id, name string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = canonicalIMUserID(id)
	user, ok := s.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	if user.AgentState == "deleted" {
		return nil, ErrAgentIdentityDeleted
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if user.Name != name {
		return nil, fmt.Errorf("IM user %q name changed", id)
	}
	return s.holdNameLocked(id, key)
}

// ReserveAgentUserName protects a worker name while updating a standalone
// worker or one with only aliases. An empty ID reserves an unowned name.
func (s *Service) ReserveAgentUserName(id, name string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = canonicalIMUserID(id)
	if user, ok := s.users[s.resolveUserIDLocked(name)]; ok && user.ID != id {
		return nil, fmt.Errorf("name %q %w", name, identity.ErrNameExists)
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if owner, ok := s.byName[key]; ok && owner != id {
		return nil, fmt.Errorf("name %q %w", name, identity.ErrNameExists)
	}
	return s.holdNameLocked(id, key)
}

func (s *Service) holdNameLocked(id, key string) (func(), error) {
	if _, held := s.heldNames[key]; held {
		return nil, fmt.Errorf("name %q %w", key, identity.ErrNameExists)
	}
	if s.heldNames == nil {
		s.heldNames = make(map[string]string)
	}
	s.heldNames[key] = id
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.heldNames, key)
	}, nil
}

func (s *Service) nameOwnerLocked(name string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if owner, ok := s.heldNames[key]; ok {
		return owner, true
	}
	owner, ok := s.byName[key]
	return owner, ok
}

// ValidateAgentUser checks identity availability before provisioning a runtime.
// Deleted identities remain addressable by ID for history, but do not own names.
func (s *Service) ValidateAgentUser(req EnsureAgentUserRequest) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validateAgentUserLocked(canonicalIMUserID(req.ID), strings.TrimSpace(req.Name))
}

// ValidateAgentReplacement checks an existing runtime against its actual chat
// identities. Runtime IDs and chat user IDs are separate namespaces.
func (s *Service) ValidateAgentReplacement(agentID string, userIDs []string, name string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.AgentState == "deleted" && user.AgentID != "" && canonicalIMUserID(user.AgentID) == canonicalIMUserID(agentID) {
			return ErrAgentIdentityDeleted
		}
	}
	for _, id := range userIDs {
		if user, ok := s.users[id]; ok && user.AgentState == "deleted" {
			return ErrAgentIdentityDeleted
		}
	}
	if owner, ok := s.nameOwnerLocked(name); ok && !slices.Contains(userIDs, owner) {
		return fmt.Errorf("name %q already exists", name)
	}
	return nil
}

func (s *Service) validateAgentUserLocked(id, name string) error {
	if user, ok := s.users[id]; ok && user.AgentState == "deleted" {
		return ErrAgentIdentityDeleted
	}
	if id != "" {
		for _, user := range s.users {
			if user.AgentState == "deleted" && user.AgentID != "" && canonicalIMUserID(user.AgentID) == id {
				return ErrAgentIdentityDeleted
			}
		}
	}
	if name != "" {
		if existingID, ok := s.nameOwnerLocked(name); ok && existingID != id {
			return fmt.Errorf("name %q %w", name, identity.ErrNameExists)
		}
	}
	return nil
}

const (
	AgentArchivedEventKey = "agent_archived"
	AgentRestoredEventKey = "agent_restored"
	AgentDeletedEventKey  = "agent_deleted"
)

// RecordAgentLifecycle keeps the notice in existing collaboration rooms only.
// Raw membership is intentional: archived members are hidden by the read view.
func (s *Service) RecordAgentLifecycle(id, key, transition string) error {
	if key != AgentArchivedEventKey && key != AgentRestoredEventKey {
		return fmt.Errorf("unsupported agent lifecycle event %q", key)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.userForLocalIDLocked(id)
	if !ok {
		return nil
	}
	previous := map[string][]Message{}
	added := map[string]Message{}
	for roomID, room := range s.rooms {
		if room.IsDirect || !slices.Contains(room.Members, user.ID) {
			continue
		}
		previous[roomID] = room.Messages
		if message, appended := s.appendAgentLifecycleEventLocked(room, user, key, transition); appended {
			added[roomID] = message
		}
	}
	if len(added) == 0 {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		for roomID, messages := range previous {
			s.rooms[roomID].Messages = messages
		}
		return err
	}
	for roomID, message := range added {
		s.publishMessageCreatedLocked(roomID, message.SenderID, s.presentMessageLocked(*s.rooms[roomID], message, ""))
	}
	return nil
}

func (s *Service) appendAgentLifecycleEventLocked(room *Room, user User, key, transition string) (Message, bool) {
	digest := sha256.Sum256([]byte(room.ID + "\x00" + user.ID + "\x00" + key + "\x00" + transition))
	id := fmt.Sprintf("msg-agent-%x", digest[:16])
	for _, message := range room.Messages {
		if message.ID == id {
			return Message{}, false
		}
	}
	message := s.newMessage(id, s.currentUserID, MessageKindEvent, "")
	message.Event = &EventPayload{Key: key, ActorID: s.currentUserID, TargetIDs: []string{user.ID}}
	room.Messages = append(room.Messages, message)
	return message, true
}

// SetAgentStateLookup installs a read-only identity lookup. It must not call IM
// methods: it runs while the IM read/write lock is held.
func (s *Service) SetAgentStateLookup(lookup func(User) string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentState = lookup
}
func (s *Service) agentStateLocked(id string) string {
	user, ok := s.userForLocalIDLocked(id)
	if !ok {
		return ""
	}
	if user.AgentState == "deleted" {
		return "deleted"
	}
	if s.agentState != nil {
		return s.agentState(user)
	}
	return user.AgentState
}

// SetLegacyNotifierLookup installs an identity lookup that must not call IM.
func (s *Service) SetLegacyNotifierLookup(lookup func(User) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.legacyNotifier = lookup
}

func (s *Service) isLegacyNotifierLocked(user User) bool {
	return strings.EqualFold(strings.TrimSpace(user.Role), "notifier") ||
		(s.legacyNotifier != nil && s.legacyNotifier(user))
}

func (s *Service) presentUserLocked(user User) User {
	user.LegacyNotifier = s.isLegacyNotifierLocked(user)
	user.AgentState = s.agentStateLocked(user.ID)
	if user.AgentState != "" {
		user.IsOnline = false
	}
	return user
}
func (s *Service) hiddenAgentRoomLocked(room Room) bool {
	if !room.IsDirect {
		return false
	}
	for _, id := range room.Members {
		if s.agentStateLocked(id) != "" {
			return true
		}
	}
	return false
}

// RetireAgentUser retains attribution and collaboration messages/attachments,
// removes memberships, and removes direct conversations only.
func (s *Service) RetireAgentUser(id string, agentIDs ...string) error {
	s.mu.Lock()
	// The caller supplies a bound user ID. Never fall back to a display-name
	// lookup: a missing identity must not retire a different user with that name.
	id = canonicalIMUserID(id)
	user, ok := s.users[id]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	if user.AgentState == "deleted" {
		s.mu.Unlock()
		return nil
	}
	if id == s.currentUserID || id == ManagerUserID {
		s.mu.Unlock()
		return fmt.Errorf("cannot retire protected user")
	}
	previousUser := user
	previousRooms := make(map[string]*Room, len(s.rooms))
	for key, room := range s.rooms {
		copy := cloneRoom(*room)
		previousRooms[key] = &copy
	}
	user.AgentState = "deleted"
	user.AgentID = "agent-" + strings.TrimPrefix(id, "user-")
	if len(agentIDs) > 0 && strings.TrimSpace(agentIDs[0]) != "" {
		user.AgentID = strings.TrimSpace(agentIDs[0])
	}
	now := time.Now().UTC()
	user.DeletedAt = &now
	user.IsOnline = false
	s.users[id] = user
	nameKey := strings.ToLower(strings.TrimSpace(user.Name))
	if s.byName[nameKey] == id {
		delete(s.byName, nameKey)
	}
	added := map[string]Message{}
	for key, room := range s.rooms {
		if !slices.Contains(room.Members, id) {
			continue
		}
		if room.IsDirect {
			delete(s.rooms, key)
			continue
		}
		if message, appended := s.appendAgentLifecycleEventLocked(room, user, AgentDeletedEventKey, "deleted"); appended {
			added[key] = message
		}
		room.Members = slices.DeleteFunc(slices.Clone(room.Members), func(member string) bool { return canonicalIMUserID(member) == id })
		room.Subtitle = formatRoomSubtitle(len(room.Members))
	}
	s.rebuildSessionRoomIndexLocked()
	if err := s.saveLocked(); err != nil {
		s.users[id] = previousUser
		s.byName[nameKey] = id
		s.rooms = previousRooms
		s.rebuildSessionRoomIndexLocked()
		s.mu.Unlock()
		return err
	}
	for roomID, message := range added {
		s.publishMessageCreatedLocked(roomID, message.SenderID, s.presentMessageLocked(*s.rooms[roomID], message, ""))
	}
	bus := s.bus
	s.mu.Unlock()
	if bus != nil {
		bus.Publish(Event{Type: EventTypeUserUpdated, User: &user})
	}
	return nil
}

func (s *Service) PublishAgentState(id string) {
	s.mu.RLock()
	user, ok := s.userForLocalIDLocked(id)
	if ok {
		user = s.presentUserLocked(user)
	}
	bus := s.bus
	s.mu.RUnlock()
	if ok && bus != nil {
		bus.Publish(Event{Type: EventTypeUserUpdated, User: &user})
	}
}
