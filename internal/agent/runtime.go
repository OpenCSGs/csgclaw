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

func (s *Service) ensureRuntime(agentName string) (sandbox.Runtime, error) {
	if testEnsureRuntimeHook != nil {
		return testEnsureRuntimeHook(s, agentName)
	}
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	homeDir, err := s.sandboxRuntimeHome(agentName)
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
	homeDir, err := s.sandboxRuntimeHome(ManagerName)
	if err != nil {
		return nil, nil, err
	}
	rt, err := s.ensureRuntimeAtHome(homeDir)
	if err != nil {
		return nil, nil, err
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

func sandboxRuntimeHome(agentName string) (string, error) {
	return sandboxRuntimeHomeWithDirName(agentName, config.DefaultSandboxHomeDirName)
}

func (s *Service) sandboxRuntimeHome(agentName string) (string, error) {
	homeDirName := config.DefaultSandboxHomeDirName
	if s != nil && strings.TrimSpace(s.sandboxHome) != "" {
		homeDirName = s.sandboxHome
	}
	return sandboxRuntimeHomeWithDirName(agentName, homeDirName)
}

func sandboxRuntimeHomeWithDirName(agentName, homeDirName string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	homeDirName = strings.TrimSpace(homeDirName)
	if homeDirName == "" {
		homeDirName = config.DefaultSandboxHomeDirName
	}
	return filepath.Join(agentHome, homeDirName), nil
}

func agentHomeDir(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName), nil
}
