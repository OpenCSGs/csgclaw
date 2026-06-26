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
	if userID == legacyAdminUserID || userID == legacyBareAdminUserID {
		if _, ok := s.users[adminUserID]; ok {
			return adminUserID
		}
	}
	if _, ok := s.users[userID]; ok {
		return userID
	}
	if canonical := canonicalIMUserID(userID); canonical != userID {
		if _, ok := s.users[canonical]; ok {
			return canonical
		}
	}
	name := strings.ToLower(strings.TrimPrefix(userID, "@"))
	if resolved, ok := s.byName[name]; ok {
		return resolved
	}
	return userID
}

func (s *Service) resolveRoomUserIDLocked(userID string) string {
	userID = s.resolveUserIDLocked(userID)
	if userID == legacyManagerUserID || userID == legacyManagerParticipantID || userID == legacyTypedManagerParticipantID {
		if _, ok := s.users[managerParticipantUserID]; ok {
			return managerParticipantUserID
		}
	}
	return userID
}

func canonicalIMUserID(id string) string {
	id = strings.TrimSpace(id)
	switch id {
	case "":
		return ""
	case legacyBareAdminUserID, legacyAdminUserID, adminParticipantID:
		return adminUserID
	case legacyManagerParticipantID, legacyTypedManagerParticipantID, legacyManagerUserID:
		return managerParticipantUserID
	}
	if strings.HasPrefix(id, "user-") {
		return id
	}
	if suffix := trimLocalIdentityPrefixes(id); suffix != "" {
		return "user-" + suffix
	}
	return "user-" + id
}

func canonicalIMParticipantID(id string) string {
	id = strings.TrimSpace(id)
	switch id {
	case "":
		return ""
	case legacyBareAdminUserID, legacyAdminUserID, adminUserID:
		return adminParticipantID
	case legacyManagerParticipantID, legacyTypedManagerParticipantID, legacyManagerUserID, managerParticipantUserID:
		return managerParticipantID
	}
	if strings.HasPrefix(id, "pt-") {
		return id
	}
	if suffix := trimLocalIdentityPrefixes(id); suffix != "" {
		return "pt-" + suffix
	}
	return "pt-" + id
}

func trimLocalIdentityPrefixes(id string) string {
	id = strings.TrimSpace(id)
	for {
		next := id
		for _, prefix := range []string{"user-", "agent-", "pt-", "u-"} {
			if strings.HasPrefix(next, prefix) {
				next = strings.TrimPrefix(next, prefix)
				break
			}
		}
		if next == id {
			break
		}
		id = next
	}
	return strings.TrimSpace(id)
}

func participantIDForUserID(userID string) string {
	return canonicalIMParticipantID(canonicalIMUserID(userID))
}

func userIDForParticipantID(participantID string) string {
	return canonicalIMUserID(canonicalIMParticipantID(participantID))
}

func (s *Service) resolveParticipantIDLocked(id string) string {
	if s == nil {
		return canonicalIMParticipantID(id)
	}
	if userID := s.resolveUserIDLocked(id); userID != "" {
		if _, ok := s.users[userID]; ok {
			return participantIDForUserID(userID)
		}
	}
	return canonicalIMParticipantID(id)
}

func (s *Service) userForParticipantLocked(participantID string) (User, bool) {
	if s == nil {
		return User{}, false
	}
	userID := userIDForParticipantID(participantID)
	return s.userForLocalIDLocked(userID)
}

func (s *Service) userForLocalIDLocked(id string) (User, bool) {
	if s == nil {
		return User{}, false
	}
	userID := canonicalIMUserID(id)
	if user, ok := s.users[userID]; ok {
		return user, true
	}
	for _, alias := range participantUserLookupAliases(id) {
		if user, ok := s.users[alias]; ok {
			return user, true
		}
	}
	return User{}, false
}

func participantUserLookupAliases(participantID string) []string {
	participantID = strings.TrimSpace(participantID)
	rawSuffix := strings.TrimPrefix(participantID, "pt-")
	rawSuffix = strings.TrimPrefix(rawSuffix, "user-")
	rawSuffix = strings.TrimPrefix(rawSuffix, "u-")
	suffix := trimLocalIdentityPrefixes(participantID)
	aliases := []string{
		participantID,
		rawSuffix,
		suffix,
		"u-" + suffix,
		canonicalIMUserID(suffix),
		canonicalIMParticipantID(suffix),
	}
	if rawSuffix != suffix {
		aliases = append(aliases,
			"u-"+rawSuffix,
			"user-"+rawSuffix,
			canonicalIMUserID(rawSuffix),
			canonicalIMParticipantID(rawSuffix),
		)
	}
	if base, ok := trimStableHashSuffix(suffix); ok {
		aliases = append(aliases,
			"pt-"+base,
			base,
			"u-"+base,
			"user-"+base,
			"user-agent-"+base,
			userIDForParticipantID("pt-"+base),
		)
	}
	if base, ok := trimStableHashSuffix(rawSuffix); ok {
		aliases = append(aliases,
			"pt-"+base,
			base,
			"u-"+base,
			"user-"+base,
			userIDForParticipantID("pt-"+base),
		)
	}
	return aliases
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
