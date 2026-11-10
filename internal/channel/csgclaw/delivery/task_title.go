package delivery

import (
	"context"
	"log/slog"
	"strings"
	"time"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
	"csgclaw/internal/taskconfig"
)

type TaskTitleProfiles interface {
	ResolvedAgentProfile(string) (agent.AgentProfile, error)
	ResolvedTaskProfile(string, taskconfig.Config) (agent.AgentProfile, error)
}

type TaskTitleModel interface {
	GenerateTaskTitle(context.Context, string, agent.AgentProfile, string, string) (string, error)
}

// NewTaskTitleCallback schedules title generation for a delivered local task reply.
func NewTaskTitleCallback(ctx context.Context, rooms *im.Service, models TaskTitleProfiles, model TaskTitleModel) func(channel.TurnContext, string) {
	return func(turn channel.TurnContext, answer string) {
		if ctx.Err() != nil || turn.ChatType != "direct" || turn.TaskID != "" || turn.ThreadRootID != "" || strings.TrimSpace(answer) == "" {
			return
		}
		question, cfg, claimed := rooms.ClaimTaskTitle(turn.RoomID, turn.SourceMessageID)
		if !claimed {
			return
		}
		go func() {
			defer rooms.ReleaseTaskTitle(turn.RoomID, turn.SourceMessageID)
			var profile agent.AgentProfile
			var err error
			if cfg == nil {
				profile, err = models.ResolvedAgentProfile(turn.AgentID)
			} else {
				profile, err = models.ResolvedTaskProfile(turn.AgentID, *cfg)
			}
			if err != nil {
				slog.Debug("task title model unavailable", "room_id", turn.RoomID)
				return
			}
			for range 2 {
				if ctx.Err() != nil || !rooms.TaskTitlePending(turn.RoomID, turn.SourceMessageID) {
					return
				}
				titleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				title, err := model.GenerateTaskTitle(titleCtx, turn.AgentID, profile, question, answer)
				completed := err == nil && titleCtx.Err() == nil
				cancel()
				if !completed {
					continue
				}
				if err := rooms.ApplyTaskTitle(turn.RoomID, turn.SourceMessageID, title); err != nil {
					slog.Warn("save task title failed", "room_id", turn.RoomID)
				}
				return
			}
			slog.Debug("task title generation failed", "room_id", turn.RoomID)
		}()
	}
}
