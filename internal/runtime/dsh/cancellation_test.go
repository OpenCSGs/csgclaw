package dsh

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
)

type testACPFrame struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
}

func TestConversationCancellationWaitsForPromptTerminalResponse(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	client := newACPClient(clientReader, clientWriter)
	t.Cleanup(func() {
		_ = clientWriter.Close()
		_ = serverReader.Close()
		_ = serverWriter.Close()
		_ = clientReader.Close()
	})

	frames := make(chan testACPFrame, 4)
	go func() {
		scanner := bufio.NewScanner(serverReader)
		for scanner.Scan() {
			var item testACPFrame
			if json.Unmarshal(scanner.Bytes(), &item) == nil {
				frames <- item
			}
		}
	}()

	proc := &process{
		client:    client,
		workspace: t.TempDir(),
		meta: runtimeMetadata{
			RuntimeID: "rt-agent-test",
			Sessions:  map[string]string{"room-1": "session-1"},
		},
		done:   make(chan struct{}),
		active: make(map[string]*activeTurn),
		ready:  map[string]bool{"session-1": true},
	}
	runtime := &Runtime{
		processes: map[string]*process{"rt-agent-test": proc},
		pending:   make(map[string]*pendingPermission),
	}
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan contract.TurnResult, 1)
	go func() {
		resultCh <- runtime.Conversation("rt-agent-test").Run(ctx, contract.TurnRequest{
			ID: "turn-1", ConversationKey: "room-1", Interaction: contract.InteractionResolve,
			Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "hello"}},
		}, nil)
	}()

	prompt := receiveACPFrame(t, frames)
	if prompt.Method != "session/prompt" || prompt.ID == 0 {
		t.Fatalf("first ACP frame = %+v, want session/prompt request", prompt)
	}
	cancel()
	cancelFrame := receiveACPFrame(t, frames)
	if cancelFrame.Method != "session/cancel" {
		t.Fatalf("cancellation ACP frame = %+v, want session/cancel", cancelFrame)
	}
	select {
	case result := <-resultCh:
		t.Fatalf("Run() returned before prompt terminal response: %+v", result)
	case <-time.After(30 * time.Millisecond):
	}

	if err := json.NewEncoder(serverWriter).Encode(map[string]any{
		"jsonrpc": "2.0", "id": prompt.ID, "result": map[string]any{"stopReason": "cancelled"},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-resultCh:
		if result.Status != contract.TurnCanceled || result.Error == nil || result.Error.Code != contract.ErrorCanceled {
			t.Fatalf("Run() after cancellation acknowledgement = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not finish after prompt terminal response")
	}
}

func TestACPClientCancellationWaitIsBounded(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	client := newACPClient(clientReader, io.Discard)
	t.Cleanup(func() {
		_ = serverWriter.Close()
		_ = clientReader.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	accepted := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- client.callRetainingOnCancel(ctx, "session/prompt", map[string]any{}, nil, func() {
			close(accepted)
		}, 20*time.Millisecond, nil)
	}()
	<-accepted
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, errACPCancellationTimeout) {
			t.Fatalf("callRetainingOnCancel() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("callRetainingOnCancel() exceeded its cancellation bound")
	}
	client.mu.Lock()
	pending := len(client.pending)
	client.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending ACP requests = %d, want 0", pending)
	}
}

func receiveACPFrame(t *testing.T, frames <-chan testACPFrame) testACPFrame {
	t.Helper()
	select {
	case item := <-frames:
		return item
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ACP frame")
		return testACPFrame{}
	}
}
