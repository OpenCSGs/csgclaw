package codex

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAppServerClientRequestHandlesResponse(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := client.request(context.Background(), "thread/start", map[string]any{"cwd": "/tmp/work"})
		resultCh <- result
		errCh <- err
	}()

	req := waitForJSONRPCLine(t, writer)
	if got := req["method"]; got != "thread/start" {
		t.Fatalf("request method = %v, want thread/start", got)
	}
	client.handleLine(`{"jsonrpc":"2.0","id":1,"result":{"threadId":"thread-1"}}`)

	if err := <-errCh; err != nil {
		t.Fatalf("request() error = %v", err)
	}
	var result struct {
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(<-resultCh, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ThreadID != "thread-1" {
		t.Fatalf("threadId = %q, want thread-1", result.ThreadID)
	}
}

func TestAppServerClientRequestHandlesJSONRPCError(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)

	errCh := make(chan error, 1)
	go func() {
		_, err := client.request(context.Background(), "thread/resume", map[string]any{"threadId": "missing"})
		errCh <- err
	}()

	_ = waitForJSONRPCLine(t, writer)
	client.handleLine(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"thread not found","data":{"threadId":"missing"}}}`)

	err := <-errCh
	if err == nil {
		t.Fatal("request() error = nil, want JSON-RPC error")
	}
	for _, want := range []string{"thread/resume", "thread not found", "code=-32000", `"threadId":"missing"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("request() error = %q, want it to contain %q", err.Error(), want)
		}
	}
}

func TestAppServerClientIgnoresUnknownResponseID(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)

	client.handleLine(`{"jsonrpc":"2.0","id":99,"result":{"ok":true}}`)

	if got := writer.String(); got != "" {
		t.Fatalf("writer = %q, want empty", got)
	}
	if got := appServerPendingLen(client); got != 0 {
		t.Fatalf("pending len = %d, want 0", got)
	}
}

func TestAppServerClientDispatchesServerRequest(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)
	client.onServerRequest = func(req appServerServerRequest) (any, error) {
		if got := req.Method; got != "item/commandExecution/requestApproval" {
			t.Fatalf("server request method = %q, want approval request", got)
		}
		var params struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Fatalf("unmarshal params: %v", err)
		}
		if params.Command != "go test ./..." {
			t.Fatalf("command = %q, want go test ./...", params.Command)
		}
		return map[string]any{"decision": "accept"}, nil
	}

	client.handleLine(`{"jsonrpc":"2.0","id":7,"method":"item/commandExecution/requestApproval","params":{"command":"go test ./..."}}`)

	resp := waitForJSONRPCLine(t, writer)
	if got := resp["id"]; got != float64(7) {
		t.Fatalf("response id = %v, want 7", got)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("response result = %#v, want object", resp["result"])
	}
	if got := result["decision"]; got != "accept" {
		t.Fatalf("decision = %v, want accept", got)
	}
}

func TestAppServerClientRespondsErrorForUnhandledServerRequest(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)

	client.handleLine(`{"jsonrpc":"2.0","id":8,"method":"unknown/request"}`)

	resp := waitForJSONRPCLine(t, writer)
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("response error = %#v, want object", resp["error"])
	}
	if got := errObj["code"]; got != float64(-32601) {
		t.Fatalf("error code = %v, want -32601", got)
	}
}

func TestAppServerClientDispatchesNotification(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)
	gotCh := make(chan appServerNotification, 1)
	client.onNotification = func(notification appServerNotification) {
		gotCh <- notification
	}

	client.handleLine(`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thread-1","status":"completed"}}`)

	select {
	case got := <-gotCh:
		if got.Method != "turn/completed" {
			t.Fatalf("notification method = %q, want turn/completed", got.Method)
		}
		if !strings.Contains(string(got.Params), `"threadId":"thread-1"`) {
			t.Fatalf("notification params = %s, want thread id", got.Params)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
	if writer.String() != "" {
		t.Fatalf("writer = %q, want empty for notification", writer.String())
	}
}

func TestAppServerClientIgnoresMalformedLine(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)
	called := false
	client.onNotification = func(appServerNotification) {
		called = true
	}

	client.handleLine(`{not-json`)

	if called {
		t.Fatal("notification handler called for malformed line")
	}
	if got := writer.String(); got != "" {
		t.Fatalf("writer = %q, want empty", got)
	}
}

func TestAppServerClientRequestContextCancellationCleansPending(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := client.request(ctx, "turn/start", map[string]any{"threadId": "thread-1"})
		errCh <- err
	}()

	_ = waitForJSONRPCLine(t, writer)
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request() error = %v, want context.Canceled", err)
	}
	if got := appServerPendingLen(client); got != 0 {
		t.Fatalf("pending len = %d, want 0", got)
	}
}

func TestAppServerClientCloseAllPending(t *testing.T) {
	writer := &lockedStringWriter{}
	client := newAppServerClient(writer, nil)

	errCh := make(chan error, 1)
	go func() {
		_, err := client.request(context.Background(), "turn/start", map[string]any{"threadId": "thread-1"})
		errCh <- err
	}()

	_ = waitForJSONRPCLine(t, writer)
	client.closeAllPending(errors.New("process exited"))

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "process exited") {
		t.Fatalf("request() error = %v, want process exited", err)
	}
	if got := appServerPendingLen(client); got != 0 {
		t.Fatalf("pending len = %d, want 0", got)
	}
}

type lockedStringWriter struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *lockedStringWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedStringWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func waitForJSONRPCLine(t *testing.T, writer *lockedStringWriter) map[string]any {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		raw := strings.TrimSpace(writer.String())
		if raw == "" {
			time.Sleep(time.Millisecond)
			continue
		}
		line := strings.Split(raw, "\n")[0]
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("unmarshal json-rpc line %q: %v", line, err)
		}
		return msg
	}
	t.Fatal("timed out waiting for json-rpc line")
	return nil
}

func appServerPendingLen(client *appServerClient) int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return len(client.pending)
}
