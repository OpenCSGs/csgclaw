package binding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"csgclaw/internal/agentengine"
	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu"
	agentruntime "csgclaw/internal/runtime"
)

var (
	// ErrAuthoritativeBindingConflict marks a complete credential snapshot that
	// is unsafe to activate. Unlike a transient provider error, callers may use
	// the accompanying partial snapshot to stop conflicting workers.
	ErrAuthoritativeBindingConflict = errors.New("authoritative feishu binding conflict")
	// ErrAppOwnershipConflict identifies the authoritative conflict in which one
	// Feishu AppID is selected by more than one Agent/Participant owner.
	ErrAppOwnershipConflict = fmt.Errorf("%w: app id has multiple owners", ErrAuthoritativeBindingConflict)
)

// AppOwner is the non-secret control-plane identity selecting an AppID.
type AppOwner struct {
	AgentID       string
	ParticipantID string
}

// AppOwnershipConflictError reports one unsafe AppID without exposing its
// AppSecret. It unwraps to both ErrAppOwnershipConflict and
// ErrAuthoritativeBindingConflict.
type AppOwnershipConflictError struct {
	AppID  string
	Owners []AppOwner
}

func (e *AppOwnershipConflictError) Error() string {
	if e == nil {
		return ErrAppOwnershipConflict.Error()
	}
	ordered := append([]AppOwner(nil), e.Owners...)
	sort.Slice(ordered, func(i, j int) bool {
		return appOwnerKey(ordered[i]) < appOwnerKey(ordered[j])
	})
	owners := make([]string, 0, len(ordered))
	for _, owner := range ordered {
		owners = append(owners, fmt.Sprintf("agent=%q participant=%q",
			owner.AgentID, owner.ParticipantID))
	}
	return fmt.Sprintf("%s: app_id=%q owners=[%s]", ErrAppOwnershipConflict, strings.TrimSpace(e.AppID), strings.Join(owners, ", "))
}

func (*AppOwnershipConflictError) Unwrap() error { return ErrAppOwnershipConflict }

// AgentLister is the narrow Agent Engine read port used by the Feishu control
// plane. Resolver never reads RuntimeID or gates a binding on runtime status.
type AgentLister interface {
	List(context.Context, agentengine.AgentListOptions) ([]agentengine.Agent, error)
}

// Resolved contains one desired App Binding and its transport credentials.
// Credentials must never be logged or serialized by channel state.
type Resolved struct {
	Binding channeltypes.Binding
	App     feishu.AppConfig
}

func (r Resolved) String() string {
	return fmt.Sprintf("Resolved{BindingID:%q AgentID:%q ParticipantID:%q AppID:%q AppSecret:[redacted]}",
		r.Binding.ID, r.Binding.AgentID, r.Binding.ParticipantID, strings.TrimSpace(r.App.AppID))
}

// Fingerprint detects credential rotation without exposing secret material.
func (r Resolved) Fingerprint() string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(r.App.AppID) + "\x00" + strings.TrimSpace(r.App.AppSecret)))
	return hex.EncodeToString(sum[:])
}

type Resolver struct {
	agents   AgentLister
	provider feishu.AgentCredentialProvider
}

type credentialProviderWithError interface {
	BotConfigForAgentWithError(agentID string) (participantID string, app feishu.AppConfig, ok bool, err error)
}

func NewResolver(agents AgentLister, provider feishu.AgentCredentialProvider) *Resolver {
	return &Resolver{agents: agents, provider: provider}
}

