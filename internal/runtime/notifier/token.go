package notifier

import "crypto/subtle"

// SecretMatch compares secrets in constant time when lengths match (recommended: fixed-length random tokens).
func SecretMatch(expected, got string) bool {
	if len(expected) == 0 || len(got) == 0 {
		return false
	}
	if len(expected) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}
