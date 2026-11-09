package codex

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

type quietTurnRequests chan appServerWireMessage

// The fixture has one outstanding request and a buffered writer.
func (r quietTurnRequests) SetWriteDeadline(time.Time) error { return nil }

func (r quietTurnRequests) Write(data []byte) (int, error) {
	var req appServerWireMessage
	if err := json.Unmarshal(data, &req); err != nil {
		return 0, err
	}
	r <- req
	return len(data), nil
}

type quietTurnFixture struct {
	requests quietTurnRequests
	live     *liveSession
	manager  *appServerManager
	response PromptResponse
	err      error
	done     chan struct{}
}

func startQuietTurn(t *testing.T) *quietTurnFixture {
	t.Helper()
	requests := make(quietTurnRequests, 8)
	f := &quietTurnFixture{
		requests: requests,
		live:     &liveSession{appClient: newAppServerClient(requests, nil), done: make(chan struct{}), spec: SessionSpec{RuntimeID: "rt"}},
		manager:  newAppServerManager(testAppServerManagerDeps()),
		done:     make(chan struct{}),
	}
	waiter, err := f.live.registerAppServerTurnWaiter("thread")
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID("turn")
	go func() {
		f.response, f.err = f.manager.waitAppServerTurn(context.Background(), f.live, waiter)
		close(f.done)
	}()
	synctest.Wait()
	return f
}

func (f *quietTurnFixture) receive(t *testing.T, method string) appServerWireMessage {
	t.Helper()
	synctest.Wait()
	select {
	case req := <-f.requests:
		var params map[string]any
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Fatal(err)
		}
		if req.Method != method || params["threadId"] != "thread" {
			t.Fatalf("unexpected RPC %s %s", req.Method, req.Params)
		}
		if method == "thread/read" && params["includeTurns"] != false {
			t.Fatalf("unbounded thread/read: %s", req.Params)
		}
		if method == "thread/turns/list" && (params["limit"] != float64(1) || params["sortDirection"] != "desc" || params["itemsView"] != "full") {
			t.Fatalf("unexpected turns query: %s", req.Params)
		}
		return req
	default:
		t.Fatalf("expected RPC %s", method)
		return appServerWireMessage{}
	}
}

func (f *quietTurnFixture) reply(t *testing.T, method, result string) {
	t.Helper()
	req := f.receive(t, method)
	f.live.appClient.handleMessage(appServerWireMessage{ID: req.ID, Result: json.RawMessage(result)})
}

func (f *quietTurnFixture) requireWaiting(t *testing.T) {
	t.Helper()
	synctest.Wait()
	select {
	case req := <-f.requests:
		t.Fatalf("unexpected RPC %s", req.Method)
	case <-f.done:
		t.Fatalf("turn finished prematurely: %+v, %v", f.response, f.err)
	default:
	}
}

const idleThread = `{"thread":{"id":"thread","status":{"type":"idle"}}}`

