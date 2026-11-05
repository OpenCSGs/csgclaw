package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/apiclient"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func testClient(t *testing.T, endpoint, token string) *Client {
	t.Helper()
	c, err := NewClient(endpoint, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Relay integration cases require a real service and three distinct users.
func liveRelayClients(t *testing.T) (string, *Client, *Client, *Client) {
	t.Helper()
	endpoint := os.Getenv("CSGCLAW_TEST_RELAY_URL")
	tokens := []string{os.Getenv("CSGCLAW_TEST_RELAY_TOKEN"), os.Getenv("CSGCLAW_TEST_RELAY_CALLER_TOKEN"), os.Getenv("CSGCLAW_TEST_RELAY_OTHER_TOKEN")}
	if endpoint == "" || tokens[0] == "" || tokens[1] == "" || tokens[2] == "" {
		t.Skip("live CSGRelay with publisher/caller/other tokens is not configured")
	}
	clients := make([]*Client, 0, 3)
	for _, token := range tokens {
		c, err := NewClient(endpoint, token, scopedTestHTTP{prefix: uuid.NewString() + "-", client: &http.Client{Timeout: 30 * time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
	}
	return endpoint, clients[0], clients[1], clients[2]
}

type scopedTestHTTP struct {
	prefix string
	client *http.Client
}

func (c scopedTestHTTP) Do(req *http.Request) (*http.Response, error) {
	if key := req.Header.Get("Idempotency-Key"); key != "" {
		req.Header.Set("Idempotency-Key", c.prefix+key)
	}
	return c.client.Do(req)
}
func writeTestJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func testPublish(t *testing.T, c *Client, source, visibility string) Publication {
	t.Helper()
	p, err := c.Publish(context.Background(), "publish-"+source, PublishRequest{Name: "Test agent", SourceAgentID: source + "-" + uuid.NewString(), Visibility: visibility})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Request(ctx, http.MethodDelete, "/v1/agents/"+p.Agent.ID, "", nil, nil)
	})
	return p
}
func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}
func startExecutor(t *testing.T, e *Executor) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("executor did not stop")
		}
	})
	return cancel
}
func waitOnline(t *testing.T, c *Client, id string) {
	t.Helper()
	waitFor(t, func() bool {
		var a Agent
		return c.Request(context.Background(), "GET", "/v1/agents/"+id, "", nil, &a) == nil && a.Status == "online"
	})
}
func textInput(s string) json.RawMessage { b, _ := json.Marshal(s); return b }
func requireHTTP(t *testing.T, err error, status int) {
	t.Helper()
	var h *HTTPError
	if !errors.As(err, &h) || h.Status != status {
		t.Fatalf("error %v, want HTTP %d", err, status)
	}
}

