package codex

import (
	"context"
	agentruntime "csgclaw/internal/runtime"
	"testing"
)

func TestContextUsageUsesLastAndRejectsStaleTurns(t *testing.T) {
	sink := &recordingSink{}
	m := newAppServerManager(managerDeps{EventSink: sink})
	live := &liveSession{spec: SessionSpec{Profile: agentruntime.Profile{ModelID: "m"}}}
	waiter, err := live.registerAppServerTurnWaiter("thread")
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID("current")
	params := map[string]any{"turnId": "current", "tokenUsage": map[string]any{"modelContextWindow": float64(16000), "last": map[string]any{"totalTokens": float64(1000)}, "total": map[string]any{"totalTokens": float64(900000)}}}
	m.publishContextUsage("r", live, "thread", params, "")
	u := live.contextUsage["thread"]
	if u.UsedTokens == nil || *u.UsedTokens != 1000 || u.ContextWindow != 16000 {
		t.Fatalf("%+v", u)
	}
	params["turnId"] = "old"
	params["tokenUsage"] = map[string]any{"last": map[string]any{"totalTokens": float64(9999)}}
	m.publishContextUsage("r", live, "thread", params, "")
	if *live.contextUsage["thread"].UsedTokens != 1000 {
		t.Fatal("stale turn changed usage")
	}
	m.publishContextUsage("r", live, "thread", nil, "item/completed")
	if live.contextUsage["thread"].UsedTokens != nil {
		t.Fatal("compaction fabricated usage")
	}
}
func TestContextErrorClassification(t *testing.T) {
	for _, value := range []any{"contextWindowExceeded", map[string]any{"message": `{"error":{"code":"context_length_exceeded"}}`}} {
		if !contextErrorInfo(value) {
			t.Fatalf("missed %v", value)
		}
	}
	for _, value := range []any{"unauthorized", "context window", map[string]any{"code": "invalid_request_error", "httpStatusCode": 400}} {
		if contextErrorInfo(value) {
			t.Fatalf("misclassified %v", value)
		}
	}
}

func TestContextRecoveryDoesNotReplayToolsOrOutput(t *testing.T) {
	m := newAppServerManager(managerDeps{})
	live := &liveSession{}
	waiter, err := live.registerAppServerTurnWaiter("s")
	if err != nil {
		t.Fatal(err)
	}
	waiter.visibleActivity = true
	original := waiter
	if _, err := m.recoverContext(context.Background(), live, &waiter); !isContextWindowError(err) {
		t.Fatalf("%v", err)
	}
	if waiter != original {
		t.Fatal("replaced waiter despite existing work")
	}
}
