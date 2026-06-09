package team

import "testing"

func TestCleanParticipantIDDoesNotInferUserID(t *testing.T) {
	if got := cleanParticipantID(" p-w-0604 "); got != "p-w-0604" {
		t.Fatalf("cleanParticipantID() = %q, want p-w-0604", got)
	}
	if ParticipantIDsMatch("u-p-w-0604", "p-w-0604") {
		t.Fatal("ParticipantIDsMatch() = true, want false for distinct stored ids")
	}
}

func TestRequireCanonicalParticipantIDRejectsLegacyUserID(t *testing.T) {
	if _, err := requireCanonicalParticipantID("participant_id", "u-p-w-0604"); err == nil {
		t.Fatal("requireCanonicalParticipantID() error = nil, want legacy user id rejection")
	}
}
