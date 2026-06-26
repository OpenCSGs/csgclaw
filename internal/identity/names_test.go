package identity

import "testing"

func TestValidateMentionNameAcceptsUnicodeMentionSafeNames(t *testing.T) {
	for _, name := range []string{"qa", "qa.bot", "测试工程师", "agent_1"} {
		if err := ValidateMentionName(name); err != nil {
			t.Fatalf("ValidateMentionName(%q) error = %v", name, err)
		}
	}
}

func TestValidateMentionNameRejectsUnsafeNames(t *testing.T) {
	longName := "abcdefghijklmnopqrstuvwxyzabcdefg"
	for _, name := range []string{"", "qa bot", "@qa", "qa/bot", "<qa>", "qa😀", longName} {
		if err := ValidateMentionName(name); err == nil {
			t.Fatalf("ValidateMentionName(%q) error = nil, want error", name)
		}
	}
}
