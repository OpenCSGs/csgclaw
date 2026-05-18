package notifier

import (
	"context"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestAgentRuntimeKind(t *testing.T) {
	r := NewAgentRuntime()
	if got := r.Kind(); got != agentruntime.KindNotifier {
		t.Fatalf("Kind() = %q, want %q", got, agentruntime.KindNotifier)
	}
}

func TestAgentRuntimeCreateStartInfo(t *testing.T) {
	r := NewAgentRuntime()
	ctx := context.Background()
	h, err := r.New(ctx, agentruntime.Spec{RuntimeID: "rt-u-test", AgentID: "u-test", AgentName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if h.RuntimeID != "rt-u-test" {
		t.Fatalf("RuntimeID = %q", h.RuntimeID)
	}
	st, err := r.Start(ctx, h)
	if err != nil || st != agentruntime.StateRunning {
		t.Fatalf("Start = %v, %v", st, err)
	}
	info, err := r.Info(ctx, h)
	if err != nil || info.State != agentruntime.StateRunning {
		t.Fatalf("Info = %+v, %v", info, err)
	}
}
