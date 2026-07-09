package agentmanager

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/connectors"
)

type AgentService interface {
	EnsureManager(context.Context, bool) (agent.Agent, error)
}

type Service struct {
	agents      AgentService
	credentials ConnectorCredentialProvider
}

func NewService(agents AgentService, credentials ConnectorCredentialProvider) *Service {
	return &Service{agents: agents, credentials: credentials}
}

func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.agents == nil {
		return nil
	}
	_, err := s.agents.EnsureManager(ctx, false)
	return err
}

func (s *Service) Close() error {
	return nil
}

type GitHubPRReviewInput struct {
	Repo        string
	PullRequest int
}

type GitHubPRReviewWorkflow struct {
	Credentials ConnectorCredentialProvider
}

func (w GitHubPRReviewWorkflow) Validate(ctx context.Context, input GitHubPRReviewInput) error {
	if strings.TrimSpace(input.Repo) == "" {
		return fmt.Errorf("repo is required")
	}
	if input.PullRequest <= 0 {
		return fmt.Errorf("pull_request is required")
	}
	if w.Credentials == nil {
		return fmt.Errorf("connector credential provider is required")
	}
	_, err := w.Credentials.ManagedCredentialLease(ctx, AgentConnectorRef{
		AgentID:   agent.ManagerUserID,
		AgentRole: agent.RoleManager,
	}, connectors.ProviderGitHub)
	return err
}

func (w GitHubPRReviewWorkflow) Run(ctx context.Context, input GitHubPRReviewInput) error {
	if err := w.Validate(ctx, input); err != nil {
		return err
	}
	return fmt.Errorf("github pr review workflow is not implemented")
}
