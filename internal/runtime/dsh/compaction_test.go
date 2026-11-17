package dsh

import (
	"context"
	"testing"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
)

func TestManualCompactionRejectsScopedSessions(t *testing.T) {
	for _, tc := range []struct {
		name, binding string
		reason        modelcap.CompactionFailureReason
		status        string
	}{
		{"task", "room:test:task:7", modelcap.CompactionScopedSessionUnsupported, "unsupported"},
		{"thread", "room:test:thread:root", modelcap.CompactionScopedSessionUnsupported, "unsupported"},
		{"other room", "room:other:task:7", modelcap.CompactionBindingMissing, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proc := &process{done: make(chan struct{}), meta: runtimeMetadata{Sessions: map[string]string{tc.binding: "saved"}}}
			rt := New(Dependencies{})
			rt.processes["test"] = proc
			result, err := (&conversation{runtime: rt, runtimeID: "test"}).Compact(context.Background(), contract.CompactRequest{ConversationKey: "room:test", Usage: modelcap.ContextUsage{SessionID: "saved"}, OnStarted: func() error { t.Error("scoped session announced compaction start"); return nil }})
			if err != nil || result.Status != tc.status || result.Reason != tc.reason {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if proc.inFlightTurns != 0 || result.Usage != nil {
				t.Fatal("rejected compaction left an active turn or invented usage")
			}
		})
	}
}
