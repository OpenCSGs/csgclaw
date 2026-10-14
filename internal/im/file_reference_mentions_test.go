package im

import "testing"

func TestQuotedFilePathsNeverMentionParticipants(t *testing.T) {
	s := NewService()
	_, room, err := s.EnsureAgentUser(EnsureAgentUserRequest{ID: "agent-alice", Name: "alice", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{`Read @"alice"`, `Read @"alice/report.md"`, `Read @"docs/@alice/report.md"`, `Read @"docs/quote\" @alice.md"`} {
		message, err := s.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: content})
		if err != nil || len(message.Mentions) != 0 {
			t.Fatalf("file path became a participant: %q %+v %v", content, message.Mentions, err)
		}
	}
	message, err := s.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: `@alice Read @"alice/report.md"`})
	if err != nil || len(message.Mentions) != 1 {
		t.Fatalf("lost real participant mention: %+v %v", message.Mentions, err)
	}
}
