package larkcli

import (
	"context"
	"crypto/sha256"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
	larkextension "csgclaw/internal/runtimeextension/larkcli"
	"encoding/hex"
	"fmt"
	"strings"
)

type ParticipantReader interface {
	Get(channel, id string) (apitypes.Participant, bool)
}

type Options struct {
	Participants ParticipantReader
	BaseURL      func() string
	AccessToken  func(agentID, participantID, credentialRevision string) (string, error)
	HelperPath   func() (string, error)
}

// Source resolves a Feishu Participant into the transient payload consumed by
// the Runtime-owned lark-cli extension driver.
type Source struct {
	options Options
}

func NewSource(options Options) (*Source, error) {
	if options.Participants == nil || options.BaseURL == nil || options.AccessToken == nil || options.HelperPath == nil {
		return nil, fmt.Errorf("Feishu lark-cli source dependencies are required")
	}
	return &Source{options: options}, nil
}

func (s *Source) Resolve(_ context.Context, agentID, ref string) (agentengine.ResolvedRuntimeExtension, error) {
	agentID = agent.CanonicalID(agentID)
	ref = strings.TrimSpace(ref)
	item, ok := s.options.Participants.Get(participant.ChannelFeishu, ref)
	if !ok {
		return agentengine.ResolvedRuntimeExtension{}, fmt.Errorf("Feishu participant %q not found", ref)
	}
	if item.Type != participant.TypeAgent || agent.CanonicalID(item.AgentID) != agentID {
		return agentengine.ResolvedRuntimeExtension{}, fmt.Errorf("Feishu participant %q does not belong to agent %q", ref, agentID)
	}
	appID := configString(item.ChannelAppConfig, "app_id")
	appSecret := configString(item.ChannelAppConfig, participant.ChannelAppConfigAppSecretKey)
	if appID == "" || appSecret == "" || appSecret == participant.RedactedSecretValue {
		return agentengine.ResolvedRuntimeExtension{}, fmt.Errorf("Feishu bot credentials are incomplete for participant %q", ref)
	}
	credentialRevision := CredentialRevision(item.ID, appID, appSecret)
	token, err := s.options.AccessToken(agentID, item.ID, credentialRevision)
	if err != nil {
		return agentengine.ResolvedRuntimeExtension{}, err
	}
	helperPath, err := s.options.HelperPath()
	if err != nil {
		return agentengine.ResolvedRuntimeExtension{}, err
	}
	payload, err := larkextension.Encode(larkextension.Payload{
		AgentID:       agentID,
		ParticipantID: item.ID,
		AppID:         appID,
		BaseURL:       strings.TrimRight(strings.TrimSpace(s.options.BaseURL()), "/"),
		AccessToken:   token,
		HelperPath:    helperPath,
	})
	if err != nil {
		return agentengine.ResolvedRuntimeExtension{}, err
	}
	revisionBytes := sha256.Sum256(append([]byte(credentialRevision+"\x00"), payload...))
	return agentengine.ResolvedRuntimeExtension{
		SourceRevision: hex.EncodeToString(revisionBytes[:]),
		Payload:        payload,
	}, nil
}

// CredentialRevision binds a source capability to the current business fact.
func CredentialRevision(participantID, appID, appSecret string) string {
	sum := sha256.Sum256([]byte(participantID + "\x00" + appID + "\x00" + appSecret))
	return hex.EncodeToString(sum[:])
}

func configString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

var _ agentengine.RuntimeExtensionSource = (*Source)(nil)
