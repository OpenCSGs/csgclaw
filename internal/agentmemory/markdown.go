package agentmemory

import (
	"strings"
)

// parseMarkdown accepts a complete human-authored memory document. Recognized
// category headings determine scope; other Markdown is retained as fact text.
func parseMarkdown(content string, existing []Entry) ([]Entry, error) {
	if len(content) > maxDocumentBytes {
		return nil, ErrCapacity
	}
	type identity struct{ kind, text string }
	byText := make(map[identity][]Entry)
	for _, entry := range existing {
		key := identity{entry.Kind, cleanText(entry.Text)}
		byText[key] = append(byText[key], entry)
	}
	var entries []Entry
	var lines []string
	kind := "fact"
	bullet := false
	fence := ""
	flush := func() {
		text := cleanText(strings.Join(lines, "\n"))
		lines = nil
		bullet = false
		if text == "" {
			return
		}
		key := identity{kind, text}
		if matches := byText[key]; len(matches) > 0 {
			entries = append(entries, matches[0])
			byText[key] = matches[1:]
			return
		}
		entries = append(entries, Entry{Kind: kind, Text: text})
	}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if bullet {
				line = strings.TrimPrefix(line, "  ")
			}
			lines = append(lines, line)
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			lines = append(lines, line)
			continue
		}
		if strings.HasPrefix(line, "# ") && (trimmed == "# Agent memory" || trimmed == "# Agent 记忆" || trimmed == "# 记忆") {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if category := markdownCategory(heading); category != "" {
				flush()
				kind = category
				continue
			}
			flush()
			kind = "fact"
		}
		listItem := strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ")
		if listItem {
			flush()
			bullet = true
			text := line[2:]
			if strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~") {
				fence = text[:3]
			}
			lines = append(lines, text)
			continue
		}
		if trimmed == "" && !bullet {
			flush()
			continue
		}
		if bullet {
			line = strings.TrimPrefix(line, "  ")
		}
		lines = append(lines, line)
	}
	flush()
	return entries, nil
}

func markdownCategory(heading string) string {
	heading = strings.ToLower(strings.TrimSpace(heading))
	switch heading {
	case "preference", "preferences", "偏好", "用户偏好", "长期偏好":
		return "preference"
	case "working agreement", "working agreements", "convention", "conventions", "工作约定", "协作约定":
		return "convention"
	case "fact", "facts", "confirmed fact", "confirmed facts", "已确认事实", "事实":
		return "fact"
	default:
		return ""
	}
}
