package installstate

import (
	"fmt"
	"os"
	"path/filepath"
)

// Use a separate, persistent file: the JSON file is atomically replaced on write.
// Never remove the lock file on release; waiters must keep using the same file.
// Closing the handle releases the OS lock, including when the process exits.
func acquireRecordsLock(skillsRoot string, createRoot bool) (*os.File, error) {
	if createRoot {
		if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
			return nil, fmt.Errorf("create skills root: %w", err)
		}
	}
	file, err := os.OpenFile(filepath.Join(skillsRoot, lockFileName+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		if !createRoot && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open skill records lock: %w", err)
	}
	if err := lockRecordsFile(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock skill records: %w", err)
	}
	return file, nil
}
