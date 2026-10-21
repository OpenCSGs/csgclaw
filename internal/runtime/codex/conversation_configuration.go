package codex

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"

	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/conversationconfig"
	"csgclaw/internal/sandbox"
	skill "csgclaw/internal/skill/state"
)

const conversationReadOnlyOption = "_conversation_read_only"

func (r *Runtime) persistedRoomConversations(meta sessionMetadata, profile agentruntime.Profile) (map[string]string, map[string]bool) {
	if r.deps.ResolveRoomProfile != nil {
		return cloneConversationSessions(meta.ConversationSessions), cloneFilePublishingConversations(meta.FilePublishingConversations)
	}
	return persistedConversationsForProfile(meta, profile)
}

func (r *Runtime) conversationState(runtimeID string) *conversationconfig.State {
	state, _ := r.conversationSettings.LoadOrStore(runtimeID, &conversationconfig.State{})
	return state.(*conversationconfig.State)
}

func (r *Runtime) configuredConversation(runtimeID string) contract.RuntimeConversation {
	base := NewConversationAdapter(runtimeID, r)
	if r.deps.ResolveRoomProfile == nil {
		return base
	}
	state := r.conversationState(runtimeID)
	return &conversationconfig.Adapter{RuntimeConversation: base, Coordinator: &state.Coordinator, Prepare: func(ctx context.Context, request contract.TurnRequest) error {
		h := agentruntime.Handle{RuntimeID: runtimeID}
		ref, err := r.deps.ResolveAgent(h)
		if err != nil {
			return err
		}
		options := maps.Clone(ref.RuntimeOptions)
		if options == nil {
			options = map[string]any{}
		}
		if room := request.RoomConfiguration; room != nil {
			ref.Profile, err = r.deps.ResolveRoomProfile(ref.ID, room.RoomID, room.Configuration)
			if err != nil {
				return err
			}
			if room.Configuration.WorkspaceDir != "" {
				options[localWorkspaceDirOptionKey] = room.Configuration.WorkspaceDir
			}
			options[conversationReadOnlyOption] = room.Configuration.ExecutionMode == ExecutionModeReadOnly
		}
		desired := conversationconfig.Settings{Profile: ref.Profile.Normalized(), Options: options}
		_, applied := state.Current()
		if !state.Matches(desired) {
			if applied || request.RoomConfiguration != nil {
				if _, err = r.Stop(ctx, h); err != nil && !errors.Is(err, sandbox.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			state.Set(desired)
		}
		_, err = r.Start(ctx, h)
		return err
	}}
}

func conversationRuntimeOptions(raw map[string]any) (RuntimeOptions, error) {
	opts, err := DecodeRuntimeOptions(raw)
	if raw[conversationReadOnlyOption] == true {
		opts.ExecutionMode = ExecutionModeReadOnly
	}
	return opts, err
}

func (r *Runtime) conversationSkillStates(home, mode string, states map[string]skill.State) (map[string]skill.State, error) {
	if r.deps.ResolveRoomProfile == nil || mode != ExecutionModeReadOnly {
		return states, nil
	}
	states = maps.Clone(states)
	if states == nil {
		states = map[string]skill.State{}
	}
	entries, err := os.ReadDir(filepath.Join(home, "skills"))
	if os.IsNotExist(err) {
		return states, nil
	}
	if err != nil {
		return nil, err
	}
	// Disable discovery without deleting Agent-owned skills when a room is read-only.
	for _, entry := range entries {
		if entry.Name() != ".system" {
			states[entry.Name()] = skill.State{Enabled: false}
		}
	}
	return states, nil
}

func (r *Runtime) seedAgentWorkspaceSkills(spec SessionSpec) error {
	if spec.PreserveSkills {
		return nil
	}
	ref, err := r.deps.ResolveAgent(agentruntime.Handle{RuntimeID: spec.RuntimeID})
	if err != nil {
		return err
	}
	workspace, err := r.resolveWorkspaceDir(spec.AgentID, ref.RuntimeOptions)
	if err != nil {
		return err
	}
	return r.seedCodexHomeWorkspaceSkills(spec.AgentID, workspace, spec.CodexHomeDir)
}
