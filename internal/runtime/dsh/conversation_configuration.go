package dsh

import (
	"context"
	"maps"

	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/conversationconfig"
)

func (r *Runtime) conversationState(runtimeID string) *conversationconfig.State {
	state, _ := r.conversationSettings.LoadOrStore(runtimeID, &conversationconfig.State{})
	return state.(*conversationconfig.State)
}

func (r *Runtime) resolveAgent(h agentruntime.Handle) (AgentRef, error) {
	ref, err := r.deps.ResolveAgent(h)
	if err != nil {
		return ref, err
	}
	if value, ok := r.conversationSettings.Load(h.RuntimeID); ok {
		if settings, applied := value.(*conversationconfig.State).Current(); applied {
			ref.Profile, ref.RuntimeOptions = settings.Profile, maps.Clone(settings.Options)
		}
	}
	return ref, nil
}

func (r *Runtime) configuredConversation(runtimeID string) contract.RuntimeConversation {
	base := &conversation{runtime: r, runtimeID: runtimeID}
	if r.deps.ResolveRoomProfile == nil {
		return base
	}
	state := r.conversationState(runtimeID)
	return &conversationconfig.Adapter{RuntimeConversation: base, Coordinator: &state.Coordinator, Prepare: func(ctx context.Context, request contract.TurnRequest) error {
		r.mcpRefreshMu.Lock()
		defer r.mcpRefreshMu.Unlock()
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
				options["local_workspace_dir"] = room.Configuration.WorkspaceDir
			}
			if room.Configuration.ExecutionMode != "" {
				options["permission_mode"] = room.Configuration.ExecutionMode
			}
		}
		desired := conversationconfig.Settings{Profile: ref.Profile.Normalized(), Options: options}
		_, applied := state.Current()
		if !state.Matches(desired) {
			if applied || request.RoomConfiguration != nil {
				if _, err = r.Stop(ctx, h); err != nil {
					return err
				}
			}
			state.Set(desired)
		}
		_, err = r.Start(ctx, h)
		return err
	}}
}
