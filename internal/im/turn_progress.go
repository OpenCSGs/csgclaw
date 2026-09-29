package im

// A saved running snapshot belongs to a prior process. Never restart its clock
// or present its draft as a successful response after loading persisted rooms.
func interruptSavedTurnProgress(messages []Message) {
	for i := range messages {
		meta, _ := messages[i].Metadata["csgclaw"].(map[string]any)
		progress, _ := meta["turn_progress"].(map[string]any)
		if progress == nil {
			continue
		}
		switch progress["status"] {
		case "running", "waiting":
			progress["status"] = "interrupted"
			progress["ended_at"] = progress["updated_at"]
			items, _ := progress["items"].([]any)
			for _, raw := range items {
				item, _ := raw.(map[string]any)
				tool, _ := item["tool"].(map[string]any)
				if tool == nil {
					continue
				}
				switch tool["status"] {
				case "", "started", "running", "pending", "in_progress", "inProgress":
					tool["status"] = "interrupted"
				}
			}
			if revision, ok := progress["revision"].(float64); ok {
				progress["revision"] = revision + 1
			}
		}
	}
}

// Reload refreshes external changes without losing live, coalesced progress.
// Only service startup interrupts saved turns; an HTTP history read is not a
// process restart. Compare revisions so a newer persisted terminal event wins.
func (s *Service) preserveTurnProgressLocked(state *Bootstrap) {
	for r := range state.Rooms {
		loaded := &state.Rooms[r]
		current := s.rooms[loaded.ID]
		if current == nil {
			continue
		}
		byID := make(map[string]int, len(loaded.Messages))
		for i, message := range loaded.Messages {
			byID[message.ID] = i
		}
		for _, message := range current.Messages {
			live := messageTurnProgress(message)
			if live == nil {
				continue
			}
			i, exists := byID[message.ID]
			if !exists {
				if live["status"] == "running" || live["status"] == "waiting" {
					loaded.Messages = append(loaded.Messages, cloneMessage(message))
				}
				continue
			}
			saved := messageTurnProgress(loaded.Messages[i])
			liveRevision, _ := live["revision"].(float64)
			savedRevision, _ := saved["revision"].(float64)
			if saved != nil && live["id"] == saved["id"] && liveRevision >= savedRevision {
				loaded.Messages[i] = cloneMessage(message)
			}
		}
	}
}

func messageTurnProgress(message Message) map[string]any {
	meta, _ := message.Metadata["csgclaw"].(map[string]any)
	progress, _ := meta["turn_progress"].(map[string]any)
	return progress
}
