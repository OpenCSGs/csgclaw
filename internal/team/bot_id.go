package team

import (
	"fmt"
	"strings"
)

func cleanBotID(id string) string {
	return strings.TrimSpace(id)
}

func requireCanonicalBotID(field, id string) (string, error) {
	id = cleanBotID(id)
	if id == "" {
		return "", nil
	}
	if !strings.HasPrefix(id, "u-") {
		return "", invalidCanonicalBotIDError(field, id)
	}
	return id, nil
}

func invalidCanonicalBotIDError(field, id string) error {
	return fmt.Errorf("%s must be a canonical user id starting with u-: %s", field, id)
}

// BotIDsMatch reports whether two bot ids refer to the same stored user id.
func BotIDsMatch(left, right string) bool {
	left = cleanBotID(left)
	right = cleanBotID(right)
	return left != "" && left == right
}