// Exercise both network hops, including the existing local Session API shape.
func TestRemoteInvocationSessionIsolationAndAccess(t *testing.T) {
	endpoint, publisher, caller, other := liveRelayClients(t)
	p := testPublish(t, publisher, "local-agent", "public")
	var mu sync.Mutex
	var sessions []string
	var executions int
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-only-secret" {
			t.Error("missing local credential")
		}
		if r.Method == "GET" {
			writeTestJSON(w, 200, map[string]any{"id": "local-agent", "name": "Test agent", "runtime": map[string]any{"name": "codex", "state": "running"}})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			t.Errorf("unexpected local path %s", r.URL.Path)
		}
		var req struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		sessions = append(sessions, r.URL.Path)
		executions++
		mu.Unlock()
		writeTestJSON(w, 200, Response{Status: "completed", Output: []Output{{Content: []Content{{Type: "output_text", Text: "answer:" + req.Input}}}}})
	}))
	defer local.Close()
	e := &Executor{Client: publisher, AgentID: p.Agent.ID, StatePath: filepath.Join(t.TempDir(), "state.json"), Handler: &LocalExecutor{Client: apiclient.New(local.URL, "local-only-secret", local.Client()), AgentID: "local-agent", Scope: endpoint + ":" + p.Agent.ID}}
	startExecutor(t, e)
	waitOnline(t, caller, p.Agent.ID)
	first, err := caller.Call(context.Background(), "call-1", CallRequest{Model: p.Agent.ID, Input: textInput("hello")})
	if err != nil || first.Text() != "answer:hello" {
		t.Fatalf("%+v %v", first, err)
	}
	replay, err := caller.Call(context.Background(), "call-1", CallRequest{Model: p.Agent.ID, Input: textInput("hello")})
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	second, err := caller.Call(context.Background(), "call-2", CallRequest{Model: p.Agent.ID, Input: textInput("follow up"), PreviousResponseID: first.ID})
	if err != nil || second.Status != "completed" {
		t.Fatalf("continuation: %+v %v", second, err)
	}
	_, err = caller.Call(context.Background(), "call-3", CallRequest{Model: p.Agent.ID, Input: textInput("new session")})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if executions != 3 || sessions[0] != sessions[1] || sessions[0] == sessions[2] {
		t.Errorf("unexpected executions/sessions: %d %v", executions, sessions)
	}
	mu.Unlock()
	var response Response
	err = other.Request(context.Background(), "GET", "/v1/responses/"+first.ID, "", nil, &response)
	requireHTTP(t, err, 404)
	_, err = other.Call(context.Background(), "steal-session", CallRequest{Model: p.Agent.ID, Input: textInput("steal"), PreviousResponseID: first.ID})
	requireHTTP(t, err, 404)
	_, err = caller.Call(context.Background(), "call-1", CallRequest{Model: p.Agent.ID, Input: textInput("changed")})
	requireHTTP(t, err, 409)
	var sse strings.Builder
	err = caller.Stream(context.Background(), "call-1", CallRequest{Model: p.Agent.ID, Input: textInput("hello")}, &sse)
	if err != nil || !strings.Contains(sse.String(), "event: response.completed") || !strings.Contains(sse.String(), first.ID) {
		t.Fatalf("SSE replay %s %v", sse.String(), err)
	}
	waitFor(t, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		for _, record := range e.records {
			if !record.Acked {
				return false
			}
		}
		return true
	})
	info, err := os.Stat(e.StatePath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions: %v %v", info, err)
	}
}

type handlerFunc struct {
	run func(context.Context, Frame) Frame
}

func (handlerFunc) Ready(context.Context) bool                   { return true }
func (h handlerFunc) Execute(ctx context.Context, f Frame) Frame { return h.run(ctx, f) }

func TestCancelInflightIdempotencyAndClientDisconnect(t *testing.T) {
	_, publisher, caller, _ := liveRelayClients(t)
	p := testPublish(t, publisher, "cancel-agent", "public")
	started := make(chan Frame, 1)
	var calls atomic.Int32
	e := &Executor{Client: publisher, AgentID: p.Agent.ID, StatePath: filepath.Join(t.TempDir(), "state.json"), Handler: handlerFunc{run: func(ctx context.Context, f Frame) Frame {
		calls.Add(1)
		started <- f
		<-ctx.Done()
		return Frame{Type: "response.cancelled"}
	}}}
	startExecutor(t, e)
	waitOnline(t, caller, p.Agent.ID)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	req := CallRequest{Model: p.Agent.ID, Input: textInput("wait")}
	go func() { _, err := caller.Call(ctx, "slow-call", req); done <- err }()
	var frame Frame
	select {
	case frame = <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("not started")
	}
	_, err := caller.Call(context.Background(), "slow-call", req)
	requireHTTP(t, err, 409)
	cancel()
	<-done // Closing the consumer connection must not cancel execution.
	if calls.Load() != 1 {
		t.Fatal("duplicate execution")
	}
	var live Response
	if err = caller.Request(context.Background(), "GET", "/v1/responses/"+frame.ResponseID, "", nil, &live); err != nil || live.Status != "in_progress" {
		t.Fatalf("disconnected call: %+v %v", live, err)
	}
	var ack map[string]any
	err = caller.Request(context.Background(), "POST", "/v1/responses/"+frame.ResponseID+"/cancel", "", map[string]any{}, &ack)
	if err != nil || ack["cancel_requested"] != true {
		t.Fatalf("cancel: %v %v", ack, err)
	}
	waitFor(t, func() bool {
		var response Response
		return caller.Request(context.Background(), http.MethodGet, "/v1/responses/"+frame.ResponseID, "", nil, &response) == nil && response.Status == "cancelled"
	})
}

