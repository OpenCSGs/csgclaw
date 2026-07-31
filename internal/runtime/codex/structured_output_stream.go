package codex

import "strings"

// assistantStructuredOutputStream keeps a possible structured-output suffix out
// of text deltas without buffering ordinary assistant text. A line break is
// retained until the first bytes of the next line establish whether that line
// is a control record.
type assistantStructuredOutputStream struct {
	atLineStart bool
	lineBreak   string
	candidate   string
	suppressed  bool
	emitted     strings.Builder
}

func newAssistantStructuredOutputStream() *assistantStructuredOutputStream {
	return &assistantStructuredOutputStream{atLineStart: true}
}

func (s *assistantStructuredOutputStream) append(delta string) string {
	if s == nil || delta == "" || s.suppressed {
		return ""
	}
	var output strings.Builder
	emit := func(text string) {
		output.WriteString(text)
		s.emitted.WriteString(text)
	}
	for _, ch := range delta {
		if !s.atLineStart {
			if ch == '\n' {
				s.lineBreak = "\n"
				s.atLineStart = true
			} else {
				emit(string(ch))
			}
			continue
		}

		if s.candidate == "" && ch != ':' {
			emit(s.lineBreak)
			s.lineBreak = ""
			if ch == '\n' {
				s.lineBreak = "\n"
			} else {
				emit(string(ch))
				s.atLineStart = false
			}
			continue
		}

		s.candidate += string(ch)
		if isStructuredAssistantPrefix(s.candidate) {
			s.lineBreak = ""
			s.candidate = ""
			s.suppressed = true
			break
		}
		if isPossibleStructuredAssistantPrefix(s.candidate) {
			continue
		}
		emit(s.lineBreak)
		emit(s.candidate)
		s.lineBreak = ""
		s.candidate = ""
		s.atLineStart = false
	}
	return output.String()
}

func (s *assistantStructuredOutputStream) finish(cleaned string) string {
	if s == nil {
		return cleaned
	}
	emitted := s.emitted.String()
	if strings.HasPrefix(cleaned, emitted) {
		return strings.TrimPrefix(cleaned, emitted)
	}
	return ""
}

func isStructuredAssistantPrefix(candidate string) bool {
	return candidate == structuredOutputPrefix || candidate == structuredOutputAssistantPrefix
}

func isPossibleStructuredAssistantPrefix(candidate string) bool {
	return strings.HasPrefix(structuredOutputPrefix, candidate) ||
		strings.HasPrefix(structuredOutputAssistantPrefix, candidate)
}
