//go:build windows

package codex

import "csgclaw/internal/processgroup"

func stopProcessTree(pid int) error { return processgroup.Stop(pid, true) }
