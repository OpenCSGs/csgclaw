package api

import (
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
)

func TestRoomHumanIntakeAlwaysReachesOnlyManager(t *testing.T) {
	for _, mentions := range [][]string{nil, {"dev"}, {"manager"}, {"dev", "qa"}, {"manager", "dev"}, {"outsider"}} {
		t.Run(fmtMentionCase(mentions), func(t *testing.T) {
			messages := im.NewService()
			for _, id := range []string{"dev", "qa", "outsider"} {
				if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
					t.Fatal(err)
				}
			}
			room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Work", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeOnDemand})
			if err != nil {
				t.Fatal(err)
			}
			bridge := im.NewParticipantBridge("")
			h := &Handler{im: messages, participantBridge: bridge}
			manager, closeManager := bridge.Subscribe("manager")
			defer closeManager()
			dev, closeDev := bridge.Subscribe("pt-dev")
			defer closeDev()
			qa, closeQA := bridge.Subscribe("pt-qa")
			defer closeQA()
			msg, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "开发一个上海 AI 咨询的网站"})
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range mentions {
				msg.Mentions = append(msg.Mentions, im.Mention{ID: "user-" + id})
			}
			sender, _ := messages.User(msg.SenderID)
			event := im.Event{Type: im.EventTypeMessageCreated, RoomID: room.ID, Message: &msg, Sender: &sender}
			h.PublishParticipantEvent(event)
			select {
			case got := <-manager:
				if got.MessageID != msg.ID || !got.Mentioned || !got.Context.Mentioned || got.TaskID != "" {
					t.Fatalf("lost implicit manager address: %+v", got)
				}
			default:
				t.Fatal("human input did not reach manager")
			}
			for name, events := range map[string]<-chan im.ParticipantEvent{"dev": dev, "qa": qa} {
				select {
				case <-events:
					t.Fatalf("human mention bypassed manager: %s", name)
				default:
				}
			}
			h.PublishParticipantEvent(event)
			select {
			case <-manager:
				t.Fatal("duplicate user message woke manager again")
			default:
			}
			// A new connection must apply the same intake policy to missed messages.
			replay := im.NewParticipantBridge("")
			h.participantBridge = replay
			replayEvents, closeReplay := replay.Subscribe("manager")
			defer closeReplay()
			h.replayRecentParticipantMessages("manager", "")
			select {
			case got := <-replayEvents:
				if got.MessageID != msg.ID || !got.Mentioned {
					t.Fatal("replay lost manager address")
				}
			default:
				t.Fatal("missed human message not replayed")
			}
		})
	}
}

func fmtMentionCase(mentions []string) string {
	if len(mentions) == 0 {
		return "unmentioned"
	}
	name := ""
	for _, m := range mentions {
		name += "@" + m
	}
	return name
}
