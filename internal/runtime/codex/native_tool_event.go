package codex

// Project native tool data for display. Executing tools and publishing their
// artifacts belong to live notifications, never to historical items.
func nativeToolEvent(item map[string]any, completed bool) (SessionEvent, bool) {
	event := SessionEvent{ToolCallID: appServerString(item, "id"), Payload: item}
	var input, output any
	switch appServerString(item, "type") {
	case "commandExecution":
		event.ToolKind, event.ToolTitle = "exec_command", "Run shell command"
		input = map[string]any{"command": appServerString(item, "command")}
		text, _ := item["aggregatedOutput"].(string)
		output = map[string]any{"output": text}
	case "fileChange":
		event.ToolKind, event.ToolTitle = "patch_apply", "Apply patch"
		input, output = item, item
	case "mcpToolCall":
		event.ToolKind, event.ToolTitle = "mcp_tool_call", appServerToolTitle(item, "Run MCP tool")
		input = map[string]any{"server": item["server"], "tool": item["tool"], "arguments": item["arguments"]}
		output = map[string]any{"result": item["result"], "error": item["error"]}
	case "dynamicToolCall":
		event.ToolKind, event.ToolTitle = "dynamic_tool_call", appServerToolTitle(item, "Run dynamic tool")
		input = map[string]any{"tool": item["tool"], "arguments": item["arguments"]}
		output = map[string]any{"content_items": item["contentItems"], "success": item["success"]}
	case "webSearch":
		event.ToolKind, event.ToolTitle = "web_search", "Web search"
		input = map[string]any{"query": item["query"], "action": item["action"]}
		output = item
	default:
		return SessionEvent{}, false
	}
	if completed {
		event.Kind = SessionEventToolCallUpdate
		event.ToolStatus = appServerToolStatus(item, "completed")
		event.ToolOutputSummary = summarizeToolValue(output)
	} else {
		event.Kind = SessionEventToolCallStart
		event.ToolStatus = "started"
		switch event.ToolKind {
		case "mcp_tool_call", "dynamic_tool_call", "web_search":
			event.ToolStatus = appServerToolStatus(item, "started")
		}
		event.ToolInputSummary = summarizeToolValue(input)
	}
	return event, true
}
