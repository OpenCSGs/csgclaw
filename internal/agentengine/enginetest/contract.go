package enginetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
)

// InterfaceFactory creates one isolated Engine implementation with seeded
// Agents and programmable Turn behavior.
type InterfaceFactory func(testing.TB, []agentengine.Agent, TurnBehavior) agentengine.Interface

// RunInterfaceContract runs the same public Agent Engine behavior checks
// against any Interface implementation.
func RunInterfaceContract(t *testing.T, factory InterfaceFactory) {
	t.Helper()
	t.Run("agent lifecycle and secret handling", func(t *testing.T) {
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateStopped, "codex")}, nil)
		agents := client.Agents()
		created, err := agents.Create(context.Background(), agentengine.AgentSpec{
			Name: "created", Role: agentengine.AgentRoleWorker,
			Runtime: agentengine.RuntimeSpec{Adapter: "openclaw", Sandboxed: true, Image: "contract-openclaw:latest"},
			Model:   agentengine.ModelSpec{ProviderID: "provider-a", ModelID: "model-a"},
		})
		if err != nil || created.ID == "" || created.Spec.Runtime.Credentials != nil {
			t.Fatalf("Create() = %+v, %v", created, err)
		}

		got, err := agents.Get(context.Background(), "agent-a")
		if err != nil || got.Spec.Runtime.Credentials != nil || got.Status.State != agentengine.AgentStateStopped {
			t.Fatalf("Get() = %+v, %v", got, err)
		}
		listed, err := agents.List(context.Background())
		if err != nil || len(listed) != 2 {
			t.Fatalf("List() = %+v, %v", listed, err)
		}
		for _, item := range listed {
			if item.Spec.Runtime.Credentials != nil {
				t.Fatalf("List() leaked credentials: %+v", item)
			}
		}

		updatedSpec := got.Spec
		updatedSpec.Name = "updated"
		updatedSpec.Description = "updated description"
		updatedSpec.Skills = []string{"skill-creator"}
		updatedSpec.Runtime.Credentials = map[string]string{"secrets/token.txt": "contract-secret"}
		updatedSpec.Runtime.InitShell = "test -f secrets/token.txt"
		updatedSpec.MCPServers = map[string]agentengine.MCPServerConfig{
			"local":  {"command": "local-server"},
			"remote": {"url": "https://mcp.example.test"},
		}
		updated, err := agents.Update(context.Background(), got.ID, updatedSpec)
		if err != nil || updated.Spec.Name != "updated" || updated.Spec.Description != "updated description" || len(updated.Spec.MCPServers) != 2 || len(updated.Spec.Skills) != 1 || updated.Spec.Skills[0] != "skill-creator" || updated.Spec.Runtime.Credentials != nil || updated.Spec.Runtime.InitShell != "test -f secrets/token.txt" {
			t.Fatalf("Update() = %+v, %v", updated, err)
		}
		replacement := updated.Spec
		replacement.Skills = []string{"skill-installer"}
		replacement.Runtime.Credentials = map[string]string{"secrets/token.txt": "contract-secret"}
		replaced, err := agents.Update(context.Background(), got.ID, replacement)
		if err != nil || len(replaced.Spec.Skills) != 1 || replaced.Spec.Skills[0] != "skill-installer" {
			t.Fatalf("complete Skill replacement = %+v, %v", replaced, err)
		}

		started, err := agents.Start(context.Background(), got.ID)
		if err != nil || started.Status.State != agentengine.AgentStateRunning || !started.Status.Ready {
			t.Fatalf("Start() = %+v, %v", started, err)
		}
		stopped, err := agents.Stop(context.Background(), got.ID)
		if err != nil || stopped.Status.State != agentengine.AgentStateStopped || stopped.Status.Ready {
			t.Fatalf("Stop() = %+v, %v", stopped, err)
		}
		recreated, err := agents.Recreate(context.Background(), got.ID)
		if err != nil || recreated.ID != got.ID || recreated.Status.State == "" || recreated.Status.RuntimeID == "" || recreated.Spec.Runtime.Credentials != nil {
			t.Fatalf("Recreate() = %+v, %v", recreated, err)
		}
		if err := agents.Delete(context.Background(), got.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if _, err := agents.Get(context.Background(), got.ID); agentengine.ErrorCodeOf(err) != agentengine.ErrorAgentUnavailable {
			t.Fatalf("Get(deleted) error = %v", err)
		}
	})

	t.Run("run events result and recording", func(t *testing.T) {
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			events := []agentengine.TurnEvent{
				{Kind: agentengine.TurnEventThoughtDelta, Thought: "thinking"},
				{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool-1", Kind: "exec_command", InputSummary: "pwd"}},
				{Kind: agentengine.TurnEventActivityUpdate, Activity: &agentengine.ActivityUpdate{ID: "plan-1", Kind: "plan_update", Status: "running"}},
				{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{Kind: agentengine.OutputItemResourceLink, Payload: activity.ResourceLink{Name: "report", URI: "file:///report"}}},
				{Kind: agentengine.TurnEventTextDelta, Text: "answer"},
			}
			for _, event := range events {
				if err := sink.Emit(ctx, event); err != nil {
					return contractFailure(agentengine.ErrorRuntimeFailed, err)
				}
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer", Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		var events []agentengine.TurnEvent
		result := client.Conversations("agent-a").Run(context.Background(), contractTurn("turn-1", "conversation-1"), agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			events = append(events, event)
			return nil
		}))
		if result.Status != agentengine.TurnSucceeded || result.Output != "answer" || !result.Dispatched || result.Error != nil {
			t.Fatalf("Run() = %+v", result)
		}
		if len(events) != 5 {
			t.Fatalf("events = %+v", events)
		}
		for index, event := range events {
			if event.Sequence != uint64(index+1) {
				t.Fatalf("event %d sequence = %d", index, event.Sequence)
			}
		}
		if events[0].Thought != "thinking" || events[1].Tool == nil || events[2].Activity == nil || events[3].Output == nil || events[4].Text != "answer" {
			t.Fatalf("normalized events = %+v", events)
		}
	})

	t.Run("agent role changes are rejected", func(t *testing.T) {
		worker := contractAgent("agent-worker", agentengine.AgentStateStopped, "codex")
		manager := contractAgent("agent-manager", agentengine.AgentStateStopped, "codex")
		manager.Spec.Role = agentengine.AgentRoleManager
		client := factory(t, []agentengine.Agent{worker, manager}, nil)
		workerUpdate, err := client.Agents().Get(context.Background(), worker.ID)
		if err != nil {
			t.Fatal(err)
		}
		workerUpdate.Spec.Role = agentengine.AgentRoleManager
		if _, err := client.Agents().Update(context.Background(), worker.ID, workerUpdate.Spec); agentengine.ErrorCodeOf(err) != agentengine.ErrorInvalidRequest {
			t.Fatalf("worker-to-manager Update error = %v", err)
		}
		managerUpdate, err := client.Agents().Get(context.Background(), manager.ID)
		if err != nil {
			t.Fatal(err)
		}
		managerUpdate.Spec.Role = agentengine.AgentRoleWorker
		if _, err := client.Agents().Update(context.Background(), manager.ID, managerUpdate.Spec); agentengine.ErrorCodeOf(err) != agentengine.ErrorInvalidRequest {
			t.Fatalf("manager-to-worker Update error = %v", err)
		}
	})

	t.Run("conversation admission and concurrency", func(t *testing.T) {
		started := make(chan agentengine.ConversationKey, 2)
		release := make(chan struct{})
		behavior := func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
			started <- request.ConversationKey
			<-release
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		firstDone := make(chan agentengine.TurnResult, 1)
		go func() {
			firstDone <- conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-started
		overlap := conversations.Run(context.Background(), contractTurn("turn-2", "conversation-1"), nil)
		if overlap.Error == nil || overlap.Error.Code != agentengine.ErrorConversationBusy || overlap.Dispatched {
			t.Fatalf("overlap = %+v", overlap)
		}
		secondDone := make(chan agentengine.TurnResult, 1)
		go func() {
			secondDone <- conversations.Run(context.Background(), contractTurn("turn-3", "conversation-2"), nil)
		}()
		<-started
		close(release)
		if (<-firstDone).Status != agentengine.TurnSucceeded || (<-secondDone).Status != agentengine.TurnSucceeded {
			t.Fatal("different conversations did not complete")
		}
	})

	t.Run("exact cancellation waits for cleanup", func(t *testing.T) {
		started := make(chan struct{})
		cleanupStarted := make(chan struct{})
		releaseCleanup := make(chan struct{})
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
			close(started)
			<-ctx.Done()
			close(cleanupStarted)
			<-releaseCleanup
			return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		runDone := make(chan agentengine.TurnResult, 1)
		go func() {
			runDone <- conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-started
		if err := conversations.Cancel(context.Background(), "conversation-1", "other-turn"); err != nil {
			t.Fatalf("Cancel(other) error = %v", err)
		}
		cancelDone := make(chan error, 1)
		go func() { cancelDone <- conversations.Cancel(context.Background(), "conversation-1", "turn-1") }()
		<-cleanupStarted
		select {
		case err := <-cancelDone:
			t.Fatalf("Cancel returned before cleanup: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		close(releaseCleanup)
		if err := <-cancelDone; err != nil {
			t.Fatal(err)
		}
		result := <-runDone
		if result.Status != agentengine.TurnCanceled || result.Error == nil || result.Error.Code != agentengine.ErrorCanceled || !result.Dispatched {
			t.Fatalf("canceled result = %+v", result)
		}
		if err := conversations.Cancel(context.Background(), "conversation-1", "turn-1"); err != nil {
			t.Fatalf("idempotent Cancel error = %v", err)
		}
	})

	t.Run("agent mutations drain active turns", func(t *testing.T) {
		operations := map[string]func(context.Context, agentengine.AgentInterface, agentengine.Agent) error{
			"stop": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				_, err := agents.Stop(ctx, item.ID)
				return err
			},
			"update": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				item.Spec.Description = "updated after drain"
				_, err := agents.Update(ctx, item.ID, item.Spec)
				return err
			},
			"recreate": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				_, err := agents.Recreate(ctx, item.ID)
				return err
			},
			"delete": func(ctx context.Context, agents agentengine.AgentInterface, item agentengine.Agent) error {
				return agents.Delete(ctx, item.ID)
			},
		}
		for name, operation := range operations {
			operation := operation
			t.Run(name, func(t *testing.T) {
				started := make(chan struct{})
				release := make(chan struct{})
				behavior := func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
					close(started)
					<-release
					return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
				}
				seed := contractAgent("agent-a", agentengine.AgentStateRunning, "codex")
				client := factory(t, []agentengine.Agent{seed}, behavior)
				current, err := client.Agents().Get(context.Background(), seed.ID)
				if err != nil {
					t.Fatal(err)
				}
				runDone := make(chan agentengine.TurnResult, 1)
				go func() {
					runDone <- client.Conversations(seed.ID).Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
				}()
				<-started
				operationDone := make(chan error, 1)
				go func() { operationDone <- operation(context.Background(), client.Agents(), current) }()
				select {
				case err := <-operationDone:
					t.Fatalf("operation returned before active Turn drained: %v", err)
				case <-time.After(20 * time.Millisecond):
				}
				close(release)
				if result := <-runDone; result.Status != agentengine.TurnSucceeded {
					t.Fatalf("Run() = %+v", result)
				}
				if err := <-operationDone; err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	t.Run("lifecycle drain timeout preserves the running agent", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		behavior := func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
			close(started)
			<-release
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		seed := contractAgent("agent-a", agentengine.AgentStateRunning, "codex")
		client := factory(t, []agentengine.Agent{seed}, behavior)
		runDone := make(chan agentengine.TurnResult, 1)
		go func() {
			runDone <- client.Conversations(seed.ID).Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		}()
		<-started
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if _, err := client.Agents().Stop(ctx, seed.ID); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop() error = %v", err)
		}
		got, err := client.Agents().Get(context.Background(), seed.ID)
		if err != nil || got.Status.State != agentengine.AgentStateRunning || got.Status.RuntimeID != seed.Status.RuntimeID {
			t.Fatalf("Agent changed after timeout: %+v, %v", got, err)
		}
		close(release)
		if result := <-runDone; result.Status != agentengine.TurnSucceeded {
			t.Fatalf("Run() = %+v", result)
		}
	})

	t.Run("reset removes strict continuation mapping", func(t *testing.T) {
		behavior := func(context.Context, string, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		if result := conversations.Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil); result.Status != agentengine.TurnSucceeded {
			t.Fatalf("first Run() = %+v", result)
		}
		if err := conversations.Reset(context.Background(), "conversation-1"); err != nil {
			t.Fatal(err)
		}
		strict := contractTurn("turn-2", "conversation-1")
		strict.Continuation = agentengine.ContinuationRequireExisting
		result := conversations.Run(context.Background(), strict, nil)
		if result.Error == nil || result.Error.Code != agentengine.ErrorConversationNotResumable || result.Dispatched {
			t.Fatalf("strict Run() = %+v", result)
		}
	})

	t.Run("interaction resolution", func(t *testing.T) {
		resolved := make(chan struct{})
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventInteractionRequest, Interaction: &agentengine.InteractionRequest{ID: "question-1", Kind: agentengine.InteractionUserInput, Title: "Continue?"}}); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			<-resolved
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		conversations := client.Conversations("agent-a")
		interactionID := make(chan string, 1)
		runDone := make(chan agentengine.TurnResult, 1)
		go func() {
			request := contractTurn("turn-1", "conversation-1")
			request.Interaction = agentengine.InteractionResolve
			runDone <- conversations.Run(context.Background(), request, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
				if event.Interaction != nil {
					interactionID <- event.Interaction.ID
				}
				return nil
			}))
		}()
		id := <-interactionID
		if err := conversations.Resolve(context.Background(), agentengine.InteractionResolution{
			ConversationKey: "conversation-1", InteractionID: id, ResponderID: "tester",
			Answers: map[string]agentengine.InteractionAnswer{"choice": {Values: []string{"Yes"}}},
		}); err != nil {
			t.Fatal(err)
		}
		close(resolved)
		if result := <-runDone; result.Status != agentengine.TurnSucceeded || !result.Dispatched {
			t.Fatalf("Run() = %+v", result)
		}
	})

	t.Run("interaction policies", func(t *testing.T) {
		interaction := agentengine.TurnEvent{Kind: agentengine.TurnEventInteractionRequest, Interaction: &agentengine.InteractionRequest{ID: "question-1", Kind: agentengine.InteractionUserInput}}
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			if err := sink.Emit(ctx, interaction); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		reject := contractTurn("turn-reject", "conversation-reject")
		reject.Interaction = agentengine.InteractionReject
		result := client.Conversations("agent-a").Run(context.Background(), reject, nil)
		if result.Status != agentengine.TurnFailed || result.Error == nil || result.Error.Code != agentengine.ErrorInteractionUnsupported || !result.Dispatched {
			t.Fatalf("reject Run() = %+v", result)
		}

		var emitted []agentengine.TurnEvent
		skip := contractTurn("turn-skip", "conversation-skip")
		skip.Interaction = agentengine.InteractionSkipUserInput
		result = client.Conversations("agent-a").Run(context.Background(), skip, agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			emitted = append(emitted, event)
			return nil
		}))
		if result.Status != agentengine.TurnSucceeded || result.Error != nil || !result.Dispatched {
			t.Fatalf("skip Run() = %+v", result)
		}
		for _, event := range emitted {
			if event.Interaction != nil {
				t.Fatalf("skip_user_input exposed interaction: %+v", emitted)
			}
		}
	})

	t.Run("sink failure terminates the turn", func(t *testing.T) {
		behavior := func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			if err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "partial"}); err != nil {
				return contractFailure(agentengine.ErrorRuntimeFailed, err)
			}
			<-ctx.Done()
			return agentengine.TurnResult{Status: agentengine.TurnCanceled, Dispatched: true, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
		}
		client := factory(t, []agentengine.Agent{contractAgent("agent-a", agentengine.AgentStateRunning, "codex")}, behavior)
		result := client.Conversations("agent-a").Run(context.Background(), contractTurn("turn-1", "conversation-1"), agentengine.EventSinkFunc(func(context.Context, agentengine.TurnEvent) error {
			return errors.New("sink closed")
		}))
		if result.Status != agentengine.TurnFailed || result.Error == nil || result.Error.Code != agentengine.ErrorRuntimeFailed || !result.Dispatched {
			t.Fatalf("Run() = %+v", result)
		}
	})

	t.Run("stable validation and availability errors", func(t *testing.T) {
		client := factory(t, []agentengine.Agent{
			contractAgent("agent-running", agentengine.AgentStateRunning, "codex"),
			contractAgent("agent-stopped", agentengine.AgentStateStopped, "codex"),
			contractAgent("agent-unsupported", agentengine.AgentStateRunning, "openclaw_sandbox"),
		}, nil)
		invalid := client.Conversations("agent-running").Run(context.Background(), agentengine.TurnRequest{}, nil)
		if invalid.Error == nil || invalid.Error.Code != agentengine.ErrorInvalidRequest || invalid.Dispatched {
			t.Fatalf("invalid Run() = %+v", invalid)
		}
		invalidFile := contractTurn("turn-invalid-file", "conversation-invalid-file")
		invalidFile.Input = []agentengine.InputPart{{Kind: agentengine.InputPartFile, File: &agentengine.InputFile{ID: "file"}}}
		if result := client.Conversations("agent-running").Run(context.Background(), invalidFile, nil); result.Error == nil || result.Error.Code != agentengine.ErrorInvalidRequest || result.Dispatched {
			t.Fatalf("invalid file Run() = %+v", result)
		}
		invalidContinuation := contractTurn("turn-invalid-continuation", "conversation-invalid-continuation")
		invalidContinuation.Continuation = agentengine.ContinuationPolicy("future")
		if result := client.Conversations("agent-running").Run(context.Background(), invalidContinuation, nil); result.Error == nil || result.Error.Code != agentengine.ErrorInvalidRequest || result.Dispatched {
			t.Fatalf("invalid continuation Run() = %+v", result)
		}
		invalidInteraction := contractTurn("turn-invalid-interaction", "conversation-invalid-interaction")
		invalidInteraction.Interaction = agentengine.InteractionPolicy("future")
		if result := client.Conversations("agent-running").Run(context.Background(), invalidInteraction, nil); result.Error == nil || result.Error.Code != agentengine.ErrorInvalidRequest || result.Dispatched {
			t.Fatalf("invalid interaction Run() = %+v", result)
		}
		canceledContext, cancel := context.WithCancel(context.Background())
		cancel()
		canceled := client.Conversations("agent-running").Run(canceledContext, contractTurn("turn-canceled", "conversation-canceled"), nil)
		if canceled.Status != agentengine.TurnCanceled || canceled.Error == nil || canceled.Error.Code != agentengine.ErrorCanceled || canceled.Dispatched {
			t.Fatalf("pre-canceled Run() = %+v", canceled)
		}
		unavailable := client.Conversations("agent-stopped").Run(context.Background(), contractTurn("turn-1", "conversation-1"), nil)
		if unavailable.Error == nil || unavailable.Error.Code != agentengine.ErrorAgentUnavailable || unavailable.Dispatched {
			t.Fatalf("unavailable Run() = %+v", unavailable)
		}
		unsupported := client.Conversations("agent-unsupported").Run(context.Background(), contractTurn("turn-2", "conversation-1"), nil)
		if unsupported.Error == nil || unsupported.Error.Code != agentengine.ErrorRuntimeAdapterUnavailable || unsupported.Dispatched {
			t.Fatalf("unsupported Run() = %+v", unsupported)
		}
		unsupportedAgent, err := client.Agents().Get(context.Background(), "agent-unsupported")
		if err != nil {
			t.Fatal(err)
		}
		unsupportedAgent.Spec.Runtime.Credentials = map[string]string{"secret.txt": "secret"}
		if _, err := client.Agents().Update(context.Background(), unsupportedAgent.ID, unsupportedAgent.Spec); agentengine.ErrorCodeOf(err) != agentengine.ErrorUnsupportedRuntimeProvision {
			t.Fatalf("unsupported provisioning error = %v", err)
		}
	})
}

func contractAgent(id string, state agentengine.AgentState, adapter string) agentengine.Agent {
	return agentengine.Agent{
		ID: id,
		Spec: agentengine.AgentSpec{
			Name: id, Role: agentengine.AgentRoleWorker,
			Runtime: agentengine.RuntimeSpec{Adapter: adapter, Credentials: map[string]string{"seed": "must-not-leak"}},
			Model:   agentengine.ModelSpec{ProviderID: "provider-a", ModelID: "model-a"},
		},
		Status: agentengine.AgentStatus{State: state, RuntimeID: "runtime-" + id, Ready: state == agentengine.AgentStateRunning},
	}
}

func contractTurn(id agentengine.TurnID, key agentengine.ConversationKey) agentengine.TurnRequest {
	return agentengine.TurnRequest{
		ID: id, ConversationKey: key,
		Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "hello"}},
	}
}

func contractFailure(code agentengine.ErrorCode, err error) agentengine.TurnResult {
	return agentengine.TurnResult{Status: agentengine.TurnFailed, Dispatched: true, Error: &agentengine.TurnError{Code: code, Message: err.Error()}}
}
