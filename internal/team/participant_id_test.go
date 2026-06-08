package team

import "testing"

func TestCleanBotIDDoesNotInferCanonicalID(t *testing.T) {
	if got := cleanParticipantID(" p-w-0604 "); got != "p-w-0604" {
		t.Fatalf("cleanParticipantID() = %q, want p-w-0604", got)
	}
	if ParticipantIDsMatch("u-p-w-0604", "p-w-0604") {
		t.Fatal("ParticipantIDsMatch() = true, want false for distinct stored ids")
	}
}
