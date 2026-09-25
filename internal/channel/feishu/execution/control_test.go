package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel/feishu/interaction"
	feishustate "csgclaw/internal/channel/feishu/state"
)

type blockingControlEngine struct {
	agentengine.Interface
	conversation agentengine.ConversationInterface
}

func (e blockingControlEngine) Conversations(string) agentengine.ConversationInterface {
	return e.conversation
}

type blockingControlConversation struct {
	*fakeConversation
	entered chan struct{}
	release chan struct{}
}

func (c *blockingControlConversation) Reset(ctx context.Context, _ agentengine.ConversationKey) error {
	return c.wait(ctx)
}

func (c *blockingControlConversation) Resolve(ctx context.Context, _ agentengine.InteractionResolution) error {
	return c.wait(ctx)
}

func (c *blockingControlConversation) wait(ctx context.Context) error {
	close(c.entered)
	select {
	case <-c.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestConversationControlDoesNotBlockOtherConversations(t *testing.T) {
	for _, operation := range []string{"reset", "resolve"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conversation := &blockingControlConversation{fakeConversation: &fakeConversation{}, entered: make(chan struct{}), release: make(chan struct{})}
			runner, err := NewRunner(RunnerOptions{Engine: blockingControlEngine{conversation: conversation}, State: feishustate.NewStore()})
			if err != nil {
				t.Fatal(err)
			}
			message := runnerMessage("event-a", "turn-a", "chat-a", "/new")
			message.Source.SenderID = "human"
			if operation == "resolve" {
				runner.latest[message.ConversationKey] = message.TurnID
				request := agentengine.InteractionRequest{ID: "permission", Kind: agentengine.InteractionPermission, Payload: activity.ActivitySnapshot{Status: activity.ActionStatusPending}}
				if err := runner.registerInteraction(ctx, message, request); err != nil {
					t.Fatal(err)
				}
			}
			controlled := make(chan error, 1)
			go func() {
				if operation == "reset" {
					controlled <- runner.Reset(ctx, message)
				} else {
					controlled <- runner.ResolveInteraction(ctx, interaction.Input{AgentID: message.AgentID, ConversationKey: message.ConversationKey, TurnID: message.TurnID, InteractionID: "permission", ResponderID: "human", OptionID: "allow"})
				}
			}()
			select {
			case <-conversation.entered:
			case <-ctx.Done():
				t.Fatal("control did not reach Engine")
			}
			submitted := make(chan error, 1)
			go func() { submitted <- runner.Submit(ctx, runnerMessage("event-b", "turn-b", "chat-b", "hello")) }()
			select {
			case err := <-submitted:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(time.Second):
				t.Error("another conversation waited for the control operation")
			}
			close(conversation.release)
			if err := <-controlled; err != nil {
				t.Error(err)
			}
			if err := runner.Wait(ctx); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestConversationControlSerializesAndReleasesCanceledWaiter(t *testing.T) {
	runner, err := NewRunner(RunnerOptions{Engine: fakeEngine{&fakeConversation{}}, State: feishustate.NewStore()})
	if err != nil {
		t.Fatal(err)
	}
	release, err := runner.acquireControl(context.Background(), "chat")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	secondRelease, err := runner.acquireControl(ctx, "chat")
	if secondRelease != nil {
		secondRelease()
		t.Error("concurrent control acquired for the same conversation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waiting caller: %v", err)
	}
	release()
	release, err = runner.acquireControl(context.Background(), "chat")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if len(runner.controls) != 0 {
		t.Fatalf("unused control entries retained: %d", len(runner.controls))
	}
}
