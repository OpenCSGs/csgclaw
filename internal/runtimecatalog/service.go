package runtimecatalog

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/codexcli"
	"csgclaw/internal/dshcli"
)

const (
	RuntimeCodex      = "codex"
	RuntimeDSH        = "dsh"
	RuntimeClaudeCode = "claude_code"

	StatusComingSoon = "coming_soon"
)

var ErrRuntimeNotInstallable = errors.New("runtime is not installable")

const (
	InstallStatusIdle      = "idle"
	InstallStatusRunning   = "running"
	InstallStatusSucceeded = "succeeded"
	InstallStatusFailed    = "failed"
)

type Runtime struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Supported   bool   `json:"supported"`
	Installed   bool   `json:"installed"`
	Installable bool   `json:"installable"`
	Status      string `json:"status"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	DocsURL     string `json:"docs_url,omitempty"`
	Message     string `json:"message,omitempty"`
	MessageCode string `json:"message_code,omitempty"`
}

type RuntimeResolver interface {
	Ensure(context.Context) (string, error)
}

type RuntimeInstaller interface {
	Install(context.Context) (dshcli.Info, error)
}

type ProgressRuntimeInstaller interface {
	InstallWithProgress(context.Context, dshcli.ProgressReporter) (dshcli.Info, error)
}

type Installation struct {
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Stage         string   `json:"stage,omitempty"`
	ActivityCount int      `json:"activity_count,omitempty"`
	StartedAt     string   `json:"started_at,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
	Runtime       *Runtime `json:"runtime,omitempty"`
	ErrorCode     string   `json:"error_code,omitempty"`
	Message       string   `json:"message,omitempty"`
}

type Option func(*Service)

type Service struct {
	codex          RuntimeResolver
	dsh            RuntimeResolver
	dshInstaller   RuntimeInstaller
	goos           string
	goarch         string
	installMu      sync.Mutex
	installationMu sync.RWMutex
	installations  map[string]Installation
}