func TestQuietTurnReadsOnlyMatchingTerminalHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := startQuietTurn(t)
		defer close(f.live.done)
		time.Sleep(quietTurnInterval - time.Second)
		f.requireWaiting(t)
		f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: "turn", activity: "commandExecution", progress: true})
		synctest.Wait()
		time.Sleep(time.Second)
		f.requireWaiting(t)
		time.Sleep(quietTurnInterval - time.Second)
		f.reply(t, "thread/read", `{"thread":{"id":"thread","status":{"type":"active"}}}`)
		f.requireWaiting(t)
		time.Sleep(quietTurnInterval)
		f.reply(t, "thread/read", `{"thread":{"id":"other","status":{"type":"idle"}}}`)
		f.requireWaiting(t)
		for _, turn := range []string{
			`{"id":"previous","status":"completed","completedAt":10}`, `{"id":"turn","status":"inProgress","completedAt":10}`, `{"id":"turn"}`, `{"id":"turn","status":"interrupted","completedAt":null}`,
		} {
			time.Sleep(quietTurnInterval)
			f.reply(t, "thread/read", idleThread)
			f.reply(t, "thread/turns/list", `{"data":[`+turn+`]}`)
			f.requireWaiting(t)
		}
		time.Sleep(quietTurnInterval)
		f.reply(t, "thread/read", idleThread)
		pending := f.receive(t, "thread/turns/list")
		f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: "turn", progress: true, activity: "commandExecution"})
		f.live.appClient.handleMessage(appServerWireMessage{ID: pending.ID, Result: json.RawMessage(`{"data":[{"id":"turn","status":"completed","completedAt":10}]}`)})
		f.requireWaiting(t)
		time.Sleep(quietTurnInterval)
		f.reply(t, "thread/read", `{"thread":{"id":"thread","status":{"type":"systemError"}}}`)
		f.reply(t, "thread/turns/list", `{"data":[{"id":"turn","status":"completed","completedAt":10}]}`)
		f.requireWaiting(t)
		time.Sleep(quietTurnInterval)
		req := f.receive(t, "thread/read")
		f.live.appClient.handleMessage(appServerWireMessage{ID: req.ID, Error: json.RawMessage(`{"code":-32601,"message":"method not found"}`)})
		f.requireWaiting(t)
		time.Sleep(quietTurnInterval)
		f.receive(t, "thread/read") // No reply. The query expires and the turn stays active.
		time.Sleep(5 * time.Second)
		f.requireWaiting(t)
		time.Sleep(quietTurnInterval - 5*time.Second)
		f.reply(t, "thread/read", idleThread)
		f.reply(t, "thread/turns/list", `{"data":[{"id":"turn","status":"completed","completedAt":10,"itemsView":"full","items":[
			{"id":"cmd","type":"commandExecution","command":"echo secret"},
			{"id":"a","type":"agentMessage","text":"comment","phase":"commentary"},
			{"id":"b","type":"agentMessage","text":"answer","phase":"final_answer"},
			{"id":"c","type":"agentMessage","text":" and more"}
		]}]}`)
		<-f.done
		if f.err != nil || f.response.StopReason != StopReasonEndTurn || f.response.history == nil {
			t.Fatalf("recovered turn = %+v, %v", f.response, f.err)
		}
		output := conversationOutput{}
		output.apply("b", "final_answer", "ans", false)
		event := promptCompletedEvent("rt", "thread", f.response)
		var deltas []string
		if result := (&ConversationAdapter{}).handleEvent(context.Background(), contract.TurnRequest{}, contract.EventSinkFunc(func(_ context.Context, e contract.TurnEvent) error {
			if e.Kind != contract.TurnEventTextDelta || e.TextSnapshot {
				t.Fatalf("unexpected history event: %+v", e)
			}
			deltas = append(deltas, e.ItemID+":"+e.Text)
			return nil
		}), event, &output, nil); result != nil || output.String() != "answer and more" || !slices.Equal(deltas, []string{"b:wer", "c: and more"}) {
			t.Fatalf("result=%+v output=%q deltas=%v", result, output.String(), deltas)
		}
	})
}

func TestQuietTurnRetainsFailureAndLiveCompletion(t *testing.T) {
	for _, status := range []string{"failed", "interrupted", "live"} {
		t.Run(status, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := startQuietTurn(t)
				defer close(f.live.done)
				time.Sleep(quietTurnInterval)
				thread := idleThread
				if status == "failed" {
					thread = `{"thread":{"id":"thread","status":{"type":"systemError"}}}`
				}
				f.reply(t, "thread/read", thread)
				req := f.receive(t, "thread/turns/list")
				if status == "live" {
					f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: "turn", success: true})
					<-f.done
					if f.err != nil || f.response.StopReason != StopReasonEndTurn || f.response.history != nil {
						t.Fatalf("live turn = %+v, %v", f.response, f.err)
					}
					return
				}
				f.live.appClient.handleMessage(appServerWireMessage{ID: req.ID, Result: json.RawMessage(`{"data":[{"id":"turn","status":"` + status + `","completedAt":10,"error":{"message":"native failure"},"itemsView":"full","items":[{"id":"cmd","type":"commandExecution","status":"failed"}]}]}`)})
				<-f.done
				want := "native failure"
				if status == "interrupted" {
					want = "interrupted"
				}
				var failure *quietTurnFailure
				if !errors.As(f.err, &failure) || len(failure.history) != 2 || !strings.Contains(f.err.Error(), want) {
					t.Fatalf("recovered %s error = %v", status, f.err)
				}
			})
		})
	}
}

