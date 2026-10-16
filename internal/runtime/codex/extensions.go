package codex

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
	cliextension "csgclaw/internal/runtimeextension/connectorcli"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
)

var (
	larkCLILookPath                  = exec.LookPath
	larkCLICommandContext            = exec.CommandContext
	feishuLarkCLIManagedInstructions = larkextension.ManagedInstructions("command_execution")
)

var (
	_ agentruntime.ExtensionHost           = (*Runtime)(nil)
	_ agentruntime.ExtensionDriverProvider = (*Runtime)(nil)
)

func extensionStore(home string) (*extensionstate.Store, error) {
	return extensionstate.New(filepath.Join(home, "runtime-extensions"))
}

func managedExtensionInstructions(home string) ([]string, error) {
	store, err := extensionStore(home)
	if err != nil {
		return nil, err
	}
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	return agentruntime.ExtensionInstructions(items), nil
}

func (r *Runtime) RuntimeExtensionDriver(kind string) (agentruntime.ExtensionDriver, bool) {
	if r != nil && kind == cliextension.Kind {
		return cliextension.Driver{ResolveHome: r.resolveCodexHomeDir, RuntimeLoaded: func(agentID string, p agentruntime.ExtensionProjection) bool {
			session, err := r.SessionManager().LiveSession(SessionHandle{RuntimeID: "rt-" + canonicalRuntimeAgentID(agentID)})
			return err == nil && session != nil && session.ProcessID > 0 && session.ExtensionDigests[p.Name] == p.Digest
		}}, true
	}
	if r == nil || strings.TrimSpace(kind) != larkextension.Kind {
		return nil, false
	}
	driver := larkextension.NewDriver(larkextension.DriverOptions{
		ResolveHome:  r.resolveCodexHomeDir,
		Instructions: feishuLarkCLIManagedInstructions,
		LookPath:     func(name string) (string, error) { return larkCLILookPath(name) },
		CommandContext: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return larkCLICommandContext(ctx, name, args...)
		},
		RuntimeLoaded: func(agentID string, projection agentruntime.ExtensionProjection) bool {
			session, err := r.SessionManager().LiveSession(SessionHandle{RuntimeID: "rt-" + canonicalRuntimeAgentID(agentID)})
			return err == nil && session != nil && session.ProcessID > 0 && session.ExtensionDigests[projection.Name] == projection.Digest
		},
	})
	return driver, true
}

func (r *Runtime) ExtensionProjections(agentID string) ([]agentruntime.ExtensionProjection, error) {
	home, err := r.resolveCodexHomeDir(agentID)
	if err != nil {
		return nil, err
	}
	store, err := extensionStore(home)
	if err != nil {
		return nil, err
	}
	return store.List()
}

func (r *Runtime) RenderExtensions(ctx context.Context, agentID string, projections []agentruntime.ExtensionProjection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	home, err := r.resolveCodexHomeDir(agentID)
	if err != nil {
		return err
	}
	fragments := agentruntime.ExtensionInstructions(projections)
	return r.refreshCodexHomeAgentsFileWithFragments(agentruntime.Handle{RuntimeID: "rt-" + canonicalRuntimeAgentID(agentID)}, home, fragments)
}

func (r *Runtime) PrepareExtensionDelete(ctx context.Context, agentID, name string) (agentruntime.PreparedExtension, error) {
	home, err := r.resolveCodexHomeDir(agentID)
	if err != nil {
		return nil, err
	}
	store, err := extensionStore(home)
	if err != nil {
		return nil, err
	}
	return store.Delete(name)
}
