package codex

import (
	"context"
	"strings"
	"time"
)

const memoryMaintenancePrompt = "Internal memory maintenance trigger. Do not perform user work or call tools. Reply exactly OK."

var (
	appServerMemoryCheckpointInterval = time.Hour
	appServerMemoryCheckpointIdleTime = time.Hour + time.Minute
	appServerMemoryCheckpointTimeout  = 5 * time.Second
	appServerMemoryMaintenanceTimeout = 15 * time.Minute
	appServerMemoryMaintenanceDedup   = 5 * time.Minute
	appServerMemoryCheckpointCleanup  = 30 * time.Minute
)

// maybeCreateMemoryCheckpoint snapshots a completed conversation without
// changing the thread used by the room. Codex never extracts memories from the
// currently active thread, so the fork becomes an idle, eligible input while
// the user keeps chatting in the original conversation.
func (m *appServerManager) maybeCreateMemoryCheckpoint(live *liveSession, sourceThreadID string) {
	if m == nil || live == nil || live.appClient == nil || !live.spec.MemoryEnabled {
		return
	}
	sourceThreadID = strings.TrimSpace(sourceThreadID)
	conversationKey := live.memoryConversationKey(sourceThreadID)
	if sourceThreadID == "" || conversationKey == "" {
		return
	}

	now := time.Now()
	live.memoryCheckpointMu.Lock()
	last := live.memoryCheckpointLast[conversationKey]
	if live.memoryCheckpointBusy[conversationKey] || (!last.IsZero() && now.Sub(last) < appServerMemoryCheckpointInterval) {
		live.memoryCheckpointMu.Unlock()
		return
	}
	live.memoryCheckpointBusy[conversationKey] = true
	live.memoryCheckpointMu.Unlock()

	// Keep the checkpoint RPC on the Prompt return path. The Engine and channel
	// ingress do not admit the next turn for this conversation until Prompt
	// returns, so the source thread cannot change while Codex is forking it.
	ctx, cancel := context.WithTimeout(context.Background(), appServerMemoryCheckpointTimeout)
	raw, err := live.appClient.request(ctx, "thread/fork", map[string]any{
		"threadId": sourceThreadID,
	})
	cancel()
	checkpointThreadID := ""
	if err == nil {
		checkpointThreadID, err = appServerThreadIDFromResult(raw)
	}

	live.memoryCheckpointMu.Lock()
	delete(live.memoryCheckpointBusy, conversationKey)
	// Throttle failed attempts too. If the local App Server cannot fork, do not
	// make every queued user message pay the same timeout.
	live.memoryCheckpointLast[conversationKey] = now
	live.memoryCheckpointMu.Unlock()
	if err != nil {
		live.appClient.logDebug("create Codex memory checkpoint failed",
			"thread_id", sourceThreadID,
			"conversation_key", conversationKey,
			"error", err,
		)
		return
	}

	live.appClient.logDebug("created Codex memory checkpoint",
		"thread_id", sourceThreadID,
		"checkpoint_thread_id", checkpointThreadID,
		"conversation_key", conversationKey,
	)
	go m.waitForMemoryCheckpoint(live, checkpointThreadID)
}

func (m *appServerManager) waitForMemoryCheckpoint(live *liveSession, checkpointThreadID string) {
	timer := time.NewTimer(appServerMemoryCheckpointIdleTime)
	defer timer.Stop()
	select {
	case <-live.done:
		return
	case <-timer.C:
		m.runMemoryMaintenance(live, checkpointThreadID)
	}
}

func (m *appServerManager) runMemoryMaintenance(live *liveSession, checkpointThreadID string) {
	if m == nil || live == nil || live.appClient == nil || !live.spec.MemoryEnabled {
		return
	}
	live.memoryMaintenanceMu.Lock()
	defer live.memoryMaintenanceMu.Unlock()

	now := time.Now()
	live.memoryCheckpointMu.Lock()
	if !live.memoryLastMaintenance.IsZero() && now.Sub(live.memoryLastMaintenance) < appServerMemoryMaintenanceDedup {
		live.memoryCheckpointMu.Unlock()
		go m.cleanupMemoryCheckpoint(live, checkpointThreadID)
		return
	}
	maintenanceThreadID := ""
	if live.session != nil {
		maintenanceThreadID = strings.TrimSpace(live.session.SessionID)
	}
	if maintenanceThreadID == "" {
		live.memoryCheckpointMu.Unlock()
		return
	}
	live.memoryLastMaintenance = now
	live.memoryMaintenanceID = maintenanceThreadID
	live.memoryCheckpointMu.Unlock()
	defer func() {
		live.memoryCheckpointMu.Lock()
		live.memoryMaintenanceID = ""
		live.memoryCheckpointMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), appServerMemoryMaintenanceTimeout)
	_, err := m.Prompt(ctx, SessionHandle{RuntimeID: live.spec.RuntimeID}, PromptRequest{
		SessionID: maintenanceThreadID,
		Prompt:    []PromptContentBlock{TextBlock(memoryMaintenancePrompt)},
	})
	cancel()
	if err != nil {
		live.memoryCheckpointMu.Lock()
		live.memoryLastMaintenance = time.Time{}
		live.memoryCheckpointMu.Unlock()
		live.appClient.logDebug("Codex memory maintenance trigger failed",
			"checkpoint_thread_id", checkpointThreadID,
			"error", err,
		)
		return
	}
	live.appClient.logDebug("Codex memory maintenance triggered",
		"checkpoint_thread_id", checkpointThreadID,
		"maintenance_thread_id", maintenanceThreadID,
	)
	go m.cleanupMemoryCheckpoint(live, checkpointThreadID)
}

func (m *appServerManager) cleanupMemoryCheckpoint(live *liveSession, checkpointThreadID string) {
	checkpointThreadID = strings.TrimSpace(checkpointThreadID)
	if m == nil || live == nil || live.appClient == nil || checkpointThreadID == "" {
		return
	}
	timer := time.NewTimer(appServerMemoryCheckpointCleanup)
	defer timer.Stop()
	select {
	case <-live.done:
		return
	case <-timer.C:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, err := live.appClient.request(ctx, "thread/delete", map[string]any{"threadId": checkpointThreadID})
	cancel()
	if err != nil {
		live.appClient.logDebug("delete Codex memory checkpoint failed", "checkpoint_thread_id", checkpointThreadID, "error", err)
		return
	}
	live.appClient.logDebug("deleted Codex memory checkpoint", "checkpoint_thread_id", checkpointThreadID)
}

func (s *liveSession) memoryConversationKey(threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if s == nil || threadID == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, candidate := range s.conversationSessions {
		if strings.TrimSpace(candidate) == threadID {
			return key
		}
	}
	return ""
}

func (s *liveSession) isMemoryMaintenanceThread(threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if s == nil || threadID == "" {
		return false
	}
	s.memoryCheckpointMu.Lock()
	defer s.memoryCheckpointMu.Unlock()
	return strings.TrimSpace(s.memoryMaintenanceID) == threadID
}
