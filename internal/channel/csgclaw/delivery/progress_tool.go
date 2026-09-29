package delivery

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
)

func progressTool(tool agentengine.ToolActivity, previous *activity.ProgressTool) *activity.ProgressTool {
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
	// Only copy known display fields; arbitrary provider payloads never reach Web.
	for _, field := range []struct {
		key  string
		dest *string
	}{{"command", &value.Command}, {"cwd", &value.Cwd}, {"aggregatedOutput", &value.Output}, {"output", &value.Output}} {
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
	if n, ok := progressNumber(payload["exitCode"]); ok {
		v := int(n)
		value.ExitCode = &v
	}
	if n, ok := progressNumber(payload["durationMs"]); ok {
		value.DurationMS = &n
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
	if previous == nil || payload["commandActions"] != nil {
		value.Actions = actions
	}
	if truncated, _ := payload["preview_truncated"].(bool); truncated {
		value.Truncated = true
	}
	for _, s := range []*string{&value.Input, &value.Output, &value.Command, &value.Cwd} {
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
func progressNumber(value any) (int64, bool) {
	switch n := value.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}
