package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
)

const sandboxRuntimeLockRetryAttempts = 8

var sandboxRuntimeLockRetryDelay = 50 * time.Millisecond

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
	var box sandbox.Instance
	var err error
	for attempt := 0; attempt < sandboxRuntimeLockRetryAttempts; attempt++ {
		if testGetBoxHook != nil {
			box, err = testGetBoxHook(s, ctx, rt, idOrName)
		} else {
			box, err = rt.Get(ctx, idOrName)
		}
		if !shouldRetrySandboxRuntimeLock(ctx, err, attempt) {
			return box, err
		}
	}
	return box, err
}

func (s *Service) startBox(ctx context.Context, box sandbox.Instance) error {
	var err error
	for attempt := 0; attempt < sandboxRuntimeLockRetryAttempts; attempt++ {
		if testStartBoxHook != nil {
			err = testStartBoxHook(s, ctx, box)
		} else {
			err = box.Start(ctx)
		}
		if !shouldRetrySandboxRuntimeLock(ctx, err, attempt) {
			return err
		}
	}
	return err
}

func (s *Service) stopBox(ctx context.Context, box sandbox.Instance, opts sandbox.StopOptions) error {
	var err error
	for attempt := 0; attempt < sandboxRuntimeLockRetryAttempts; attempt++ {
		if testStopBoxHook != nil {
			err = testStopBoxHook(s, ctx, box, opts)
		} else {
			err = box.Stop(ctx, opts)
		}
		if !shouldRetrySandboxRuntimeLock(ctx, err, attempt) {
			return err
		}
	}
	return err
}

func (s *Service) boxInfo(ctx context.Context, box sandbox.Instance) (sandbox.Info, error) {
	var info sandbox.Info
	var err error
	for attempt := 0; attempt < sandboxRuntimeLockRetryAttempts; attempt++ {
		if testBoxInfoHook != nil {
			info, err = testBoxInfoHook(s, ctx, box)
		} else {
			info, err = box.Info(ctx)
		}
		if !shouldRetrySandboxRuntimeLock(ctx, err, attempt) {
			return info, err
		}
	}
	return info, err
}

func isSandboxRuntimeLockError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "runtime lock") || strings.Contains(text, "another boxliteruntime")
}

func shouldRetrySandboxRuntimeLock(ctx context.Context, err error, attempt int) bool {
	if err == nil || !isSandboxRuntimeLockError(err) || attempt == sandboxRuntimeLockRetryAttempts-1 {
		return false
	}
	return waitSandboxRuntimeLockRetry(ctx, attempt) == nil
}

func waitSandboxRuntimeLockRetry(ctx context.Context, attempt int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delay := sandboxRuntimeLockRetryDelay * time.Duration(attempt+1)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) createBox(ctx context.Context, rt sandbox.Runtime, spec sandbox.CreateSpec) (sandbox.Instance, error) {
	var box sandbox.Instance
	var err error
	for attempt := 0; attempt < sandboxRuntimeLockRetryAttempts; attempt++ {
		if testCreateBoxHook != nil {
			box, err = testCreateBoxHook(s, ctx, rt, spec)
		} else {
			box, err = rt.Create(ctx, spec)
		}
		if !shouldRetrySandboxRuntimeLock(ctx, err, attempt) {
			return box, err
		}
	}
	return box, err
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
