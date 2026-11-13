package server

import (
	"context"
	"log/slog"
	"net"
	"path/filepath"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentmemory"
	"csgclaw/internal/api"
	"csgclaw/internal/identity"
	"csgclaw/internal/localstore"
)

func startAgentMemory(ctx context.Context, opts Options, listener net.Listener, handler *api.Handler) func() {
	engine, ok := opts.AgentEngine.(*agentengine.Engine)
	controller, ownsAgents := opts.AgentServices.Records.(*agent.Controller)
	if !ok || !ownsAgents || opts.IM == nil {
		return func() {}
	}
	mode := agentMemoryMode(opts, listener)

	managerHome, err := controller.RuntimeHost().AgentHome(identity.ManagerAgentID)
	root := filepath.Dir(managerHome)
	var store *agentmemory.Store
	if err == nil {
		store, err = agentmemory.Open(root, filepath.Join(filepath.Dir(root), localstore.RootStateFileName), mode)
	}
	if err != nil {
		slog.Warn("agent memory could not be opened", "error", err)
	}
	handler.SetAgentMemory(store, mode)
	controller.SetManagedMemory(func(id string) error {
		if store == nil {
			if err != nil {
				return err
			}
			return nil
		}
		return store.DeleteAgent(identity.CanonicalAgentID(id))
	}, func(id, content string) error {
		if store == nil {
			return agentmemory.ErrUnavailable
		}
		return store.SeedTemplate(identity.CanonicalAgentID(id), content)
	})
	if store == nil {
		return func() {}
	}
	engine.SetMemory(store)
	if mode == "disabled" || opts.LLM == nil {
		return func() {}
	}
	worker := agentmemory.Worker{
		Store:   store,
		History: opts.IM.MemoryHistory,
		Extract: opts.LLM.ExtractAgentMemory,
		Eligible: func(id string) bool {
			item, ok := controller.Agent(id)
			return ok && item.ArchivedAt == nil
		},
		Commit: engine.WithMemoryExecution,
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(workerCtx)
	}()
	return func() { cancel(); <-done }
}

// Personal memory follows the actual bound listener, not a user-maintained flag.
// A loopback URL alone does not make a network-facing deployment personal.
func agentMemoryMode(opts Options, listener net.Listener) string {
	if opts.MemoryCloud || listener == nil {
		return "disabled"
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr == nil || !addr.IP.IsLoopback() {
		return "disabled"
	}
	if opts.Desktop != nil {
		return "desktop"
	}
	return "local"
}
