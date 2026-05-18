package runtimewiring

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

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

// WireNotifierDelivery registers POST {NotifyHTTPPathPrefix}{agent_id} on router and starts the pull supervisor.
// router may be nil (tests): routing is skipped but the pull supervisor still runs when agents and deliver are non-nil.
// IM may be nil (deliver is nil; webhook auth still works but delivery returns 503).
func WireNotifierDelivery(ctx context.Context, router chi.Router, agents *agent.Service, imSvc *im.Service, apiBaseURL, accessToken string) {
	if agents == nil {
		return
	}
	deliver := NewNotifierDeliver(imSvc, apiBaseURL, accessToken)
	deps := NotifierWebhookDeps(agents, deliver)
	if router != nil {
		router.Post(runtimenotifier.NotifyHTTPPathPrefix+"{agent_id}", func(w http.ResponseWriter, r *http.Request) {
			runtimenotifier.ServeNotifyHTTP(w, r, deps)
		})
	}
	go RunNotifierPullSupervisor(ctx, agents, deliver)
}
