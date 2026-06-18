package codexmanager

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/channelbridge/codexbridge"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
)

func TestShouldStartCodexBridge(t *testing.T) {
	cases := []struct {
		name  string
		agent agent.Agent
		want  bool
	}{
		{
			name: "running codex worker with complete profile",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateRunning),
				ProfileComplete: true,
			},
			want: true,
		},
		{
			name: "stopped worker",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateStopped),
				ProfileComplete: true,
			},
		},
		{
			name: "manager is excluded",
			agent: agent.Agent{
				ID:              agent.ManagerUserID,
				Role:            agent.RoleManager,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateRunning),
				ProfileComplete: true,
			},
		},
		{
			name: "non-codex worker is excluded",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindPicoClawSandbox,
				Status:          string(agentruntime.StateRunning),
				ProfileComplete: true,
			},
		},
		{
			name: "incomplete profile is excluded",
			agent: agent.Agent{
				ID:          "u-alice",
				Role:        agent.RoleWorker,
				RuntimeKind: agent.RuntimeKindCodex,
				Status:      string(agentruntime.StateRunning),
			},
		},
	}

	for _, tc := range cases {
		if got := shouldStartCodexBridge(tc.agent); got != tc.want {
			t.Fatalf("%s: shouldStartCodexBridge() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestShouldRestoreCodexBridgeOnStartup(t *testing.T) {
	cases := []struct {
		name  string
		agent agent.Agent
		want  bool
	}{
		{
			name: "running codex worker is restored",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateRunning),
				ProfileComplete: true,
			},
			want: true,
		},
		{
			name: "exited codex worker is restored",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateExited),
				ProfileComplete: true,
			},
			want: true,
		},
		{
			name: "stopped codex worker stays stopped",
			agent: agent.Agent{
				ID:              "u-alice",
				Role:            agent.RoleWorker,
				RuntimeKind:     agent.RuntimeKindCodex,
				Status:          string(agentruntime.StateStopped),
				ProfileComplete: true,
			},
		},
	}

	for _, tc := range cases {
		if got := shouldRestoreCodexBridgeOnStartup(tc.agent); got != tc.want {
			t.Fatalf("%s: shouldRestoreCodexBridgeOnStartup() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBindingForAgentUsesParticipantIDForWorker(t *testing.T) {
	binding := bindingForAgent(agent.Agent{
		ID:          "u-agent-3l6htd",
		Name:        "dev",
		RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID:   "rt-u-agent-3l6htd",
	}, "sess-dev")

	if binding.BotID != "agent-3l6htd" {
		t.Fatalf("BotID = %q, want participant ID agent-3l6htd", binding.BotID)
	}
	if binding.RuntimeID != "rt-u-agent-3l6htd" || binding.SessionID != "sess-dev" {
		t.Fatalf("binding = %+v, want runtime/session preserved", binding)
	}
}

func TestFeishuManagerStopAgentStopsRememberedParticipant(t *testing.T) {
	client := newRecordingBotClient()
	bridge := codexbridge.NewService(client, noopPrompter{}, runtimecodex.NewEventSink())
	manager := &feishuManager{
		bridge:             bridge,
		provider:           testCredentialProvider{},
		activeParticipants: map[string]string{"agent-1": "pt-old"},
	}

	if err := bridge.StartBot(context.Background(), codexbridge.Binding{BotID: "pt-old", RuntimeID: "rt-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	client.waitStarted(t, "pt-old")

	manager.StopAgent("agent-1")

	client.waitStopped(t, "pt-old")
	if got := manager.clearActiveParticipant("agent-1"); got != "" {
		t.Fatalf("active participant = %q, want cleared", got)
	}
}

func TestFeishuManagerStopAgentBridgeStopsPreviousAndCurrentParticipants(t *testing.T) {
	client := newRecordingBotClient()
	bridge := codexbridge.NewService(client, noopPrompter{}, runtimecodex.NewEventSink())
	manager := &feishuManager{
		bridge:             bridge,
		provider:           testCredentialProvider{"agent-1": "pt-new"},
		activeParticipants: map[string]string{"agent-1": "pt-old"},
	}

	for _, botID := range []string{"pt-old", "pt-new"} {
		if err := bridge.StartBot(context.Background(), codexbridge.Binding{BotID: botID, RuntimeID: "rt-1"}); err != nil {
			t.Fatalf("StartBot(%s) error = %v", botID, err)
		}
		client.waitStarted(t, botID)
	}

	manager.stopAgentBridgeForAgent("agent-1", "pt-new", agent.Agent{ID: "agent-1"})

	client.waitStopped(t, "pt-old")
	client.waitStopped(t, "pt-new")
}

func TestFeishuManagerEnsureAgentStopsStaleParticipantBeforeSessionRestore(t *testing.T) {
	client := newRecordingBotClient()
	bridge := codexbridge.NewService(client, noopPrompter{}, runtimecodex.NewEventSink())
	manager := &feishuManager{
		runtime:            runtimecodex.New(runtimecodex.Dependencies{EventSink: runtimecodex.NewEventSink()}),
		bridge:             bridge,
		provider:           testCredentialProvider{"agent-1": "pt-new"},
		activeParticipants: map[string]string{"agent-1": "pt-old"},
	}

	if err := bridge.StartBot(context.Background(), codexbridge.Binding{BotID: "pt-old", RuntimeID: "rt-1"}); err != nil {
		t.Fatalf("StartBot() error = %v", err)
	}
	client.waitStarted(t, "pt-old")

	err := manager.EnsureAgent(context.Background(), agent.Agent{
		ID:              "agent-1",
		Name:            "dev",
		Role:            agent.RoleWorker,
		RuntimeKind:     agent.RuntimeKindCodex,
		RuntimeID:       "rt-1",
		Status:          string(agentruntime.StateRunning),
		ProfileComplete: true,
	})
	if err == nil {
		t.Fatal("EnsureAgent() error = nil, want missing session error")
	}

	client.waitStopped(t, "pt-old")
	if got := manager.clearActiveParticipant("agent-1"); got != "" {
		t.Fatalf("active participant = %q, want cleared after stale stop", got)
	}
}

type noopPrompter struct{}

func (noopPrompter) Prompt(context.Context, runtimecodex.SessionHandle, runtimecodex.PromptRequest) (runtimecodex.PromptResponse, error) {
	return runtimecodex.PromptResponse{}, nil
}

type testCredentialProvider map[string]string

func (p testCredentialProvider) BotConfigForAgent(agentID string) (string, feishu.AppConfig, bool) {
	participantID := strings.TrimSpace(p[strings.TrimSpace(agentID)])
	return participantID, feishu.AppConfig{}, participantID != ""
}

type recordingBotClient struct {
	mu      sync.Mutex
	started map[string]chan struct{}
	stopped map[string]chan struct{}
}

func newRecordingBotClient() *recordingBotClient {
	return &recordingBotClient{
		started: make(map[string]chan struct{}),
		stopped: make(map[string]chan struct{}),
	}
}

func (c *recordingBotClient) StreamEvents(ctx context.Context, botID string, _ string) (<-chan codexbridge.BotEvent, <-chan error) {
	events := make(chan codexbridge.BotEvent)
	errs := make(chan error)
	c.signal(c.started, botID)
	go func() {
		defer close(events)
		defer close(errs)
		<-ctx.Done()
		c.signal(c.stopped, botID)
	}()
	return events, errs
}

func (c *recordingBotClient) SendMessage(context.Context, string, codexbridge.SendMessageRequest) (codexbridge.SendMessageResponse, error) {
	return codexbridge.SendMessageResponse{MessageID: "msg-1"}, nil
}

func (c *recordingBotClient) waitStarted(t *testing.T, botID string) {
	t.Helper()
	c.wait(t, c.started, botID, "start")
}

func (c *recordingBotClient) waitStopped(t *testing.T, botID string) {
	t.Helper()
	c.wait(t, c.stopped, botID, "stop")
}

func (c *recordingBotClient) wait(t *testing.T, signals map[string]chan struct{}, botID string, action string) {
	t.Helper()
	ch := c.signalChannel(signals, botID)
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for bot %s to %s", botID, action)
	}
}

func (c *recordingBotClient) signal(signals map[string]chan struct{}, botID string) {
	ch := c.signalChannel(signals, botID)
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (c *recordingBotClient) signalChannel(signals map[string]chan struct{}, botID string) chan struct{} {
	botID = strings.TrimSpace(botID)
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := signals[botID]
	if ch == nil {
		ch = make(chan struct{}, 8)
		signals[botID] = ch
	}
	return ch
}
