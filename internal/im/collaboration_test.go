package im

import (
	"testing"

	"csgclaw/internal/apitypes"
)

func TestOnDemandHumanIntakeUsesSenderIdentity(t *testing.T) {
	room := Room{ID: "room-test", Type: apitypes.RoomTypeOnDemand, ManagerID: "u-manager", Members: []string{"u-person", "u-manager"}}
	message := Message{SenderID: "u-person", Kind: MessageKindMessage}
	for _, role := range []string{"admin", "human", "user", "worker", "manager", ""} {
		t.Run(role, func(t *testing.T) {
			sender := User{ID: "u-person", Role: role}
			want := role == "admin" || role == "human" || role == "user"
			if got := shouldNotifyParticipant(room, sender, message, "pt-manager"); got != want {
				t.Fatalf("role %q: got %v, want %v", role, got, want)
			}
		})
	}
	for _, sender := range []User{{ID: "u-someone-else", Role: "human"}, {ID: "u-outsider", Role: "human"}} {
		if onDemandHumanMessage(room, sender, message) {
			t.Fatal("mismatched sender was treated as human intake")
		}
	}
	message.SenderID = "u-outsider"
	if onDemandHumanMessage(room, User{ID: "u-outsider", Role: "human"}, message) {
		t.Fatal("non-member was treated as human intake")
	}
}

func TestOnDemandRouting(t *testing.T) {
	room := Room{ID: "room-test", Type: apitypes.RoomTypeOnDemand, ManagerID: "u-manager", Members: []string{"u-admin", "u-manager", "u-dev", "u-qa"}, NotifyAllAgents: true}
	cases := []struct {
		name             string
		message          Message
		manager, dev, qa bool
	}{
		{"unmentioned user", Message{SenderID: "u-admin", Kind: MessageKindMessage}, true, false, false},
		{"multiple mentions", Message{SenderID: "u-admin", Kind: MessageKindMessage, Mentions: []Mention{{ID: "u-dev"}, {ID: "u-qa"}}}, true, false, false},
		{"manager prose mention", Message{SenderID: "u-manager", Kind: MessageKindMessage, Mentions: []Mention{{ID: "u-dev"}}}, false, false, false},
		{"manager scoped relay", Message{SenderID: "u-manager", Kind: MessageKindMessage, Mentions: []Mention{{ID: "u-dev"}}, Metadata: map[string]any{"task_id": "task-1"}}, false, true, false},
		{"worker asks QA", Message{SenderID: "u-dev", Kind: MessageKindMessage, Mentions: []Mention{{ID: "u-qa"}}}, true, false, false},
		{"user mentions manager", Message{SenderID: "u-admin", Kind: MessageKindMessage, Mentions: []Mention{{ID: "u-manager"}}}, true, false, false},
		{"worker mentions manager", Message{SenderID: "u-dev", Kind: MessageKindMessage, Mentions: []Mention{{ID: "u-manager"}}}, true, false, false},
		{"worker final without mention", Message{SenderID: "u-dev", Kind: MessageKindMessage, Content: "Done"}, false, false, false},
		{"tool envelope with mention", Message{SenderID: "u-dev", Kind: MessageKindMessage, Content: `{"type":"com.opencsg.csgclaw.agent.activity"}`, Mentions: []Mention{{ID: "u-manager"}}}, false, false, false},
		{"thought with mention", Message{SenderID: "u-dev", Kind: MessageKindMessage, Metadata: map[string]any{"codex": map[string]any{"delivery_kind": "thought"}}, Mentions: []Mention{{ID: "u-manager"}}}, false, false, false},
		{"dispatch", Message{SenderID: "u-manager", Kind: MessageKindEvent, Event: &EventPayload{Key: "task_assigned"}, Mentions: []Mention{{ID: "u-dev"}}}, false, true, false},
		{"activity", Message{SenderID: "u-dev", Kind: MessageKindEvent, Event: &EventPayload{Key: "agent_activity"}}, false, false, false},
		{"feedback", Message{SenderID: "u-admin", Kind: MessageKindEvent, Event: &EventPayload{Key: "task_feedback"}, Mentions: []Mention{{ID: "u-manager"}}}, true, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := User{ID: tc.message.SenderID, Role: "worker"}
			if tc.message.SenderID == "u-admin" {
				sender.Role = "admin"
			}
			if tc.message.SenderID == "u-manager" {
				sender.Role = "manager"
			}
			for i, id := range []string{"pt-manager", "pt-dev", "pt-qa"} {
				want := []bool{tc.manager, tc.dev, tc.qa}[i]
				if got := shouldNotifyParticipant(room, sender, tc.message, id); got != want {
					t.Fatalf("%s: got %v want %v", id, got, want)
				}
			}
		})
	}
}
