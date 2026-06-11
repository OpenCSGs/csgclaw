package im

import "strings"

// ResolveUserID returns the canonical IM user id for agent identities.
func (s *Service) ResolveUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" || s == nil {
		return userID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resolveUserIDLocked(userID)
}

func (s *Service) resolveUserIDLocked(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" || s == nil {
		return userID
	}
	if userID == legacyAdminUserID {
		if _, ok := s.users[adminUserID]; ok {
			return adminUserID
		}
	}
	if _, ok := s.users[userID]; ok {
		return userID
	}
	handle := strings.ToLower(strings.TrimPrefix(userID, "@"))
	if resolved, ok := s.byHandle[handle]; ok {
		return resolved
	}
	return userID
}

func (s *Service) resolveRoomUserIDLocked(userID string) string {
	userID = s.resolveUserIDLocked(userID)
	if userID == legacyManagerUserID {
		if _, ok := s.users[managerParticipantUserID]; ok {
			return managerParticipantUserID
		}
	}
	return userID
}
