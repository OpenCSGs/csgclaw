package runtimecatalog

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"csgclaw/internal/codexcli"
)

const (
	RuntimeCodex      = "codex"
	RuntimeClaudeCode = "claude_code"

	StatusComingSoon  = "coming_soon"
	StatusUnsupported = "unsupported"
)

var (
	ErrRuntimeNotFound    = errors.New("agent runtime not found")
	ErrInstallUnsupported = errors.New("agent runtime installation is not supported")
)

type Runtime struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Supported   bool   `json:"supported"`
	Installed   bool   `json:"installed"`
	Installable bool   `json:"installable"`
	Status      string `json:"status"`
	Path        string `json:"path,omitempty"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	DocsURL     string `json:"docs_url,omitempty"`
	Message     string `json:"message,omitempty"`
}

type Option func(*Service)

type Service struct {
	codex  *codexcli.Installer
	goos   string
	goarch string
}

func NewService(opts ...Option) *Service {
	service := &Service{
		codex:  codexcli.NewInstaller(codexcli.InstallerOptions{}),
		goos:   runtime.GOOS,
		goarch: runtime.GOARCH,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func WithCodexInstaller(installer *codexcli.Installer) Option {
	return func(service *Service) {
		if installer != nil {
			service.codex = installer
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
	return []Runtime{s.codexRuntime(), s.claudeCodeRuntime()}
}

func (s *Service) Install(ctx context.Context, name string) (Runtime, error) {
	switch normalizeRuntimeName(name) {
	case RuntimeCodex:
		return s.EnsureCodex(ctx)
	case RuntimeClaudeCode:
		return s.claudeCodeRuntime(), fmt.Errorf("%w: %s", ErrInstallUnsupported, RuntimeClaudeCode)
	default:
		return Runtime{}, fmt.Errorf("%w: %s", ErrRuntimeNotFound, strings.TrimSpace(name))
	}
}

func (s *Service) EnsureCodex(ctx context.Context) (Runtime, error) {
	if s == nil || s.codex == nil {
		return Runtime{}, errors.New("Codex runtime installer is not configured")
	}
	_, err := s.codex.Ensure(ctx)
	return s.codexRuntime(), err
}

func (s *Service) codexRuntime() Runtime {
	status := codexcli.InstallStatus{State: codexcli.InstallStateFailed, Message: "Codex runtime installer is not configured"}
	if s != nil && s.codex != nil {
		status = s.codex.Status()
	}
	goos := s.resolvedGOOS()
	goarch := s.resolvedGOARCH()
	platformSupported := codexcli.SupportedPlatform(goos, goarch)
	if !status.Installed && !platformSupported {
		status.State = codexcli.InstallState(StatusUnsupported)
		status.Message = fmt.Sprintf("Codex CLI auto-install is not supported on %s/%s", goos, goarch)
	}
	return Runtime{
		Name:        RuntimeCodex,
		Label:       "Codex CLI",
		Supported:   status.Installed || platformSupported,
		Installed:   status.Installed,
		Installable: platformSupported,
		Status:      string(status.State),
		Path:        status.Path,
		OS:          goos,
		Arch:        goarch,
		DocsURL:     "https://developers.openai.com/codex",
		Message:     status.Message,
	}
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

func normalizeRuntimeName(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "-", "_")
}