func NewService(opts ...Option) *Service {
	service := &Service{
		codex:         codexcli.Provider{},
		dsh:           dshcli.Provider{},
		dshInstaller:  dshcli.Installer{},
		goos:          runtime.GOOS,
		goarch:        runtime.GOARCH,
		installations: make(map[string]Installation),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func WithDSHInstaller(installer RuntimeInstaller) Option {
	return func(service *Service) {
		if installer != nil {
			service.dshInstaller = installer
		}
	}
}

func WithCodexResolver(resolver RuntimeResolver) Option {
	return func(service *Service) {
		if resolver != nil {
			service.codex = resolver
		}
	}
}

func WithDSHResolver(resolver RuntimeResolver) Option {
	return func(service *Service) {
		if resolver != nil {
			service.dsh = resolver
		}
	}
}

func WithPlatform(goos, goarch string) Option {
	return func(service *Service) {
		if value := strings.TrimSpace(goos); value != "" {
			service.goos = value
		}
		if value := strings.TrimSpace(goarch); value != "" {
			service.goarch = value
		}
	}
}

func (s *Service) List() []Runtime {
	return []Runtime{s.codexRuntime(), s.dshRuntime(), s.claudeCodeRuntime()}
}

func (s *Service) Install(ctx context.Context, name string) (Runtime, error) {
	if s == nil || strings.TrimSpace(name) != RuntimeDSH || s.dshInstaller == nil {
		return Runtime{}, fmt.Errorf("%w: %s", ErrRuntimeNotInstallable, strings.TrimSpace(name))
	}
	s.installMu.Lock()
	defer s.installMu.Unlock()
	if current := s.dshRuntime(); current.Installed {
		return current, nil
	}
	if _, err := s.dshInstaller.Install(ctx); err != nil {
		return Runtime{}, err
	}
	installed := s.dshRuntime()
	if !installed.Installed {
		return Runtime{}, fmt.Errorf("%w: DSH remains unavailable after installation", dshcli.ErrVerificationFailed)
	}
	return installed, nil
}

func (s *Service) StartInstall(name string) (Installation, error) {
	name = strings.TrimSpace(name)
	if s == nil || name != RuntimeDSH || s.dshInstaller == nil {
		return Installation{}, fmt.Errorf("%w: %s", ErrRuntimeNotInstallable, name)
	}
	if current := s.dshRuntime(); current.Installed {
		return s.storeCompletedInstallation(name, current), nil
	}

	s.installationMu.Lock()
	if current, ok := s.installations[name]; ok && current.Status == InstallStatusRunning {
		s.installationMu.Unlock()
		return current, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	installation := Installation{
		Name:      name,
		Status:    InstallStatusRunning,
		Stage:     dshcli.InstallStageChecking,
		StartedAt: now,
		UpdatedAt: now,
	}
	s.installations[name] = installation
	s.installationMu.Unlock()

	go s.runInstallation(name)
	return installation, nil
}

func (s *Service) Installation(name string) (Installation, error) {
	name = strings.TrimSpace(name)
	if s == nil || name != RuntimeDSH || s.dshInstaller == nil {
		return Installation{}, fmt.Errorf("%w: %s", ErrRuntimeNotInstallable, name)
	}
	s.installationMu.RLock()
	installation, ok := s.installations[name]
	s.installationMu.RUnlock()
	if ok {
		return installation, nil
	}
	if current := s.dshRuntime(); current.Installed {
		return s.storeCompletedInstallation(name, current), nil
	}
	return Installation{Name: name, Status: InstallStatusIdle}, nil
}

func (s *Service) runInstallation(name string) {
	s.installMu.Lock()
	defer s.installMu.Unlock()

	if current := s.dshRuntime(); current.Installed {
		s.storeCompletedInstallation(name, current)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	report := func(progress dshcli.InstallProgress) {
		s.installationMu.Lock()
		installation := s.installations[name]
		if installation.Status == InstallStatusRunning {
			installation.Stage = progress.Stage
			if progress.ActivityCount > installation.ActivityCount {
				installation.ActivityCount = progress.ActivityCount
			}
			installation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			s.installations[name] = installation
		}
		s.installationMu.Unlock()
	}

	var err error
	if installer, ok := s.dshInstaller.(ProgressRuntimeInstaller); ok {
		_, err = installer.InstallWithProgress(ctx, report)
	} else {
		_, err = s.dshInstaller.Install(ctx)
	}
	if err != nil {
		s.failInstallation(name, err)
		return
	}
	installed := s.dshRuntime()
	if !installed.Installed {
		s.failInstallation(name, fmt.Errorf("%w: DSH remains unavailable after installation", dshcli.ErrVerificationFailed))
		return
	}
	s.storeCompletedInstallation(name, installed)
}

func (s *Service) storeCompletedInstallation(name string, installed Runtime) Installation {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.installationMu.Lock()
	startedAt := now
	activityCount := 0
	if current, ok := s.installations[name]; ok {
		if current.StartedAt != "" {
			startedAt = current.StartedAt
		}
		activityCount = current.ActivityCount
	}
	installation := Installation{
		Name:          name,
		Status:        InstallStatusSucceeded,
		Stage:         dshcli.InstallStageCompleted,
		ActivityCount: activityCount,
		StartedAt:     startedAt,
		UpdatedAt:     now,
		Runtime:       &installed,
	}
	s.installations[name] = installation
	s.installationMu.Unlock()
	return installation
}

func (s *Service) failInstallation(name string, err error) {
	s.installationMu.Lock()
	installation := s.installations[name]
	installation.Status = InstallStatusFailed
	installation.ErrorCode = runtimeInstallErrorCode(err)
	installation.Message = err.Error()
	installation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.installations[name] = installation
	s.installationMu.Unlock()
}

func runtimeInstallErrorCode(err error) string {
	switch {
	case errors.Is(err, dshcli.ErrNodeNotFound), errors.Is(err, dshcli.ErrNPMNotFound):
		return "dsh_node_required"
	case errors.Is(err, dshcli.ErrNodeUnsupported):
		return "dsh_node_version_unsupported"
	case errors.Is(err, dshcli.ErrNodeInstallFailed), errors.Is(err, dshcli.ErrInstallFailed), errors.Is(err, dshcli.ErrVerificationFailed):
		return "dsh_install_failed"
	default:
		return "dsh_install_failed"
	}
}

func (s *Service) dshRuntime() Runtime {
	runtimeInfo := Runtime{
		Name:        RuntimeDSH,
		Label:       "DeepSeek Harness",
		Supported:   true,
		Installable: true,
		OS:          s.resolvedGOOS(),
		Arch:        s.resolvedGOARCH(),
		DocsURL:     dshcli.DocumentationURL,
	}
	if s == nil || s.dsh == nil {
		runtimeInfo.Status = "failed"
		runtimeInfo.Message = "DSH CLI resolver is not configured"
		return runtimeInfo
	}
	var path, version string
	var err error
	if resolver, ok := s.dsh.(interface {
		Resolve(context.Context) (dshcli.Info, error)
	}); ok {
		var info dshcli.Info
		info, err = resolver.Resolve(context.Background())
		path, version = info.Path, info.Version
	} else {
		path, err = s.dsh.Ensure(context.Background())
	}
	if err != nil {
		runtimeInfo.Status = "missing"
		runtimeInfo.Message = fmt.Sprintf("DSH CLI is unavailable: %v", err)
		runtimeInfo.MessageCode = "dsh_not_installed"
		return runtimeInfo
	}
	runtimeInfo.Installed = true
	runtimeInfo.Status = "installed"
	runtimeInfo.Path = path
	runtimeInfo.Version = version
	return runtimeInfo
}

func (s *Service) codexRuntime() Runtime {
	runtimeInfo := Runtime{
		Name:        RuntimeCodex,
		Label:       "Codex CLI",
		Supported:   true,
		Installable: false,
		OS:          s.resolvedGOOS(),
		Arch:        s.resolvedGOARCH(),
		DocsURL:     "https://developers.openai.com/codex",
	}
	if s == nil || s.codex == nil {
		runtimeInfo.Status = "failed"
		runtimeInfo.Message = "Bundled Codex CLI resolver is not configured"
		return runtimeInfo
	}
	path, err := s.codex.Ensure(context.Background())
	if err != nil {
		runtimeInfo.Status = "failed"
		runtimeInfo.Message = fmt.Sprintf("Bundled Codex CLI is unavailable: %v", err)
		return runtimeInfo
	}
	runtimeInfo.Installed = true
	runtimeInfo.Status = "installed"
	runtimeInfo.Path = path
	return runtimeInfo
}

func (s *Service) claudeCodeRuntime() Runtime {
	return Runtime{
		Name:        RuntimeClaudeCode,
		Label:       "Claude Code",
		Supported:   false,
		Installed:   false,
		Installable: false,
		Status:      StatusComingSoon,
		OS:          s.resolvedGOOS(),
		Arch:        s.resolvedGOARCH(),
		DocsURL:     "https://docs.anthropic.com/en/docs/claude-code/overview",
		Message:     "Claude Code runtime support is coming soon",
	}
}

func (s *Service) resolvedGOOS() string {
	if s != nil && strings.TrimSpace(s.goos) != "" {
		return strings.TrimSpace(s.goos)
	}
	return runtime.GOOS
}

func (s *Service) resolvedGOARCH() string {
	if s != nil && strings.TrimSpace(s.goarch) != "" {
		return strings.TrimSpace(s.goarch)
	}
	return runtime.GOARCH
}
