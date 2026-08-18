package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentsession"
	"csgclaw/internal/agenttask"
	"csgclaw/internal/api"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/mcp"
	"csgclaw/internal/participant"
	"csgclaw/internal/runtimecatalog"
	"csgclaw/internal/scheduledtask"
	"csgclaw/internal/team"
	hub "csgclaw/internal/template"
	"csgclaw/internal/upgrade"
	"csgclaw/internal/worklease"
)

type Options struct {
	ListenAddr         string
	Listener           net.Listener
	SandboxListener    net.Listener
	Service            *agent.Service
	Hub                *hub.Service
	MCP                *mcp.Service
	Participant        *participant.Service
	IM                 *im.Service
	IMBus              *im.Bus
	WorkReporter       worklease.ParticipantWorkReporter
	WorkBus            *worklease.Bus
	WorkControlBus     *worklease.ControlBus
	ParticipantBridge  *im.ParticipantBridge
	Feishu             *feishu.Service
	LLM                *llm.Service
	Team               *team.Service
	AgentTask          *agenttask.Service
	ScheduledTask      *scheduledtask.Service
	AgentRuntimes      *runtimecatalog.Service
	TeamAdapters       *team.AdapterRegistry
	Upgrade            *upgrade.Manager
	ActivityDecider    api.ActivityDecider
	UserInputResponder api.UserInputResponder
	AgentEngine        agentengine.Interface
	SessionBindings    *agentsession.Store
	ConfigPath         string
	AccessToken        string
	NoAuth             bool
	AdvertiseBaseURL   string
	Desktop            *DesktopOptions
	Context            context.Context
	OnReady            func(h *api.Handler, router chi.Router)
	BeforeShutdown     func(context.Context) error
}

func newHandler(opts Options) *api.Handler {
	handler := api.NewHandlerWithAuth(opts.Service, opts.IM, opts.IMBus, opts.ParticipantBridge, opts.Feishu, opts.LLM, opts.AccessToken, opts.NoAuth)
	handler.SetParticipantService(opts.Participant)
	handler.SetParticipantWorkService(opts.WorkReporter, opts.WorkBus, opts.WorkControlBus)
	handler.SetHubService(opts.Hub)
	handler.SetMCPService(opts.MCP)
	handler.SetTeamService(opts.Team)
	handler.SetAgentTaskService(opts.AgentTask)
	handler.SetScheduledTaskService(opts.ScheduledTask)
	handler.SetAgentRuntimeService(opts.AgentRuntimes)
	if opts.TeamAdapters != nil {
		handler.SetTeamAdapterRegistry(opts.TeamAdapters)
	}
	handler.SetUpgradeManager(opts.Upgrade)
	handler.SetActivityDecider(opts.ActivityDecider)
	handler.SetUserInputResponder(opts.UserInputResponder)
	handler.SetAgentEngine(opts.AgentEngine, opts.SessionBindings)
	handler.SetUpgradeConfigPath(opts.ConfigPath)
	handler.SetConfigPath(opts.ConfigPath)
	handler.SetAdvertiseBaseURL(opts.AdvertiseBaseURL)
	if opts.Desktop != nil {
		handler.SetRuntimeDistribution("electron")
		handler.SetDesktopSessionToken(opts.Desktop.SessionToken)
	}
	return handler
}

func Run(opts Options) error {
	if opts.Context == nil {
		opts.Context = context.Background()
	}

	listener := opts.Listener
	if listener == nil {
		var err error
		listener, err = net.Listen("tcp", opts.ListenAddr)
		if err != nil {
			return err
		}
	}

	handler := newHandler(opts)
	router := handler.Routes()
	router.Handle("/*", uiFallbackHandler())

	var rootHandler http.Handler = router
	type serverEndpoint struct {
		server   *http.Server
		listener net.Listener
	}
	endpoints := make([]serverEndpoint, 0, 2)
	if opts.Desktop != nil {
		var err error
		rootHandler, err = desktopRendererSecurityHandler(rootHandler, listener.Addr(), *opts.Desktop)
		if err != nil {
			_ = listener.Close()
			if opts.SandboxListener != nil {
				_ = opts.SandboxListener.Close()
			}
			return err
		}
		if opts.SandboxListener == nil {
			_ = listener.Close()
			return fmt.Errorf("desktop sandbox listener is required")
		}
		sandboxHandler, err := desktopSandboxSecurityHandler(router, opts.SandboxListener.Addr(), *opts.Desktop)
		if err != nil {
			_ = listener.Close()
			_ = opts.SandboxListener.Close()
			return err
		}
		endpoints = append(endpoints, serverEndpoint{
			server:   newHTTPServer(opts.SandboxListener.Addr().String(), sandboxHandler),
			listener: opts.SandboxListener,
		})
	} else if opts.SandboxListener != nil {
		_ = listener.Close()
		_ = opts.SandboxListener.Close()
		return fmt.Errorf("sandbox listener requires desktop options")
	}

	endpoints = append([]serverEndpoint{{
		server:   newHTTPServer(listener.Addr().String(), rootHandler),
		listener: listener,
	}}, endpoints...)

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

	if opts.Upgrade != nil && opts.Desktop == nil {
		go opts.Upgrade.Start(opts.Context)
	}
	if opts.ScheduledTask != nil {
		go opts.ScheduledTask.Start(opts.Context)
	}

	runCtx, cancelRun := context.WithCancel(opts.Context)
	defer cancelRun()
	shutdownDone := make(chan struct{})
	var shutdownErr error
	go func() {
		<-runCtx.Done()
		if opts.BeforeShutdown != nil {
			hookCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			shutdownErr = opts.BeforeShutdown(hookCtx)
			cancel()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, endpoint := range endpoints {
			_ = endpoint.server.Shutdown(shutdownCtx)
		}
		close(shutdownDone)
	}()

	errCh := make(chan error, len(endpoints))
	for _, endpoint := range endpoints {
		go func(endpoint serverEndpoint) {
			err := endpoint.server.Serve(endpoint.listener)
			if err == http.ErrServerClosed {
				err = nil
			}
			errCh <- err
		}(endpoint)
	}

	if opts.OnReady != nil {
		go opts.OnReady(handler, router)
	}

	firstErr := <-errCh
	cancelRun()
	<-shutdownDone
	for range len(endpoints) - 1 {
		if err := <-errCh; firstErr == nil && err != nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		return errors.Join(firstErr, shutdownErr)
	}
	if opts.Service != nil {
		return errors.Join(shutdownErr, opts.Service.Close())
	}
	return shutdownErr
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           accessLog(slog.Default(), handler),
		ReadHeaderTimeout: 5 * time.Second,
	}
}
