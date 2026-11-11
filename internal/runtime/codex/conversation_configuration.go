package codex

import (
	"context"
	"errors"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/diagnostics"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/conversationconfig"
	"csgclaw/internal/sandbox"
	skill "csgclaw/internal/skill/state"
	"golang.org/x/sync/semaphore"
)

const (
	conversationReadOnlyOption = "_conversation_read_only"
	conversationProfileOption  = "_conversation_profile"
)

func (r *Runtime) persistedRoomConversations(meta sessionMetadata, profile agentruntime.Profile) (map[string]string, map[string]bool) {
	if r.deps.ResolveRoomProfile != nil {
		return cloneConversationSessions(meta.ConversationSessions), cloneFilePublishingConversations(meta.FilePublishingConversations)
	}
	return persistedConversationsForProfile(meta, profile)
}

// Shared turns pin the process settings while they run. A transition takes all
// permits before stopping the process, so it cannot interrupt another thread.
const conversationPermits = 1 << 30

type codexConversationState struct {
	conversationconfig.State
	prepare *semaphore.Weighted
	turns   *semaphore.Weighted
}

func (r *Runtime) conversationState(runtimeID string) *codexConversationState {
	if state, ok := r.conversationSettings.Load(runtimeID); ok {
		return state.(*codexConversationState)
	}
	state, _ := r.conversationSettings.LoadOrStore(runtimeID, &codexConversationState{
		prepare: semaphore.NewWeighted(1), turns: semaphore.NewWeighted(conversationPermits),
	})
	return state.(*codexConversationState)
}

type configuredConversation struct {
	contract.RuntimeConversation
	runtime   *Runtime
	runtimeID string
}

func (r *Runtime) configuredConversation(runtimeID string) contract.RuntimeConversation {
	base := NewConversationAdapter(runtimeID, r)
	if r.deps.ResolveRoomProfile == nil {
		return base
	}
	return &configuredConversation{RuntimeConversation: base, runtime: r, runtimeID: runtimeID}
}

func (c *configuredConversation) Run(ctx context.Context, request contract.TurnRequest, sink contract.EventSink) contract.TurnResult {
	r := c.runtime
	h := agentruntime.Handle{RuntimeID: c.runtimeID}
	state := r.conversationState(c.runtimeID)
	desired, scoped, err := r.conversationRunSettings(h, request.RoomConfiguration)
	if err != nil {
		return resultFromContext(ctx, err)
	}
	finishPrepare := diagnostics.Measure(ctx, "runtime.conversation.prepare", "runtime")
	release, err := r.prepareConversation(ctx, h, state, desired, scoped != nil || request.RoomConfiguration == nil)
	finishPrepare()
	if err != nil {
		return resultFromContext(ctx, err)
	}
	defer release()
	if scoped != nil {
		if err := r.mkdirAll(scoped.WorkspaceDir, 0o755); err != nil {
			return resultFromContext(ctx, err)
		}
		ctx = context.WithValue(ctx, roomThreadSettingsKey{}, *scoped)
	}
	return c.RuntimeConversation.Run(ctx, request, sink)
}

func (c *configuredConversation) Reset(ctx context.Context, key contract.ConversationKey) *contract.TurnError {
	state := c.runtime.conversationState(c.runtimeID)
	if err := state.turns.Acquire(ctx, conversationPermits); err != nil {
		return resultFromContext(ctx, err).Error
	}
	defer state.turns.Release(conversationPermits)
	return c.RuntimeConversation.Reset(ctx, key)
}

func (r *Runtime) prepareConversation(ctx context.Context, h agentruntime.Handle, state *codexConversationState, desired conversationconfig.Settings, shared bool) (func(), error) {
	if err := state.prepare.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer state.prepare.Release(1)
	if shared && state.Matches(desired) {
		if manager, ok := r.currentSessionManager().(*appServerManager); ok {
			if live := manager.liveSession(h.RuntimeID); live != nil && live.session != nil && processAlive(live.session.ProcessID) {
				if err := state.turns.Acquire(ctx, 1); err != nil {
					return nil, err
				}
				return func() { state.turns.Release(1) }, nil
			}
		}
	}
	if err := state.turns.Acquire(ctx, conversationPermits); err != nil {
		return nil, err
	}
	if !state.Matches(desired) {
		_, applied := state.Current()
		if applied || !shared {
			finishStop := diagnostics.Measure(ctx, "runtime.conversation.stop", "runtime")
			_, err := r.Stop(ctx, h)
			finishStop()
			if err != nil && !errors.Is(err, sandbox.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
				state.turns.Release(conversationPermits)
				return nil, err
			}
		}
		state.Set(desired)
	}
	finishStart := diagnostics.Measure(ctx, "runtime.conversation.start", "runtime")
	_, err := r.Start(ctx, h)
	finishStart()
	if err != nil {
		state.turns.Release(conversationPermits)
		return nil, err
	}
	if shared {
		state.turns.Release(conversationPermits - 1)
		return func() { state.turns.Release(1) }, nil
	}
	return func() { state.turns.Release(conversationPermits) }, nil
}

