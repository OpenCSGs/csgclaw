package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/agent"
	"csgclaw/internal/agenttask"
	"csgclaw/internal/api"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/participant"
	"csgclaw/internal/scheduledtask"
	"csgclaw/internal/team"
	hub "csgclaw/internal/template"
	"csgclaw/internal/upgrade"
)

type Options struct {
	ListenAddr        string
	Service           *agent.Service
	Hub               *hub.Service
	Participant       *participant.Service
	IM                *im.Service
	IMBus             *im.Bus
	ParticipantBridge *im.ParticipantBridge
	Feishu            *feishu.Service
	LLM               *llm.Service
	Team              *team.Service
	AgentTask         *agenttask.Service
	ScheduledTask     *scheduledtask.Service
	TeamAdapters      *team.AdapterRegistry
	Upgrade           *upgrade.Manager
	ActivityDecider   api.ActivityDecider
	ConfigPath        string
	AccessToken       string
	NoAuth            bool
	Context           context.Context
	OnReady           func(h *api.Handler, router chi.Router)
}

func newHandler(opts Options) *api.Handler {
	handler := api.NewHandlerWithAuth(opts.Service, opts.IM, opts.IMBus, opts.ParticipantBridge, opts.Feishu, opts.LLM, opts.AccessToken, opts.NoAuth)
	handler.SetParticipantService(opts.Participant)
	handler.SetHubService(opts.Hub)
	handler.SetTeamService(opts.Team)
	handler.SetAgentTaskService(opts.AgentTask)
	handler.SetScheduledTaskService(opts.ScheduledTask)
	if opts.TeamAdapters != nil {
		handler.SetTeamAdapterRegistry(opts.TeamAdapters)
	}
	handler.SetUpgradeManager(opts.Upgrade)
	handler.SetActivityDecider(opts.ActivityDecider)
	handler.SetUpgradeConfigPath(opts.ConfigPath)
	handler.SetConfigPath(opts.ConfigPath)
	return handler
}

func Run(opts Options) error {
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	handler := newHandler(opts)
	router := handler.Routes()
	router.Handle("/*", uiFallbackHandler())

	httpServer := &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           accessLog(slog.Default(), router),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if opts.IMBus != nil && opts.ParticipantBridge != nil {
		events, cancel := opts.IMBus.Subscribe()
		defer cancel()

		go func() {
			for {
				select {
				case <-opts.Context.Done():
					return
				case evt, ok := <-events:
					if !ok {
						return
					}
					handler.PublishParticipantEvent(evt)
				}
			}
		}()
	}

	if opts.Upgrade != nil {
		go opts.Upgrade.Start(opts.Context)
	}
	if opts.ScheduledTask != nil {
		go opts.ScheduledTask.Start(opts.Context)
	}

	errCh := make(chan error, 1)
	go func() {
		<-opts.Context.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return err
	}
	if opts.OnReady != nil {
		go opts.OnReady(handler, router)
	}

	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		errCh <- err
	}

	close(errCh)
	if err := <-errCh; err != nil {
		return err
	}
	if opts.Service != nil {
		return opts.Service.Close()
	}
	return nil
}
