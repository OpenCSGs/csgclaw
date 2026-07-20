package agentmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/connectors"
)

type AgentConnectorRef struct {
	AgentID   string
	AgentRole string
}

type ConnectorGrantPolicy interface {
	AllowsConnectorCredential(context.Context, AgentConnectorRef, string) bool
}

type ConnectorStatusProvider interface {
	ConnectorStatus(context.Context, AgentConnectorRef, string, string) (connectors.Status, error)
}

type ConnectorCredentialProvider interface {
	ManagedCredentialLease(context.Context, AgentConnectorRef, string) (ManagedCredentialLease, error)
}

type ManagedCredentialLease struct {
	Provider    string              `json:"provider"`
	BaseURL     string              `json:"base_url,omitempty"`
	Account     *connectors.Account `json:"account,omitempty"`
	AccessToken string              `json:"access_token,omitempty"`
	TokenType   string              `json:"token_type,omitempty"`
	Scopes      []string            `json:"scopes,omitempty"`
	ExpiresAt   time.Time           `json:"expires_at,omitempty"`
}

func (l ManagedCredentialLease) Redact(value string) string {
	secret := strings.TrimSpace(l.AccessToken)
	if secret != "" {
		value = strings.ReplaceAll(value, secret, "[redacted]")
	}
	return value
}

var ErrConnectorCredentialAccessDenied = errors.New("connector credential access denied")

type DefaultConnectorGrantPolicy struct{}

func (DefaultConnectorGrantPolicy) AllowsConnectorCredential(_ context.Context, ref AgentConnectorRef, provider string) bool {
	return strings.TrimSpace(ref.AgentID) == agent.ManagerUserID &&
		strings.EqualFold(strings.TrimSpace(ref.AgentRole), agent.RoleManager) &&
		(strings.EqualFold(strings.TrimSpace(provider), connectors.ProviderGitHub) ||
			strings.EqualFold(strings.TrimSpace(provider), connectors.ProviderGitLab))
}

type ConnectorServiceCredentialProvider struct {
	service *connectors.Service
	policy  ConnectorGrantPolicy
	now     func() time.Time
}

func NewConnectorServiceCredentialProvider(service *connectors.Service, policy ConnectorGrantPolicy) *ConnectorServiceCredentialProvider {
	if policy == nil {
		policy = DefaultConnectorGrantPolicy{}
	}
	return &ConnectorServiceCredentialProvider{
		service: service,
		policy:  policy,
		now:     time.Now,
	}
}

func (p *ConnectorServiceCredentialProvider) ConnectorStatus(ctx context.Context, ref AgentConnectorRef, provider, callbackURL string) (connectors.Status, error) {
	if p == nil || p.service == nil {
		return connectors.Status{}, fmt.Errorf("connector service is required")
	}
	if !p.policy.AllowsConnectorCredential(ctx, ref, provider) {
		return connectors.Status{}, ErrConnectorCredentialAccessDenied
	}
	return p.service.Status(ctx, provider, callbackURL)
}

func (p *ConnectorServiceCredentialProvider) ManagedCredentialLease(ctx context.Context, ref AgentConnectorRef, provider string) (ManagedCredentialLease, error) {
	if p == nil || p.service == nil {
		return ManagedCredentialLease{}, fmt.Errorf("connector service is required")
	}
	if !p.policy.AllowsConnectorCredential(ctx, ref, provider) {
		return ManagedCredentialLease{}, ErrConnectorCredentialAccessDenied
	}
	credential, err := p.service.Credential(ctx, provider)
	if err != nil {
		return ManagedCredentialLease{}, err
	}
	lease := ManagedCredentialLease{
		Provider:    strings.TrimSpace(credential.Provider),
		BaseURL:     strings.TrimRight(strings.TrimSpace(credential.BaseURL), "/"),
		AccessToken: strings.TrimSpace(credential.AccessToken),
		TokenType:   strings.TrimSpace(credential.TokenType),
		Scopes:      append([]string(nil), credential.Scopes...),
	}
	status, statusErr := p.service.Status(ctx, provider, "")
	if statusErr == nil && status.Account != nil {
		account := *status.Account
		lease.Account = &account
	}
	switch strings.TrimSpace(credential.Provider) {
	case connectors.ProviderGitHub:
		if lease.TokenType == "" {
			lease.TokenType = "bearer"
		}
	case connectors.ProviderGitLab:
		if lease.BaseURL == "" {
			return ManagedCredentialLease{}, fmt.Errorf("gitlab connector base URL is empty")
		}
		if lease.TokenType == "" {
			lease.TokenType = "private-token"
		}
	default:
		return ManagedCredentialLease{}, fmt.Errorf("connector provider %q is not supported for managed credentials", credential.Provider)
	}
	if p.now != nil {
		lease.ExpiresAt = p.now().UTC().Add(15 * time.Minute)
	}
	return lease, nil
}
