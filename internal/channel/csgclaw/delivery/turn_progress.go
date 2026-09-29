package delivery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
)

type turnProgressStore interface {
	DeliverTurnProgress(context.Context, channel.TurnContext, activity.TurnProgress, string, bool, []agentengine.OutputFile) error
}

// progressState serializes timer flushes with terminal delivery. A completed
// snapshot can never be overwritten by a delayed streaming update.
type progressState struct {
	lastFlush       time.Time
	pendingPersist  bool
	mu              sync.Mutex
	snapshot        activity.TurnProgress
	answer          string
	answerPhase     string
	answerID        string
	lastTextID      string
	lastTextKind    string
	tools           map[string]int
	lastProvisional string
	timer           *time.Timer
	done            bool
	lastErr         error
	store           turnProgressStore
	turn            channel.TurnContext
}

func newProgressState(store turnProgressStore, turn channel.TurnContext) *progressState {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return &progressState{store: store, turn: turn, tools: map[string]int{}, snapshot: activity.TurnProgress{ID: string(turn.TurnID), Status: "running", StartedAt: now, UpdatedAt: now, Items: []activity.ProgressItem{}}}
}

func (r *TranscriptRenderer) Start(ctx context.Context, turn channel.TurnContext) error {
	store, ok := r.store.(turnProgressStore)
	if !ok {
		return nil
	}
	r.mu.Lock()
	if r.turns == nil {
		r.turns = make(map[turnBufferKey]*turnRenderState)
	}
	key := bufferKey(turn)
	if _, done := r.completed[key]; done {
		r.mu.Unlock()
		return nil
	}
	state := r.turns[key]
	if state == nil {
		state = newTurnRenderState(turn.Locale)
		r.turns[key] = state
	}
	if state.progress != nil {
		r.mu.Unlock()
		return nil
	}
	state.progress = newProgressState(store, turn)
	p := state.progress
	r.mu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.flush(ctx, true, nil)
}

func (p *progressState) observe(ctx context.Context, event agentengine.TurnEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return nil
	}
	if p.lastErr != nil {
		return p.lastErr
	}
	immediate := false
	switch event.Kind {
	case agentengine.TurnEventTextDelta:
		if event.TextSnapshot {
			if event.ItemID == p.answerID && event.Phase != "commentary" {
				p.answer = event.Text
				p.answerPhase = event.Phase
			} else {
				for i := len(p.snapshot.Items) - 1; i >= 0; i-- {
					if p.snapshot.Items[i].Kind == "commentary" && strings.HasSuffix(p.snapshot.Items[i].ID, ":"+event.ItemID) {
						p.snapshot.Items[i].Text = event.Text
						break
					}
				}
			}
			return p.flush(ctx, true, nil)
		}
		if event.Text == "" {
			if event.ItemID == p.answerID && event.Phase != "" {
				p.answerPhase = event.Phase
				return p.flush(ctx, true, nil)
			}
			return nil
		}
		p.snapshot.Status = "running"
		kind := "commentary"
		if event.Phase == "commentary" {
			p.appendText(event.ItemID, kind, event.Text)
		} else {
			if event.ItemID != "" && p.answerID != "" && event.ItemID != p.answerID && p.answer != "" && p.answerPhase != "final_answer" {
				p.appendText(p.answerID, "commentary", p.answer)
				p.answer = ""
				immediate = true
			}
			immediate = immediate || p.answer == ""
			p.answer += event.Text
			p.answerPhase = event.Phase
			p.answerID = event.ItemID
			p.lastTextKind = ""
			p.lastTextID = ""
		}
	case agentengine.TurnEventThoughtDelta:
		if event.Thought == "" {
			return nil
		}
		p.appendText(event.ItemID, "reasoning", event.Thought)
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		if event.Tool == nil {
			return nil
		}
		p.snapshot.Status = "running"
		if event.Kind == agentengine.TurnEventToolCallStart && p.answer != "" && p.answerPhase != "final_answer" {
			p.appendText(p.answerID, "commentary", p.answer)
			p.lastProvisional = p.snapshot.Items[len(p.snapshot.Items)-1].ID
			p.answer = ""
			p.answerID = ""
		}
		p.lastTextKind = ""
		p.lastTextID = ""
		id := event.Tool.ID
		if id == "" {
			id = fmt.Sprintf("tool-%d", event.Sequence)
		}
		index, exists := p.tools[id]
		if !exists {
			index = len(p.snapshot.Items)
			p.tools[id] = index
			p.snapshot.Items = append(p.snapshot.Items, activity.ProgressItem{ID: id, Kind: "tool"})
		}
		p.snapshot.Items[index].Tool = progressTool(*event.Tool, p.snapshot.Items[index].Tool)
		immediate = !exists || terminalTool(event.Tool.Status)
	case agentengine.TurnEventInteractionRequest:
		if event.Interaction != nil && !event.Interaction.Detached {
			p.snapshot.Status = "waiting"
			immediate = true
		}
	case agentengine.TurnEventActivityUpdate:
		if event.Activity != nil && (event.Activity.Kind == string(activity.RuntimeEventActionDecision) || event.Activity.Kind == string(activity.RuntimeEventUserInputResolved)) {
			p.snapshot.Status = "running"
			immediate = true
		} else {
			return nil
		}
	default:
		return nil
	}
	persist := immediate
	if event.Kind == agentengine.TurnEventToolCallStart || event.Kind == agentengine.TurnEventToolCallUpdate {
		immediate = immediate && time.Since(p.lastFlush) >= 50*time.Millisecond
	}
	p.pendingPersist = p.pendingPersist || persist
	if immediate || p.snapshot.Revision == 0 {
		return p.flush(ctx, p.pendingPersist, nil)
	}
	if p.timer == nil {
		p.timer = time.AfterFunc(50*time.Millisecond, func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.timer = nil
			if !p.done {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				p.lastErr = p.flush(ctx, p.pendingPersist, nil)
			}
		})
	}
	return nil
}

