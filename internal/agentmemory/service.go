package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

type Ticket struct {
	scope      Scope
	policy     string
	lifetime   string
	writeEpoch uint64
}

func (s *Store) Policy() string { return s.instance.Epoch }

func (s *Store) Snapshot(scope Scope, query string) (Ticket, string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(scope)
	if err != nil || !current.Enabled {
		return Ticket{}, "", "", err
	}
	if err := s.initialize(&current); err != nil {
		return Ticket{}, "", "", err
	}
	token := digest(scope.UserID + ":" + current.Lifetime + ":" + current.contextVersion())
	return Ticket{scope, current.Policy, current.Lifetime, current.WriteEpoch}, token, contextMarkdown(current.Entries, query), nil
}

func (s *Store) Get(scope Scope) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(scope)
	return current.document(), err
}

// GetOrInitialize schedules the one-time history job on the first authorized use.
func (s *Store) GetOrInitialize(scope Scope) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(scope)
	if err != nil {
		return Document{}, err
	}
	if err := s.initialize(&current); err != nil {
		return Document{}, err
	}
	return current.document(), nil
}

func (s *Store) initialize(current *state) error {
	if !current.Enabled || current.Initialization != "" {
		return nil
	}
	if current.Lifetime == "" {
		current.Lifetime = newID()
		current.ContextEpoch++
	}
	current.startHistory(time.Now())
	current.Revision++
	return s.commit(*current)
}

func (s *Store) Update(scope Scope, request Update) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(scope)
	if err != nil {
		return Document{}, err
	}
	if request.Revision == nil || *request.Revision != current.Revision {
		return Document{}, ErrConflict
	}
	if request.Content != nil {
		if request.Entries != nil || request.Enabled != nil || request.Retry || request.Reset {
			return Document{}, ErrInvalid
		}
		entries, err := parseMarkdown(*request.Content, current.Entries)
		if err != nil {
			return Document{}, err
		}
		request.Entries = &entries
	}
	switch {
	case request.Reset && request.Enabled == nil && request.Entries == nil && !request.Retry && current.Enabled:
		if current.Lifetime == "" {
			current.Lifetime = newID()
		}
		current.invalidate()
		current.Entries = nil
		current.ContextEpoch++
		now := time.Now()
		current.HistoryThrough = now
		current.startHistory(now)
	case request.Enabled != nil && request.Entries == nil && !request.Retry && !request.Reset:
		if current.Enabled == *request.Enabled {
			if err := s.initialize(&current); err != nil {
				return Document{}, err
			}
			return current.document(), nil
		}
		if current.Lifetime == "" {
			current.Lifetime = newID()
		}
		current.invalidate()
		current.Enabled = *request.Enabled
		current.ContextEpoch++
		if current.Enabled && current.Initialization == "" {
			current.startHistory(time.Now())
		}
	case request.Entries != nil && request.Enabled == nil && !request.Retry && !request.Reset:
		if current.Lifetime == "" {
			current.Lifetime = newID()
		}
		next, err := editEntries(current.Entries, *request.Entries, time.Now())
		if err != nil {
			return Document{}, err
		}
		current.invalidate()
		current.Initialization = "complete"
		current.ContextEpoch++
		current.Entries = next
	case request.Retry && !request.Reset && request.Enabled == nil && request.Entries == nil && current.Enabled && len(current.Jobs) == 0:
		// An explicit history request includes conversations since the previous
		// initialization. Ordinary reads and refreshes must never replay history.
		now := time.Now()
		current.HistoryThrough = now
		current.startHistory(now)
	default:
		return Document{}, ErrInvalid
	}
	current.Revision++
	if err := s.commit(current); err != nil {
		return Document{}, err
	}
	if request.Enabled != nil || request.Entries != nil || request.Reset {
		s.cancelAttempt(scope)
	}
	return current.document(), nil
}

func (s *Store) Forget(scope Scope, source Source) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(scope)
	if err != nil {
		return Document{}, err
	}
	next := slices.DeleteFunc(current.Entries, func(entry Entry) bool {
		return entry.Source != nil && *entry.Source == source
	})
	removed := len(next) != len(current.Entries)
	if slices.Contains(current.Forgotten, source) && !removed {
		return current.document(), nil
	}
	current.Entries = next
	current.Forgotten = append(current.Forgotten, source)
	current.Jobs = slices.DeleteFunc(current.Jobs, func(task job) bool {
		return task.Message != nil && task.Message.Source == source
	})
	if removed {
		current.ContextEpoch++
	}
	current.Revision++
	if err := s.commit(current); err != nil {
		return Document{}, err
	}
	s.cancelAttempt(scope)
	return current.document(), nil
}

