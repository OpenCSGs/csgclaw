package enginetest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
)

func runDetachedInteractionContract(t *testing.T, factory InterfaceFactory) {
	newClient := func(t *testing.T, expiry *uint64) agentengine.Interface {
		t.Helper()
		return factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{Kind: agentengine.OutputItemRequestUserInput, Payload: activity.RequestUserInputArgs{
				Questions: []activity.RequestUserInputQuestion{{ID: "secret", Header: "Secret", Question: "Disposable value?", IsOther: true, IsSecret: true}}, AutoResolutionMS: expiry,
			}}})
			if err != nil {
				return failed(agentengine.ErrorRuntimeFailed, err.Error())
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		})
	}
	t.Run("detached interaction answer and replay", func(t *testing.T) {
		client := newClient(t, nil)
		conversations := client.Conversations("agent-a")
		request := contractTurn("question", "conversation")
		updates := make(chan agentengine.TurnEvent, 8)
		result := conversations.Run(context.Background(), request, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			if event.Activity != nil && event.Activity.Kind == string(activity.RuntimeEventUserInputResolved) {
				updates <- event
			}
			return nil
		}))
		if result.Status != agentengine.TurnSucceeded || len(result.Interactions) != 1 || !result.Interactions[0].Detached {
			t.Fatalf("Run = %+v", result)
		}
		id := result.Interactions[0].ID
		result.Interactions[0].ID = "mutated-copy"
		replayed := conversations.Run(context.Background(), request, nil)
		if len(replayed.Interactions) != 1 || replayed.Interactions[0].ID != id {
			t.Fatalf("replay lost immutable question: %+v", replayed)
		}
		resolution := agentengine.InteractionResolution{ConversationKey: request.ConversationKey, InteractionID: id, ResponderID: "tester", Answers: map[string]agentengine.InteractionAnswer{"secret": {Values: []string{"user_note: disposable-test-secret"}}}}
		resolution.BeforeResolve = func(context.Context, agentengine.InteractionRequest) error {
			return errors.New("transcript store failed")
		}
		if err := conversations.Resolve(context.Background(), resolution); err == nil {
			t.Fatal("failed transcript was acknowledged")
		}
		pending, err := conversations.GetInteraction(context.Background(), request.ConversationKey, id)
		if err != nil || pending.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusPending {
			t.Fatalf("pending = %+v, %v", pending, err)
		}
		resolution.BeforeResolve = func(_ context.Context, item agentengine.InteractionRequest) error {
			raw, _ := json.Marshal(item)
			if strings.Contains(string(raw), "disposable-test-secret") {
				t.Fatal("transcript received secret")
			}
			return nil
		}
		if err := conversations.Resolve(context.Background(), resolution); err != nil {
			t.Fatal(err)
		}
		if err := conversations.Resolve(context.Background(), resolution); agentengine.ErrorCodeOf(err) != agentengine.ErrorInteractionAlreadyResolved {
			t.Fatalf("duplicate = %v", err)
		}
		select {
		case event := <-updates:
			if event.Sequence == 0 || event.Activity.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusAnswered {
				t.Fatalf("terminal event = %+v", event)
			}
			raw, _ := json.Marshal(event)
			if strings.Contains(string(raw), "disposable-test-secret") {
				t.Fatal("terminal event leaked secret")
			}
		case <-time.After(time.Second):
			t.Fatal("missing post-Turn resolution event")
		}
	})
	t.Run("invalid recreate preserves pending question", func(t *testing.T) {
		client := newClient(t, nil)
		conversations := client.Conversations("agent-a")
		result := conversations.Run(context.Background(), contractTurn("question", "conversation"), nil)
		if len(result.Interactions) != 1 {
			t.Fatalf("question=%+v", result)
		}
		if _, err := client.Agents().Recreate(context.Background(), "agent-a", agentengine.AgentRecreateOptions{Update: &agentengine.AgentUpdateRequest{}}); err == nil {
			t.Fatal("invalid recreate was accepted")
		}
		item, err := conversations.GetInteraction(context.Background(), "conversation", result.Interactions[0].ID)
		if err != nil || item.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusPending {
			t.Fatalf("failed recreate canceled question: %+v %v", item, err)
		}
	})
	for _, action := range []string{"cancel", "reset", "stop", "recreate", "new_turn", "expire"} {
		for _, resolving := range []bool{false, true} {
			name := "detached invalidation " + action
			if resolving {
				name += " during response"
			}
			t.Run(name, func(t *testing.T) {
				var expiry *uint64
				if action == "expire" {
					value := uint64(80)
					expiry = &value
				}
				client := newClient(t, expiry)
				conversations := client.Conversations("agent-a")
				request := contractTurn("question", "conversation")
				result := conversations.Run(context.Background(), request, nil)
				if len(result.Interactions) != 1 {
					t.Fatalf("question = %+v", result)
				}
				var responseDone chan error
				var releaseResponse func()
				if resolving {
					entered, release := make(chan struct{}), make(chan struct{})
					releaseResponse = sync.OnceFunc(func() { close(release) })
					t.Cleanup(releaseResponse)
					responseDone = make(chan error, 1)
					go func() {
						responseDone <- conversations.Resolve(context.Background(), agentengine.InteractionResolution{
							ConversationKey: request.ConversationKey, InteractionID: result.Interactions[0].ID, ResponderID: "tester",
							Answers:       map[string]agentengine.InteractionAnswer{"secret": {Values: []string{"user_note: test-only"}}},
							BeforeResolve: func(context.Context, agentengine.InteractionRequest) error { close(entered); <-release; return nil },
						})
					}()
					select {
					case <-entered:
					case <-time.After(time.Second):
						t.Fatal("response did not reach transcript callback")
					}
				}
				var err error
				switch action {
				case "cancel":
					err = conversations.Cancel(context.Background(), request.ConversationKey, request.ID)
				case "reset":
					err = conversations.Reset(context.Background(), request.ConversationKey)
				case "stop":
					_, err = client.Agents().Update(context.Background(), "agent-a", agentengine.AgentUpdateRequest{Spec: agentengine.AgentSpec{DesiredState: agentengine.AgentDesiredStateStopped}, FieldMask: []string{"desired_state"}})
				case "recreate":
					_, err = client.Agents().Recreate(context.Background(), "agent-a", agentengine.AgentRecreateOptions{})
				case "new_turn":
					conversations.Run(context.Background(), contractTurn("next", "conversation"), nil)
				case "expire":
					deadline := time.Now().Add(time.Second)
					for time.Now().Before(deadline) {
						item, _ := conversations.GetInteraction(context.Background(), request.ConversationKey, result.Interactions[0].ID)
						if item.Payload.(activity.UserInputSnapshot).Status == activity.UserInputStatusExpired {
							break
						}
						time.Sleep(time.Millisecond)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if resolving {
					releaseResponse()
					select {
					case err := <-responseDone:
						if agentengine.ErrorCodeOf(err) != agentengine.ErrorInteractionGone {
							t.Fatalf("in-flight response after %s = %v", action, err)
						}
					case <-time.After(time.Second):
						t.Fatal("interrupted response did not return")
					}
				}
				err = conversations.Resolve(context.Background(), agentengine.InteractionResolution{ConversationKey: request.ConversationKey, InteractionID: result.Interactions[0].ID, ResponderID: "tester"})
				if agentengine.ErrorCodeOf(err) != agentengine.ErrorInteractionGone {
					t.Fatalf("old question after %s = %v", action, err)
				}
			})
		}
	}
}
