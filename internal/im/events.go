package im

import "sync"

const (
	EventTypeMessageCreated           = "message.created"
	EventTypeRoomCreated              = "room.created"
	EventTypeRoomMembersAdded         = "room.members_added"
	EventTypeConversationCreated      = "conversation.created"
	EventTypeConversationMembersAdded = "conversation.members_added"
	EventTypeUserCreated              = "user.created"
)

type Event struct {
	Type    string   `json:"type"`
	RoomID  string   `json:"room_id,omitempty"`
	Room    *Room    `json:"room,omitempty"`
	User    *User    `json:"user,omitempty"`
	Message *Message `json:"message,omitempty"`
	Sender  *User    `json:"sender,omitempty"`
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
	targets := make([]chan Event, 0, len(b.subscribers))
	for _, ch := range b.subscribers {
		targets = append(targets, ch)
	}
	b.mu.Unlock()

	for _, ch := range targets {
		select {
		case ch <- evt:
		default:
		}
	}
}
