package codex

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/codexmodel"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
)

type modelAcceptanceFixture struct {
	manager  *appServerManager
	live     *liveSession
	requests quietTurnRequests
	selected contract.TurnModel
	sink     *recordingSink
}

func newModelAcceptanceFixture(t *testing.T) *modelAcceptanceFixture {
	t.Helper()
	profile := agentruntime.Profile{Provider: "api", ModelID: "model-a", ModelMetadata: modelcap.Resolved{ContextWindow: 32000, ContextSource: "provider"}}
	selected := contract.TurnModel{ModelID: "model-b", Metadata: modelcap.Resolved{ContextWindow: 64000, ContextSource: "provider"}}
	requests := make(quietTurnRequests, 8)
	live := &liveSession{
		session: &Session{RuntimeID: "runtime", SessionID: "thread"},
		spec: SessionSpec{RuntimeID: "runtime", Profile: profile, ModelCatalog: []codexmodel.Profile{
			{Provider: profile.Provider, ModelID: selected.ModelID, Metadata: selected.Metadata},
		}},
		appClient: newAppServerClient(requests, nil), done: make(chan struct{}),
	}
	sink := &recordingSink{}
	manager := newAppServerManager(managerDeps{EventSink: sink})
	manager.sessions["runtime"] = live
	return &modelAcceptanceFixture{manager: manager, live: live, requests: requests, selected: selected, sink: sink}
}

func (f *modelAcceptanceFixture) receive(t *testing.T, method string) appServerWireMessage {
	t.Helper()
	synctest.Wait()
	select {
	case request := <-f.requests:
		if request.Method != method {
			t.Fatalf("RPC = %s, want %s", request.Method, method)
		}
		return request
	default:
		t.Fatalf("missing RPC %s", method)
		return appServerWireMessage{}
	}
}

func (f *modelAcceptanceFixture) snapshot(t *testing.T, model string, window int64) modelcap.ContextUsage {
	t.Helper()
	f.manager.publishContextUsage("runtime", f.live, "thread", nil, "snapshot")
	usage := f.live.contextUsage["thread"]
	if usage.ModelID != model || usage.ContextWindow != window {
		t.Fatalf("context snapshot = %+v, want %s/%d", usage, model, window)
	}
	return usage
}

func TestAppServerRejectedStartPreservesContextModel(t *testing.T) {
	for _, previousOverride := range []bool{false, true} {
		name := "profile_default"
		if previousOverride {
			name = "previous_override"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newModelAcceptanceFixture(t)
				previous := turnModelFromProfile(f.live.spec.Profile)
				selected := f.selected
				if previousOverride {
					previous, selected = selected, previous
					waiter, err := f.live.registerAppServerTurnWaiterWithModel("thread", previous, true)
					if err != nil {
						t.Fatal(err)
					}
					waiter.setTurnID("previous")
					f.live.removeAppServerTurnWaiter("thread", waiter)
				}
				f.manager.publishContextUsage("runtime", f.live, "thread", map[string]any{
					"tokenUsage": map[string]any{"last": map[string]any{"totalTokens": float64(1200)}},
				}, "thread/tokenUsage/updated")
				done := make(chan error, 1)
				go func() {
					_, err := f.manager.Prompt(context.Background(), SessionHandle{RuntimeID: "runtime"}, PromptRequest{SessionID: "thread", Prompt: []PromptContentBlock{{Text: &PromptTextBlock{Text: "hello"}}}, Model: &selected})
					done <- err
				}()
				request := f.receive(t, "turn/start")
				for _, event := range f.sink.snapshot() {
					if event.Kind == activity.RuntimeEventContextUsage && event.Payload.(modelcap.ContextUsage).ModelID != previous.ModelID {
						t.Fatalf("pending start published an unaccepted model: %+v", event.Payload)
					}
				}
				f.snapshot(t, selected.ModelID, selected.Metadata.ContextWindow)
				f.live.appClient.handleMessage(appServerWireMessage{ID: request.ID, Error: json.RawMessage(`{"code":-32600,"message":"turn rejected"}`)})
				synctest.Wait()
				if err := <-done; err == nil || !strings.Contains(err.Error(), "turn rejected") {
					t.Fatalf("Prompt error = %v", err)
				}
				usage := f.snapshot(t, previous.ModelID, previous.Metadata.ContextWindow)
				if usage.UsedTokens == nil || *usage.UsedTokens != 1200 {
					t.Fatalf("rejected start changed prior occupancy: %+v", usage)
				}
			})
		})
	}
}

