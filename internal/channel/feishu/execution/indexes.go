package execution

// pruneConversationIndexes releases idle conversation routing. Admission and
// answer handlers reserve a control before updating these maps; retaining those
// reservations closes the gap before a new Run is registered in runs.
func (r *Runner) pruneConversationIndexes() {
	r.controlMu.Lock()
	defer r.controlMu.Unlock()
	r.interactionMu.Lock()
	defer r.interactionMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	retained := make(map[string]bool, len(r.controls)+len(r.runs))
	for key := range r.controls {
		retained[key] = true
	}
	for run := range r.runs {
		retained[run.key] = true
	}
	for _, item := range r.interactions {
		key := item.message.ConversationKey
		if !item.finished && r.latest[key] == item.message.TurnID {
			retained[key] = true
		}
	}
	for key := range r.latest {
		if !retained[key] {
			delete(r.latest, key)
			delete(r.workerContexts, key)
		}
	}
	for key := range r.workerContexts {
		if !retained[key] {
			delete(r.workerContexts, key)
		}
	}
}
