package apitypes

type WorkspaceListing struct {
	Kind    string           `json:"kind"`
	Path    string           `json:"path"`
	Entries []WorkspaceEntry `json:"entries,omitempty"`
}

type WorkspaceEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Depth int    `json:"depth,omitempty"`
	Size  int64  `json:"size,omitempty"`
}

type WorkspaceFile struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
}