func (s *Store) Enqueue(ticket Ticket, id string, message Message) error {
	if ticket.scope.AgentID == "" {
		return nil
	}
	message.Text = boundText(cleanText(message.Text), maxMessageSize)
	if message.Text == "" || id == "" || message.MessageID == "" || message.RoomID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(ticket.scope)
	if err != nil || !current.Enabled || current.Policy != ticket.policy ||
		current.Lifetime != ticket.lifetime || current.WriteEpoch != ticket.writeEpoch {
		return err
	}
	if slices.Contains(current.Forgotten, message.Source) {
		return nil
	}
	id = digest(id)
	for _, task := range current.Jobs {
		if task.ID == id {
			return nil
		}
	}
	now := time.Now()
	current.pruneReceipts(now)
	for _, record := range current.Receipts {
		if record.ID == id {
			return nil
		}
	}
	if len(current.Jobs) >= maxJobs {
		current.Failure = true
		current.LastError = failureDetails(ErrCapacity, "update", now)
		return errors.Join(ErrCapacity, s.commit(current))
	}
	current.Jobs = append(current.Jobs, job{ID: id, Message: &message, CreatedAt: now, NextAttempt: now})
	if err := s.commit(current); errors.Is(err, ErrCapacity) {
		current.Jobs = current.Jobs[:len(current.Jobs)-1]
		current.Failure = true
		current.LastError = failureDetails(err, "update", now)
		return errors.Join(err, s.commit(current))
	} else {
		return err
	}
}

func (current *state) startHistory(now time.Time) {
	if current.HistoryThrough.IsZero() {
		current.HistoryThrough = now
	}
	task := job{ID: newID(), HistoryThrough: current.HistoryThrough, CreatedAt: now, NextAttempt: now}
	if len(current.Jobs) == maxJobs {
		current.Jobs = current.Jobs[:maxJobs-1]
	}
	current.Jobs = append([]job{task}, current.Jobs...)
	current.Initialization = "pending"
	current.Failure = false
}

func (current *state) invalidate() {
	current.WriteEpoch++
	current.Jobs = nil
	current.Failure = false
	current.LastError = nil
	if current.Initialization == "pending" {
		current.Initialization = ""
	}
}

func (current *state) pruneReceipts(now time.Time) {
	current.Receipts = slices.DeleteFunc(current.Receipts, func(r receipt) bool {
		return r.At.Before(now.Add(-7 * 24 * time.Hour))
	})
	if len(current.Receipts) > 64 {
		current.Receipts = current.Receipts[len(current.Receipts)-64:]
	}
}

func (current state) document() Document {
	status := "idle"
	switch {
	case len(current.Jobs) > 0:
		status = "pending"
	case current.Failure:
		status = "failed"
	}
	entries := current.Entries
	if entries == nil {
		entries = []Entry{}
	}
	return Document{Available: true, Enabled: current.Enabled, Ready: len(entries) > 0,
		Name: "MEMORY.md", Content: markdown(entries), Entries: entries, Revision: current.Revision,
		Status: status, Initialization: current.Initialization, LastError: current.LastError}
}

func editEntries(old, requested []Entry, now time.Time) ([]Entry, error) {
	byID := make(map[string]Entry, len(old))
	for _, entry := range old {
		byID[entry.ID] = entry
	}
	result := make([]Entry, 0, len(requested))
	for _, entry := range requested {
		entry.Text = cleanText(entry.Text)
		if !validKind(entry.Kind) || entry.Text == "" {
			return nil, ErrInvalid
		}
		previous, known := byID[entry.ID]
		if entry.ID == "" {
			entry.ID = newID()
		} else if !known {
			return nil, ErrInvalid
		} else {
			delete(byID, entry.ID)
		}
		entry.Source = nil
		entry.UpdatedAt = now
		if known && previous.Kind == entry.Kind && previous.Text == entry.Text {
			entry = previous
		}
		result = append(result, entry)
	}
	return result, nil
}

type Worker struct {
	Commit   func(context.Context, string, func() error) error
	Store    *Store
	Eligible func(agentID string) bool
	History  func(ctx context.Context, scope Scope, after, through time.Time, cursor HistoryCursor) ([]HistoryMessage, error)
	Extract  func(ctx context.Context, agentID string, input Extraction) ([]Change, error)
}

