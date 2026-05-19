package codexmodel

import "strings"

type Profile struct {
	ModelID         string
	ReasoningEffort string
}

func Catalog(profile Profile) map[string]any {
	return map[string]any{
		"models": []map[string]any{Metadata(profile)},
	}
}

func Metadata(profile Profile) map[string]any {
	modelID := strings.TrimSpace(profile.ModelID)
	if modelID == "" {
		modelID = "unknown"
	}
	reasoningEffort := strings.ToLower(strings.TrimSpace(profile.ReasoningEffort))
	switch reasoningEffort {
	case "none", "minimal", "low", "medium", "high", "xhigh":
	default:
		reasoningEffort = "medium"
	}
	baseInstructions := "You are Codex, a coding agent. Follow the user's instructions and use available tools carefully."
	return map[string]any{
		"slug":                    modelID,
		"display_name":            modelID,
		"description":             "CSGClaw OpenAI-compatible provider model",
		"default_reasoning_level": reasoningEffort,
		"supported_reasoning_levels": []map[string]any{
			{"effort": "low", "description": "low"},
			{"effort": "medium", "description": "medium"},
			{"effort": "high", "description": "high"},
		},
		"shell_type":                   "default",
		"visibility":                   "list",
		"supported_in_api":             true,
		"priority":                     1,
		"availability_nux":             nil,
		"upgrade":                      nil,
		"base_instructions":            baseInstructions,
		"model_messages":               modelMessages(baseInstructions),
		"supports_search_tool":         false,
		"supports_reasoning_summaries": false,
		"default_reasoning_summary":    "auto",
		"support_verbosity":            false,
		"default_verbosity":            nil,
		"apply_patch_tool_type":        nil,
		"web_search_tool_type":         "text",
		"truncation_policy": map[string]any{
			"mode":  "bytes",
			"limit": 10000,
		},
		"supports_parallel_tool_calls":     false,
		"supports_image_detail_original":   false,
		"context_window":                   272000,
		"auto_compact_token_limit":         nil,
		"effective_context_window_percent": 95,
		"experimental_supported_tools":     []any{},
		"input_modalities":                 []string{"text", "image"},
	}
}

func modelMessages(baseInstructions string) map[string]any {
	return map[string]any{
		"instructions_template": "{{ personality }}\n\n" + baseInstructions,
		"instructions_variables": map[string]any{
			"personality_default":   "",
			"personality_friendly":  "You are collaborative, supportive, and clear.",
			"personality_pragmatic": "You are a deeply pragmatic, effective software engineer.",
		},
	}
}
