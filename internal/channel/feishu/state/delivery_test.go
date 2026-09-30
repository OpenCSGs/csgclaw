package state

import (
	"fmt"
	"testing"

	channeltypes "csgclaw/internal/channel"
)

func TestStoreBoundsTerminalTurnRecords(t *testing.T) {
	store := NewStore()
	for index := 0; index <= maxTurnRecords; index++ {
		id := fmt.Sprintf("turn-%04d", index)
		if err := store.Put(channeltypes.TurnRecord{TurnID: id, Status: channeltypes.TurnSucceeded}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(store.turns); got != maxTurnRecords {
		t.Fatalf("turn records = %d, want %d", got, maxTurnRecords)
	}
	if _, exists := store.Get("turn-0000"); exists {
		t.Fatal("oldest terminal turn was not evicted")
	}
}

func TestStoreBoundsTerminalDeliveryRecords(t *testing.T) {
	store := NewStore()
	for index := 0; index <= maxDeliveryRecords; index++ {
		id := fmt.Sprintf("delivery-%04d", index)
		if err := store.Enqueue(channeltypes.DeliveryIntent{
			ID: id, BindingID: "binding-1", TurnID: "missing-turn", Kind: channeltypes.DeliveryCard,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.MarkFailed(id, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(store.deliveries); got != maxDeliveryRecords {
		t.Fatalf("delivery records = %d, want %d", got, maxDeliveryRecords)
	}
	if _, exists := store.Delivery("delivery-0000"); exists {
		t.Fatal("oldest terminal delivery was not evicted")
	}
}

func TestStoreRetainsPendingDeliveryDependencyAtLimit(t *testing.T) {
	store := NewStore()
	for index := 0; index < maxDeliveryRecords-1; index++ {
		id := fmt.Sprintf("old-%04d", index)
		if err := store.Enqueue(channeltypes.DeliveryIntent{
			ID: id, BindingID: "binding-1", TurnID: "old-turn", Kind: channeltypes.DeliveryCard,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.MarkFailed(id, nil); err != nil {
			t.Fatal(err)
		}
	}
	create := channeltypes.DeliveryIntent{
		ID: "create", BindingID: "binding-1", TurnID: "turn-1", Kind: channeltypes.DeliveryCard,
	}
	if err := store.Enqueue(create); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDelivered(create); err != nil {
		t.Fatal(err)
	}
	update := channeltypes.DeliveryIntent{
		ID: "update", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCardUpdate, RelatedID: create.ID,
	}
	if err := store.Enqueue(update); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.Delivery(create.ID); !exists {
		t.Fatal("delivered create dependency was evicted")
	}
	if pending, exists := store.Delivery(update.ID); !exists || pending.Status != channeltypes.DeliveryPending {
		t.Fatalf("pending update = %#v, found=%t", pending, exists)
	}
}