func (w Worker) Run(ctx context.Context) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		w.Store.repairProjections()
		for _, scope := range w.Store.pendingScopes() {
			if ctx.Err() != nil {
				return
			}
			if w.Eligible == nil || w.Eligible(scope.AgentID) {
				w.process(ctx, scope)
			} else {
				w.Store.expire(scope)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-w.Store.wake:
		case <-timer.C:
		}
	}
}

func (s *Store) pendingScopes() []Scope {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Scope, 0, len(s.pending))
	for scope := range s.pending {
		result = append(result, scope)
	}
	return result
}

func (s *Store) repairProjections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scope := range s.project {
		current, err := s.load(scope)
		if err != nil || current.Lifetime == "" || writeProjection(s.scopeDir(scope), markdown(current.Entries)) == nil {
			delete(s.project, scope)
		}
	}
}

func (s *Store) expire(scope Scope) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(scope)
	if err != nil {
		delete(s.pending, scope)
		return
	}
	if current.expireJobs(time.Now()) {
		_ = s.commit(current)
	}
}

func (current *state) expireJobs(now time.Time) bool {
	initial := len(current.Jobs)
	current.Jobs = slices.DeleteFunc(current.Jobs, func(task job) bool {
		expired := task.CreatedAt.Add(jobLifetime).Before(now)
		if expired {
			current.LastError = failureDetails(errors.New("Memory task expired before it could complete. Please retry."), task.operation(), now)
			if task.Message == nil {
				current.Initialization = "failed"
			}
		}
		return expired
	})
	if len(current.Jobs) < initial {
		current.Failure = true
		return true
	}
	return false
}

func (s *Store) next(scope Scope, now time.Time) (state, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(scope)
	if err != nil || len(current.Jobs) == 0 || !current.Enabled {
		delete(s.pending, scope)
		return state{}, false
	}
	if current.expireJobs(now) {
		if err := s.commit(current); err != nil || len(current.Jobs) == 0 {
			return state{}, false
		}
	}
	if current.Jobs[0].NextAttempt.After(now) {
		return state{}, false
	}
	return current, true
}

func (w Worker) process(ctx context.Context, scope Scope) {
	current, ok := w.Store.next(scope, time.Now())
	if !ok {
		return
	}
	task := current.Jobs[0]
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if !w.Store.beginAttempt(current, cancel) {
		return
	}
	defer w.Store.endAttempt(scope)
	started := time.Now()
	var messages []Message
	var next *HistoryCursor
	var err error
	if task.Message != nil {
		messages = []Message{*task.Message}
	} else if w.History != nil {
		var history []HistoryMessage
		history, err = w.History(callCtx, scope, w.Store.instance.HistoryAfter, task.HistoryThrough, task.HistoryCursor)
		if err == nil && len(history) > 0 {
			var cursor HistoryCursor
			// Keep forgotten positions in the scan so the cursor still advances.
			for i := range history {
				if slices.Contains(current.Forgotten, history[i].Source) {
					history[i].Text = ""
				}
			}
			messages, cursor = historySegment(history, task.HistoryCursor)
			if !cursor.advances(task.HistoryCursor) {
				err = fmt.Errorf("%w: history reader did not advance", ErrInvalid)
			} else {
				next = &cursor
			}
		}
	}
	var changes []Change
	if err == nil && len(messages) > 0 {
		if w.Extract == nil {
			err = ErrUnavailable
		} else {
			changes, err = w.Extract(callCtx, scope.AgentID, Extraction{Entries: current.Entries, Messages: messages, History: task.Message == nil})
		}
	}
	if ctx.Err() != nil || (w.Eligible != nil && !w.Eligible(scope.AgentID)) {
		return
	}
	commit := func() error { return w.Store.finish(current, task, messages, changes, err, next) }
	var commitErr error
	if w.Commit == nil {
		commitErr = commit()
	} else {
		commitErr = w.Commit(ctx, scope.AgentID, commit)
	}
	if commitErr != nil && !errors.Is(commitErr, ErrUnavailable) {
		slog.Warn("agent memory job could not be completed", "agent_id", scope.AgentID, "task", task.ID)
	} else if commitErr == nil {
		slog.Debug("agent memory attempt completed", "agent_id", scope.AgentID, "task", task.ID,
			"initialization", task.Message == nil, "elapsed_ms", time.Since(started).Milliseconds(), "model_failed", err != nil)
	}
}

func (s *Store) beginAttempt(before state, cancel context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(before.Scope)
	if err != nil || !current.Enabled || current.Lifetime != before.Lifetime ||
		current.WriteEpoch != before.WriteEpoch || current.Revision != before.Revision || len(current.Jobs) == 0 || current.Jobs[0].ID != before.Jobs[0].ID {
		return false
	}
	s.attempts[before.Scope] = cancel
	return true
}

