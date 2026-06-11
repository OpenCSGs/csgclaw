package codex

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/activity"
)

func TestPermissionBrokerRequestIDUsesProcessPrefix(t *testing.T) {
	t.Parallel()

	broker := NewPermissionBroker(nil)
	decision, err := broker.Request(context.Background(), PendingPermissionRequest{})
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if decision.Snapshot.ID == "perm-1" || !strings.HasPrefix(decision.Snapshot.ID, "perm-") {
		t.Fatalf("request id = %q, want process-prefixed permission id", decision.Snapshot.ID)
	}
}

func TestPermissionBrokerDecideSelectsOption(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewPermissionBroker(sink)
	resultCh := make(chan PermissionDecision, 1)
	go func() {
		decision, _ := broker.Request(context.Background(), PendingPermissionRequest{
			ExecutionRef: activity.ExecutionRef{
				RuntimeID:  "rt-1",
				SessionID:  "sess-1",
				ToolCallID: "tool-1",
			},
			ToolTitle: "Run shell command",
			Options: []PermissionOptionSnapshot{
				{ID: "once", Kind: "allow_once", Label: "Allow once"},
				{ID: "reject", Kind: "reject_once", Label: "Reject"},
			},
		})
		resultCh <- decision
	}()

	waitForRuntime(t, func() bool {
		events := sink.snapshot()
		return len(events) == 1 && events[0].ActionID != ""
	})
	requestID := sink.snapshot()[0].ActionID
	snapshot, err := broker.Decide(context.Background(), requestID, "reject")
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if snapshot.Status != PermissionStatusRejected || snapshot.Decision == nil || snapshot.Decision.OptionID != "reject" {
		t.Fatalf("snapshot = %+v, want rejected reject decision", snapshot)
	}

	select {
	case decision := <-resultCh:
		if decision.Snapshot.Status != PermissionStatusRejected {
			t.Fatalf("decision = %+v, want rejected", decision)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("broker request did not return")
	}
}

func TestNormalizePermissionOptionsMarksRememberedDecisionsAsAgentScoped(t *testing.T) {
	t.Parallel()

	options := NormalizePermissionOptions([]ExternalPermissionOption{
		{
			ID:    "always",
			Kind:  PermissionOptionKindAllowAlways,
			Label: "Allow always",
		},
		{
			ID:    "once",
			Kind:  PermissionOptionKindAllowOnce,
			Label: "Allow once",
		},
	})

	if options[0].Scope != activity.ActionOptionScopeAgent {
		t.Fatalf("allow_always scope = %q, want %q", options[0].Scope, activity.ActionOptionScopeAgent)
	}
	if options[1].Scope != "" {
		t.Fatalf("allow_once scope = %q, want empty", options[1].Scope)
	}
}

func TestPermissionBrokerDuplicateDecisionReturnsConflictSnapshot(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewPermissionBroker(sink)
	resultCh := make(chan PermissionDecision, 1)
	go func() {
		decision, _ := broker.Request(context.Background(), PendingPermissionRequest{
			ExecutionRef: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "sess-1"},
			Options:      []PermissionOptionSnapshot{{ID: "once", Kind: "allow_once", Label: "Allow once"}},
		})
		resultCh <- decision
	}()

	waitForRuntime(t, func() bool {
		events := sink.snapshot()
		return len(events) == 1 && events[0].ActionID != ""
	})
	requestID := sink.snapshot()[0].ActionID
	if _, err := broker.Decide(context.Background(), requestID, "once"); err != nil {
		t.Fatalf("first Decide() error = %v", err)
	}
	<-resultCh
	snapshot, err := broker.Decide(context.Background(), requestID, "once")
	if !errors.Is(err, ErrPermissionAlreadyDecided) {
		t.Fatalf("duplicate Decide() error = %v, want already decided", err)
	}
	if snapshot.Status != PermissionStatusAllowed {
		t.Fatalf("snapshot status = %s, want allowed", snapshot.Status)
	}
}

func TestPermissionBrokerCompletedCacheExpires(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewPermissionBroker(sink)
	broker.cacheTTL = 25 * time.Millisecond
	resultCh := make(chan PermissionDecision, 1)
	go func() {
		decision, _ := broker.Request(context.Background(), PendingPermissionRequest{
			ExecutionRef: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "sess-1"},
			Options:      []PermissionOptionSnapshot{{ID: "once", Kind: "allow_once", Label: "Allow once"}},
		})
		resultCh <- decision
	}()

	waitForRuntime(t, func() bool {
		events := sink.snapshot()
		return len(events) == 1 && events[0].ActionID != ""
	})
	requestID := sink.snapshot()[0].ActionID
	if _, err := broker.Decide(context.Background(), requestID, "once"); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	<-resultCh
	if _, ok := broker.Get(requestID); !ok {
		t.Fatalf("Get(%q) = false before cache TTL", requestID)
	}
	time.Sleep(50 * time.Millisecond)
	if _, ok := broker.Get(requestID); ok {
		t.Fatalf("Get(%q) = true after cache TTL", requestID)
	}
	if _, err := broker.Decide(context.Background(), requestID, "once"); !errors.Is(err, ErrPermissionNotFound) {
		t.Fatalf("Decide() after cache TTL error = %v, want not found", err)
	}
}

func TestPermissionBrokerTimeoutCancelsACP(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewPermissionBroker(sink)
	broker.timeout = 25 * time.Millisecond
	decision, err := broker.Request(context.Background(), PendingPermissionRequest{
		ExecutionRef: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "sess-1"},
		Options:      []PermissionOptionSnapshot{{ID: "once", Kind: "allow_once", Label: "Allow once"}},
	})
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if decision.Snapshot.Status != PermissionStatusExpired {
		t.Fatalf("decision status = %s, want expired", decision.Snapshot.Status)
	}
}

func TestPermissionBrokerCancelSessionCancelsPendingRequests(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewPermissionBroker(sink)
	resultCh := make(chan PermissionDecision, 1)
	go func() {
		decision, _ := broker.Request(context.Background(), PendingPermissionRequest{
			ExecutionRef: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "sess-1"},
			Options:      []PermissionOptionSnapshot{{ID: "once", Kind: "allow_once", Label: "Allow once"}},
		})
		resultCh <- decision
	}()

	waitForRuntime(t, func() bool { return len(sink.snapshot()) == 1 })
	broker.CancelSession("rt-1", "sess-1")
	select {
	case decision := <-resultCh:
		if decision.Snapshot.Status != PermissionStatusCanceled {
			t.Fatalf("decision status = %s, want canceled", decision.Snapshot.Status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("broker request did not return")
	}
}
