package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/apiclient"
	"github.com/gorilla/websocket"
)

func writeExecutorFixture(t *testing.T, path string, records map[string]executionRecord, sessions map[string]executionSession) {
	t.Helper()
	data, err := json.Marshal(executorState{Version: 1, Records: records, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryReclaimsOriginalOwnerBeforeFreshAdmission(t *testing.T) {
	for _, kind := range []string{"response.completed", "request.accepted"} {
		t.Run(kind, func(t *testing.T) {
			owner := "original-owner"
			received := make(chan Frame, 1)
			fresh := make(chan Frame, 1)
			upgrade := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrade.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				var hello Frame
				if conn.ReadJSON(&hello) != nil {
					return
				}
				if r.Header.Get("X-CSGRelay-Instance") != hello.InstanceID {
					t.Error("handshake and hello ownership differ")
				}
				if err = conn.WriteJSON(Frame{Type: "hello.ack", HeartbeatIntervalSeconds: 1, HeartbeatTimeoutSeconds: 5}); err != nil {
					return
				}
				if hello.InstanceID == owner {
					if hello.Ready {
						t.Error("recovery connection admitted new work")
					}
					var result Frame
					if conn.ReadJSON(&result) != nil {
						return
					}
					received <- result
					_ = conn.WriteJSON(Frame{Type: "result.ack", ResponseID: result.ResponseID})
				} else {
					fresh <- hello
				}
				var frame Frame
				for conn.ReadJSON(&frame) == nil {
				}
			}))
			defer server.Close()
			state := filepath.Join(t.TempDir(), "state.json")
			writeExecutorFixture(t, state, map[string]executionRecord{"resp_old": {InstanceID: owner, Result: Frame{Type: kind, ResponseID: "resp_old", Sequence: 1, OutputText: "persisted"}}}, map[string]executionSession{"session": {Dirty: kind == "request.accepted"}})
			e := &Executor{Client: testClient(t, server.URL, "fake"), AgentID: "agt_recovery", StatePath: state, Handler: handlerFunc{run: func(context.Context, Frame) Frame { t.Error("old work was executed again"); return Frame{} }}}
			startExecutor(t, e)
			select {
			case frame := <-received:
				if kind == "response.completed" && (frame.Type != kind || frame.OutputText != "persisted") {
					t.Fatalf("terminal result changed: %+v", frame)
				}
				if kind == "request.accepted" && (frame.Type != "response.failed" || frame.Error == nil || frame.Error.Code != "execution_interrupted") {
					t.Fatalf("interrupted work was not reconciled: %+v", frame)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("old owner result missing")
			}
			select {
			case hello := <-fresh:
				if !hello.Ready || hello.InstanceID == owner {
					t.Fatal("fresh instance did not take over after ACK")
				}
			case <-time.After(4 * time.Second):
				t.Fatal("fresh admission did not resume")
			}
			e.mu.Lock()
			acked := e.records["resp_old"].Acked
			e.mu.Unlock()
			if !acked {
				t.Fatal("restored result not acknowledged")
			}
		})
	}
}

func TestLegacyUnownedResultsFailClosedWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data, _ := json.Marshal(map[string]executionRecord{"resp_old": {Result: Frame{Type: "response.completed", ResponseID: "resp_old"}}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	e := &Executor{Client: testClient(t, "http://127.0.0.1:1", "fake"), AgentID: "agt_old", StatePath: path, Handler: handlerFunc{}}
	if err := e.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "original executor instance") {
		t.Fatalf("legacy state was silently replayed: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("unrecoverable evidence was overwritten")
	}
}

func TestContinuationRejectsNonTipAndUncertainHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var executions atomic.Int32
	e := &Executor{StatePath: path, instance: "instance", records: map[string]executionRecord{}, sessions: map[string]executionSession{}, Handler: handlerFunc{run: func(_ context.Context, f Frame) Frame {
		executions.Add(1)
		if f.PreviousResponseID != "" && !f.parentValidated {
			t.Error("parent not validated before dispatch")
		}
		return Frame{Type: "response.completed", OutputText: string(f.Input)}
	}}}
	send := func(Frame) error { return nil }
	invoke := func(response, parent string) Frame {
		t.Helper()
		if err := e.accept(context.Background(), Frame{Type: "request", ResponseID: response, SessionID: "session", PreviousResponseID: parent, Input: textInput(response), Deadline: time.Now().Add(time.Minute)}, send); err != nil {
			t.Fatal(err)
		}
		e.workers.Wait()
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.records[response].Result
	}
	if invoke("A", "").Type != "response.completed" || invoke("B", "A").Type != "response.completed" {
		t.Fatal("linear continuation failed")
	}
	rejected := invoke("C", "A")
	if rejected.Error == nil || rejected.Error.Code != "relay_branch_unsupported" || executions.Load() != 2 {
		t.Fatalf("branch inherited latest history: %+v executions=%d", rejected, executions.Load())
	}
	if invoke("D", "B").Type != "response.completed" {
		t.Fatal("rejected branch damaged the real tip")
	}
	// Restore the journal and check that an old parent is still rejected.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var restored executorState
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	e.records, e.sessions = restored.Records, restored.Sessions
	if invoke("E", "B").Error == nil || executions.Load() != 3 {
		t.Fatal("restart lost continuation tip")
	}
	e.sessions["session"] = executionSession{Tip: "D", Dirty: true}
	if invoke("F", "D").Error == nil || executions.Load() != 3 {
		t.Fatal("uncertain local history was reused")
	}
}

func TestContinuationJournalBoundsCachedTipsWithoutUsingLatestHistory(t *testing.T) {
	e := &Executor{StatePath: filepath.Join(t.TempDir(), "state.json"), instance: "owner", records: map[string]executionRecord{}, sessions: map[string]executionSession{}, Handler: handlerFunc{run: func(context.Context, Frame) Frame { return Frame{Type: "response.completed"} }}}
	e.sessions["old"] = executionSession{Tip: "old-response"}
	for i := 0; i < 1023; i++ {
		e.sessions[fmt.Sprintf("session-%d", i)] = executionSession{Tip: "response", UpdatedAt: time.Now()}
	}
	run := func(frame Frame) Frame {
		t.Helper()
		if err := e.accept(context.Background(), frame, func(Frame) error { return nil }); err != nil {
			t.Fatal(err)
		}
		e.workers.Wait()
		return e.records[frame.ResponseID].Result
	}
	fresh := Frame{ResponseID: "fresh-response", SessionID: "fresh", Input: textInput("new"), Deadline: time.Now().Add(time.Minute)}
	if run(fresh).Type != "response.completed" || len(e.sessions) != 1024 {
		t.Fatal("session journal was not bounded")
	}
	stale := Frame{ResponseID: "stale-branch", SessionID: "old", PreviousResponseID: "old-response", Input: textInput("old branch"), Deadline: time.Now().Add(time.Minute)}
	if result := run(stale); result.Error == nil || result.Error.Code != "relay_branch_unsupported" {
		t.Fatal("evicted context was silently reused")
	}
}

func TestLegacyAcknowledgedStateMigratesWithoutInventingContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := map[string]executionRecord{"old": {Acked: true, Result: Frame{Type: "response.completed", ResponseID: "old"}}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := &Executor{Client: testClient(t, "http://127.0.0.1:1", "fake"), AgentID: "agt_old", StatePath: path, Handler: handlerFunc{}}
	if err := e.Run(ctx); err != nil {
		t.Fatal(err)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state executorState
	if err = json.Unmarshal(migrated, &state); err != nil {
		t.Fatal(err)
	}
	if state.Version != 1 || !state.Records["old"].Acked || len(state.Sessions) != 0 {
		t.Fatalf("unsafe legacy migration: %+v", state)
	}
}

func TestLocalExecutorRejectsAnUnvalidatedParent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	e := &LocalExecutor{Client: apiclient.New(server.URL, "", server.Client()), AgentID: "local", Scope: "test"}
	result := e.Execute(context.Background(), Frame{ResponseID: "resp_child", SessionID: "session", PreviousResponseID: "resp_parent", Input: textInput("branch")})
	if calls.Load() != 0 || result.Error == nil || result.Error.Code != "relay_branch_unsupported" {
		t.Fatal("direct local handler silently ignored its parent")
	}
}

func TestCachedResultRequiresMatchingInvocation(t *testing.T) {
	for _, field := range []string{"input", "session", "parent", "deadline", "instance", "legacy"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			var executions atomic.Int32
			e := &Executor{
				StatePath: path, instance: "instance",
				records: map[string]executionRecord{}, sessions: map[string]executionSession{},
				Handler: handlerFunc{run: func(context.Context, Frame) Frame {
					executions.Add(1)
					return Frame{Type: "response.completed", OutputText: "first result"}
				}},
			}
			first := Frame{ResponseID: "reused", SessionID: "session", Input: textInput("first"), Deadline: time.Now().Add(time.Minute)}
			invoke := func(request Frame) Frame {
				t.Helper()
				var sent Frame
				if err := e.accept(context.Background(), request, func(frame Frame) error {
					sent = frame
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				e.workers.Wait()
				return sent
			}
			invoke(first)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var saved executorState
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			e.records, e.sessions = saved.Records, saved.Sessions
			if replay := invoke(first); replay.Type != "response.completed" || replay.OutputText != "first result" {
				t.Fatalf("matching redelivery lost its result: %+v", replay)
			}

			second := first
			switch field {
			case "input":
				second.Input = textInput("second")
			case "session":
				second.SessionID = "other"
			case "parent":
				second.PreviousResponseID = "parent"
			case "deadline":
				second.Deadline = second.Deadline.Add(time.Second)
			case "instance":
				e.instance = "replacement"
			case "legacy":
				old := e.records[first.ResponseID]
				old.InvocationFingerprint = ""
				e.records[first.ResponseID] = old
			}
			for range 2 {
				result := invoke(second)
				if result.Type != "response.failed" || result.Error == nil || result.Error.Code != "relay_invocation_identity_mismatch" || result.OutputText != "" {
					t.Fatalf("mismatch exposed cached result: %+v", result)
				}
			}
			if executions.Load() != 1 {
				t.Fatalf("mismatch executed work: %d executions", executions.Load())
			}
		})
	}
}

func TestSupersededExecutionCannotOverwriteIdentityRejection(t *testing.T) {
	finish := make(chan struct{})
	e := &Executor{
		StatePath: filepath.Join(t.TempDir(), "state.json"), instance: "instance",
		records: map[string]executionRecord{}, sessions: map[string]executionSession{},
		Handler: handlerFunc{run: func(context.Context, Frame) Frame {
			<-finish
			return Frame{Type: "response.completed", OutputText: "old result"}
		}},
	}
	request := Frame{ResponseID: "reused", SessionID: "old", Input: textInput("first"), Deadline: time.Now().Add(time.Minute)}
	send := func(Frame) error { return nil }
	if err := e.accept(context.Background(), request, send); err != nil {
		close(finish)
		t.Fatal(err)
	}
	request.SessionID = "new"
	err := e.accept(context.Background(), request, send)
	if err == nil {
		request.SessionID = "old"
		err = e.accept(context.Background(), request, send)
	}
	close(finish)
	e.workers.Wait()
	if err != nil {
		t.Fatal(err)
	}
	result := e.records[request.ResponseID].Result
	if result.Error == nil || result.Error.Code != "relay_invocation_identity_mismatch" || result.OutputText != "" {
		t.Fatalf("old execution overwrote the rejection: %+v", result)
	}
}