func (s *Store) endAttempt(scope Scope) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.attempts, scope)
}

func (s *Store) cancelAttempt(scope Scope) {
	if cancel := s.attempts[scope]; cancel != nil {
		cancel()
	}
}

func (s *Store) finish(before state, task job, messages []Message, changes []Change, failure error, cursor *HistoryCursor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.load(before.Scope)
	if err != nil {
		return err
	}
	if !current.Enabled || current.Policy != before.Policy || current.Lifetime != before.Lifetime ||
		current.WriteEpoch != before.WriteEpoch || current.Revision != before.Revision || len(current.Jobs) == 0 || current.Jobs[0].ID != task.ID {
		return nil
	}
	now := time.Now()
	if failure == nil {
		var revised, replaced bool
		next, revised, replaced, err := applyChanges(current.Entries, messages, changes, now)
		failure = err
		if failure == nil {
			updated := current
			updated.Jobs = slices.Clone(current.Jobs)
			updated.Entries = next
			if revised {
				updated.Revision++
			}
			if replaced {
				updated.ContextEpoch++
			}
			if cursor != nil {
				task.HistoryCursor, task.Attempts, task.NextAttempt = *cursor, 0, now
				updated.Jobs[0] = task
				if updated.LastError != nil && updated.LastError.Operation == "initialization" {
					updated.LastError = nil
				}
				updated.Failure = updated.LastError != nil
			} else {
				updated.complete(task, now, false)
				if task.Message == nil && (len(messages) > 0 || !task.HistoryCursor.At.IsZero()) && len(updated.Entries) == 0 {
					updated.Initialization = "empty"
				}
			}
			if err := s.commit(updated); !errors.Is(err, ErrCapacity) {
				return err
			}
			failure = ErrCapacity
		}
	}
	current.LastError = failureDetails(failure, task.operation(), now)
	current.Failure = true
	task.Attempts++
	if task.Attempts >= 3 || errors.Is(failure, ErrCapacity) {
		current.complete(task, now, true)
	} else {
		task.NextAttempt = now.Add(time.Duration(task.Attempts) * 15 * time.Second)
		current.Jobs[0] = task
	}
	return s.commit(current)
}

func (task job) operation() string {
	if task.Message == nil {
		return "initialization"
	}
	return "update"
}

func (current *state) complete(task job, now time.Time, failed bool) {
	current.Jobs = current.Jobs[1:]
	current.Receipts = append(current.Receipts, receipt{ID: task.ID, At: now})
	current.pruneReceipts(now)
	if !failed && current.LastError != nil && current.LastError.Operation == task.operation() {
		current.LastError = nil
	}
	current.Failure = failed || current.LastError != nil
	if task.Message == nil {
		current.Initialization = "complete"
		if failed {
			current.Initialization = "failed"
		}
	}
}

func applyChanges(entries []Entry, messages []Message, changes []Change, now time.Time) ([]Entry, bool, bool, error) {
	next := slices.Clone(entries)
	sourceByID := make(map[string]Source, len(messages))
	for _, message := range messages {
		sourceByID[message.MessageID] = message.Source
	}
	var changed, replaced bool
	for _, change := range changes {
		source, ok := sourceByID[change.SourceMessageID]
		value := cleanText(change.Text)
		if !ok {
			return nil, false, false, fmt.Errorf("%w: model cited a source_message_id outside the supplied messages", ErrInvalid)
		}
		if !validKind(change.Kind) || value == "" {
			return nil, false, false, fmt.Errorf("%w: model returned an unsupported category or empty memory text", ErrInvalid)
		}
		target := -1
		if change.EntryID != "" {
			target = slices.IndexFunc(next, func(entry Entry) bool { return entry.ID == change.EntryID })
			if target < 0 {
				return nil, false, false, fmt.Errorf("%w: model referred to an unknown entry_id", ErrInvalid)
			}
		}
		duplicate := slices.IndexFunc(next, func(entry Entry) bool {
			return entry.Kind == change.Kind && strings.EqualFold(entry.Text, value)
		})
		if duplicate >= 0 {
			if target >= 0 && target != duplicate {
				next = slices.Delete(next, target, target+1)
				changed, replaced = true, true
			}
			continue
		}
		entry := Entry{ID: change.EntryID, Kind: change.Kind, Text: value, Source: &source, UpdatedAt: now}
		if entry.ID == "" {
			entry.ID = newID()
			next = append(next, entry)
		} else {
			next[target] = entry
			replaced = true
		}
		changed = true
	}
	return next, changed, replaced, nil
}
