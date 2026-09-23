package dsh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"csgclaw/internal/identity"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/extensionstate"
	runtimeinstructions "csgclaw/internal/runtime/instructions"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
)

var (
	larkCLILookPath                  = exec.LookPath
	larkCLICommandContext            = exec.CommandContext
	feishuLarkCLIManagedInstructions = larkextension.ManagedInstructions("bash")
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

func managedExtensionExecutables(projections []agentruntime.ExtensionProjection) map[string]string {
	executables := make(map[string]string, len(projections))
	for _, projection := range projections {
		if executable := strings.TrimSpace(projection.Executable); executable != "" {
			executables[projection.Name] = executable
		}
	}
	return executables
}

func (r *Runtime) resolveDSHHomeDir(agentID string) (string, error) {
	if r == nil || r.deps.AgentHome == nil {
		return "", fmt.Errorf("DSH agent home resolver is required")
	}
	agentHome, err := r.deps.AgentHome(identity.CanonicalAgentID(agentID))
	if err != nil {
		return "", err
	}
	layout, err := r.ensureRuntimeDirs(agentHome)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(layout.WorkspaceRoot), homeDirName), nil
}

func (r *Runtime) RuntimeExtensionDriver(kind string) (agentruntime.ExtensionDriver, bool) {
	if r == nil || strings.TrimSpace(kind) != larkextension.Kind {
		return nil, false
	}
	driver := larkextension.NewDriver(larkextension.DriverOptions{
		ResolveHome:  r.resolveDSHHomeDir,
		Instructions: feishuLarkCLIManagedInstructions,
		LookPath:     func(name string) (string, error) { return larkCLILookPath(name) },
		CommandContext: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return larkCLICommandContext(ctx, name, args...)
		},
		RuntimeLoaded: func(agentID string, projection agentruntime.ExtensionProjection) bool {
			proc, err := r.process("rt-" + identity.CanonicalAgentID(agentID))
			return err == nil && proc.extensionDigests[projection.Name] == projection.Digest
		},
	})
	return driver, true
}

func (r *Runtime) ExtensionProjections(agentID string) ([]agentruntime.ExtensionProjection, error) {
	home, err := r.resolveDSHHomeDir(agentID)
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
	home, err := r.resolveDSHHomeDir(agentID)
	if err != nil {
		return err
	}
	instructionsPath := filepath.Join(home, "AGENTS.md")
	var instructions *string
	if r.deps.ResolveAgent != nil {
		ref, resolveErr := r.deps.ResolveAgent(agentruntime.Handle{RuntimeID: "rt-" + identity.CanonicalAgentID(agentID)})
		if resolveErr != nil {
			return resolveErr
		}
		instructions = &ref.Instructions
	}
	return r.renderExtensionInstructions(ctx, instructionsPath, agentID, instructions, projections)
}

func (r *Runtime) renderExtensionInstructions(ctx context.Context, instructionsPath, agentID string, instructions *string, projections []agentruntime.ExtensionProjection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := os.ReadFile(instructionsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read DSH AGENTS.md: %w", err)
	}
	userInstructions := runtimeinstructions.ExtractUserInstructionsFromAgentsDocument(string(current))
	if instructions != nil {
		userInstructions = *instructions
	}
	fragments := agentruntime.ExtensionInstructions(projections)
	block := runtimeinstructions.RenderRuntimeAgentsInstructionsBlockWithOptions(agentID, userInstructions, runtimeinstructions.RuntimeManagedInstructionsOptions{Extensions: fragments})
	document := mergeDSHInstructionsDocument(stripManagedInstructions(string(current)), block)
	if string(current) == document {
		return nil
	}
	if err := os.WriteFile(instructionsPath, []byte(document), 0o644); err != nil {
		return fmt.Errorf("write DSH AGENTS.md: %w", err)
	}
	return nil
}

func mergeDSHInstructionsDocument(base, block string) string {
	document := strings.TrimSpace(base)
	if document != "" {
		document += "\n\n"
	}
	return document + strings.TrimSpace(block) + "\n"
}

func (r *Runtime) PrepareExtensionDelete(_ context.Context, agentID, name string) (agentruntime.PreparedExtension, error) {
	home, err := r.resolveDSHHomeDir(agentID)
	if err != nil {
		return nil, err
	}
	store, err := extensionStore(home)
	if err != nil {
		return nil, err
	}
	return store.Delete(name)
}
