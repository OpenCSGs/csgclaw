package im

import (
	"sync"

	"csgclaw/internal/apitypes"
)

const (
	EventTypeMessageCreated           = "message.created"
	EventTypeThreadCreated            = "thread.created"
	EventTypeThreadUpdated            = "thread.updated"
	EventTypeRoomCreated              = "room.created"
	EventTypeRoomUpdated              = "room.updated"
	EventTypeRoomDeleted              = "room.deleted"
	EventTypeRoomMembersAdded         = "room.members_added"
	EventTypeRoomMembersRemoved       = "room.members_removed"
	EventTypeRoomMessagesCleared      = "room.messages_cleared"
	EventTypeConversationCreated      = "conversation.created"
	EventTypeConversationMembersAdded = "conversation.members_added"
	EventTypeParticipantCreated       = "participant.created"
	EventTypeParticipantUpdated       = "participant.updated"
	EventTypeParticipantDeleted       = "participant.deleted"
	EventTypeUserCreated              = "user.created"
	EventTypeUserUpdated              = "user.updated"
	EventTypeUserDeleted              = "user.deleted"
	EventTypeTeamCreated              = "team.created"
	EventTypeTeamUpdated              = "team.updated"
	EventTypeTeamDeleted              = "team.deleted"
	EventTypeUpgradeStatusChanged     = "upgrade.status_changed"
)

type Event struct {
	Type        string                  `json:"type"`
	RoomID      string                  `json:"room_id,omitempty"`
	TeamID      string                  `json:"team_id,omitempty"`
	Room        *Room                   `json:"room,omitempty"`
	User        *User                   `json:"user,omitempty"`
	Message     *Message                `json:"message,omitempty"`
	Participant *apitypes.Participant   `json:"participant,omitempty"`
	Team        *apitypes.Team          `json:"team,omitempty"`
	Thread      *ThreadView             `json:"thread,omitempty"`
	Sender      *User                   `json:"sender,omitempty"`
	Upgrade     *apitypes.UpgradeStatus `json:"upgrade,omitempty"`
}

type Bus struct {
	mu          sync.Mutex
	nextID      int
	subscribers map[int]chan Event
}

func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[int]chan Event),
	}
}

func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)

	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subscribers[id] = ch
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if existing, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(existing)
		}
		b.mu.Unlock()
	}

	return ch, cancel
}

func (b *Bus) Publish(evt Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// Cancellation closes channels under the same lock. Sends never block.
	for _, ch := range b.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}
