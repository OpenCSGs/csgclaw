package retiredclaws

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func InstructionFile(source fs.FS, root, current string) string {
	if _, err := fs.Stat(source, root+"/"+current); errors.Is(err, fs.ErrNotExist) {
		if _, err := fs.Stat(source, root+"/AGENT.md"); err == nil {
			return "AGENT.md"
		}
	}
	return current
}

func CopiedInstructions(root, source, current string) error {
	if source == current {
		return nil
	}
	return os.Rename(filepath.Join(root, source), filepath.Join(root, current))
}
