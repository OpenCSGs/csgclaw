package workspace

import (
	"fmt"
	"os"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/utils/filebrowse"
)

// OpenInput opens the authorized relative path under the active worker's workspace.
// The returned handle remains bound to the checked file if the worker renames it.
func OpenInput(root string, ref *contract.WorkspaceFile) (*os.File, string, error) {
	if !ref.Valid() {
		return nil, "", fmt.Errorf("invalid workspace file reference")
	}
	f, err := filebrowse.OpenFile(root, ref.Path)
	if err != nil {
		return nil, "", err
	}
	prompt := fmt.Sprintf("Attached file %q is saved in your workspace at %q. You can use this relative path in this and later turns.", ref.Name, ref.Path)
	return f, prompt, nil
}
