package csghub

import (
	"errors"
	"strings"

	"csgclaw/internal/sandbox/csghub/csghubsdk"
)

// isSandboxUpOrComingUp reports whether a Hub-reported state means the
// sandbox does not need an explicit start call.
func isSandboxUpOrComingUp(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "deploying", "starting", "creating", "created", "pending", "ready":
		return true
	}
	return false
}

// isSandboxRunning is the "terminal healthy" check used by waitForRunning.
func isSandboxRunning(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return true
	}
	return false
}

// isSandboxTerminalFailure is the "terminal unhealthy" check used by polling.
func isSandboxTerminalFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "errored", "crashed", "stopped", "terminated", "dead":
		return true
	}
	return false
}

// isStartUnsupported returns true when the hub does not expose /status/start.
func isStartUnsupported(err error) bool {
	var httpErr *csghubsdk.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case 404, 405, 501:
			return true
		}
	}
	return false
}
