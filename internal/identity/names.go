package identity

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxMentionNameRunes = 32

func ValidateMentionName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if utf8.RuneCountInString(name) > MaxMentionNameRunes {
		return fmt.Errorf("name must be at most %d characters", MaxMentionNameRunes)
	}
	var first, last rune
	count := 0
	for _, r := range name {
		if !validMentionNameRune(r, count == 0) {
			return fmt.Errorf("name %q contains unsupported character %q", name, r)
		}
		if count == 0 {
			first = r
		}
		last = r
		count++
	}
	if !isMentionNameBoundary(first) || !isMentionNameBoundary(last) {
		return fmt.Errorf("name must start and end with a letter or number")
	}
	return nil
}

func validMentionNameRune(r rune, _ bool) bool {
	if unicode.IsControl(r) || unicode.IsSpace(r) {
		return false
	}
	if unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) {
		return true
	}
	switch r {
	case '.', '_', '-':
		return true
	default:
		return false
	}
}

func isMentionNameBoundary(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}
