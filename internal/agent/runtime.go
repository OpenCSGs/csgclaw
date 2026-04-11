package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

func (s *Service) ensureRuntime(agentName string) (*boxlite.Runtime, error) {
	if testEnsureRuntimeHook != nil {
		return testEnsureRuntimeHook(s, agentName)
	}
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	homeDir, err := boxRuntimeHome(agentName)
	if err != nil {
		return nil, err
	}
	return s.ensureRuntimeAtHome(homeDir)
}

func (s *Service) ensureRuntimeAtHome(homeDir string) (*boxlite.Runtime, error) {
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

	opts := []boxlite.RuntimeOption{boxlite.WithHomeDir(homeDir)}
	rt, err := boxlite.NewRuntime(opts...)
	if err != nil {
		return nil, fmt.Errorf("create boxlite runtime: %w", err)
	}
	s.runtimes[homeDir] = rt
	return rt, nil
}

func (s *Service) lookupBootstrapManager(ctx context.Context) (*boxlite.Runtime, *boxlite.Box, error) {
	homeDir, err := boxRuntimeHome(ManagerName)
	if err != nil {
		return nil, nil, err
	}
	rt, err := s.ensureRuntimeAtHome(homeDir)
	if err != nil {
		return nil, nil, err
	}
	keys := []string{s.bootstrapManagerBoxIDOrName()}
	if keys[0] != ManagerName {
		keys = append(keys, ManagerName)
	}
	for _, key := range keys {
		box, err := s.getBox(ctx, rt, key)
		if err == nil {
			return rt, box, nil
		}
		if !boxlite.IsNotFound(err) {
			return nil, nil, err
		}
	}
	return rt, nil, nil
}

func (s *Service) getBox(ctx context.Context, rt *boxlite.Runtime, idOrName string) (*boxlite.Box, error) {
	if testGetBoxHook != nil {
		return testGetBoxHook(s, ctx, rt, idOrName)
	}
	return rt.Get(ctx, idOrName)
}

func (s *Service) startBox(ctx context.Context, box *boxlite.Box) error {
	if testStartBoxHook != nil {
		return testStartBoxHook(s, ctx, box)
	}
	return box.Start(ctx)
}

func (s *Service) boxInfo(ctx context.Context, box *boxlite.Box) (*boxlite.BoxInfo, error) {
	if testBoxInfoHook != nil {
		return testBoxInfoHook(s, ctx, box)
	}
	return box.Info(ctx)
}

func (s *Service) createBox(ctx context.Context, rt *boxlite.Runtime, image string, opts ...boxlite.BoxOption) (*boxlite.Box, error) {
	if testCreateBoxHook != nil {
		return testCreateBoxHook(s, ctx, rt, image, opts...)
	}
	return rt.Create(ctx, image, opts...)
}

func (s *Service) runBoxCommand(ctx context.Context, box *boxlite.Box, name string, args []string, w io.Writer) (int, error) {
	if testRunBoxCommandHook != nil {
		return testRunBoxCommandHook(s, ctx, box, name, args, w)
	}
	cmd := box.Command(name, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(ctx); err != nil {
		return 0, err
	}
	return cmd.ExitCode(), nil
}

func (s *Service) closeBox(box *boxlite.Box) error {
	if box == nil {
		return nil
	}
	if testCloseBoxHook != nil {
		return testCloseBoxHook(s, box)
	}
	return box.Close()
}

func (s *Service) closeRuntime(homeDir string, rt *boxlite.Runtime) error {
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

func runtimeValid(rt *boxlite.Runtime) bool {
	if rt == nil {
		return false
	}

	v := reflect.ValueOf(rt)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return false
	}
	elem := v.Elem()
	if !elem.IsValid() {
		return false
	}
	handle := elem.FieldByName("handle")
	if !handle.IsValid() || handle.Kind() != reflect.Ptr {
		return false
	}
	return !handle.IsNil()
}

func boxRuntimeHome(agentName string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, config.RuntimeHomeDirName), nil
}

func agentHomeDir(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName), nil
}
