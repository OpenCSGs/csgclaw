package modelcap

// CompactionOperation is a human control record, never model input.
type CompactionOperation struct {
	Revision   uint64             `json:"revision"`
	ID         string             `json:"id"`
	Epoch      uint64             `json:"epoch"`
	Running    bool               `json:"running"`
	StartedAt  string             `json:"started_at"`
	FinishedAt string             `json:"finished_at,omitempty"`
	Targets    []CompactionTarget `json:"targets"`
}
type CompactionTarget struct {
	AgentID string        `json:"agent_id"`
	UserID  string        `json:"user_id"`
	Name    string        `json:"name"`
	Status  string        `json:"status"`
	Before  *ContextUsage `json:"before,omitempty"`
	After   *ContextUsage `json:"after,omitempty"`
}

func ManualCompactionEligible(u *ContextUsage) bool {
	return u != nil && u.UsedTokens != nil && *u.UsedTokens >= max(int64(8192), (u.ContextWindow+9)/10) && u.ContextWindow > 0
}