func TestAppServerAcceptedStartRetainsContextModel(t *testing.T) {
	for _, mode := range []string{"ack", "ack_without_id", "started_before_ack", "canceled_before_ack"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newModelAcceptanceFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					_, err := f.manager.Prompt(ctx, SessionHandle{RuntimeID: "runtime"}, PromptRequest{SessionID: "thread", Prompt: []PromptContentBlock{{Text: &PromptTextBlock{Text: "hello"}}}, Model: &f.selected})
					done <- err
				}()
				request := f.receive(t, "turn/start")
				turnID := "accepted"
				if mode == "started_before_ack" || mode == "canceled_before_ack" {
					f.manager.handleRawAppServerNotification("runtime", f.live, "turn/started", map[string]any{"threadId": "thread", "turnId": turnID})
					f.manager.handleRawAppServerNotification("runtime", f.live, "thread/tokenUsage/updated", map[string]any{
						"threadId": "thread", "turnId": turnID,
						"tokenUsage": map[string]any{"last": map[string]any{"totalTokens": float64(900)}},
					})
					events := f.sink.snapshot()
					var published modelcap.ContextUsage
					for _, event := range events {
						if event.Kind == activity.RuntimeEventContextUsage {
							published = event.Payload.(modelcap.ContextUsage)
						}
					}
					if published.ModelID != "model-b" || published.ContextWindow != 64000 || published.UsedTokens == nil || *published.UsedTokens != 900 {
						t.Fatalf("usage before ACK = %+v", published)
					}
				}
				if mode == "canceled_before_ack" {
					cancel()
					request = f.receive(t, "turn/interrupt")
					f.live.appClient.handleMessage(appServerWireMessage{ID: request.ID, Result: json.RawMessage(`{}`)})
				} else {
					result := json.RawMessage(`{"turn":{"id":"accepted"}}`)
					if mode == "ack_without_id" {
						result, turnID = json.RawMessage(`{}`), ""
					}
					f.live.appClient.handleMessage(appServerWireMessage{ID: request.ID, Result: result})
				}
				synctest.Wait()
				f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: turnID, success: true})
				synctest.Wait()
				err := <-done
				if mode == "canceled_before_ack" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancel error = %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				usage := f.snapshot(t, "model-b", 64000)
				if (mode == "started_before_ack" || mode == "canceled_before_ack") && (usage.UsedTokens == nil || *usage.UsedTokens != 900) {
					t.Fatalf("accepted turn lost measured occupancy: %+v", usage)
				}
			})
		})
	}
}

func TestAppServerRejectedRecoveryPreservesAcceptedContextModel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newModelAcceptanceFixture(t)
		done := make(chan error, 1)
		go func() {
			_, err := f.manager.Prompt(context.Background(), SessionHandle{RuntimeID: "runtime"}, PromptRequest{SessionID: "thread", Prompt: []PromptContentBlock{{Text: &PromptTextBlock{Text: "hello"}}}, Model: &f.selected})
			done <- err
		}()
		request := f.receive(t, "turn/start")
		f.live.appClient.handleMessage(appServerWireMessage{ID: request.ID, Result: json.RawMessage(`{"turn":{"id":"initial"}}`)})
		synctest.Wait()
		f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: "initial", err: &contextWindowError{}})
		request = f.receive(t, "thread/compact/start")
		f.live.appClient.handleMessage(appServerWireMessage{ID: request.ID, Result: json.RawMessage(`{}`)})
		f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: "compact", started: true})
		f.live.notifyAppServerTurn("thread", appServerTurnResult{turnID: "compact", success: true})
		request = f.receive(t, "turn/start")
		f.live.appClient.handleMessage(appServerWireMessage{ID: request.ID, Error: json.RawMessage(`{"code":-32600,"message":"recovery rejected"}`)})
		synctest.Wait()
		if err := <-done; err == nil || !strings.Contains(err.Error(), "recovery rejected") {
			t.Fatalf("recovery error = %v", err)
		}
		if usage := f.snapshot(t, "model-b", 64000); usage.Compacting {
			t.Fatalf("rejected recovery restored an active compaction: %+v", usage)
		}
	})
}
