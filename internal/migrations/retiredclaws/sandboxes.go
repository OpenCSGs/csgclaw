package retiredclaws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/sandbox"
)

// Stop every persisted sandbox handle before reading its workspace or replacing
// its Agent record. Persisted lifecycle state is not proof of provider state.
func stopRetiredSandboxes(ctx context.Context, agentID string, agent, runtime map[string]json.RawMessage, openSandbox func(context.Context, string) (sandbox.Runtime, error)) error {
	var ids []string
	seen := make(map[string]bool)
	for _, record := range []map[string]json.RawMessage{runtime, agent} {
		for _, key := range []string{"sandbox_id", "box_id"} {
			id := strings.TrimSpace(migrationString(record[key]))
			if id != "" && !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	if len(ids) == 0 {
		for _, raw := range []json.RawMessage{runtime["state"], agent["status"], agent["desired_state"]} {
			switch strings.ToLower(strings.TrimSpace(migrationString(raw))) {
			case "starting", "running", "paused", "restarting", "removing":
				return fmt.Errorf("retired Agent %q is marked active without a sandbox identifier; stop its previous sandbox and restore its identifier before retrying the upgrade", agentID)
			}
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if openSandbox == nil {
		return fmt.Errorf("cannot confirm retired sandboxes for Agent %q: sandbox provider is required; original record retained", agentID)
	}
	rt, err := openSandbox(ctx, agentID)
	if err != nil {
		return fmt.Errorf("open retired sandbox for Agent %q: %w; original record retained", agentID, err)
	}
	defer rt.Close()
	for _, id := range ids {
		if err := stopRetiredSandbox(ctx, rt, id); err != nil {
			return fmt.Errorf("stop retired sandbox %q for Agent %q: %w; original record retained, retry the upgrade after restoring sandbox access", id, agentID, err)
		}
	}
	return nil
}

func stopRetiredSandbox(ctx context.Context, rt sandbox.Runtime, id string) error {
	instance, err := rt.Get(ctx, id)
	if sandbox.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer instance.Close()
	info, err := instance.Info(ctx)
	if sandbox.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if retiredSandboxStopped(info.State) {
		return nil
	}
	// Docker stop waits for exit; other providers may report a pending transition.
	if err := instance.Stop(ctx, sandbox.StopOptions{}); err != nil && !sandbox.IsNotFound(err) {
		return err
	}
	for {
		info, err = instance.Info(ctx)
		if sandbox.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if retiredSandboxStopped(info.State) {
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("sandbox did not stop (state %q): %w", info.State, ctx.Err())
		case <-timer.C:
		}
	}
}

func retiredSandboxStopped(state sandbox.State) bool {
	// StateCreated denotes an inactive instance, unlike StateStarting, whose
	// deployment can still become running without an explicit Start request.
	switch state {
	case sandbox.StateCreated, sandbox.StateStopped, sandbox.StateExited, sandbox.StateDead:
		return true
	default:
		return false
	}
}
