package dockercli

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"csgclaw/internal/sandbox"
)

type ExitError struct {
	Op       string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = "command failed"
	}
	if e.Op == "" {
		return fmt.Sprintf("docker exited with code %d: %s", e.ExitCode, msg)
	}
	return fmt.Sprintf("%s: docker exited with code %d: %s", e.Op, e.ExitCode, msg)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func wrapRunError(op string, result CommandResult, err error) error {
	if err == nil {
		return nil
	}
	stderr := strings.TrimSpace(string(result.Stderr))
	// Availability must win over not-found classification. Docker Desktop
	// connection errors vary by platform and may themselves contain "not
	// found" for a missing socket or named pipe; that does not establish that
	// the requested container is absent.
	if isUnavailable(stderr) || errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s: %w: %w", op, sandbox.ErrUnavailable, &ExitError{
			Op:       op,
			ExitCode: result.ExitCode,
			Stderr:   stderr,
			Err:      err,
		})
	}
	if isNotFound(stderr) {
		return fmt.Errorf("%s: %w: %w", op, sandbox.ErrNotFound, &ExitError{
			Op:       op,
			ExitCode: result.ExitCode,
			Stderr:   stderr,
			Err:      err,
		})
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) || result.ExitCode != 0 {
		return &ExitError{
			Op:       op,
			ExitCode: result.ExitCode,
			Stderr:   stderr,
			Err:      err,
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}

func isNotFound(stderr string) bool {
	text := strings.ToLower(stderr)
	return strings.Contains(text, "no such container") ||
		strings.Contains(text, "no such object") ||
		strings.Contains(text, "container not found")
}

func isUnavailable(stderr string) bool {
	text := strings.ToLower(stderr)
	return strings.Contains(text, "cannot connect to the docker daemon") ||
		strings.Contains(text, "failed to connect to the docker api") ||
		strings.Contains(text, "docker daemon is not running") ||
		strings.Contains(text, "is the docker daemon running") ||
		strings.Contains(text, "check if the path is correct and if the daemon is running") ||
		strings.Contains(text, "error during connect") && strings.Contains(text, "docker") ||
		strings.Contains(text, "docker_engine") && strings.Contains(text, "cannot find the file") ||
		strings.Contains(text, "docker_engine") && strings.Contains(text, "the system cannot find the file specified") ||
		strings.Contains(text, "docker.sock") && strings.Contains(text, "no such file or directory") ||
		strings.Contains(text, "docker.sock") && strings.Contains(text, "connection refused") ||
		strings.Contains(text, "docker.sock") && strings.Contains(text, "permission denied") ||
		strings.Contains(text, "docker.sock") && strings.Contains(text, "not found") ||
		strings.Contains(text, "dockerdesktoplinuxengine") && strings.Contains(text, "cannot find")
}
