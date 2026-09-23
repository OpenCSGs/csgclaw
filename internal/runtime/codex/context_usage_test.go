package codex

import (
	"context"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"testing"
)

func TestContextUsageUsesLastAndRejectsStaleTurns(t *testing.T) {
	sink := &recordingSink{}
	m := newAppServerManager(managerDeps{EventSink: sink})
	live := &liveSession{spec: SessionSpec{Profile: agentruntime.Profile{ModelID: "m", ModelMetadata: modelcap.Resolved{ContextWindow: 16000, ContextSource: "user"}}}}
	waiter, err := live.registerAppServerTurnWaiter("thread")
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID("current")
	params := map[string]any{"turnId": "current", "tokenUsage": map[string]any{"modelContextWindow": float64(15200), "last": map[string]any{"totalTokens": float64(1000)}, "total": map[string]any{"totalTokens": float64(900000)}}}
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

func TestContextUsageKeepsConfiguredCeilingAcrossSnapshots(t *testing.T) {
	for _, window := range []int64{200000, 300000, 1000000} {
		m := newAppServerManager(managerDeps{EventSink: &recordingSink{}})
		live := &liveSession{spec: SessionSpec{Profile: agentruntime.Profile{ModelID: "m", ModelMetadata: modelcap.Resolved{ContextWindow: window, ContextSource: "user"}}}}
		params := map[string]any{"tokenUsage": map[string]any{"modelContextWindow": float64(window * 95 / 100), "last": map[string]any{"totalTokens": float64(window * 3 / 4)}}}
		m.publishContextUsage("r", live, "s", params, "thread/tokenUsage/updated")
		for _, method := range []string{"snapshot", "item/started", "item/completed", "snapshot"} {
			m.publishContextUsage("r", live, "s", nil, method)
			u := live.contextUsage["s"]
			if u.ContextWindow != window || u.ContextSource != "user" || u.CompactThreshold != window*3/4 {
				t.Fatalf("%s: %+v", method, u)
			}
			if u.UsedTokens != nil && *u.UsedTokens*100/u.ContextWindow != 75 {
				t.Fatalf("wrong percentage %+v", u)
			}
		}
	}
}
