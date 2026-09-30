package transport

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	channel "csgclaw/internal/channel"
)

// Conservative local wire budgets, not verified Feishu API limits. Event size
// includes its envelope and the second JSON escaping of Content in the body.
const (
	cotEventBytes    = 1024
	cotRequestBytes  = 16000
	cotRequestEvents = 16
)

func cotUpdateBody(r COTUpdateRequest) map[string]any {
	return map[string]any{"cot_id": r.Ref.COTID, "message_id": r.Ref.MessageID, "events": r.Events}
}

func wireSize(v any) int { b, _ := json.Marshal(v); return len(b) }

func validateCOTUpdate(r COTUpdateRequest) error {
	if r.Ref.COTID == "" || r.Ref.MessageID == "" {
		return fmt.Errorf("COT identifiers are required")
	}
	if len(r.Events) > cotRequestEvents || wireSize(cotUpdateBody(r)) > cotRequestBytes {
		return fmt.Errorf("COT update exceeds local wire budget")
	}
	for _, e := range r.Events {
		if wireSize(e) > cotEventBytes {
			return fmt.Errorf("COT event exceeds local wire budget")
		}
	}
	return nil
}

// SplitCOTUpdates validates the whole append before any request is sent. Only
// append-style deltas are split; lifecycle and association fields stay intact.
// Callers must stop on the first failed request and never replay an append.
func SplitCOTUpdates(r COTUpdateRequest) ([]COTUpdateRequest, error) {
	var out []COTUpdateRequest
	current := COTUpdateRequest{Ref: r.Ref}
	for _, event := range r.Events {
		events, err := splitCOTEvent(event)
		if err != nil {
			return nil, err
		}
		for _, e := range events {
			candidate := COTUpdateRequest{Ref: r.Ref, Events: append(current.Events, e)}
			if len(candidate.Events) > cotRequestEvents || wireSize(cotUpdateBody(candidate)) > cotRequestBytes {
				if len(current.Events) == 0 {
					return nil, fmt.Errorf("COT identifiers leave no room for an event")
				}
				out = append(out, current)
				current = COTUpdateRequest{Ref: r.Ref}
			}
			current.Events = append(current.Events, e)
			if err := validateCOTUpdate(current); err != nil {
				return nil, err
			}
		}
	}
	if len(current.Events) > 0 {
		out = append(out, current)
	}
	return out, nil
}

func splitCOTEvent(e channel.COTEvent) ([]channel.COTEvent, error) {
	if !utf8.ValidString(e.Content) {
		return nil, fmt.Errorf("COT content is not valid UTF-8")
	}
	if wireSize(e) <= cotEventBytes {
		return []channel.COTEvent{e}, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(e.Content), &body); err != nil {
		return nil, fmt.Errorf("invalid COT content: %w", err)
	}
	field := ""
	delta := false
	switch e.EventType {
	case "REASONING_MESSAGE_CONTENT", "TOOL_CALL_ARGS":
		field, delta = "delta", true
	case "TOOL_CALL_RESULT":
		field = "content"
	case "TOOL_CALL_START":
		field = "title"
	default:
		return nil, fmt.Errorf("COT lifecycle event exceeds local wire budget")
	}
	var text string
	if err := json.Unmarshal(body[field], &text); err != nil {
		return nil, fmt.Errorf("invalid COT text field: %w", err)
	}
	render := func(s string) channel.COTEvent {
		body[field], _ = json.Marshal(s)
		b, _ := json.Marshal(body)
		next := e
		next.Content = string(b)
		return next
	}
	suffix := ""
	if !delta {
		suffix = "…"
	}
	if wireSize(render(suffix)) > cotEventBytes {
		return nil, fmt.Errorf("COT metadata exceeds local wire budget")
	}
	runes := []rune(text)
	var out []channel.COTEvent
	for len(runes) > 0 {
		// Bound the search window so large deltas do not cause quadratic encoding.
		lo, hi := 0, min(len(runes), cotEventBytes)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if wireSize(render(string(runes[:mid])+suffix)) <= cotEventBytes {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		if lo == 0 {
			return nil, fmt.Errorf("COT metadata leaves no room for text")
		}
		out = append(out, render(string(runes[:lo])+suffix))
		if !delta {
			return out, nil
		}
		runes = runes[lo:]
	}
	if len(out) == 0 {
		out = append(out, render(suffix))
	}
	return out, nil
}
