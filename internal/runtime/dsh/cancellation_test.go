package dsh

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
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

func TestConversationCancellationTimeoutStopsProcessBeforeReturningDeliveryError(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX launcher")
	}
	originalTimeout := dshPromptCancellationTimeout
	dshPromptCancellationTimeout = 20 * time.Millisecond
	t.Cleanup(func() { dshPromptCancellationTimeout = originalTimeout })

	root := t.TempDir()
	launcher := filepath.Join(root, "dsh-test")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexec \"$DSH_TEST_BINARY\" -test.run=TestDSHHelperProcess\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GO_WANT_DSH_HELPER_PROCESS", "1")
	t.Setenv("DSH_TEST_BINARY", os.Args[0])
	t.Setenv("DSH_TEST_HANG_PROMPT", "1")
	profile := agentruntime.Profile{BaseURL: "https://gateway.example/v1", APIKey: "secret-key", ModelID: "test-model", ReasoningEffort: "auto"}
	ref := AgentRef{ID: "agent-timeout", RuntimeID: "rt-agent-timeout", Profile: profile}
	runtime := New(Dependencies{
		ResolveBinary: func(context.Context, string) (dshcli.Info, error) {
			return dshcli.Info{Path: launcher, Version: "0.1.5-rc.2"}, nil
		},
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) { return ref, nil },
		AgentHome:    func(string) (string, error) { return filepath.Join(root, "agent"), nil },
	})
	t.Cleanup(func() { _ = runtime.Close() })
	if err := runtime.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: ref.RuntimeID, AgentID: ref.ID, AgentName: "timeout", Instructions: "Be exact.", Profile: profile,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if _, err := runtime.New(context.Background(), agentruntime.Spec{RuntimeID: ref.RuntimeID, AgentID: ref.ID, Profile: profile}); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	deliveryFailed := make(chan struct{})
	resultCh := make(chan contract.TurnResult, 1)
	go func() {
		resultCh <- runtime.Conversation(ref.RuntimeID).Run(ctx, contract.TurnRequest{
			ID: "turn-timeout", ConversationKey: "room-timeout", Interaction: contract.InteractionResolve,
			Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "hello"}},
		}, contract.EventSinkFunc(func(context.Context, contract.TurnEvent) error {
			select {
			case <-deliveryFailed:
			default:
				close(deliveryFailed)
			}
			return errors.New("transcript delivery failed")
		}))
	}()
	select {
	case <-deliveryFailed:
	case <-time.After(time.Second):
		t.Fatal("DSH helper did not emit an event")
	}
	cancel()

	select {
	case result := <-resultCh:
		if result.Status != contract.TurnFailed || result.Error == nil || result.Error.Code != contract.ErrorRuntimeFailed {
			t.Fatalf("Run() = %+v, want original delivery failure", result)
		}
		if result.Error.Message != "emit DSH turn event: transcript delivery failed" {
			t.Fatalf("Run() error = %q, want original delivery failure", result.Error.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not finish after cancellation timeout cleanup")
	}
	state, err := runtime.State(context.Background(), agentruntime.Handle{RuntimeID: ref.RuntimeID})
	if err != nil {
		t.Fatalf("State() error = %v", err)
	}
	if state != agentruntime.StateStopped {
		t.Fatalf("runtime state = %q, want stopped", state)
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
