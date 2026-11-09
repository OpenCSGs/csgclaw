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
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/modelusage"
)

type turnProgressStore interface {
	DeliverTurnProgress(context.Context, channel.TurnContext, activity.TurnProgress, string, bool, []agentengine.OutputFile) error
}

// progressState serializes timer flushes with terminal delivery. A completed
// snapshot can never be overwritten by a delayed streaming update.
type progressState struct {
	diagnostic     *diagnostics.Record
	lastFlush      time.Time
	pendingPersist bool
	mu             sync.Mutex
	snapshot       activity.TurnProgress
	answer         string
	texts          []*progressText
	textItems      map[string]*progressText
	currentText    *progressText
	lastAnswer     *progressText
	lastTextID     string
	lastTextKind   string
	tools          map[string]int
	timer          *time.Timer
	done           bool
	lastErr        error
	store          turnProgressStore
	turn           channel.TurnContext
}

// Native text keeps its identity and phase while displayed in progress.
// Progress indices remain stable until terminal delivery.
type progressText struct {
	itemID   string
	phase    string
	text     string
	index    int
	promoted bool
}

func newProgressState(store turnProgressStore, turn channel.TurnContext) *progressState {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return &progressState{store: store, turn: turn, tools: map[string]int{}, textItems: map[string]*progressText{}, snapshot: activity.TurnProgress{ID: string(turn.TurnID), Status: "running", StartedAt: now, UpdatedAt: now, Items: []activity.ProgressItem{}}}
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
	p.diagnostic = diagnostics.From(ctx)
	defer p.mu.Unlock()
	return p.flush(ctx, true, nil)
}

func (p *progressState) observe(ctx context.Context, event agentengine.TurnEvent) error {
	wait := diagnostics.MeasureMetric(ctx, "delivery.progress_lock_wait")
	p.mu.Lock()
	wait()
	hold := diagnostics.MeasureMetric(ctx, "delivery.progress_lock_hold")
	defer func() { hold(); p.mu.Unlock() }()
	if p.diagnostic == nil {
		p.diagnostic = diagnostics.From(ctx)
	}
	if p.done {
		return nil
	}
	if p.lastErr != nil {
		return p.lastErr
	}
	immediate := false
	switch event.Kind {
	case agentengine.TurnEventTextDelta:
		immediate = p.observeText(event)
	case agentengine.TurnEventThoughtDelta:
		if event.Thought == "" {
			return nil
		}
		p.appendText(event.ItemID, "reasoning", event.Thought)
		if p.currentText != nil && p.currentText.itemID == "" && p.currentText.phase == "commentary" {
			p.currentText = nil
		}
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		if event.Tool == nil {
			return nil
		}
		p.snapshot.Status = "running"
		if event.Kind == agentengine.TurnEventToolCallStart {
			for _, item := range p.texts {
				if item.phase != "final_answer" {
					p.moveTextToProgress(item)
				}
			}
			p.currentText = nil
			p.updateAnswer()
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
		if event.Activity != nil && event.Activity.Kind == modelusage.Kind {
			usage := modelusage.Decode(event.Activity.Payload)
			if usage == nil {
				return nil
			}
			p.snapshot.Usage = usage
			immediate = true
		} else if event.Activity != nil && (event.Activity.Kind == string(activity.RuntimeEventActionDecision) || event.Activity.Kind == string(activity.RuntimeEventUserInputResolved)) {
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
				ctx, cancel := context.WithTimeout(diagnostics.WithRecord(context.Background(), p.diagnostic), 5*time.Second)
				defer cancel()
				p.lastErr = p.flush(ctx, p.pendingPersist, nil)
			}
		})
	}
	return nil
}

