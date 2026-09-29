package activity

// TurnProgress is the Web presentation snapshot of one Engine turn.
// It contains display data only; it never controls execution or permissions.
type TurnProgress struct {
	ID        string         `json:"id"`
	Revision  uint64         `json:"revision"`
	Status    string         `json:"status"`
	StartedAt string         `json:"started_at"`
	UpdatedAt string         `json:"updated_at"`
	EndedAt   string         `json:"ended_at,omitempty"`
	Items     []ProgressItem `json:"items"`
	Error     string         `json:"error,omitempty"`
	// ErrorCode is passed to the presentation store; its public form lives in message metadata.
	ErrorCode string `json:"-"`
}

type ProgressItem struct {
	ID   string        `json:"id"`
	Kind string        `json:"kind"`
	Text string        `json:"text,omitempty"`
	Tool *ProgressTool `json:"tool,omitempty"`
}

type ProgressTool struct {
	Name       string   `json:"name"`
	Actions    []string `json:"actions"`
	Status     string   `json:"status"`
	Input      string   `json:"input,omitempty"`
	Output     string   `json:"output,omitempty"`
	Command    string   `json:"command,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	DurationMS *int64   `json:"duration_ms,omitempty"`
	Truncated  bool     `json:"truncated,omitempty"`
}