func TestQuietTurnPreservesStreamedOutputWithoutFullHistory(t *testing.T) {
	item := map[string]any{"type": "agentMessage", "id": "final", "text": "final"}
	for _, view := range []string{"", "summary", "full"} {
		t.Run(view, func(t *testing.T) {
			var output conversationOutput
			output.apply("previous", "unknown", "already streamed", false)
			native := quietTurnItems(map[string]any{"itemsView": view, "items": []any{item}})
			if view != "full" && native != nil {
				t.Fatal("incomplete history returned a text snapshot")
			}
			if err := output.restore(t.Context(), nil, native); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != "already streamed" {
				t.Fatalf("restored incomplete item list: %q", got)
			}
		})
	}
}

func TestQuietTurnCompletesLegacyTextWithoutItemID(t *testing.T) {
	var output conversationOutput
	output.apply("", "unknown", "first part", false)
	native := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"type": "agentMessage", "id": "answer", "text": "first part and final part"},
	}})
	if err := output.restore(t.Context(), nil, native); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "first part and final part" {
		t.Fatalf("restored text = %q", got)
	}
}

func TestQuietTurnRestoresMissingItemsInNativeOrder(t *testing.T) {
	var output conversationOutput
	output.apply("intro", "unknown", "Checking files.", false)
	output.apply("partial", "final_answer", "par", false)
	output.observeTool(contract.TurnEventToolCallStart, &contract.ToolActivity{ID: "running", Kind: "exec_command", Status: "started"})
	output.observeTool(contract.TurnEventToolCallUpdate, &contract.ToolActivity{ID: "running", Kind: "exec_command", Status: "running"})
	output.observeTool(contract.TurnEventToolCallUpdate, &contract.ToolActivity{ID: "finished", Kind: "exec_command", Status: "completed"})
	var turn map[string]any
	if err := json.Unmarshal([]byte(`{"status":"completed","itemsView":"full","items":[
		{"id":"intro","type":"agentMessage","phase":"commentary","text":"Checking files."},
		{"id":"finished","type":"fileChange","status":"completed","changes":[]},
		{"id":"inactive","type":"fileChange","status":"inProgress"},
		{"id":"running","type":"commandExecution","command":"pwd","status":"completed","aggregatedOutput":"/workspace\n::csgclaw-output::resource_link {\"type\":\"resource_link\",\"uri\":\"https://example.test/r\",\"name\":\"ignored link\"}"},
		{"id":"partial","type":"agentMessage","phase":"final_answer","text":"partial."},
		{"id":"missed","type":"mcpToolCall","server":"local","tool":"lookup","arguments":{"api_key":"fixture-secret"},"status":"failed","error":"missing"},
		{"id":"interact","type":"requestUserInput"},
		{"id":"file","type":"fileOutput","path":"not-published.txt"},
		{"id":"ask","type":"agentMessage","text":"::csgclaw-output::request_user_input {\"questions\":[{\"id\":\"test\",\"header\":\"Choose\",\"question\":\"Continue?\",\"options\":[{\"label\":\"Sure\",\"description\":\"Continue\"}]}]}"},
		{"id":"thought","type":"reasoning","summary":["private"]},
		{"id":"final","type":"agentMessage","phase":"final_answer","text":"Answer."}
	]}`), &turn); err != nil {
		t.Fatal(err)
	}

	history := quietTurnItems(turn)
	var received []string
	sink := contract.EventSinkFunc(func(_ context.Context, e contract.TurnEvent) error {
		if e.Tool != nil {
			received = append(received, e.Tool.ID+":"+e.Tool.Status)
			if e.Tool.ID == "running" && !strings.Contains(e.Tool.OutputSummary, "/workspace") {
				t.Fatalf("missing tool output: %+v", e.Tool)
			}
			payload, _ := json.Marshal(e.Tool)
			if strings.Contains(string(payload), structuredOutputPrefix) {
				t.Fatal("replayed structured command output")
			}
			if strings.Contains(string(payload), "fixture-secret") {
				t.Fatal("exposed tool credential")
			}
		} else {
			if e.TextSnapshot {
				t.Fatalf("expected appendable text: %+v", e)
			}
			received = append(received, e.ItemID+":"+e.Phase+":"+e.Text)
		}
		return nil
	})
	for range 2 {
		if err := output.restore(t.Context(), sink, history); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"intro:commentary:", "running:completed", "partial:final_answer:tial.", "missed:started", "missed:failed", "final:final_answer:Answer."}
	if !slices.Equal(received, want) || output.String() != "partial.Answer." {
		t.Fatalf("history events=%v output=%q", received, output.String())
	}
}

