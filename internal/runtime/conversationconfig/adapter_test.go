package conversationconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
)

type testConversation struct {
	contract.RuntimeConversation
	run func(contract.TurnRequest) contract.TurnResult
}

func (c testConversation) Run(_ context.Context, req contract.TurnRequest, _ contract.EventSink) contract.TurnResult {
	return c.run(req)
}

func TestCoordinatorWaitsWithoutChangingAnotherRoomsSettings(t *testing.T) {
	var coordinator Coordinator
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	adapter := &Adapter{Coordinator: &coordinator, Prepare: func(context.Context, contract.TurnRequest) error { return nil }, RuntimeConversation: testConversation{run: func(req contract.TurnRequest) contract.TurnResult {
		if req.ConversationKey != "room-a" || req.Input[0].Text != "original" {
			t.Error("changed identity or input")
		}
		close(entered)
		<-release
		return contract.TurnResult{Status: contract.TurnSucceeded}
	}}}
	go func() {
		defer close(finished)
		adapter.Run(context.Background(), contract.TurnRequest{ConversationKey: "room-a", Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "original"}}}, nil)
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	other := &Adapter{Coordinator: &coordinator, Prepare: func(context.Context, contract.TurnRequest) error {
		t.Error("reconfigured active runtime")
		return errors.New("unexpected")
	}}
	result := other.Run(ctx, contract.TurnRequest{ConversationKey: "room-b"}, nil)
	if result.Status != contract.TurnCanceled || result.Dispatched {
		t.Fatalf("waiting cancellation: %+v", result)
	}
	close(release)
	<-finished
	if err := coordinator.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-coordinator.gate
}

func TestDifferentAgentsDoNotShareCoordinator(t *testing.T) {
	var first, second Coordinator
	if err := first.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { <-first.gate }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := second.acquire(ctx); err != nil {
		t.Fatal("another Agent was blocked", err)
	}
	<-second.gate
}
