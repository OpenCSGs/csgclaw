package delivery

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/diagnostics"
)

func progressTool(tool agentengine.ToolActivity, previous *activity.ProgressTool) *activity.ProgressTool {
	return progressToolDetails(tool, previous, false)
}

func progressToolDetails(tool agentengine.ToolActivity, previous *activity.ProgressTool, collaboration bool) *activity.ProgressTool {
	value := activity.ProgressTool{}
	if previous != nil {
		value = *previous
	}
	if tool.Title != "" {
		value.Name = tool.Title
	}
	if value.Name == "" {
		value.Name = tool.Kind
	}
	if tool.Status != "" {
		value.Status = tool.Status
	}
	if tool.InputSummary != "" {
		value.Input = tool.InputSummary
	}
	if tool.OutputSummary != "" {
		value.Output = tool.OutputSummary
	}
	payload, _ := tool.Payload.(map[string]any)
	if collaboration {
		value.DetailsVersion = 1
		// Runtime adapters already redact this payload. Preserve known argument
		// and result fields instead of the 240-character presentation summaries.
		for _, key := range []string{"arguments", "rawInput"} {
			if input, ok := payload[key]; ok && input != nil {
				value.Input = progressDetailJSON(input)
			}
		}
		for _, key := range []string{"result", "rawOutput", "contentItems", "content"} {
			if output, ok := payload[key]; ok && output != nil {
				value.Output = progressDetailJSON(output)
			}
		}
		if failure, ok := payload["error"]; ok && failure != nil {
			value.Output = progressDetailJSON(map[string]any{"result": payload["result"], "error": failure})
		}
	}
	// Only copy known display fields; arbitrary provider payloads never reach Web.
	for _, field := range []struct {
		key  string
		dest *string
	}{{"command", &value.Command}, {"workdir", &value.Cwd}, {"cwd", &value.Cwd}, {"aggregatedOutput", &value.Output}, {"output", &value.Output}} {
		if text, ok := payload[field.key].(string); ok {
			*field.dest = text
		}
	}
	if input, ok := payload["rawInput"].(map[string]any); ok {
		for _, key := range []string{"command", "cmd"} {
			if text, ok := input[key].(string); ok {
				value.Command = text
			}
		}
		for _, key := range []string{"cwd", "workdir"} {
			if text, ok := input[key].(string); ok {
				value.Cwd = text
			}
		}
	}
	if output, ok := payload["content"]; ok && value.Output == "" {
		if b, err := json.Marshal(output); err == nil {
			value.Output = string(b)
		}
	}
	if n, ok := progressNumberField(payload, "exitCode", "exit_code"); ok {
		v := int(n)
		value.ExitCode = &v
	}
	if n, ok := progressNumberField(payload, "durationMs", "duration_ms"); ok {
		value.DurationMS = &n
	} else if duration, ok := payload["duration"].(map[string]any); ok {
		if seconds, ok := progressNumber(duration["secs"]); ok {
			nanos, _ := progressNumber(duration["nanos"])
			n := seconds*1000 + nanos/1_000_000
			value.DurationMS = &n
		}
	}
	actions := []string{}
	add := func(action string) {
		for _, v := range actions {
			if v == action {
				return
			}
		}
		actions = append(actions, action)
	}
	if raw, ok := payload["commandActions"].([]any); ok {
		for _, item := range raw {
			v, _ := item.(map[string]any)
			kind, _ := v["type"].(string)
			if value.Summary == "" {
				value.Summary = progressToolText(v, "query", "name", "path")
				if kind == "search" && progressToolText(v, "query") != "" {
					if path := progressToolText(v, "path"); path != "" {
						value.Summary += " " + path
					}
				}
			}
			switch kind {
			case "read":
				add("read")
			case "search":
				add("search")
			case "listFiles":
				add("list")
			default:
				add("execute")
			}
		}
	}
	if len(actions) == 0 {
		switch strings.ToLower(tool.Kind) {
		case "read", "read_file", "readfile":
			add("read")
		case "search", "grep", "glob":
			add("search")
		case "list", "list_files":
			add("list")
		case "edit", "write", "file_change", "apply_patch", "patch_apply", "write_file":
			add("edit")
		case "execute", "exec", "exec_command", "command_execution", "shell":
			add("execute")
		case "web_search", "websearch":
			add("web")
		case "tool_search", "toolsearch":
			add("load")
		default:
			add("tool")
		}
	}
	if value.Summary == "" {
		action, _ := payload["action"].(map[string]any)
		arguments, _ := payload["arguments"].(map[string]any)
		for _, fields := range []map[string]any{action, arguments, payload} {
			if summary := progressToolText(fields, "query", "q", "url"); summary != "" {
				value.Summary = summary
				break
			}
		}
	}
	if previous == nil || payload["commandActions"] != nil {
		value.Actions = actions
	}
	if truncated, _ := payload["preview_truncated"].(bool); truncated {
		value.Truncated = true
	}
	for _, s := range []*string{&value.Input, &value.Output, &value.Command, &value.Cwd, &value.Summary} {
		if len(*s) > 64<<10 {
			n := 64 << 10
			for n > 0 && !utf8.ValidString((*s)[:n]) {
				n--
			}
			*s = (*s)[:n]
			value.Truncated = true
		}
	}
	return &value
}

func progressDetailJSON(value any) string {
	// Reuse diagnostic redaction for JSON embedded in MCP text blocks and for
	// binary content. This preserves numeric precision and avoids persisting credentials.
	data := diagnostics.Protect(value)
	var text string
	if json.Unmarshal(data, &text) == nil {
		return text
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return string(data)
	}
	return formatted.String()
}
func progressToolText(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := fields[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func progressNumberField(fields map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		if value, ok := progressNumber(fields[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func progressNumber(value any) (int64, bool) {
	switch n := value.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		value, err := n.Int64()
		return value, err == nil
	}
	return 0, false
}
