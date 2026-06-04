package team

import "testing"

func TestCleanBotIDDoesNotInferCanonicalID(t *testing.T) {
	if got := cleanBotID(" p-w-0604 "); got != "p-w-0604" {
		t.Fatalf("cleanBotID() = %q, want p-w-0604", got)
	}
	if BotIDsMatch("u-p-w-0604", "p-w-0604") {
		t.Fatal("BotIDsMatch() = true, want false for distinct stored ids")
	}
}
