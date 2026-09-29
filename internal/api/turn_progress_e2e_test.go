package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/auth"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/im"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/worklease"
	webui "csgclaw/web"
)

type progressBrowserRuntime struct {
	fakeCompatRuntime
	mu    sync.Mutex
	gates map[string]chan struct{}
}

func (r *progressBrowserRuntime) Conversation(string) agentengine.RuntimeConversation { return r }
func (r *progressBrowserRuntime) Reset(context.Context, agentengine.ConversationKey) *agentengine.TurnError {
	return nil
}
func (r *progressBrowserRuntime) Resolve(context.Context, agentengine.InteractionRequest, agentengine.InteractionResolution) *agentengine.TurnError {
	return nil
}
func (r *progressBrowserRuntime) Run(ctx context.Context, req agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	id := string(req.ID)
	phase := "commentary"
	if strings.HasPrefix(id, "unphased") {
		phase = ""
	}
	r.mu.Lock()
	gate := make(chan struct{})
	r.gates[id] = gate
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.gates, id); r.mu.Unlock() }()
	emit := func(event agentengine.TurnEvent) bool { return sink.Emit(ctx, event) == nil }
	emit(agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "intro", Phase: phase, Text: "我会先检查项目文件，再验证命令结果。"})
	emit(agentengine.TurnEvent{Kind: agentengine.TurnEventThoughtDelta, Thought: "这是运行时提供的中间文字。"})
	count := 1
	readability := strings.HasPrefix(id, "readability")
	if readability {
		count = 3
	}
	if strings.HasPrefix(id, "long") {
		count = 1000
	}
	for i := 0; i < count; i++ {
		toolID := fmt.Sprintf("tool-%d", i)
		command, action, status, output := "cat README.md", "read", "completed", strings.Repeat("verification output\n", 15)
		exitCode := float64(0)
		cwd := ""
		if readability {
			cwd = "/workspace/csgclaw/agents/manager/project"
			command = []string{`/opt/homebrew/bin/zsh -lc "find /workspace/csgclaw/agents/manager/skills /workspace/shared/skills -maxdepth 2 -name SKILL.md -print"`, `rg --files /workspace/shared/skills | rg '/(build|review)/SKILL.md$'`, "cat README.md"}[i]
			output = "/workspace/csgclaw/agents/manager/skills/build/SKILL.md"
			if i == 1 {
				status = "failed"
				exitCode = 1
				output = ""
				action = "search"
			}
		}

		if !emit(agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: toolID, Kind: "exec_command", Title: "Shell", Status: "started", Payload: map[string]any{"command": command, "cwd": cwd, "commandActions": []any{map[string]any{"type": action}}}}}) {
			break
		}
		emit(agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: toolID, Kind: "exec_command", Status: status, Payload: map[string]any{"output": output, "exitCode": exitCode, "durationMs": float64(123)}}})
	}
	if strings.HasPrefix(id, "hover") {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		waiting := true
		for waiting {
			select {
			case <-ctx.Done():
				return agentengine.TurnResult{Status: agentengine.TurnCanceled}
			case <-gate:
				waiting = false
			case <-ticker.C:
				emit(agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "live-progress", Phase: "commentary", Text: "继续检查文件与执行结果。\n\n"})
			}
		}
	}
	phase = "final_answer"
	if strings.HasPrefix(id, "unphased") {
		phase = ""
	}
	emit(agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: phase, Text: "**验证结果**：文件已读取"})
	time.Sleep(120 * time.Millisecond)
	emit(agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: phase, Text: "，命令执行成功。"})
	select {
	case <-ctx.Done():
		return agentengine.TurnResult{Status: agentengine.TurnCanceled}
	case <-gate:
	}
	if strings.HasPrefix(id, "failed") {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Message: "The test runtime failed."}}
	}
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "**验证结果**：文件已读取，命令执行成功。"}
}

