package codex

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
)

func TestTurnUpdatesSteerCurrentTurnAfterCompaction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		signal := make(chan struct{}, 2)
		const notice = "Two unread messages. Read the current conversation."
		ctx := contract.WithTurnInputUpdates(context.Background(), contract.TurnInputUpdates{
			Ready: signal,
			Latest: func(after uint64) (uint64, string) {
				if after == 2 {
					return after, ""
				}
				return 2, notice
			},
		})
		f := startQuietTurnWithContext(t, ctx)
		defer close(f.live.done)
		f.live.mu.Lock()
		f.live.contextUsage = map[string]modelcap.ContextUsage{"thread": {Compacting: true}}
		f.live.mu.Unlock()
		signal <- struct{}{}
		signal <- struct{}{}
		time.Sleep(turnUpdateBatchDelay)
		f.requireWaiting(t)
		f.live.mu.Lock()
		delete(f.live.contextUsage, "thread")
		f.live.mu.Unlock()
		f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: "turn", progress: true, activity: "item/completed"})
		time.Sleep(turnUpdateBatchDelay)
		request := f.receive(t, "turn/steer")
		var params struct {
			ExpectedTurnID string `json:"expectedTurnId"`
			Input          []struct{ Type, Text string }
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.ExpectedTurnID != "turn" || len(params.Input) != 1 || params.Input[0].Type != "text" || params.Input[0].Text != notice {
			t.Fatalf("wrong steer payload: %s", request.Params)
		}
		f.live.appClient.handleMessage(appServerWireMessage{ID: request.ID, Result: json.RawMessage(`{"turnId":"turn"}`)})
		synctest.Wait()

		signal <- struct{}{}
		time.Sleep(turnUpdateBatchDelay)
		f.requireWaiting(t)
	})
}
