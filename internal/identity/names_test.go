package identity

import (
	"strings"
	"testing"
)

func TestValidateMentionNameAcceptsUnicodeMentionSafeNames(t *testing.T) {
	for _, name := range []string{"qa", "qa.bot", "测试工程师", "agent_1", strings.Repeat("界", 64)} {
		if err := ValidateMentionName(name); err != nil {
			t.Fatalf("ValidateMentionName(%q) error = %v", name, err)
		}
	}
}

func TestValidateMentionNameRejectsUnsafeNames(t *testing.T) {
	longName := strings.Repeat("界", 65)
	for _, name := range []string{"", "qa bot", "@qa", "qa/bot", "<qa>", "qa😀", longName} {
		if err := ValidateMentionName(name); err == nil {
			t.Fatalf("ValidateMentionName(%q) error = nil, want error", name)
		}
	}
}
