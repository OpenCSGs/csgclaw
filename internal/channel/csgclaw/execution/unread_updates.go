package execution

import (
	"context"
	"fmt"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
)

type unreadUpdateReader func(roomID, participantID, threadRootID string, after uint64) (apitypes.UnreadNotification, bool)

func WithUnreadUpdates(reader unreadUpdateReader, taskThread func(roomID, sourceID string) string) Option {
	return func(a *Adapter) {
		a.unreadUpdates = reader
		a.taskThread = taskThread
	}
}

// WithUnreadSignal checks the current scope and the original request's task
// topic. The queued notification remains available for normal admission.
func (a *Adapter) WithUnreadSignal(ctx context.Context, binding channel.Binding, event channel.Event, ready <-chan struct{}) context.Context {
	if !event.NotificationOnly || ready == nil || a.unreadUpdates == nil {
		return ctx
	}
	return contract.WithTurnInputUpdates(ctx, contract.TurnInputUpdates{
		Ready: ready,
		Latest: func(after uint64) (uint64, string) {
			through := after
			input := ""
			if notice, ok := a.unreadUpdates(event.RoomID, binding.ParticipantID, event.ThreadRootID, after); ok {
				through = notice.Through
				input = fmt.Sprintf("[CSGClaw unread notice] %d unread in your current scope.\n", notice.Count)
			}
			if event.ThreadRootID == "" && a.taskThread != nil {
				if root := a.taskThread(event.RoomID, event.MessageID); root != "" {
					if notice, ok := a.unreadUpdates(event.RoomID, binding.ParticipantID, root, after); ok {
						through = max(through, notice.Through)
						input += fmt.Sprintf("[CSGClaw unread notice] %d unread in the original request's topic (thread_root_id=%q).\n", notice.Count, root)
					}
				}
			}
			if input != "" {
				input += "Finish your current step before checking these scopes with messages_read. Acknowledge pages with ack_token in the same scope. Continue useful work after reading, or finish with only <csgclaw-silent/> if you are waiting on a teammate. Do not keep checking for more messages."
			}
			return through, input
		},
	})
}
