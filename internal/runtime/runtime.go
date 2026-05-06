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

type Runtime interface {
	Kind() string

	Create(ctx context.Context, spec Spec) (Handle, error)
	Start(ctx context.Context, h Handle) (State, error)
	Stop(ctx context.Context, h Handle) (State, error)
	Delete(ctx context.Context, h Handle) error
	State(ctx context.Context, h Handle) (State, error)
	Info(ctx context.Context, h Handle) (Info, error)
}

type LogStreamer interface {
	StreamLogs(ctx context.Context, h Handle, opts LogOptions) error
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