func TestQuietTurnRestoresTextBehindMissingToolResult(t *testing.T) {
	var output conversationOutput
	output.observeTool(contract.TurnEventToolCallStart, &contract.ToolActivity{ID: "tool", Kind: "exec_command"})
	output.apply("second", "unknown", "Sec", false)
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"id": "tool", "type": "commandExecution", "status": "completed"},
		map[string]any{"id": "first", "type": "agentMessage", "text": "First. "},
		map[string]any{"id": "second", "type": "agentMessage", "phase": "final_answer", "text": "Second."},
	}})
	var parts []contract.RecoveredTextPart
	sink := contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
		if event.RecoveredText != nil {
			if event.Kind != contract.TurnEventToolCallUpdate || event.Tool.ID != "tool" {
				t.Fatalf("unexpected history boundary: %+v", event)
			}
			parts = append(parts, event.RecoveredText...)
		}
		return nil
	})
	for range 2 {
		if err := output.restore(t.Context(), sink, history); err != nil {
			t.Fatal(err)
		}
	}
	want := []contract.RecoveredTextPart{{ToolID: "tool"}, {Text: "First. "}, {Text: "Second."}}
	if !slices.Equal(parts, want) || output.String() != "First. Second." {
		t.Fatalf("history=%+v answer=%q", parts, output.String())
	}
}

func TestQuietTurnFailureDeliversStoredToolOutcome(t *testing.T) {
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"id": "cmd", "type": "commandExecution", "status": "failed", "aggregatedOutput": "exit 1"},
	}})
	cause := errors.New("native failure")
	err := history.failure(cause)
	if !errors.Is(err, cause) {
		t.Fatalf("lost original error: %v", err)
	}
	var received []string
	sink := contract.EventSinkFunc(func(_ context.Context, e contract.TurnEvent) error {
		received = append(received, e.Tool.ID+":"+e.Tool.Status)
		return nil
	})
	var output conversationOutput
	output.observeTool(contract.TurnEventToolCallStart, &contract.ToolActivity{ID: "cmd", Kind: "exec_command", Status: "started"})
	result := (&ConversationAdapter{}).handleEvent(t.Context(), contract.TurnRequest{}, sink, promptFailedEvent("runtime", "thread", err), &output, nil)
	if result == nil || result.Status != contract.TurnFailed || result.Error.Message != cause.Error() || !slices.Equal(received, []string{"cmd:failed"}) {
		t.Fatalf("result=%+v, events=%v", result, received)
	}
}

