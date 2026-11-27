package participant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/identity"
	"csgclaw/internal/im"
)

type bindingNameChange struct {
	before, after apitypes.Participant
	syncUser      bool
}

// UpdateNamedAgent coordinates a name-bearing Agent PATCH with its local
// bindings. The engine still validates and applies the complete update.
func (s *Service) UpdateNamedAgent(ctx context.Context, agents agentengine.AgentInterface, current agentengine.Agent, update agentengine.AgentUpdateRequest) (agentengine.Agent, error) {
	ctx, release, err := s.lifecycle.Mutation(ctx, current.ID)
	if err != nil {
		return agentengine.Agent{}, err
	}
	defer release()
	name := strings.TrimSpace(update.Spec.Name)
	if err := identity.ValidateMentionName(name); err != nil {
		return agentengine.Agent{}, err
	}
	var updated agentengine.Agent
	err = s.withWorkerNames(ctx, current.ID, current.Spec.Name, name, nil, func() (bool, error) {
		var updateErr error
		updated, updateErr = updateWorkerName(ctx, agents, current, update)
		return updated.ID != "", updateErr
	})
	return updated, err
}

// A mixed PATCH can persist the name before a later runtime operation fails.
// Return the observed worker alongside that error so its bindings follow it.
func updateWorkerName(ctx context.Context, agents agentengine.AgentInterface, worker agentengine.Agent, update agentengine.AgentUpdateRequest) (agentengine.Agent, error) {
	updated, err := agents.Update(ctx, worker.ID, update)
	if err == nil {
		return updated, nil
	}
	name := strings.TrimSpace(update.Spec.Name)
	if name != worker.Spec.Name {
		actual, getErr := agents.Get(context.WithoutCancel(ctx), worker.ID, agentengine.AgentGetOptions{})
		if getErr == nil && actual.ID == worker.ID && actual.Spec.Name == name {
			return actual, err
		}
	}
	return agentengine.Agent{}, err
}

// The caller owns the worker mutation lease and, on a create retry, the
// initiating participant lock. Each sibling keeps its independent display name.
func (s *Service) withWorkerNames(ctx context.Context, agentID, previousName, name string, initiating *bindingNameChange, apply func() (bool, error)) error {
	var changes []bindingNameChange
	if initiating != nil {
		changes = append(changes, *initiating)
	}
	var nameOwner string
	var followsLocalUser bool
	for _, ref := range s.store.List(ListOptions{Channel: ChannelCSGClaw, AgentID: agentID, Type: TypeAgent}) {
		if initiating != nil && ref.ID == initiating.before.ID {
			continue
		}
		release, err := s.lockParticipant(ctx, ref.Channel, ref.ID)
		if err != nil {
			return err
		}
		defer release()
		item, ok := s.store.Get(ref.Channel, ref.ID)
		if !ok || item.AgentID != agentID {
			continue
		}
		next := item
		next.AgentName = name
		follows := initiating == nil && (item.Name == item.AgentName || item.Name == previousName)
		if follows && next.Name != name {
			next.Name = name
			next.UpdatedAt = time.Now().UTC()
		}
		if next.ChannelUserKind == ChannelUserKindLocalUserID && strings.EqualFold(next.Name, name) {
			nameOwner = next.ChannelUserRef
			followsLocalUser = followsLocalUser || follows
		}
		changes = append(changes, bindingNameChange{before: item, after: next, syncUser: follows})
	}
	if initiating == nil && !followsLocalUser && s.im != nil {
		if len(changes) == 0 {
			// Standalone agents use their legacy local user ID on publication.
			nameOwner = agentID
		}
		release, err := s.im.ReserveAgentUserName(nameOwner, name)
		if err != nil {
			return err
		}
		defer release()
	}
	return s.applyBindingNames(changes, apply)
}

// apply reports whether the worker committed its name, including partial updates.
func (s *Service) applyBindingNames(changes []bindingNameChange, apply func() (bool, error)) (err error) {
	var before, after []apitypes.Participant
	var committed bool
	for _, change := range changes {
		if change.syncUser && s.im != nil && change.after.ChannelUserKind == ChannelUserKindLocalUserID {
			finish, stageErr := s.stageUserName(change.after)
			if stageErr != nil {
				return stageErr
			}
			if finish != nil {
				defer func() {
					if rollbackErr := finish(committed); rollbackErr != nil {
						err = errors.Join(err, rollbackErr)
					}
				}()
			}
		}
		if change.before.Name != change.after.Name || change.before.AgentName != change.after.AgentName || change.before.AgentID != change.after.AgentID {
			before = append(before, change.before)
			after = append(after, change.after)
		}
	}
	if err := s.store.saveAll(after); err != nil {
		return err
	}
	committed, err = apply()
	if !committed {
		err = errors.Join(err, s.store.saveAll(before))
	}
	return err
}

// Claim the new name before updating the worker, and keep the old name
// reserved until the worker accepts or rejects the change.
func (s *Service) stageUserName(item apitypes.Participant) (func(bool) error, error) {
	user, ok := s.im.LocalUserByID(item.ChannelUserRef)
	if !ok {
		return nil, im.ErrUserNotFound
	}
	if user.AgentState == "deleted" {
		return nil, im.ErrAgentIdentityDeleted
	}
	if owner, ok := s.im.User(item.Name); ok && owner.ID != user.ID {
		return nil, fmt.Errorf("name %q %w", item.Name, identity.ErrNameExists)
	}
	if user.Name == item.Name {
		return nil, nil
	}
	release, err := s.im.HoldAgentUserName(user.ID, user.Name)
	if err != nil {
		return nil, err
	}
	if err := s.syncParticipantChannelUser(item); err != nil {
		release()
		return nil, err
	}
	return func(commit bool) error {
		defer release()
		if commit {
			return nil
		}
		item.Name = user.Name
		return s.syncParticipantChannelUser(item)
	}, nil
}
