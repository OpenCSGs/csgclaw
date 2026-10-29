package im

import (
	"bytes"
	"csgclaw/internal/diagnostics"
	"os"
	"runtime"
	"time"
)

// Reuse a committed regular file only when its complete contents still match.
// Changed data retains the same synchronous, atomic replacement as before.
// Do not follow a symlink or trust an in-memory digest after an external edit.
func writeFileIfChanged(path string, data []byte, perm os.FileMode, operations ...*diagnostics.Operation) error {
	operation := diagnostics.OperationArg(operations)
	start := time.Now()
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() == int64(len(data)) && persistedModeMatches(info.Mode(), perm) {
		if previous, err := os.ReadFile(path); err == nil && bytes.Equal(previous, data) {
			operation.Observe("im.compare_read", time.Since(start), int64(len(data)), 1)
			operation.Observe("im.file_skipped", 0, int64(len(data)), 1)
			return nil
		}
	}
	operation.Observe("im.compare_read", time.Since(start), 0, 1)
	return atomicWriteFile(path, data, perm, operations...)
}

func persistedModeMatches(mode, requested os.FileMode) bool {
	if runtime.GOOS == "windows" {
		// Windows Chmod implements only the owner-write (read-only) attribute.
		return mode.Perm()&0o200 == requested.Perm()&0o200
	}
	return mode.Perm() == requested.Perm()
}
