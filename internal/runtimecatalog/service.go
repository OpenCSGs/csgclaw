package runtimecatalog

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"csgclaw/internal/codexcli"
)

const (
	RuntimeCodex      = "codex"
	RuntimeClaudeCode = "claude_code"

	StatusComingSoon = "coming_soon"
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

type CodexResolver interface {
	Ensure(context.Context) (string, error)
}

type Option func(*Service)

type Service struct {
	codex  CodexResolver
	goos   string
	goarch string
}

func NewService(opts ...Option) *Service {
	service := &Service{
		codex:  codexcli.Provider{},
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

func WithCodexResolver(resolver CodexResolver) Option {
	return func(service *Service) {
		if resolver != nil {
			service.codex = resolver
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
