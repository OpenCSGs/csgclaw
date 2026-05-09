package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/api"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/upgrade"
)

type Options struct {
	ListenAddr  string
	Service     *agent.Service
	Bot         *bot.Service
	IM          *im.Service
	IMBus       *im.Bus
	BotBridge   *im.BotBridge
	Feishu      *feishu.Service
	LLM         *llm.Service
	Upgrade     *upgrade.Manager
	ConfigPath  string
	AccessToken string
	NoAuth      bool
	Context     context.Context
	OnReady     func()
}

func Run(opts Options) error {
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	handler := api.NewHandlerWithBotAndAuth(opts.Service, opts.Bot, opts.IM, opts.IMBus, opts.BotBridge, opts.Feishu, opts.LLM, opts.AccessToken, opts.NoAuth)
	handler.SetUpgradeManager(opts.Upgrade)
	handler.SetUpgradeConfigPath(opts.ConfigPath)
	mux := handler.Routes()
	mux.Handle("/", uiHandler())

	httpServer := &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           accessLog(slog.Default(), mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if opts.IMBus != nil && opts.BotBridge != nil {
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
					handler.PublishBotEvent(evt)
				}
			}
		}()
	}

	if opts.Upgrade != nil {
		go opts.Upgrade.Start(opts.Context)
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
		go opts.OnReady()
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
