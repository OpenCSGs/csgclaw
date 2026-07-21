package participant

import (
	"fmt"
	"testing"

	"csgclaw/internal/im"
)

func TestDefaultParticipantAvatarSelectsAnUnusedBuiltInAvatar(t *testing.T) {
	imSvc := im.NewService()
	for index, avatar := range builtInAvatarOptions[:len(builtInAvatarOptions)-1] {
		name := fmt.Sprintf("avatar-user-%d", index)
		if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
			ID: name, Name: name, Role: "worker", Avatar: avatar,
		}); err != nil {
			t.Fatalf("EnsureAgentUser(%d) error = %v", index, err)
		}
	}
	svc := NewService(NewMemoryStore(nil), WithIMService(imSvc))

	if got, want := svc.defaultParticipantAvatar(""), builtInAvatarOptions[len(builtInAvatarOptions)-1]; got != want {
		t.Fatalf("defaultParticipantAvatar() = %q, want only unused avatar %q", got, want)
	}
}

func TestDefaultParticipantAvatarPreservesExplicitAvatar(t *testing.T) {
	svc := NewService(NewMemoryStore(nil), WithIMService(im.NewService()))
	if got, want := svc.defaultParticipantAvatar(" custom-avatar "), "custom-avatar"; got != want {
		t.Fatalf("defaultParticipantAvatar(explicit) = %q, want %q", got, want)
	}
}