func (p *progressState) observeText(event agentengine.TurnEvent) bool {
	var item *progressText
	if event.ItemID != "" {
		item = p.textItems[event.ItemID]
	} else if current := p.currentText; current != nil && current.itemID == "" && (current.phase == "commentary") == (event.Phase == "commentary") {
		item = current
	}
	// Legacy deltas can acquire their native ID within the same text segment.
	if current := p.currentText; item == nil && event.ItemID != "" && current != nil {
		if current.itemID == "" && current.phase != "commentary" && event.Phase != "commentary" {
			item = current
			item.itemID = event.ItemID
			p.textItems[event.ItemID] = item
		}
	}
	immediate := event.TextSnapshot
	if item == nil {
		if event.Text == "" {
			return false
		}
		if event.Phase != "commentary" {
			for _, previous := range p.texts {
				if previous.phase != "final_answer" {
					p.moveTextToProgress(previous)
				}
			}
		}
		item = &progressText{itemID: event.ItemID, phase: event.Phase, index: -1}
		p.texts = append(p.texts, item)
		if event.ItemID != "" {
			p.textItems[event.ItemID] = item
		}
		p.currentText = item
		if event.Phase != "commentary" {
			p.lastAnswer = item
			immediate = true
		}
	}
	if event.Phase != "" {
		immediate = immediate || event.Phase != item.phase
		item.phase = event.Phase
	}
	if event.TextSnapshot {
		item.text = event.Text
	} else {
		item.text += event.Text
	}
	if item.phase == "commentary" {
		p.moveTextToProgress(item)
	}
	if item.index >= 0 {
		p.snapshot.Items[item.index].Text = item.text
	} else {
		p.lastTextKind = ""
		p.lastTextID = ""
	}
	p.snapshot.Status = "running"
	p.updateAnswer()
	return immediate
}

func (p *progressState) moveTextToProgress(item *progressText) {
	if item.index < 0 && item.text != "" {
		p.appendText(item.itemID, "commentary", item.text)
		item.index = len(p.snapshot.Items) - 1
	}
}

func (p *progressState) updateAnswer() {
	var text strings.Builder
	for _, item := range p.texts {
		if item.index < 0 && item.phase != "commentary" {
			text.WriteString(item.text)
		}
	}
	p.answer = text.String()
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
	if p.snapshot.Usage != nil && result.Status != agentengine.TurnSucceeded {
		usage := *p.snapshot.Usage
		usage.Partial = true
		p.snapshot.Usage = &usage
	}
	p.snapshot.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	// A canceled runtime call can carry context.Canceled as its cause. Keep the
	// stopped state and interrupted tools without presenting a runtime failure.
	if result.Status == agentengine.TurnFailed && result.Error != nil {
		p.snapshot.Error = result.Error.Message
		p.snapshot.ErrorCode = string(result.Error.Code)
	}
	if result.Status == agentengine.TurnFailed && strings.TrimSpace(p.snapshot.Error) == "" {
		p.snapshot.Error = "turn failed"
	}
	if result.Status == agentengine.TurnSucceeded {
		// Tool updates use indices into Items until this turn finishes.
		promoted := make(map[int]bool)
		for _, item := range p.texts {
			if item.promoted && item.index >= 0 {
				promoted[item.index] = true
			}
		}
		items := p.snapshot.Items[:0]
		for i, item := range p.snapshot.Items {
			if !promoted[i] {
				items = append(items, item)
			}
		}
		p.snapshot.Items = items
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

func (p *progressState) finalText(fallback string, native []agentengine.TurnEvent) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if native != nil {
		p.reconcileText(native)
	}
	var text strings.Builder
	var selected *progressText
	answerItems := 0
	for _, item := range p.texts {
		if item.phase == "commentary" {
			continue
		}
		answerItems++
		if item.phase == "final_answer" || item == p.lastAnswer {
			text.WriteString(item.text)
			item.promoted = true
			selected = item
		}
	}
	if selected == nil {
		return fallback
	}
	// Turn-wide output can extend a message only when its native identity is
	// unambiguous. Multiple messages use their own deltas and snapshots.
	if answerItems == 1 && selected.itemID != "" && fallback != "" && (selected.phase == "final_answer" || strings.HasPrefix(fallback, text.String())) {
		return fallback
	}
	return text.String()
}

func (p *progressState) reconcileText(native []agentengine.TurnEvent) {
	retained := make(map[*progressText]bool)
	ordered := make([]*progressText, 0, len(native))
	p.lastAnswer = nil
	for _, event := range native {
		item := p.textItems[event.ItemID]
		if item == nil {
			item = &progressText{itemID: event.ItemID, index: -1}
			p.textItems[event.ItemID] = item
		}
		item.phase, item.text = event.Phase, event.Text
		if item.phase == "commentary" {
			p.moveTextToProgress(item)
		} else {
			p.lastAnswer = item
		}
		if item.index >= 0 {
			p.snapshot.Items[item.index].Text = item.text
		}
		retained[item] = true
		ordered = append(ordered, item)
	}
	previous := p.texts
	p.texts = nil
	for _, item := range previous {
		if !retained[item] {
			if item.phase != "commentary" {
				// Native history has replaced this legacy message identity.
				item.phase, item.promoted = "commentary", true
			}
			p.texts = append(p.texts, item)
		}
	}
	p.texts = append(p.texts, ordered...)
}
