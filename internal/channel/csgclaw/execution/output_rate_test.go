package execution

import (
	"context"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/modelcap"
)

func TestOutputRateMeterCountsAcrossChunksAndResets(t *testing.T) {
	var meter outputRateMeter
	meter.setModel("gpt-5.4")
	start := time.Unix(100, 0)
	if !meter.observe(start, "hel") || meter.rate != 0 {
		t.Fatal("the first sample must be a baseline")
	}
	if meter.observe(start.Add(25*time.Millisecond), "lo") {
		t.Fatal("a chunk inside the sampling interval should be buffered")
	}
	now := start.Add(500 * time.Millisecond)
	if !meter.observe(now, " world again") || meter.rate != 4 || meter.at != now.UnixMilli() {
		t.Fatalf("rate = %v, sampled at %v, want 4 at %v", meter.rate, meter.at, now.UnixMilli())
	}
	if !meter.observe(now.Add(3*time.Second), "new output") || meter.rate != 0 || meter.at != 0 {
		t.Fatal("a generation pause must invalidate the previous window")
	}
	meter.setModel("unrecognized-model")
	if meter.observe(now, "hello") {
		t.Fatal("an unknown model must not produce token estimates")
	}
}

func TestAutoOutputRateReachesWorkStatusAndClearsOnTool(t *testing.T) {
	reporter := newRecordingWorkReporter()
	turn := &activeWorkTurn{statusReporter: reporter, statusStage: apitypes.ParticipantWorkStagePreparingReply}
	ctx := context.Background()
	turn.observeEvent(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventActivityUpdate, Activity: &agentengine.ActivityUpdate{
		Kind:    modelcap.ContextUsageKind,
		Payload: modelcap.ContextUsage{ModelID: "auto", ContextWindow: 128000, ContextSource: "catalog"},
	}})
	turn.observeEvent(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "hello"})
	ageRateBaseline(&turn.outputRate)
	turn.observeEvent(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: " world"})
	turn.observeEvent(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart})
	_, updates, _, _, _ := reporter.snapshot()
	if len(updates) != 4 {
		t.Fatalf("updates = %+v", updates)
	}
	if sample := updates[2]; sample.Stage != apitypes.ParticipantWorkStageGeneratingReply ||
		sample.OutputTokensPerSecond <= 0 || sample.OutputRateSampledAt <= 0 {
		t.Fatalf("unexpected rate update: %+v", sample)
	}
	if cleared := updates[3]; cleared.Stage != apitypes.ParticipantWorkStageRunningTool ||
		cleared.OutputTokensPerSecond != 0 || cleared.OutputRateSampledAt != 0 {
		t.Fatalf("tool should clear the previous sample: %+v", cleared)
	}
}

func TestOutputRateMeterEstimatesBuiltInModels(t *testing.T) {
	for _, model := range []string{
		"gpt-5.5", "gpt-6.1-sol", "glm-5.3", "mimo-v2.6-flash",
		"deepseek-v4-pro", "qwen3.7-plus", "OpenCSG/Qwen3.7-Plus", "auto", "OpenCSG/Auto",
	} {
		t.Run(model, func(t *testing.T) {
			var meter outputRateMeter
			meter.setModel(model)
			start := time.Unix(100, 0)
			if !meter.observe(start, "磁盘") ||
				!meter.observe(start.Add(100*time.Millisecond), "空间充足") || meter.rate <= 0 {
				t.Fatalf("rate = %v", meter.rate)
			}
		})
	}
}

func ageRateBaseline(m *outputRateMeter) {
	start := time.Now().Add(-500 * time.Millisecond)
	m.last, m.samples[0].at = start, start
}

func TestThinkingRateUsesRuntimeTextAndResetsForReply(t *testing.T) {
	for name, event := range map[string]func(string) agentengine.TurnEvent{
		"codex": func(text string) agentengine.TurnEvent {
			return agentengine.TurnEvent{Kind: agentengine.TurnEventActivityUpdate, Activity: &agentengine.ActivityUpdate{
				Kind: string(activity.RuntimeEventReasoningSummary), Payload: text,
			}}
		},
		"dsh": func(text string) agentengine.TurnEvent {
			return agentengine.TurnEvent{Kind: agentengine.TurnEventThoughtDelta, Thought: text}
		},
	} {
		t.Run(name, func(t *testing.T) {
			reporter := newRecordingWorkReporter()
			turn := &activeWorkTurn{statusReporter: reporter, statusStage: apitypes.ParticipantWorkStagePreparingReply}
			turn.outputRate.setModel("auto")
			ctx := context.Background()
			turn.observeEvent(ctx, event("Inspect"))
			ageRateBaseline(&turn.outputRate)
			snapshot := event(" Replayed at completion")
			snapshot.TextSnapshot = true
			turn.observeEvent(ctx, snapshot)
			turn.observeEvent(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate,
				Tool: &agentengine.ToolActivity{ID: "background", Status: "completed"}})
			if turn.statusStage != apitypes.ParticipantWorkStageThinking ||
				turn.outputRate.text.String() != "Inspect" || turn.outputRate.rate != 0 {
				t.Fatal("a snapshot or background tool update disrupted the active thinking sample")
			}
			turn.observeEvent(ctx, event(" the current configuration"))
			turn.observeEvent(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "Answer"})
			_, updates, _, _, _ := reporter.snapshot()
			if len(updates) != 3 {
				t.Fatalf("updates = %+v", updates)
			}
			if sample := updates[1]; sample.Stage != apitypes.ParticipantWorkStageThinking ||
				sample.Phase != apitypes.ParticipantWorkPhaseThinking || sample.OutputTokensPerSecond <= 0 ||
				sample.OutputRateSampledAt <= 0 {
				t.Fatalf("missing thinking rate: %+v", sample)
			}
			if reply := updates[2]; reply.Stage != apitypes.ParticipantWorkStageGeneratingReply ||
				reply.OutputTokensPerSecond != 0 || reply.OutputRateSampledAt != 0 {
				t.Fatalf("reply inherited thinking rate: %+v", reply)
			}
		})
	}
}
