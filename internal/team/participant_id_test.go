package team

import "testing"

func TestCleanParticipantIDCanonicalizesToTypedParticipantID(t *testing.T) {
	if got := cleanParticipantID(" p-w-0604 "); got != "pt-p-w-0604" {
		t.Fatalf("cleanParticipantID() = %q, want pt-p-w-0604", got)
	}
	if !ParticipantIDsMatch("u-p-w-0604", "p-w-0604") {
		t.Fatal("ParticipantIDsMatch() = false, want true for legacy alias")
	}
}

func TestRequireCanonicalParticipantIDAcceptsLegacyAlias(t *testing.T) {
	got, err := requireCanonicalParticipantID("participant_id", "u-p-w-0604")
	if err != nil {
		t.Fatalf("requireCanonicalParticipantID() error = %v", err)
	}
	if got != "pt-p-w-0604" {
		t.Fatalf("requireCanonicalParticipantID() = %q, want pt-p-w-0604", got)
	}
}