type roomThreadSettingsKey struct{}

type roomThreadSettings struct {
	Profile      agentruntime.Profile
	WorkspaceDir string
}

func (r *Runtime) conversationRunSettings(h agentruntime.Handle, room *contract.RoomConfiguration) (conversationconfig.Settings, *roomThreadSettings, error) {
	ref, err := r.deps.ResolveAgent(h)
	if err != nil {
		return conversationconfig.Settings{}, nil, err
	}
	base := conversationconfig.Settings{Profile: ref.Profile.Normalized(), Options: maps.Clone(ref.RuntimeOptions)}
	if room == nil {
		return base, nil, nil
	}
	profile, err := r.deps.ResolveRoomProfile(ref.ID, room.RoomID, room.Configuration)
	if err != nil {
		return conversationconfig.Settings{}, nil, err
	}
	profile = profile.Normalized()
	opts := maps.Clone(ref.RuntimeOptions)
	if opts == nil {
		opts = make(map[string]any)
	}
	if room.Configuration.WorkspaceDir != "" {
		opts[localWorkspaceDirOptionKey] = room.Configuration.WorkspaceDir
	}
	opts[conversationReadOnlyOption] = room.Configuration.ExecutionMode == ExecutionModeReadOnly
	opts[conversationProfileOption] = true
	runtimeOpts, err := conversationRuntimeOptions(opts)
	if err != nil {
		return conversationconfig.Settings{}, nil, err
	}
	// Room routes are stable for a given Agent and room. Keep changes that
	// require a different process environment or native model catalog on the
	// existing exclusive path.
	if runtimeOpts.ExecutionMode == ExecutionModeStandard &&
		base.Profile.BaseURL != "" && room.RoomID != "" &&
		profile.BaseURL == strings.TrimSuffix(base.Profile.BaseURL, "/llm")+"/tasks/"+url.PathEscape(room.RoomID)+"/llm" &&
		profile.APIKey == base.Profile.APIKey && maps.Equal(profile.Env, base.Profile.Env) {
		for _, model := range r.sessionModelCatalog(ref.ID, base.Profile) {
			if model.ModelID != profile.ModelID || model.Provider != profile.Provider ||
				model.Metadata.Normalized().ContextWindow != profile.ModelMetadata.Normalized().ContextWindow ||
				model.Metadata.CompactThreshold() != profile.ModelMetadata.CompactThreshold() {
				continue
			}
			workspace, err := r.resolveWorkspaceDir(ref.ID, opts)
			if err != nil {
				return conversationconfig.Settings{}, nil, err
			}
			return base, &roomThreadSettings{Profile: profile, WorkspaceDir: workspace}, nil
		}
	}
	return conversationconfig.Settings{Profile: profile, Options: opts}, nil, nil
}

func roomThreadSpec(ctx context.Context, spec SessionSpec) SessionSpec {
	if room, ok := ctx.Value(roomThreadSettingsKey{}).(roomThreadSettings); ok {
		spec.WorkspaceDir = room.WorkspaceDir
		spec.Profile = room.Profile
	}
	return spec
}

func roomThreadParams(ctx context.Context, params map[string]any) map[string]any {
	room, ok := ctx.Value(roomThreadSettingsKey{}).(roomThreadSettings)
	if !ok {
		return params
	}
	params["modelProvider"] = codexProxyProviderName
	config, _ := params["config"].(map[string]any)
	if config == nil {
		config = make(map[string]any)
		params["config"] = config
	}
	config["model_providers."+codexProxyProviderName+".base_url"] = room.Profile.BaseURL
	return params
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