func TestRegistrationOwnershipVisibilityAndValidation(t *testing.T) {
	_, publisher, caller, _ := liveRelayClients(t)
	p := testPublish(t, publisher, "private", "private")
	var a Agent
	requireHTTP(t, caller.Request(context.Background(), "GET", "/v1/agents/"+p.Agent.ID, "", nil, &a), 404)
	requireHTTP(t, caller.Request(context.Background(), "DELETE", "/v1/agents/"+p.Agent.ID, "", nil, nil), 404)
	req := PublishRequest{Name: "Changed", SourceAgentID: p.Agent.SourceAgentID, Visibility: "public"}
	updated, err := publisher.Publish(context.Background(), "new-key", req)
	if err != nil || updated.Agent.ID != p.Agent.ID {
		t.Fatalf("identity changed: %+v %v", updated, err)
	}
	requireHTTP(t, publisher.Request(context.Background(), "POST", "/v1/agents", "bad-fields", map[string]any{"name": "x", "source_agent_id": "x", "owner_id": "bob"}, nil), 400)
	requireHTTP(t, publisher.Request(context.Background(), "POST", "/v1/agents", "", req, nil), 400)
	if err = publisher.Request(context.Background(), "DELETE", "/v1/agents/"+p.Agent.ID, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	_, err = publisher.Publish(context.Background(), "new-key", req)
	requireHTTP(t, err, 404) // Old publication replay cannot resurrect.
	requireHTTP(t, publisher.Request(context.Background(), "GET", "/v1/agents/"+p.Agent.ID, "", nil, &a), 404)
	_, err = publisher.Publish(context.Background(), "republish", req)
	if err != nil {
		t.Fatal(err)
	}
	if err = caller.Request(context.Background(), "GET", "/v1/agents/"+p.Agent.ID, "", nil, &a); err != nil {
		t.Fatal(err)
	}
	_, err = caller.Call(context.Background(), "offline", CallRequest{Model: p.Agent.ID, Input: textInput("hello")})
	requireHTTP(t, err, 503)
}

// A dropped connection before result.ack must resend exactly the same terminal
// result on reconnect, with the same process identity and no second execution.
func TestExecutorReconnectResendsUnacknowledgedResult(t *testing.T) {
	var connections atomic.Int32
	var executions atomic.Int32
	frames := make(chan Frame, 2)
	instances := make(chan string, 2)
	u := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		number := connections.Add(1)
		var f Frame
		if conn.ReadJSON(&f) != nil {
			return
		}
		instances <- f.InstanceID
		_ = conn.WriteJSON(Frame{Type: "hello.ack", HeartbeatIntervalSeconds: 1, HeartbeatTimeoutSeconds: 5})
		if number == 1 {
			_ = conn.WriteJSON(Frame{Type: "request", ResponseID: "resp_retry", SessionID: "sess_retry", Input: textInput("hello"), Deadline: time.Now().Add(time.Minute)})
		}
		for conn.ReadJSON(&f) == nil {
			if f.Type == "response.completed" {
				frames <- f
				if number == 1 {
					return
				}
				_ = conn.WriteJSON(Frame{Type: "result.ack", ResponseID: f.ResponseID})
				for conn.ReadJSON(&f) == nil {
				}
				return
			}
		}
	}))
	defer server.Close()
	e := &Executor{Client: testClient(t, server.URL, "secret"), AgentID: "agt_retry", StatePath: filepath.Join(t.TempDir(), "state.json"), Handler: handlerFunc{run: func(context.Context, Frame) Frame {
		executions.Add(1)
		return Frame{Type: "response.completed", OutputText: "once"}
	}}}
	startExecutor(t, e)
	var first, second Frame
	select {
	case first = <-frames:
	case <-time.After(4 * time.Second):
		t.Fatal("first result missing")
	}
	select {
	case second = <-frames:
	case <-time.After(6 * time.Second):
		t.Fatal("result was not retransmitted")
	}
	if first.ResponseID != second.ResponseID || first.Sequence != second.Sequence || second.OutputText != "once" || executions.Load() != 1 {
		t.Fatalf("retransmission: %+v %+v executions=%d", first, second, executions.Load())
	}
	if a, b := <-instances, <-instances; a == "" || a != b {
		t.Fatalf("process instance changed: %q %q", a, b)
	}
}

