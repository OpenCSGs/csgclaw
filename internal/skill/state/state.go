package state

// State 保存单个 Agent 的 Skill 启用状态。
type State struct {
	Enabled bool `json:"enabled"`
}

func Enabled(states map[string]State, name string) bool {
	state, exists := states[name]
	return !exists || state.Enabled
}
