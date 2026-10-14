package apitypes

import "encoding/json"

type ResourceSource string
type ResourceType string

const (
	ResourceSourceSkill       ResourceSource = "skill"
	ResourceSourceMCP         ResourceSource = "mcp"
	ResourceSourceApp         ResourceSource = "app"
	ResourceTypeSkill         ResourceType   = "skill"
	ResourceTypeConnector     ResourceType   = "connector"
	ResourceTypeKnowledgeBase ResourceType   = "knowledge_base"
)

type ResourceReference struct {
	Source ResourceSource `json:"source"`
	ID     string         `json:"id"`
}

type ResourceSnapshot struct {
	ResourceReference
	Type ResourceType `json:"type"`
	Name string       `json:"name"`
}

type AgentResourceSummary struct {
	ResourceSnapshot
	Description       string `json:"description,omitempty"`
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type ResourceSelection struct {
	Version int                `json:"version"`
	AgentID string             `json:"agent_id"`
	Refs    []ResourceSnapshot `json:"refs"`
}

func (s *ResourceSelection) Clone() *ResourceSelection {
	if s == nil {
		return nil
	}
	result := *s
	result.Refs = append([]ResourceSnapshot(nil), s.Refs...)
	return &result
}

func (s *ResourceSelection) SingleSkill() bool {
	return s != nil && s.Version == 1 && s.AgentID != "" && len(s.Refs) == 1 && s.Refs[0].Source == ResourceSourceSkill && s.Refs[0].ID != ""
}

func ResourceSelectionFromMetadata(metadata map[string]any) *ResourceSelection {
	namespace, _ := metadata["csgclaw"].(map[string]any)
	value := namespace["resource_selection"]
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var selection ResourceSelection
	if json.Unmarshal(data, &selection) != nil {
		return nil
	}
	return &selection
}

// WithResourceSelection 复制资源命名空间，保留其他消息 metadata。
func WithResourceSelection(metadata map[string]any, selection *ResourceSelection) map[string]any {
	result := make(map[string]any, len(metadata)+1)
	for key, value := range metadata {
		result[key] = value
	}
	namespace := map[string]any{}
	if old, ok := metadata["csgclaw"].(map[string]any); ok {
		for key, value := range old {
			if key != "resource_selection" {
				namespace[key] = value
			}
		}
	}
	if selection != nil {
		namespace["resource_selection"] = selection.Clone()
	}
	if len(namespace) > 0 {
		result["csgclaw"] = namespace
	} else {
		delete(result, "csgclaw")
	}
	return result
}
