package team

import (
	"fmt"
	"strings"
)

func cleanParticipantID(id string) string {
	return strings.TrimSpace(id)
}

func requireCanonicalParticipantID(field, id string) (string, error) {
	id = cleanParticipantID(id)
	if id == "" {
		return "", nil
	}
	if !strings.HasPrefix(id, "u-") {
		return "", invalidCanonicalParticipantIDError(field, id)
	}
	return id, nil
}

func invalidCanonicalParticipantIDError(field, id string) error {
	return fmt.Errorf("%s must be a canonical user id starting with u-: %s", field, id)
}

// ParticipantIDsMatch reports whether two participant ids refer to the same stored user id.
func ParticipantIDsMatch(left, right string) bool {
	left = cleanParticipantID(left)
	right = cleanParticipantID(right)
	return left != "" && left == right
}