func TestQuietTurnRestoresFinalAfterUnphasedLegacyCommentary(t *testing.T) {
	manager, live, raw := testAppServerEventAdapter(t)
	manager.handleLegacyAppServerEvent("rt", live, map[string]any{"type": "agent_message", "message": "Checking files."})
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"id": "hidden", "type": "agentMessage", "phase": "commentary", "text": "Looking up tools."},
		map[string]any{"id": "intro", "type": "agentMessage", "phase": "commentary", "text": "Checking files."},
		map[string]any{"id": "answer", "type": "agentMessage", "phase": "final_answer", "text": "The answer."},
	}})
	var output conversationOutput
	var delivered []string
	sink := contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
		if event.Kind == contract.TurnEventTextDelta && event.Phase != "commentary" && !event.TextSnapshot {
			delivered = append(delivered, event.Text)
		}
		return nil
	})
	for _, event := range raw.snapshot() {
		(&ConversationAdapter{}).handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, nil)
	}
	for range 2 {
		if err := output.restore(t.Context(), sink, history); err != nil {
			t.Fatal(err)
		}
	}
	if output.String() != "The answer." || !slices.Equal(delivered, []string{"Checking files.", "The answer."}) {
		t.Fatalf("result=%q delivered=%v", output.String(), delivered)
	}
}

func TestQuietTurnRestoresToolAfterUnidentifiedLegacyResult(t *testing.T) {
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"id": "cmd", "type": "commandExecution", "status": "completed", "command": "pwd", "aggregatedOutput": "/workspace"},
	}})
	for _, started := range []bool{false, true} {
		var output conversationOutput
		if started {
			output.observeTool(contract.TurnEventToolCallStart, &contract.ToolActivity{ID: "cmd", Kind: "exec_command", Status: "started"})
		}
		output.observeTool(contract.TurnEventToolCallUpdate, &contract.ToolActivity{ID: "cmd", Status: "completed"})
		var delivered []contract.TurnEventKind
		sink := contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
			delivered = append(delivered, event.Kind)
			if event.Tool.Kind != "exec_command" {
				t.Fatalf("restored tool: %+v", event.Tool)
			}
			return nil
		})
		if err := output.restore(t.Context(), sink, history); err != nil {
			t.Fatal(err)
		}
		if started && len(delivered) != 0 || !started && !slices.Equal(delivered, []contract.TurnEventKind{contract.TurnEventToolCallStart, contract.TurnEventToolCallUpdate}) {
			t.Fatalf("started=%v delivered=%v", started, delivered)
		}
	}
}

func TestQuietTurnCorrectsEarlierItemsAndAppendsOnlyTheTail(t *testing.T) {
	var output conversationOutput
	output.apply("first", "unknown", "Step ", false)
	output.apply("last", "unknown", "Answer.", false)
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"type": "agentMessage", "id": "first", "text": "Step 1. "},
		map[string]any{"type": "agentMessage", "id": "missed", "text": "Next. "},
		map[string]any{"type": "agentMessage", "id": "last", "text": "Answer.!"},
		map[string]any{"type": "agentMessage", "id": "postscript", "text": " Done."},
	}})
	var updates []string
	sink := contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
		kind := "append"
		if event.TextSnapshot {
			kind = "snapshot"
		}
		updates = append(updates, kind+":"+event.ItemID+":"+event.Text)
		return nil
	})
	for range 2 {
		if err := output.restore(t.Context(), sink, history); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"snapshot:first:Step 1. ", "snapshot:missed:Next. ", "append:last:!", "append:postscript: Done."}
	if !slices.Equal(updates, want) || output.String() != "Step 1. Next. Answer.! Done." {
		t.Fatalf("updates=%v result=%q", updates, output.String())
	}
}

func TestQuietTurnPromotesFormerlyUnphasedTextOnce(t *testing.T) {
	var output conversationOutput
	output.apply("answer", "unknown", "Answer", false)
	output.apply("answer", "commentary", "", false)
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"type": "agentMessage", "id": "answer", "phase": "final_answer", "text": "Answer."},
	}})
	var appended string
	sink := contract.EventSinkFunc(func(_ context.Context, e contract.TurnEvent) error {
		if !e.TextSnapshot && e.Phase != "commentary" {
			appended += e.Text
		}
		return nil
	})
	for range 2 {
		if err := output.restore(t.Context(), sink, history); err != nil {
			t.Fatal(err)
		}
	}
	if appended != "." || output.String() != "Answer." {
		t.Fatalf("appended=%q result=%q", appended, output.String())
	}
}

