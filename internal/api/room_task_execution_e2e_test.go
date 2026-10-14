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
	for _, failOldProjection := range []bool{false, true} {
		t.Run(fmt.Sprintf("projection_failure=%t", failOldProjection), func(t *testing.T) {
			rt := &roomTaskExecutionRuntime{uiFixtureRuntime: uiFixtureRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}}
			profile := agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "fixture-model", ProfileComplete: true}
			controller := mustNewSeededServiceWithOptions(t, []agent.Agent{
				{ID: agent.ManagerUserID, Name: "manager", Role: agent.RoleManager, AgentProfile: profile, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-manager", Status: "running", ProfileComplete: true},
				{ID: "agent-dev", Name: "dev", Role: agent.RoleWorker, AgentProfile: profile, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-dev", Status: "running", ProfileComplete: true},
			}, agent.WithRuntime(rt))
			engine := agentengine.New(controller)
			participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{
				{ID: agent.ManagerParticipantID, Channel: "csgclaw", Type: participant.TypeAgent, Name: "manager", AgentID: agent.ManagerUserID, ChannelUserRef: im.ManagerUserID, ChannelUserKind: participant.ChannelUserKindLocalUserID, Mentionable: true},
				{ID: "pt-dev", Channel: "csgclaw", Type: participant.TypeAgent, Name: "dev", AgentID: "agent-dev", ChannelUserRef: "user-dev", ChannelUserKind: participant.ChannelUserKindLocalUserID, Mentionable: true},
			}), participant.WithAgentEngine(engine))
			bus := im.NewBus()
			messages, err := im.NewServiceFromPathWithBus(filepath.Join(t.TempDir(), "im.json"), bus)
			if err != nil {
				t.Fatal(err)
			}
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
			started, release := make(chan struct{}, 1), make(chan struct{})
			finished := make(chan error, 2)
			var runs atomic.Int32
			rt.run = func(ctx context.Context, request agentengine.TurnRequest) agentengine.TurnResult {
				_, taskID, isWorker := strings.Cut(string(request.ConversationKey), ":task:")
				if !isWorker {
					return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
				}
				runs.Add(1)
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-ctx.Done():
					return agentengine.TurnResult{Status: agentengine.TurnCanceled}
				}
				task, err := workerClient.GetRoomTask(ctx, room.ID, taskID)
				if err == nil {
					failProjection.Store(failOldProjection)
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
			adapter, err := execution.New(engine, delivery.NewTranscriptRenderer(transcripts), execution.WithRoomContextProvider(bridge.RoomContext), execution.WithDiagnostics(messages.Diagnostics()))
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
			sourceMessage, err := messages.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Build the fixture"})
			if err != nil {
				t.Fatal(err)
			}
			parent, err := managerClient.CreateRoomTask(context.Background(), room.ID, apitypes.CreateRoomTaskRequest{SourceMessageID: sourceMessage.ID, Title: "Build fixture"})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := managerClient.PlanRoomTask(context.Background(), room.ID, parent.ID, apitypes.PlanRoomTaskRequest{AutoStart: true, Tasks: []apitypes.RoomTaskPlanItem{{IDRef: "dev", Title: "Build fixture", AssignedTo: "pt-dev"}}})
			if err != nil {
				t.Fatal(err)
			}
			child := plan.CreatedTasks[0]
			dispatched, err := managerClient.DispatchRoomTask(context.Background(), room.ID, child.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("Worker never received dispatch with full broadcast buffer")
			}
			if dispatched.Attempt != 1 {
				t.Fatal("wrong execution attempt", dispatched.Attempt)
			}
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
			close(release)
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Worker claim/submit did not finish")
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
