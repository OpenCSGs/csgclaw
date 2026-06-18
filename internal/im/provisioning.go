package im

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AgentIdentity struct {
	ID          string
	Name        string
	Description string
	Handle      string
	Role        string
	Avatar      string
}

type ProvisionResult struct {
	User User
	Room *Room
}

type Provisioner struct {
	service        *Service
	bus            *Bus
	bootstrapDelay time.Duration
}

func NewProvisioner(service *Service, bus *Bus) *Provisioner {
	return &Provisioner{
		service:        service,
		bus:            bus,
		bootstrapDelay: time.Second,
	}
}

func (p *Provisioner) EnsureAgentUser(_ context.Context, identity AgentIdentity) (ProvisionResult, error) {
	if p == nil || p.service == nil {
		return ProvisionResult{}, fmt.Errorf("im service is required")
	}

	user, room, err := p.service.EnsureAgentUser(EnsureAgentUserRequest{
		ID:          identity.ID,
		Name:        identity.Name,
		Description: identity.Description,
		Handle:      identity.Handle,
		Role:        identity.Role,
		Avatar:      identity.Avatar,
	})
	if err != nil {
		return ProvisionResult{}, err
	}

	p.publishUserEvent(EventTypeUserCreated, user)
	if room != nil {
		p.publishRoomEvent(EventTypeRoomCreated, *room)
		p.scheduleBootstrapMessage(room.ID, identity.Name, identity.Description)
	}

	return ProvisionResult{
		User: user,
		Room: room,
	}, nil
}

func (p *Provisioner) scheduleBootstrapMessage(roomID, name, description string) {
	if p == nil || p.service == nil {
		return
	}
	go func() {
		time.Sleep(p.bootstrapDelay)
		message, err := p.service.CreateMessage(CreateMessageRequest{
			RoomID:   roomID,
			SenderID: adminUserID,
			Content:  buildWorkerBootstrapMessage(name, description),
		})
		if err != nil {
			return
		}
		p.publishMessageCreated(roomID, message.SenderID, message)
	}()
}

func buildWorkerBootstrapMessage(name, description string) string {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	message := fmt.Sprintf("Write this down in your memory: your name is %s.", name)
	if description == "" {
		return message
	}
	return fmt.Sprintf("%s Your responsibility is %s", message, description)
}

func (p *Provisioner) publishMessageCreated(roomID, senderID string, message Message) {
	if p == nil || p.bus == nil || p.service == nil {
		return
	}
	sender, ok := p.service.User(senderID)
	if !ok {
		return
	}
	messageCopy := message
	senderCopy := sender
	p.bus.Publish(Event{
		Type:    EventTypeMessageCreated,
		RoomID:  roomID,
		Message: &messageCopy,
		Sender:  &senderCopy,
	})
}

func (p *Provisioner) publishRoomEvent(eventType string, room Room) {
	if p == nil || p.bus == nil {
		return
	}
	roomCopy := room
	p.bus.Publish(Event{
		Type:   eventType,
		RoomID: room.ID,
		Room:   &roomCopy,
	})
}

func (p *Provisioner) publishUserEvent(eventType string, user User) {
	if p == nil || p.bus == nil {
		return
	}
	userCopy := user
	p.bus.Publish(Event{
		Type: eventType,
		User: &userCopy,
	})
}
