package runtimewiring

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/codexcli"
	"csgclaw/internal/codexmodel"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
	"fmt"
	"strings"
)

func WithCodexRuntime() agent.ControllerOption {
	return func(s *agent.Controller) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}

		if _, err := runtimecodex.DecodeRuntimeOptions(nil); err != nil {
			return err
		}

		host := s.RuntimeHost()
		events := runtimecodex.NewEventSink()
		permissions := runtimecodex.NewPermissionBroker(events)
		userInputs := runtimecodex.NewUserInputBroker(events)
		rt := runtimecodex.New(runtimecodex.Dependencies{
			ModelCatalog: func(agentID string) []codexmodel.Profile {
				profiles, err := s.Models().SessionModelProfiles(agentID)
				if err != nil {
					return nil
				}
				models := make([]codexmodel.Profile, 0, len(profiles))
				for _, profile := range profiles {
					models = append(models, codexmodel.Profile{Provider: profile.Provider, ModelID: profile.ModelID, ReasoningEffort: profile.ReasoningEffort, Metadata: profile.ModelMetadata})
				}
				return models
			},
			ResolveRoomProfile: s.TaskRuntimeProfile,
			BinaryProvider:     codexcli.Provider{},
			MaterializeMCPServers: func(ctx context.Context, servers map[string]any) (map[string]any, error) {
				return s.MaterializeRuntimeMCPServers(ctx, agent.RuntimeKindCodex, servers)
			},
			ResolveAgent: func(h agentruntime.Handle) (runtimecodex.AgentRef, error) {
				got, err := host.ResolveAgent(h)
				if err != nil {
					return runtimecodex.AgentRef{}, err
				}
				profile, err := host.ResolveRuntimeProfile(h)
				if err != nil {
					return runtimecodex.AgentRef{}, err
				}
				return runtimecodex.AgentRef{
					ID:             got.ID,
					Name:           got.Name,
					RuntimeID:      strings.TrimSpace(got.RuntimeID),
					HandleID:       strings.TrimSpace(got.BoxID),
					Instructions:   got.Instructions,
					RuntimeOptions: got.RuntimeOptions,
					MCPServers:     got.MCPServers,
					SkillStates:    got.SkillStates,
					Profile:        profile,
				}, nil
			},
			AgentHome:  host.AgentHome,
			EventSink:  events,
			Permission: permissions,
			UserInput:  userInputs,
		})
		return agent.WithRuntime(rt)(s)
	}
}