func TestQuietTurnRecoveryPreservesTextOnlyQuestions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sourceID string
		answer   string
		want     string
	}{
		{"message", "prompt", "", "Context. Choose.\n\nWhich option?"},
		{"anonymous message", "", "", "Context. Choose.\n\nWhich option?"},
		{"later answer", "prompt", " Done.", "Context. Choose. Done."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output conversationOutput
			output.apply("intro", "final_answer", "Context. ", false)
			form := activity.StructuredOutputArtifact{RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "q", Question: "Which option?"}}}}
			event := activity.RuntimeEvent{Kind: activity.RuntimeEventStructuredOutput, ToolCallID: tc.sourceID, Text: "Choose.", Payload: form}
			var files []*contract.OutputFile
			if result := (&ConversationAdapter{}).handleEvent(t.Context(), contract.TurnRequest{Interaction: contract.InteractionTextOnly}, nil, event, &output, &files); result != nil {
				t.Fatal(result)
			}
			output.apply(tc.sourceID, "final_answer", "Choose.", false)
			items := []any{
				map[string]any{"id": "intro", "type": "agentMessage", "phase": "final_answer", "text": "Context. "},
				map[string]any{"id": "prompt", "type": "agentMessage", "phase": "final_answer", "text": "Choose."},
			}
			if tc.answer != "" {
				items = append(items, map[string]any{"id": "answer", "type": "agentMessage", "phase": "final_answer", "text": tc.answer})
			}
			history := quietTurnItems(map[string]any{"itemsView": "full", "items": items})
			if err := output.restore(t.Context(), nil, history); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != tc.want {
				t.Fatalf("recovered output = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQuietTurnReclassifiesTextAfterQuestion(t *testing.T) {
	for _, tc := range []struct{ phase, want string }{
		{"commentary", "Choose.\n\nWhich option?"},
		{"final_answer", "Choose.Checking."},
	} {
		t.Run(tc.phase, func(t *testing.T) {
			var output conversationOutput
			form := activity.StructuredOutputArtifact{RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "q", Question: "Which option?"}}}}
			event := activity.RuntimeEvent{Kind: activity.RuntimeEventStructuredOutput, ToolCallID: "ask", Text: "Choose.", Payload: form}
			var files []*contract.OutputFile
			if result := (&ConversationAdapter{}).handleEvent(t.Context(), contract.TurnRequest{Interaction: contract.InteractionTextOnly}, nil, event, &output, &files); result != nil {
				t.Fatal(result)
			}
			output.apply("ask", "final_answer", "Choose.", false)
			output.apply("next", "unknown", "Checking.", false)
			history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
				map[string]any{"id": "ask", "type": "agentMessage", "phase": "final_answer", "text": "Choose."},
				map[string]any{"id": "next", "type": "agentMessage", "phase": tc.phase, "text": "Checking."},
			}})
			if err := output.restore(t.Context(), nil, history); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != tc.want {
				t.Fatalf("recovered response = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQuietTurnLocatesStructuredOnlyQuestionWithoutReplayingIt(t *testing.T) {
	const record = structuredOutputPrefix + `request_user_input {"questions":[{"id":"q","header":"Decision","question":"Which option?","options":[]}]}`
	for _, sourceID := range []string{"ask", ""} {
		t.Run("source="+sourceID, func(t *testing.T) {
			manager, live, raw := testAppServerEventAdapter(t)
			manager.handleRawItemNotification("rt", live, "main-thread", "item/completed", map[string]any{
				"item": map[string]any{"id": sourceID, "type": "agentMessage", "phase": "final_answer", "text": record},
			})
			var output conversationOutput
			var files []*contract.OutputFile
			for _, event := range raw.snapshot() {
				if result := (&ConversationAdapter{}).handleEvent(t.Context(), contract.TurnRequest{Interaction: contract.InteractionTextOnly}, nil, event, &output, &files); result != nil {
					t.Fatal(result)
				}
			}
			if got := output.String(); got != "Which option?" {
				t.Fatalf("live question = %q", got)
			}
			items := []any{
				map[string]any{"id": "intro", "type": "agentMessage", "text": "Intro. "},
				map[string]any{"id": "ask", "type": "agentMessage", "text": record},
			}
			var delivered []string
			sink := contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
				if event.Kind != contract.TurnEventTextDelta || event.Text == "" || event.Output != nil || event.Interaction != nil {
					t.Fatalf("unexpected history event: %+v", event)
				}
				delivered = append(delivered, event.Text)
				return nil
			})
			if err := output.restore(t.Context(), sink, quietTurnItems(map[string]any{"itemsView": "full", "items": items})); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != "Intro. \n\nWhich option?" {
				t.Fatalf("recovered question = %q", got)
			}
			items = append(items, map[string]any{"id": "answer", "type": "agentMessage", "phase": "final_answer", "text": "Done."})
			for range 2 {
				if err := output.restore(t.Context(), sink, quietTurnItems(map[string]any{"itemsView": "full", "items": items})); err != nil {
					t.Fatal(err)
				}
			}
			if got := output.String(); got != "Intro. Done." || !slices.Equal(delivered, []string{"Intro. ", "Done."}) {
				t.Fatalf("recovered response = %q, events = %q", got, delivered)
			}
		})
	}
}

