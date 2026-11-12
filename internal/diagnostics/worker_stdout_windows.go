//go:build windows

package diagnostics

import (
	"io"
	"os"
)

func workerStdout() io.Writer { return os.Stdout }
