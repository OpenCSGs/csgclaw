package feedback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"

	"csgclaw/internal/localstore"
)

const attemptsSection = "submissions"

const (
	AttemptNotFound  = "not_found"
	AttemptUnknown   = "unknown"
	AttemptSubmitted = "submitted"
)

type Attempt struct {
	State   string  `json:"state"`
	Receipt *Result `json:"receipt,omitempty"`
}

var attemptIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func ValidAttemptID(id string) bool { return attemptIDPattern.MatchString(id) }

type Attempts struct {
	Path string
}

func attemptKey(site, account, id string) string {
	hash := sha256.Sum256([]byte(site + "\x00" + account + "\x00" + id))
	return hex.EncodeToString(hash[:])
}

func (s Attempts) Status(site, account, id string) (Attempt, error) {
	var entries map[string]Attempt
	if _, err := localstore.ReadSection(s.Path, attemptsSection, &entries); err != nil {
		return Attempt{}, err
	}
	if attempt, ok := entries[attemptKey(site, account, id)]; ok {
		return attempt, nil
	}
	return Attempt{State: AttemptNotFound}, nil
}

// Begin reserves an ID before any upstream request. An interrupted reservation
// is deliberately treated as unknown, even after a server restart.
func (s Attempts) Begin(site, account, id string) (Attempt, bool, error) {
	key := attemptKey(site, account, id)
	var attempt Attempt
	var reserved bool
	err := localstore.UpdateObjectSection(s.Path, attemptsSection, func(entries map[string]json.RawMessage) error {
		if raw, ok := entries[key]; ok {
			return json.Unmarshal(raw, &attempt)
		}
		attempt = Attempt{State: AttemptUnknown}
		data, err := json.Marshal(attempt)
		if err != nil {
			return err
		}
		entries[key] = data
		reserved = true
		return nil
	})
	return attempt, reserved, err
}

// Finish stores a confirmed receipt, or releases the reservation when the
// upstream issue request was definitely not accepted.
func (s Attempts) Finish(site, account, id string, receipt *Result) error {
	key := attemptKey(site, account, id)
	return localstore.UpdateObjectSection(s.Path, attemptsSection, func(entries map[string]json.RawMessage) error {
		if receipt == nil {
			delete(entries, key)
			return nil
		}
		data, err := json.Marshal(Attempt{State: AttemptSubmitted, Receipt: receipt})
		if err != nil {
			return err
		}
		entries[key] = data
		return nil
	})
}