func (r *Resolver) All(ctx context.Context) ([]Resolved, error) {
	if r == nil || r.agents == nil || r.provider == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	agents, err := r.agents.List(ctx, agentengine.AgentListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Agent Engine agents for Feishu bindings: %w", err)
	}
	selections := make([]credentialSelection, 0, len(agents))
	for _, item := range agents {
		// Runtime-native channels are outside the hosted Feishu binding
		// manager. Do not read or validate their credentials here.
		if !strings.EqualFold(strings.TrimSpace(item.Spec.Runtime.Adapter), agentruntime.NameCodex) || item.Spec.Runtime.Sandboxed {
			continue
		}
		selection, ok, err := r.selectCredential(item.ID)
		if err != nil {
			// The snapshot is incomplete, so it cannot authoritatively remove any
			// last-known worker, including one that appears conflicted so far.
			return nil, err
		}
		if ok {
			selections = append(selections, selection)
		}
	}

	conflicts, conflictErr := appOwnershipConflicts(selections)
	out := make([]Resolved, 0, len(selections))
	for _, selection := range selections {
		if _, conflicted := conflicts[selection.resolved.App.AppID]; conflicted {
			continue
		}
		out = append(out, selection.resolved)
	}
	return out, conflictErr
}

type credentialSelection struct {
	resolved Resolved
	owner    AppOwner
}

func (r *Resolver) selectCredential(agentID string) (credentialSelection, bool, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return credentialSelection{}, false, nil
	}
	participantID, app, ok, err := r.credential(agentID)
	if err != nil {
		return credentialSelection{}, false, fmt.Errorf("resolve feishu binding credentials for agent %q: %w", agentID, err)
	}
	participantID = strings.TrimSpace(participantID)
	app.AppID = strings.TrimSpace(app.AppID)
	app.AppSecret = strings.TrimSpace(app.AppSecret)
	if !ok || participantID == "" || app.AppID == "" || app.AppSecret == "" {
		return credentialSelection{}, false, nil
	}
	return credentialSelection{
		resolved: Resolved{
			Binding: channeltypes.Binding{
				ID:            stableBindingID(participantID),
				Channel:       feishu.ChannelID,
				AgentID:       agentID,
				ParticipantID: participantID,
			},
			App: app,
		},
		owner: AppOwner{
			AgentID:       agentID,
			ParticipantID: participantID,
		},
	}, true, nil
}

func appOwnershipConflicts(selections []credentialSelection) (map[string]struct{}, error) {
	byApp := make(map[string]map[string]AppOwner)
	for _, selection := range selections {
		appID := strings.TrimSpace(selection.resolved.App.AppID)
		if appID == "" {
			continue
		}
		owners := byApp[appID]
		if owners == nil {
			owners = make(map[string]AppOwner)
			byApp[appID] = owners
		}
		owners[appOwnerKey(selection.owner)] = selection.owner
	}

	appIDs := make([]string, 0, len(byApp))
	for appID, owners := range byApp {
		if len(owners) > 1 {
			appIDs = append(appIDs, appID)
		}
	}
	sort.Strings(appIDs)
	conflicts := make(map[string]struct{}, len(appIDs))
	var conflictErr error
	for _, appID := range appIDs {
		conflicts[appID] = struct{}{}
		owners := make([]AppOwner, 0, len(byApp[appID]))
		for _, owner := range byApp[appID] {
			owners = append(owners, owner)
		}
		sort.Slice(owners, func(i, j int) bool {
			return appOwnerKey(owners[i]) < appOwnerKey(owners[j])
		})
		conflictErr = errors.Join(conflictErr, &AppOwnershipConflictError{AppID: appID, Owners: owners})
	}
	return conflicts, conflictErr
}

func appOwnerKey(owner AppOwner) string {
	return strings.TrimSpace(owner.AgentID) + "\x00" + strings.TrimSpace(owner.ParticipantID)
}

func (r *Resolver) credential(agentID string) (string, feishu.AppConfig, bool, error) {
	if provider, ok := r.provider.(credentialProviderWithError); ok {
		return provider.BotConfigForAgentWithError(agentID)
	}
	participantID, app, found := r.provider.BotConfigForAgent(agentID)
	return participantID, app, found, nil
}

func stableBindingID(participantID string) string {
	return feishu.ChannelID + ":" + strings.TrimSpace(participantID)
}
