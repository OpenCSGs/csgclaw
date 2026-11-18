package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/csgclaw/binding"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/channel/csgclaw/execution"
	csgclawsource "csgclaw/internal/channel/csgclaw/source"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
	"csgclaw/internal/worklease"
)

// Real HTTP, task persistence, participant routing, Source, Binding, Engine and
// transcript delivery. Only model decisions are replaced by a fixture that
// claims and submits the supplied task through the same HTTP client as the CLI.
type roomTaskExecutionRuntime struct {
	uiFixtureRuntime
	run func(context.Context, agentengine.TurnRequest) agentengine.TurnResult
}

func (r *roomTaskExecutionRuntime) Conversation(string) agentengine.RuntimeConversation { return r }
func (r *roomTaskExecutionRuntime) Run(ctx context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
	return r.run(ctx, request)
}

func TestRoomTaskDispatchClaimAndDeliveryEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name                                           string
		failOldProjection, failWorkerStart, stopWorker bool
	}{
		{name: "worker submits"},
		{name: "claim with projection unavailable", failOldProjection: true},
		{name: "runtime fails before claim", failWorkerStart: true},
		{name: "user stops one of two parallel sessions of the same worker", stopWorker: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &roomTaskExecutionRuntime{uiFixtureRuntime: uiFixtureRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}}
			profile := agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "fixture-model", ProfileComplete: true}
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{
				{ID: agent.ManagerUserID, Name: "manager", Role: agent.RoleManager, AgentProfile: profile, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-manager", Status: "running", ProfileComplete: true},
				{ID: "agent-dev", Name: "dev", Role: agent.RoleWorker, AgentProfile: profile, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-dev", Status: "running", ProfileComplete: true},
			}, agent.WithRuntime(rt))
			engine := agentengine.New(controller)
			participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{
				{ID: agent.ManagerParticipantID, Channel: "csgclaw", Type: participant.TypeAgent, Name: "manager", AgentID: agent.ManagerUserID, ChannelUserRef: im.ManagerUserID, ChannelUserKind: participant.ChannelUserKindLocalUserID, Mentionable: true, LifecycleStatus: participant.LifecycleStatusActive},
				{ID: "pt-dev", Channel: "csgclaw", Type: participant.TypeAgent, Name: "dev", AgentID: "agent-dev", ChannelUserRef: "user-dev", ChannelUserKind: participant.ChannelUserKindLocalUserID, Mentionable: true, LifecycleStatus: participant.LifecycleStatusActive},
			}), participant.WithAgentEngine(engine))
			bus := im.NewBus()
			messages, err := im.NewServiceFromPathWithBus(filepath.Join(t.TempDir(), "im.json"), bus)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := messages.Diagnostics().Flush(ctx); err != nil {
					t.Errorf("flush room task diagnostics: %v", err)
				}
			})
			if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-dev", Name: "dev", Role: "worker"}); err != nil {
				t.Fatal(err)
			}
			room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Delivery regression", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
			if err != nil {
				t.Fatal(err)
			}
			bridge := im.NewParticipantBridge("")
			h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, messages, bus, bridge, nil, nil, "room-e2e-token", true)
			h.SetParticipantService(participants)
			workBus := worklease.NewBus()
			leases := worklease.NewRegistry(participants, messages, workBus, worklease.WithTurnControlDispatcher(worklease.NewTurnControlDispatcher(nil)))
			h.SetParticipantWorkService(leases, workBus)
			taskStore, err := taskcore.NewStore(filepath.Join(t.TempDir(), "tasks"))
			if err != nil {
				t.Fatal(err)
			}
			core := taskcore.NewService(taskcore.WithStore(taskStore))
			h.SetRoomTaskCore(core)
			var failProjection atomic.Bool
			h.roomTaskSvc = roomtask.NewService(core, h.roomSchedulingContext, func(p roomtask.Projection) error {
				if failProjection.Load() {
					return fmt.Errorf("fixture IM persistence unavailable")
				}
				return h.deliverRoomTaskProjection(p)
			})
			server := httptest.NewServer(h.Routes())
			defer server.Close()
			client := &http.Client{Timeout: 5 * time.Second}
			managerClient := apiclient.New(server.URL, "room-e2e-token", client).WithCallerAgentID("manager")
			workerClient := apiclient.New(server.URL, "room-e2e-token", client).WithCallerAgentID("pt-dev")
			started, release := make(chan string, 2), make(chan struct{})
			finished := make(chan error, 2)
			managerNotified := make(chan struct{}, 1)
			const workerFailure = "fixture model quota exhausted"
			reportedError := workerFailure
			if tc.stopWorker {
				reportedError = "stopped by user"
			}
			var runs atomic.Int32
			rt.run = func(ctx context.Context, request agentengine.TurnRequest) agentengine.TurnResult {
				_, taskID, isWorker := strings.Cut(string(request.ConversationKey), ":task:")
				if !isWorker {
					for _, input := range request.Input {
						if strings.Contains(input.Text, reportedError) {
							select {
							case managerNotified <- struct{}{}:
							default:
							}
							break
						}
					}
					return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
				}
				runs.Add(1)
				select {
				case started <- taskID:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return agentengine.TurnResult{Status: agentengine.TurnCanceled}
				}
				if tc.failWorkerStart {
					finished <- nil
					return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: workerFailure}}
				}
				task, err := workerClient.GetRoomTask(ctx, room.ID, taskID)
				if err == nil {
					failProjection.Store(tc.failOldProjection)
					var claimed apitypes.RoomTask
					claimed, err = workerClient.ClaimRoomTask(ctx, room.ID, taskID, task.Attempt)
					failProjection.Store(false)
					if err == nil && (claimed.Status != taskcore.StatusInProgress || claimed.ClaimedBy != "pt-dev") {
						err = fmt.Errorf("claim not committed: %+v", claimed)
					}
				}
				if err == nil {
					_, err = workerClient.UpdateRoomTask(ctx, room.ID, taskID, apitypes.UpdateRoomTaskRequest{Attempt: task.Attempt, Status: taskcore.StatusCompleted, Result: "fixture deliverable verified"})
				}
				finished <- err
				if err != nil {
					return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
				}
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Worker delivered the verified fixture."}
			}
			transcripts, err := delivery.NewIMTranscriptStore(messages, participants, engine)
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := execution.New(engine, delivery.NewTranscriptRenderer(transcripts), execution.WithRoomContextProvider(bridge.RoomContext),
				execution.WithWorkerTurnEndHandler(bridge.EndWorkerTurn), execution.WithDiagnostics(messages.Diagnostics()),
				execution.WithParticipantWorkReporter(leases), execution.WithTurnControllerRegistrar(leases))
			if err != nil {
				t.Fatal(err)
			}
			workers, err := binding.NewManager(adapter)
			if err != nil {
				t.Fatal(err)
			}
			source, err := csgclawsource.New(bridge, bus, participants, engine.Agents(), workers)
			if err != nil {
				t.Fatal(err)
			}
			if err := source.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			// Leave the server's broadcast consumer backpressured for the entire flow.
			updates, unsubscribe := bus.Subscribe()
			defer unsubscribe()
			for i := 0; i < 16; i++ {
				if _, err := messages.DeliverMessage(im.DeliverMessageRequest{RoomID: room.ID, SenderID: room.ManagerID, MessageID: fmt.Sprintf("progress-%d", i), Content: "progress"}); err != nil {
					t.Fatal(err)
				}
			}
			dispatchGoal := func(title string) (apitypes.RoomTask, apitypes.RoomTask) {
				t.Helper()
				sourceMessage, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: title})
				if err != nil {
					t.Fatal(err)
				}
				parent, err := managerClient.CreateRoomTask(context.Background(), room.ID, apitypes.CreateRoomTaskRequest{SourceMessageID: sourceMessage.ID, Title: title})
				if err != nil {
					t.Fatal(err)
				}
				plan, err := managerClient.PlanRoomTask(context.Background(), room.ID, parent.ID, apitypes.PlanRoomTaskRequest{AutoStart: true, Tasks: []apitypes.RoomTaskPlanItem{{IDRef: "dev", Title: title, AssignedTo: "pt-dev"}}})
				if err != nil {
					t.Fatal(err)
				}
				child, err := managerClient.DispatchRoomTask(context.Background(), room.ID, plan.CreatedTasks[0].ID, "")
				if err != nil || child.Attempt != 1 {
					t.Fatalf("dispatch: %+v %v", child, err)
				}
				select {
				case taskID := <-started:
					if taskID != child.ID {
						t.Fatalf("unexpected session for %s: %s", child.ID, taskID)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("worker did not enter the task session for %s", child.ID)
				}
				return parent, child
			}
			parent, child := dispatchGoal("Build fixture")
			if _, err := managerClient.RetryRoomDelivery(context.Background(), room.ID); err != nil {
				t.Fatal(err)
			}
			// Broadcast replay and explicit retry must not create a second execution.
			for {
				select {
				case event := <-updates:
					h.PublishParticipantEvent(event)
				default:
					goto drained
				}
			}
		drained:
			if tc.stopWorker {
				// The first runtime is held at release. The same Worker must enter its
				// second session before either goal has completed.
				_, independent := dispatchGoal("Document fixture")
				stoppedWorker := false
				for _, work := range leases.ActiveWork(room.ID) {
					if work.TaskID != child.ID || work.ParticipantID != "pt-dev" {
						continue
					}
					stopped := performParticipantWorkRequest(t, h, http.MethodPost, "/api/v1/channels/csgclaw/participants/pt-dev/work:stop", map[string]any{
						"room_id": room.ID, "lease_id": work.LeaseID, "request_id": work.RequestID,
					})
					if stopped.Code != http.StatusAccepted {
						t.Fatalf("stop Worker: %d %s", stopped.Code, stopped.Body.String())
					}
					stoppedWorker = true
					break
				}
				if !stoppedWorker {
					t.Fatal("Worker did not register an active lease")
				}
				select {
				case <-managerNotified:
				case <-time.After(5 * time.Second):
					t.Fatal("Manager did not receive the stop as task feedback")
				}
				task, err := managerClient.GetRoomTask(context.Background(), room.ID, child.ID)
				if err != nil || task.Status != taskcore.StatusCancelled || task.Attempt != 1 {
					t.Fatalf("Worker's stopped attempt was not recorded: %+v %v", task, err)
				}
				goal, err := managerClient.GetRoomTask(context.Background(), room.ID, parent.ID)
				if err != nil || goal.Status != taskcore.StatusInProgress || runs.Load() != 2 {
					t.Fatalf("stopping one worker either halted the plan or replayed the attempt: %+v %v", goal, err)
				}
				close(release)
				select {
				case err := <-finished:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("independent session did not finish after stopping its peer")
				}
				other, err := managerClient.GetRoomTask(context.Background(), room.ID, independent.ID)
				if err != nil || other.Status != taskcore.StatusReview || other.Attempt != 1 || other.Result != "fixture deliverable verified" {
					t.Fatalf("wrong outcome for independent session: %+v %v", other, err)
				}
				return
			}
			close(release)
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Worker claim/submit did not finish")
			}
			if tc.failWorkerStart {
				select {
				case <-managerNotified:
				case <-time.After(5 * time.Second):
					t.Fatal("Manager did not receive runtime failure feedback")
				}
				task, err := managerClient.GetRoomTask(context.Background(), room.ID, child.ID)
				if err != nil || task.Status != taskcore.StatusFailed || task.Attempt != 1 || !strings.Contains(task.Error, workerFailure) {
					t.Fatalf("worker remained busy after runtime failure: %+v %v", task, err)
				}
				return
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				stored, _ := messages.ListMessages(room.ID)
				delivered := false
				for _, message := range stored {
					delivered = delivered || message.Content == "Worker delivered the verified fixture."
				}
				if delivered {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("Worker transcript did not reach room")
				}
				time.Sleep(10 * time.Millisecond)
			}
			task, err := managerClient.GetRoomTask(context.Background(), room.ID, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			if task.Status != taskcore.StatusReview || task.ClaimedBy != "pt-dev" || task.Result != "fixture deliverable verified" {
				t.Fatalf("incorrect submitted task %+v", task)
			}
			if _, err := managerClient.ReviewRoomTask(context.Background(), room.ID, child.ID, apitypes.ReviewRoomTaskRequest{Attempt: task.Attempt, Accept: true, Summary: "Verified"}); err != nil {
				t.Fatal(err)
			}
			if _, err := managerClient.ReportRoomTask(context.Background(), room.ID, parent.ID, "succeeded", "Fixture completed"); err != nil {
				t.Fatal(err)
			}
			reloaded := taskcore.NewService(taskcore.WithStore(taskStore))
			saved, ok := reloaded.Get(child.ID)
			if !ok || saved.Status != taskcore.StatusCompleted || saved.Attempt != 1 || saved.ClaimedBy != "pt-dev" || runs.Load() != 1 {
				t.Fatalf("lost or duplicate execution: task=%+v runs=%d", saved, runs.Load())
			}
		})
	}
}
