package im

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var ErrAgentIdentityDeleted = errors.New("agent identity was deleted; use a new ID")

// ValidateAgentUser checks identity availability before provisioning a runtime.
// Deleted identities remain addressable by ID for history, but do not own names.
func (s *Service) ValidateAgentUser(req EnsureAgentUserRequest) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validateAgentUserLocked(canonicalIMUserID(req.ID), strings.TrimSpace(req.Name))
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
		if existingID, ok := s.byName[strings.ToLower(name)]; ok && existingID != id {
			return fmt.Errorf("name %q already exists", name)
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
func (s *Service) SetAgentStateLookup(lookup func(string) string) {
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
		return s.agentState(user.ID)
	}
	return user.AgentState
}
func (s *Service) presentUserLocked(user User) User {
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
	id = s.resolveUserIDLocked(id)
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
