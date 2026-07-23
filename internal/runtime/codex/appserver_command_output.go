package codex

import (
	"fmt"
	"strings"

	"csgclaw/internal/activity"
)

const appServerCommandOutputPreviewBytes = 8 * 1024

// appServerCommandOutputAccumulator consumes the canonical
// item/commandExecution/outputDelta stream. Some app-server providers leave
// commandExecution.aggregatedOutput empty, so completion alone is not a
// reliable source of command output.
//
// Only a bounded preview of ordinary output is retained because the bridge
// renders a short tool summary. Structured records are decoded line by line,
// which keeps the protocol reliable even when a command prints a large amount
// of unrelated stdout before or after a control record.
type appServerCommandOutputAccumulator struct {
	rawPreview     strings.Builder
	cleanedPreview strings.Builder
	line           strings.Builder
	lineOversized  bool
	artifact       activity.StructuredOutputArtifact
	decodeErrors   []error
	seenLinks      map[string]struct{}
}

func (a *appServerCommandOutputAccumulator) append(delta string) {
	if a == nil || delta == "" {
		return
	}
	appendBoundedString(&a.rawPreview, delta, appServerCommandOutputPreviewBytes)

	for len(delta) > 0 {
		newline := strings.IndexByte(delta, '\n')
		part := delta
		hasNewline := newline >= 0
		if hasNewline {
			part = delta[:newline]
			delta = delta[newline+1:]
		} else {
			delta = ""
		}
		a.appendLinePart(part)
		if hasNewline {
			a.consumeLine(true)
		}
	}
}

func (a *appServerCommandOutputAccumulator) appendLinePart(part string) {
	if a == nil || part == "" || a.lineOversized {
		return
	}
	remaining := maxStructuredOutputRecordBytes + 1 - a.line.Len()
	if remaining <= 0 {
		a.lineOversized = true
		return
	}
	if len(part) > remaining {
		_, _ = a.line.WriteString(part[:remaining])
		a.lineOversized = true
		return
	}
	_, _ = a.line.WriteString(part)
}

func (a *appServerCommandOutputAccumulator) consumeLine(hasNewline bool) {
	if a == nil {
		return
	}
	line := a.line.String()
	oversized := a.lineOversized
	a.line.Reset()
	a.lineOversized = false

	if oversized {
		if strings.HasPrefix(strings.TrimSuffix(line, "\r"), structuredOutputPrefix) {
			a.decodeErrors = append(a.decodeErrors, fmt.Errorf("structured output record exceeds %d bytes", maxStructuredOutputRecordBytes))
		}
		a.appendCleaned(line, hasNewline)
		return
	}

	cleaned, decoded, decodeErrors := decodeStructuredCommandOutput(line)
	a.decodeErrors = append(a.decodeErrors, decodeErrors...)
	removed := !structuredOutputArtifactEmpty(decoded)
	if removed {
		a.mergeArtifact(decoded)
		return
	}
	a.appendCleaned(cleaned, hasNewline)
}

func (a *appServerCommandOutputAccumulator) appendCleaned(value string, newline bool) {
	appendBoundedString(&a.cleanedPreview, value, appServerCommandOutputPreviewBytes)
	if newline {
		appendBoundedString(&a.cleanedPreview, "\n", appServerCommandOutputPreviewBytes)
	}
}

func (a *appServerCommandOutputAccumulator) mergeArtifact(decoded activity.StructuredOutputArtifact) {
	if decoded.RequestUserInput != nil {
		if a.artifact.RequestUserInput == nil {
			a.artifact.RequestUserInput = decoded.RequestUserInput
		} else {
			a.decodeErrors = append(a.decodeErrors, fmt.Errorf("only one request_user_input record is allowed per command output"))
		}
	}
	if a.seenLinks == nil {
		a.seenLinks = make(map[string]struct{})
	}
	for _, link := range decoded.ResourceLinks {
		if _, exists := a.seenLinks[link.URI]; exists {
			continue
		}
		if len(a.artifact.ResourceLinks) >= maxStructuredOutputResourceLinks {
			a.decodeErrors = append(a.decodeErrors, fmt.Errorf("at most %d resource_link records are allowed", maxStructuredOutputResourceLinks))
			continue
		}
		a.seenLinks[link.URI] = struct{}{}
		a.artifact.ResourceLinks = append(a.artifact.ResourceLinks, link)
	}
}

func (a *appServerCommandOutputAccumulator) finish(success bool) (string, activity.StructuredOutputArtifact, []error) {
	if a == nil {
		return "", activity.StructuredOutputArtifact{}, nil
	}
	if a.line.Len() > 0 || a.lineOversized {
		a.consumeLine(false)
	}
	if !success {
		return a.rawPreview.String(), activity.StructuredOutputArtifact{}, nil
	}
	return a.cleanedPreview.String(), a.artifact, a.decodeErrors
}

func appendBoundedString(builder *strings.Builder, value string, limit int) {
	if builder == nil || value == "" || limit <= builder.Len() {
		return
	}
	remaining := limit - builder.Len()
	if len(value) > remaining {
		value = value[:remaining]
	}
	_, _ = builder.WriteString(value)
}

func appServerCommandOutputKey(threadID, itemID string) string {
	threadID = strings.TrimSpace(threadID)
	itemID = strings.TrimSpace(itemID)
	if threadID == "" || itemID == "" {
		return ""
	}
	return threadID + "\x00" + itemID
}

func (s *liveSession) appendAppServerCommandOutput(threadID, turnID, itemID, delta string) {
	key := appServerCommandOutputKey(threadID, itemID)
	if s == nil || key == "" || delta == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commandOutputs == nil {
		s.commandOutputs = make(map[string]*appServerCommandOutputState)
	}
	state := s.commandOutputs[key]
	if state == nil {
		state = &appServerCommandOutputState{
			threadID: strings.TrimSpace(threadID),
			turnID:   strings.TrimSpace(turnID),
			output:   &appServerCommandOutputAccumulator{},
		}
		s.commandOutputs[key] = state
	}
	if state.turnID == "" {
		state.turnID = strings.TrimSpace(turnID)
	}
	state.output.append(delta)
}

func (s *liveSession) takeAppServerCommandOutput(threadID, itemID string) *appServerCommandOutputAccumulator {
	key := appServerCommandOutputKey(threadID, itemID)
	if s == nil || key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.commandOutputs[key]
	delete(s.commandOutputs, key)
	if state == nil {
		return nil
	}
	return state.output
}

func (s *liveSession) clearAppServerCommandOutputs(threadID, turnID string) {
	if s == nil {
		return
	}
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, state := range s.commandOutputs {
		if state == nil || (threadID != "" && state.threadID != threadID) || (turnID != "" && state.turnID != turnID) {
			continue
		}
		delete(s.commandOutputs, key)
	}
}

type appServerCommandOutputState struct {
	threadID string
	turnID   string
	output   *appServerCommandOutputAccumulator
}
