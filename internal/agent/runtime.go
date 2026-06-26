package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
)

func (s *Service) ensureRuntime(agentID string) (sandbox.Runtime, error) {
	if testEnsureRuntimeHook != nil {
		return testEnsureRuntimeHook(s, agentID)
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	homeDir, err := s.sandboxRuntimeHome(agentID)
	if err != nil {
		return nil, err
	}
	return s.ensureRuntimeAtHome(homeDir)
}

func (s *Service) ensureRuntimeAtHome(homeDir string) (sandbox.Runtime, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return nil, fmt.Errorf("runtime home is required")
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create runtime home: %w", err)
	}
	if testEnsureRuntimeAtHomeHook != nil {
		return testEnsureRuntimeAtHomeHook(s, homeDir)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if rt := s.runtimes[homeDir]; rt != nil {
		return rt, nil
	}

	rt, err := s.sandbox.Open(context.Background(), homeDir)
	if err != nil {
		return nil, fmt.Errorf("create sandbox runtime: %w", err)
	}
	s.runtimes[homeDir] = rt
	return rt, nil
}

func (s *Service) lookupBootstrapManager(ctx context.Context) (sandbox.Runtime, sandbox.Instance, error) {
	rt, err := s.ensureRuntime(ManagerUserID)
	if err != nil {
		return nil, nil, err
	}
	if !s.hasBootstrapManagerRecord() {
		for _, key := range s.bootstrapManagerLookupKeys() {
			if err := s.forceRemoveBox(ctx, rt, key); err != nil {
				if sandbox.IsNotFound(err) {
					continue
				}
				return nil, nil, fmt.Errorf("remove stale bootstrap manager box %q: %w", key, err)
			}
			return rt, nil, nil
		}
		return rt, nil, nil
	}
	for _, key := range s.bootstrapManagerLookupKeys() {
		box, err := s.getBox(ctx, rt, key)
		if err == nil {
			return rt, box, nil
		}
		if !sandbox.IsNotFound(err) {
			return nil, nil, err
		}
	}
	return rt, nil, nil
}

func (s *Service) hasBootstrapManagerRecord() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.agents {
		if isManagerAgent(a) {
			return true
		}
	}
	return false
}

func (s *Service) getBox(ctx context.Context, rt sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
	if testGetBoxHook != nil {
		return testGetBoxHook(s, ctx, rt, idOrName)
	}
	return rt.Get(ctx, idOrName)
}

func (s *Service) startBox(ctx context.Context, box sandbox.Instance) error {
	if testStartBoxHook != nil {
		return testStartBoxHook(s, ctx, box)
	}
	return box.Start(ctx)
}

func (s *Service) stopBox(ctx context.Context, box sandbox.Instance, opts sandbox.StopOptions) error {
	if testStopBoxHook != nil {
		return testStopBoxHook(s, ctx, box, opts)
	}
	return box.Stop(ctx, opts)
}

func (s *Service) boxInfo(ctx context.Context, box sandbox.Instance) (sandbox.Info, error) {
	if testBoxInfoHook != nil {
		return testBoxInfoHook(s, ctx, box)
	}
	return box.Info(ctx)
}

func (s *Service) createBox(ctx context.Context, rt sandbox.Runtime, spec sandbox.CreateSpec) (sandbox.Instance, error) {
	if testCreateBoxHook != nil {
		return testCreateBoxHook(s, ctx, rt, spec)
	}
	return rt.Create(ctx, spec)
}

func (s *Service) runBoxCommand(ctx context.Context, box sandbox.Instance, name string, args []string, w io.Writer) (int, error) {
	if testRunBoxCommandHook != nil {
		return testRunBoxCommandHook(s, ctx, box, name, args, w)
	}
	result, err := box.Run(ctx, sandbox.CommandSpec{
		Name:   name,
		Args:   args,
		Stdout: w,
		Stderr: w,
	})
	if err != nil {
		return 0, err
	}
	return result.ExitCode, nil
}

func (s *Service) closeBox(box sandbox.Instance) error {
	if box == nil {
		return nil
	}
	if testCloseBoxHook != nil {
		return testCloseBoxHook(s, box)
	}
	return box.Close()
}

func (s *Service) closeRuntime(homeDir string, rt sandbox.Runtime) error {
	if rt == nil {
		return nil
	}
	s.mu.Lock()
	if cached := s.runtimes[homeDir]; cached == rt {
		delete(s.runtimes, homeDir)
	}
	s.mu.Unlock()

	if testCloseRuntimeHook != nil {
		return testCloseRuntimeHook(s, homeDir, rt)
	}
	return rt.Close()
}

func (s *Service) sandboxRuntimeHome(agentID string) (string, error) {
	agentHome, err := s.agentHomeDir(agentID)
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, config.RuntimeHomeDirName), nil
}

func SandboxRuntimeHome(agentID string) (string, error) {
	agentHome, err := agentHomeDir(agentID)
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, config.RuntimeHomeDirName), nil
}

func agentHomeDir(agentID string) (string, error) {
	root, err := config.DefaultAgentsDir()
	if err != nil {
		return "", err
	}
	return agentHomeDirInRoot(root, agentID)
}

func (s *Service) agentHomeDir(agentID string) (string, error) {
	root := ""
	if s != nil {
		root = strings.TrimSpace(s.agentsRoot)
	}
	if root == "" {
		var err error
		root, err = config.DefaultAgentsDir()
		if err != nil {
			return "", err
		}
	}
	return agentHomeDirInRoot(root, agentID)
}

func serviceAgentsRoot(statePath string) string {
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return ""
	}
	dir := filepath.Dir(statePath)
	if filepath.Base(dir) == managerAgentsDirName {
		return dir
	}
	return filepath.Join(dir, managerAgentsDirName)
}

func agentHomeDirInRoot(root, agentID string) (string, error) {
	agentID = canonicalAgentID(agentID)
	if agentID == "" {
		return "", fmt.Errorf("agent id is required")
	}
	if strings.ContainsAny(agentID, " \t\r\n/\\") {
		return "", fmt.Errorf("agent id must be path-safe: %s", agentID)
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("agents root is required")
	}
	return filepath.Join(root, agentID), nil
}