func (p *progressState) appendText(id, kind, text string) {
	originalID := id
	items := p.snapshot.Items
	if len(items) > 0 && p.lastTextKind == kind && p.lastTextID == id && items[len(items)-1].Kind == kind {
		p.snapshot.Items[len(items)-1].Text += text
	} else {
		id = fmt.Sprintf("text-%d:%s", len(items), id)
		p.snapshot.Items = append(items, activity.ProgressItem{ID: id, Kind: kind, Text: text})
	}
	p.lastTextKind = kind
	p.lastTextID = originalID
}

func (p *progressState) finish(ctx context.Context, result agentengine.TurnResult, fallback string, structured bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return nil
	}
	p.done = true
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.snapshot.Status = string(result.Status)
	p.snapshot.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if result.Error != nil {
		p.snapshot.Error = result.Error.Message
		p.snapshot.ErrorCode = string(result.Error.Code)
	}
	if result.Status == agentengine.TurnFailed && strings.TrimSpace(p.snapshot.Error) == "" {
		p.snapshot.Error = "turn failed"
	}
	if result.Status == agentengine.TurnSucceeded {
		if structured || (p.answer == "" && len(p.snapshot.Items) == 0) {
			p.answer = fallback
		}

	}
	for i := range p.snapshot.Items {
		tool := p.snapshot.Items[i].Tool
		if tool != nil && !terminalTool(tool.Status) {
			tool.Status = "interrupted"
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return p.flush(ctx, true, result.Files)
}

func (p *progressState) flush(ctx context.Context, persist bool, files []agentengine.OutputFile) error {
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.lastFlush = time.Now()
	p.pendingPersist = false
	p.snapshot.Revision++
	p.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return p.store.DeliverTurnProgress(ctx, p.turn, p.snapshot, p.answer, persist, files)
}

func terminalTool(status string) bool {
	switch strings.ToLower(status) {
	case "completed", "complete", "success", "succeeded", "failed", "error", "canceled", "cancelled", "declined", "interrupted":
		return true
	}
	return false
}

func (p *progressState) finalText(fallback string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.answerPhase == "final_answer" && fallback != "" {
		return fallback
	}
	if p.answer != "" {
		return p.answer
	}
	for i := len(p.snapshot.Items) - 1; i >= 0; i-- {
		if p.snapshot.Items[i].Kind == "commentary" && p.snapshot.Items[i].ID == p.lastProvisional {
			text := p.snapshot.Items[i].Text
			p.snapshot.Items = append(p.snapshot.Items[:i], p.snapshot.Items[i+1:]...)
			return text
		}
	}
	return fallback
}