// Explicit opt-in fixture exercises the real Engine, renderer, HTTP, SSE and
// built UI with deterministic provider events and without accessing credentials.
func TestTurnProgressBrowserFixture(t *testing.T) {
	ready := os.Getenv("CSGCLAW_PROGRESS_READY_FILE")
	if ready == "" {
		t.Skip("headless browser fixture")
	}
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	rt := &progressBrowserRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}, gates: map[string]chan struct{}{}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-e2e", Name: "过程验证", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-e2e", Status: "running", ProfileComplete: true}}, agent.WithRuntime(rt))
	engine := agentengine.New(controller)
	participants := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{ID: "pt-e2e", Channel: "csgclaw", Type: participant.TypeAgent, Name: "过程验证", AgentID: "agent-e2e", ChannelUserRef: "user-e2e", ChannelUserKind: participant.ChannelUserKindLocalUserID, LifecycleStatus: participant.LifecycleStatusActive, Mentionable: true}}), participant.WithAgentEngine(engine))
	bus := im.NewBus()
	statePath := filepath.Join(t.TempDir(), "im", "state.json")
	if err := im.SaveBootstrap(statePath, im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin", Name: "验收用户", Role: "admin"}, {ID: "user-e2e", Name: "过程验证", Role: "worker"}}, Rooms: []im.Room{{ID: "room-progress", Title: "智能体过程验收", Locale: "zh", Members: []string{"user-admin", "user-e2e"}}}}); err != nil {
		t.Fatal(err)
	}
	service, err := im.NewServiceFromPathWithBus(statePath, bus)
	if err != nil {
		t.Fatal(err)
	}
	store, err := delivery.NewIMTranscriptStore(service, participants, engine)
	if err != nil {
		t.Fatal(err)
	}
	renderer := delivery.NewTranscriptRenderer(store)
	h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, service, bus, nil, nil, nil, "", true)
	h.SetParticipantService(participants)
	workBus := worklease.NewBus()
	controls := worklease.NewTurnControlDispatcher(nil)
	workRegistry := worklease.NewRegistry(participants, service, workBus, worklease.WithTurnControlDispatcher(controls))
	h.SetParticipantWorkService(workRegistry, workBus, nil)
	router := h.Routes()
	finished := make(chan struct{})
	var finish sync.Once
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var runs sync.WaitGroup
	defer runs.Wait()
	router.Post("/__e2e/run", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			w.WriteHeader(400)
			return
		}
		thread := r.URL.Query().Get("thread")
		runs.Add(1)
		go func() {
			defer runs.Done()
			turn := channel.TurnContext{AgentID: "agent-e2e", BindingID: "e2e", ParticipantID: "pt-e2e", RoomID: "room-progress", ThreadRootID: thread, ConversationKey: agentengine.ConversationKey(id), TurnID: agentengine.TurnID(id), SourceMessageID: id, Locale: "zh"}
			turnCtx, cancelTurn := context.WithCancel(ctx)
			defer cancelTurn()
			leaseID := worklease.NewID()
			unregister := workRegistry.RegisterTurnController("pt-e2e", apiTurnControllerFunc(func(stopCtx context.Context, ref agentruntime.TurnRef) error {
				if ref.LeaseID != leaseID {
					return agentruntime.ErrTurnNotFound
				}
				return engine.Conversations(turn.AgentID).Cancel(stopCtx, turn.ConversationKey, turn.TurnID)
			}))
			defer unregister()
			lease := worklease.ParticipantWorkLease{ParticipantID: "pt-e2e", LeaseID: leaseID, RoomID: turn.RoomID, ThreadRootID: thread, RequestID: id, Kind: apitypes.ParticipantWorkKindAgentTurn, TTLSeconds: 60, TTLExplicit: true}
			if _, err := workRegistry.StartOrRenew(ctx, lease); err != nil {
				t.Error(err)
				return
			}
			renewed := make(chan struct{})
			go func() {
				defer close(renewed)
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-turnCtx.Done():
						return
					case <-ticker.C:
						_, _ = workRegistry.StartOrRenew(turnCtx, lease)
					}
				}
			}()
			defer func() {
				cancelTurn()
				<-renewed
				_ = workRegistry.Finish(context.Background(), "pt-e2e", leaseID, apitypes.ParticipantWorkOutcomeReleased)
			}()
			used := int64(32768)
			if _, _, err := workRegistry.UpdateStatus(ctx, "pt-e2e", leaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: 1, Phase: "working", Stage: "generating_reply", Capabilities: []string{"turn_stop_v1", "work_stage_v1"}, ContextUsage: &modelcap.ContextUsage{SessionID: id, ModelID: "fixture-model", UsedTokens: &used, ContextWindow: 131072, ContextSource: "default", AutoCompact: true, CompactThreshold: 98304, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}); err != nil {
				t.Error(err)
				return
			}
			if err := renderer.Start(ctx, turn); err != nil {
				t.Error(err)
				return
			}
			result := engine.Conversations(turn.AgentID).Run(turnCtx, agentengine.TurnRequest{ID: turn.TurnID, ConversationKey: turn.ConversationKey, Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: id}}}, agentengine.EventSinkFunc(func(ctx context.Context, event agentengine.TurnEvent) error { return renderer.Emit(ctx, turn, event) }))
			if err := renderer.Complete(ctx, turn, result); err != nil {
				t.Error(err)
			}
		}()
		w.WriteHeader(202)
	})
	router.Post("/__e2e/context", func(w http.ResponseWriter, r *http.Request) {
		for _, lease := range workRegistry.ActiveWork("room-progress") {
			if lease.RequestID != r.URL.Query().Get("id") || lease.Status == nil {
				continue
			}
			usage := modelcap.CloneUsage(lease.Status.ContextUsage)
			if usage == nil {
				w.WriteHeader(404)
				return
			}
			used := int64(65536)
			usage.UsedTokens = &used
			usage.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			_, _, err := workRegistry.UpdateStatus(ctx, lease.ParticipantID, lease.LeaseID, apitypes.ParticipantWorkStatusPatchRequest{Sequence: lease.Status.Sequence + 1, Phase: lease.Status.Phase, Stage: lease.Status.Stage, Capabilities: lease.Capabilities, ContextUsage: usage})
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(404)
	})
	router.Post("/__e2e/release", func(w http.ResponseWriter, r *http.Request) {
		rt.mu.Lock()
		gate := rt.gates[r.URL.Query().Get("id")]
		if gate != nil {
			select {
			case <-gate:
			default:
				close(gate)
			}
		}
		rt.mu.Unlock()
		w.WriteHeader(204)
	})
	router.Post("/__e2e/cancel", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		_ = engine.Conversations("agent-e2e").Cancel(ctx, agentengine.ConversationKey(id), agentengine.TurnID(id))
		w.WriteHeader(204)
	})
	router.Post("/__e2e/finish", func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		finish.Do(func() { close(finished) })
		w.WriteHeader(204)
	})
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	defer server.Close()
	data, _ := json.Marshal(map[string]string{"url": server.URL})
	if err := os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(10 * time.Minute):
		cancel()
		t.Error("browser verification timed out")
	}
}