func TestExecutorRestartDoesNotRepeatInterruptedWork(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	data, _ := json.Marshal(map[string]executionRecord{"resp_interrupted": {Result: Frame{Type: "request.accepted", ResponseID: "resp_interrupted"}}})
	if err := os.WriteFile(state, data, 0600); err != nil {
		t.Fatal(err)
	}
	received := make(chan Frame, 1)
	u := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := u.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var f Frame
		if conn.ReadJSON(&f) != nil {
			return
		}
		_ = conn.WriteJSON(Frame{Type: "hello.ack", HeartbeatIntervalSeconds: 15})
		if conn.ReadJSON(&f) == nil {
			received <- f
		}
		for conn.ReadJSON(&f) == nil {
		}
	}))
	defer server.Close()
	e := &Executor{Client: testClient(t, server.URL, "secret"), AgentID: "agt_restart", StatePath: state, Handler: handlerFunc{run: func(context.Context, Frame) Frame { t.Error("interrupted work repeated"); return Frame{} }}}
	startExecutor(t, e)
	select {
	case f := <-received:
		if f.Type != "response.failed" || f.Error == nil || f.Error.Code != "execution_interrupted" {
			t.Fatalf("unexpected recovery: %+v", f)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("recovery result missing")
	}
}

func TestTextInputRejectsUnsupportedContent(t *testing.T) {
	for _, input := range []string{`""`, `[]`, `null`, `[{"role":"system","content":"x"}]`, `[{"role":"user","content":[{"type":"input_image","text":"x"}]}]`, `[{"role":"user","content":"x","tools":[]}]`} {
		if _, err := InputText(json.RawMessage(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	text, err := InputText(json.RawMessage(`[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]`))
	if err != nil || text != "hello" {
		t.Fatalf("%q %v", text, err)
	}
}

func TestClientRejectsCredentialBearingURLs(t *testing.T) {
	for _, endpoint := range []string{"file:///tmp/x", "https://user:secret@localhost", "https://localhost?token=secret", "https://localhost#secret"} {
		if _, err := NewClient(endpoint, "token", nil); err == nil {
			t.Errorf("accepted %s", endpoint)
		}
	}
}

func TestLocalCancellationWaitsForCleanupAndQuarantinesUnconfirmedStop(t *testing.T) {
	for _, confirm := range []bool{true, false} {
		t.Run(fmt.Sprint(confirm), func(t *testing.T) {
			started := make(chan struct{})
			stopping := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/cancel") {
					close(stopping)
					<-release
					if confirm {
						w.WriteHeader(204)
					} else {
						w.WriteHeader(503)
					}
					return
				}
				if r.Method == "GET" {
					writeTestJSON(w, 200, map[string]any{"id": "local", "runtime": map[string]any{"name": "codex", "state": "running"}})
					return
				}
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				<-release
			}))
			defer server.Close()
			defer releaseOnce.Do(func() { close(release) })
			local := &LocalExecutor{Client: apiclient.New(server.URL, "local", server.Client()), AgentID: "local", Scope: "relay-test"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan Frame, 1)
			go func() {
				done <- local.Execute(ctx, Frame{ResponseID: "resp_cancel", SessionID: "sess_cancel", Input: textInput("wait")})
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("local request not started")
			}
			cancel()
			select {
			case <-stopping:
			case <-time.After(time.Second):
				t.Fatal("explicit cleanup was not requested")
			}
			select {
			case <-done:
				t.Fatal("reported cancellation before cleanup confirmation")
			default:
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case result := <-done:
				if confirm && result.Type != "response.cancelled" {
					t.Fatalf("%+v", result)
				}
				if !confirm && (result.Type != "response.failed" || result.Error.Code != "cancellation_unconfirmed" || local.Ready(context.Background())) {
					t.Fatalf("unconfirmed stop accepted: %+v", result)
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup did not return")
			}
		})
	}
}

func ExampleResponse_Text() {
	r := Response{Output: []Output{{Content: []Content{{Type: "output_text", Text: "hello"}}}}}
	fmt.Println(r.Text()) // Output: hello
}

func TestLocalExecutorReportsUnsupportedModelWithoutLeakingUpstream(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestJSON(w, 503, map[string]any{"error": map[string]string{"code": "agent_response_failed", "message": "this request contains Responses tool definitions or tool-use history that cannot be preserved by text-only Chat Completions; select a provider/model with native Responses tool support; secret=must-not-leak"}})
	}))
	defer local.Close()
	executor := LocalExecutor{Client: apiclient.New(local.URL, "local-test", local.Client()), AgentID: "local"}
	result := executor.Execute(context.Background(), Frame{ResponseID: "resp_test", SessionID: "sess_test", Input: textInput("hello")})
	if result.Type != "response.failed" || result.Error == nil || result.Error.Code != "model_protocol_unsupported" {
		t.Fatalf("missing model compatibility diagnosis: %+v", result)
	}
	if strings.Contains(result.Error.Message, "local-test") || strings.Contains(result.Error.Message, "must-not-leak") {
		t.Fatal("credential leaked")
	}
}