func TestQuietTurnRestoresBufferedTextWhenToolHasNoResult(t *testing.T) {
	var output conversationOutput
	output.observeTool(contract.TurnEventToolCallStart, &contract.ToolActivity{ID: "tool", Kind: "exec_command"})
	output.apply("second", "final_answer", "Sec", false)
	history := quietTurnItems(map[string]any{"itemsView": "full", "status": "interrupted", "items": []any{
		map[string]any{"id": "tool", "type": "commandExecution", "status": "inProgress"},
		map[string]any{"id": "first", "type": "agentMessage", "text": "First. "},
		map[string]any{"id": "second", "type": "agentMessage", "text": "Second."},
	}})
	var events []contract.TurnEvent
	sink := contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
		events = append(events, event)
		return nil
	})
	if err := output.restoreFailure(t.Context(), sink, history.failure(errors.New("interrupted"))); err != nil {
		t.Fatal(err)
	}
	want := []contract.RecoveredTextPart{{Text: "First. "}, {Text: "Second."}}
	if len(events) != 2 || !events[0].TextSnapshot || events[0].Text != "First. " || !slices.Equal(events[0].RecoveredText, want) ||
		events[1].Text != "ond." || len(events[1].RecoveredText) != 0 {
		t.Fatalf("recovered events = %+v; want history on the first text snapshot", events)
	}
}

func TestQuietTurnRestoresSnapshotStillBufferedBehindTool(t *testing.T) {
	for _, tc := range []struct {
		phase string
		want  []contract.RecoveredTextPart
	}{
		{"final_answer", []contract.RecoveredTextPart{{Text: "New"}}},
		{"commentary", []contract.RecoveredTextPart{}},
	} {
		t.Run(tc.phase, func(t *testing.T) {
			var output conversationOutput
			output.observeTool(contract.TurnEventToolCallStart, &contract.ToolActivity{ID: "tool", Kind: "exec_command"})
			output.apply("answer", "unknown", "Old", false)
			output.apply("answer", tc.phase, "New", true)
			history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
				map[string]any{"id": "answer", "type": "agentMessage", "text": "New", "phase": tc.phase},
			}})
			var events []contract.TurnEvent
			for range 2 {
				err := output.restore(t.Context(), contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
					events = append(events, event)
					return nil
				}), history)
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(events) != 1 || !events[0].TextSnapshot || events[0].ItemID != "answer" || events[0].RecoveredText == nil || !slices.Equal(events[0].RecoveredText, tc.want) {
				t.Fatalf("recovered events = %+v", events)
			}
		})
	}
}
