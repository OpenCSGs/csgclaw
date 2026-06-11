package runtime

import (
	"context"
	"io"
	"strings"
	"time"
)

const (
	KindPicoClawSandbox = "picoclaw_sandbox"
	KindOpenClawSandbox = "openclaw_sandbox"
	KindCodex           = "codex"
)

// Runtime owns the lightweight execution lifecycle for an agent runtime.
//
// New should stay focused on instantiating or reopening the runtime execution
// object and returning a handle. Host-side preparation such as workspace
// seeding, config materialization, or runtime directory setup belongs behind
// the optional Provisioner capability instead of being folded into New.
type Runtime interface {
	Kind() string
	Layout(agentHome string) Layout

	New(ctx context.Context, spec Spec) (Handle, error)
	Start(ctx context.Context, h Handle) (State, error)
	Stop(ctx context.Context, h Handle) (State, error)
	Delete(ctx context.Context, h Handle) error
	State(ctx context.Context, h Handle) (State, error)
	Info(ctx context.Context, h Handle) (Info, error)
}

type LogStreamer interface {
	StreamLogs(ctx context.Context, h Handle, opts LogOptions) error
}

type ConversationStartActionMode string

const (
	ConversationStartActionBotEvent ConversationStartActionMode = "bot_event"
	ConversationStartActionInternal ConversationStartActionMode = "internal"
)

type ConversationStartRequest struct {
	Channel      string
	BotID        string
	RoomID       string
	ThreadRootID string
	Reason       string
}

type ConversationStartAction struct {
	Mode         ConversationStartActionMode
	BotEventText string
	AckText      string
}

type ConversationStarter interface {
	NewConversation(ctx context.Context, h Handle, req ConversationStartRequest) (ConversationStartAction, error)
}

// HydrateTrustPersistedStopped reports whether hydrate should keep a persisted "stopped"
// agent status instead of overwriting it from runtime Info (some in-process runtimes
// always report "running" from Info/State).
func HydrateTrustPersistedStopped(r Runtime) bool {
	type trustPersisted interface {
		HydrateTrustPersistedStopped() bool
	}
	v, ok := r.(trustPersisted)
	return ok && v.HydrateTrustPersistedStopped()
}

type Handle struct {
	RuntimeID string `json:"runtime_id"`
	HandleID  string `json:"handle_id,omitempty"`
}

type Info struct {
	HandleID  string
	State     State
	CreatedAt time.Time
}

type State string

const (
	StateUnknown State = "unknown"
	StateCreated State = "created"
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateExited  State = "exited"
	StateFailed  State = "failed"
)

type Profile struct {
	Provider        string
	BaseURL         string
	APIKey          string
	ModelID         string
	ReasoningEffort string
	Env             map[string]string
}

func (p Profile) Normalized() Profile {
	p.Provider = strings.TrimSpace(p.Provider)
	p.ModelID = strings.TrimSpace(p.ModelID)
	p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	p.APIKey = strings.TrimSpace(p.APIKey)
	p.ReasoningEffort = strings.TrimSpace(p.ReasoningEffort)
	if len(p.Env) == 0 {
		p.Env = nil
		return p
	}
	env := make(map[string]string, len(p.Env))
	for key, value := range p.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		env[key] = strings.TrimSpace(value)
	}
	if len(env) == 0 {
		p.Env = nil
		return p
	}
	p.Env = env
	return p
}

type Spec struct {
	RuntimeID string
	AgentID   string
	AgentName string
	Image     string
	Profile   Profile
}

type LogOptions struct {
	Follow bool
	Tail   int
	Writer io.Writer
}

type Layout struct {
	WorkspaceRoot string
	SkillsRoot    string
	HostLogPaths  []string
}
