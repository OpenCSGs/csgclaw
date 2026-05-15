package runtimewiring

import (
	"context"
	"fmt"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel/notifierbridge"
	"csgclaw/internal/im"
	runtimenotifier "csgclaw/internal/runtime/notifier"
	notifierpull "csgclaw/internal/runtime/notifier/pull"
)

func WithNotifierRuntime() agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		return agent.WithRuntime(runtimenotifier.NewAgentRuntime())(s)
	}
}

// NewNotifierDeliver posts notifier fan-out via POST /api/v1/messages (same path as UI message create).
func NewNotifierDeliver(imSvc *im.Service, apiBaseURL, accessToken string) *notifierbridge.APIDeliver {
	if imSvc == nil {
		return nil
	}
	return notifierbridge.NewAPIDeliver(imSvc, apiBaseURL, accessToken)
}

// NotifierWebhookDeps builds webhook HTTP dependencies for notifier inbound delivery.
func NotifierWebhookDeps(agents *agent.Service, deliver runtimenotifier.RoomMessenger) runtimenotifier.WebhookHTTPDeps {
	var reload func() error
	var lookup func(string) (map[string]any, string, string, string, bool)
	if agents != nil {
		reload = agents.Reload
		lookup = func(id string) (map[string]any, string, string, string, bool) {
			a, ok := agents.Agent(id)
			if !ok {
				return nil, "", "", "", false
			}
			return a.RuntimeOptions, a.Role, a.RuntimeKind, a.Status, true
		}
	}
	return runtimenotifier.WebhookHTTPDeps{
		Reload:              reload,
		LookupNotifierAgent: lookup,
		Deliver:             deliver,
	}
}

// RunNotifierPullSupervisor blocks until ctx is cancelled, reconciling per-agent remote_pull loops.
func RunNotifierPullSupervisor(ctx context.Context, agents *agent.Service, deliver runtimenotifier.Fanouter) {
	if agents == nil || deliver == nil {
		return
	}
	notifierpull.NewSupervisor(agents, deliver).Run(ctx)
}

// WireNotifierDelivery configures webhook deps on the API handler and starts pull supervisor.
// IM may be nil (deliver posts to zero rooms; webhook auth still works but delivery returns 503).
func WireNotifierDelivery(ctx context.Context, handler interface {
	SetNotifierWebhookDeps(runtimenotifier.WebhookHTTPDeps)
}, agents *agent.Service, imSvc *im.Service, apiBaseURL, accessToken string) {
	if handler == nil || agents == nil {
		return
	}
	deliver := NewNotifierDeliver(imSvc, apiBaseURL, accessToken)
	handler.SetNotifierWebhookDeps(NotifierWebhookDeps(agents, deliver))
	go RunNotifierPullSupervisor(ctx, agents, deliver)
}
