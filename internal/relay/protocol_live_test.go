package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/apiclient"
	"github.com/google/uuid"
)

func TestLiveExecutorRestartOwnership(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "terminal", true: "interrupted"}[interrupted], func(t *testing.T) {
			_, publisher, caller, _ := liveRelayClients(t)
			publication := testPublish(t, publisher, "restart-ownership", "public")
			owner := "previous-" + uuid.NewString()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			conn, err := publisher.connect(ctx, publication.Agent.ID, owner)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err = conn.WriteJSON(Frame{Type: "hello", ProtocolVersion: 1, InstanceID: owner, Ready: true}); err != nil {
				t.Fatal(err)
			}
			var ack Frame
			if err = conn.ReadJSON(&ack); err != nil {
				t.Fatal(err)
			}
			waitOnline(t, caller, publication.Agent.ID)
			callDone := make(chan struct{})
			go func() {
				defer close(callDone)
				_, _ = caller.Call(ctx, uuid.NewString(), CallRequest{Model: publication.Agent.ID, Input: textInput("work")})
			}()
			var request Frame
			if err = conn.ReadJSON(&request); err != nil {
				t.Fatal(err)
			}
			if request.Type != "request" {
				t.Fatalf("unexpected frame: %s", request.Type)
			}
			if err = conn.WriteJSON(Frame{Type: "request.accepted", ResponseID: request.ResponseID}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, func() bool {
				var response Response
				return caller.Request(ctx, http.MethodGet, "/v1/responses/"+request.ResponseID, "", nil, &response) == nil && response.Status == "in_progress"
			})
			conn.Close()
			state := filepath.Join(t.TempDir(), "executor.json")
			result := Frame{Type: "response.completed", ResponseID: request.ResponseID, Sequence: 1, OutputText: "durable-result"}
			if interrupted {
				result = Frame{Type: "request.accepted", ResponseID: request.ResponseID}
			}
			writeExecutorFixture(t, state, map[string]executionRecord{request.ResponseID: {InstanceID: owner, Result: result}}, map[string]executionSession{request.SessionID: {Tip: request.ResponseID, Dirty: interrupted}})
			e := &Executor{Client: publisher, AgentID: publication.Agent.ID, StatePath: state, Handler: handlerFunc{run: func(context.Context, Frame) Frame { t.Error("restored execution repeated"); return Frame{} }}}
			startExecutor(t, e)
			waitFor(t, func() bool {
				var response Response
				if caller.Request(ctx, http.MethodGet, "/v1/responses/"+request.ResponseID, "", nil, &response) != nil {
					return false
				}
				if interrupted {
					return response.Status == "failed" && response.Error != nil && response.Error.Code == "execution_interrupted"
				}
				return response.Status == "completed" && response.Text() == "durable-result"
			})
			waitFor(t, func() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.records[request.ResponseID].Acked })
			waitOnline(t, caller, publication.Agent.ID)
			select {
			case <-callDone:
			case <-ctx.Done():
				t.Fatal("caller did not observe restored terminal state")
			}
		})
	}
}

func TestLiveNonTipContinuationDoesNotInheritLaterHistory(t *testing.T) {
	endpoint, publisher, caller, _ := liveRelayClients(t)
	publication := testPublish(t, publisher, "branch-guard", "public")
	var mu sync.Mutex
	var history []string
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeTestJSON(w, 200, map[string]any{"id": "local", "status": "running", "runtime": map[string]any{"name": "codex"}})
			return
		}
		var body struct {
			Input string `json:"input"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		history = append(history, body.Input)
		output := strings.Join(history, ",")
		mu.Unlock()
		writeTestJSON(w, 200, Response{Status: "completed", Output: []Output{{Content: []Content{{Type: "output_text", Text: output}}}}})
	}))
	defer local.Close()
	e := &Executor{Client: publisher, AgentID: publication.Agent.ID, StatePath: filepath.Join(t.TempDir(), "state.json"), Handler: &LocalExecutor{Client: apiclient.New(local.URL, "", local.Client()), AgentID: "local", Scope: endpoint + publication.Agent.ID}}
	startExecutor(t, e)
	waitOnline(t, caller, publication.Agent.ID)
	a, err := caller.Call(context.Background(), uuid.NewString(), CallRequest{Model: publication.Agent.ID, Input: textInput("A")})
	if err != nil || a.Status != "completed" {
		t.Fatalf("first turn failed: %v", err)
	}
	b, err := caller.Call(context.Background(), uuid.NewString(), CallRequest{Model: publication.Agent.ID, Input: textInput("B"), PreviousResponseID: a.ID})
	if err != nil || b.Text() != "A,B" {
		t.Fatalf("linear continuation failed: %v %+v", err, b)
	}
	branch, err := caller.Call(context.Background(), uuid.NewString(), CallRequest{Model: publication.Agent.ID, Input: textInput("C"), PreviousResponseID: a.ID})
	if err != nil || branch.Status != "failed" || branch.Error == nil || branch.Error.Code != "relay_branch_unsupported" {
		t.Fatalf("branch not explicitly rejected: %v %+v", err, branch)
	}
	d, err := caller.Call(context.Background(), uuid.NewString(), CallRequest{Model: publication.Agent.ID, Input: textInput("D"), PreviousResponseID: b.ID})
	if err != nil || d.Text() != "A,B,D" {
		t.Fatalf("branch contaminated the real tip: %v %+v", err, d)
	}
}
