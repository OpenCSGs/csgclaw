package boxlitecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

type CommandRequest struct {
	Path   string
	Args   []string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if req.Path == "" {
		return CommandResult{ExitCode: -1}, fmt.Errorf("boxlite cli path is required")
	}

	cmd := exec.CommandContext(ctx, req.Path, req.Args...)
	if len(req.Env) > 0 {
		cmd.Env = append(cmd.Environ(), req.Env...)
	}
	slog.DebugContext(ctx, fmt.Sprintf("running boxlite cli command: %s", cmd.String()))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	if req.Stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, req.Stdout)
	}
	cmd.Stderr = &stderr
	if req.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, req.Stderr)
	}

	err := cmd.Run()
	result := CommandResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: 0,
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if result.ExitCode == 0 {
			result.ExitCode = -1
		}
		slog.WarnContext(ctx, "boxlite cli command canceled", "action", safeBoxliteAction(req.Args), "exit_code", result.ExitCode, "ctx_err", ctxErr)
		return result, fmt.Errorf("boxlite cli command canceled: %w", ctxErr)
	}
	if err != nil {
		if result.ExitCode == 0 {
			result.ExitCode = -1
		}
		return result, err
	}
	return result, nil
}

func safeBoxliteAction(args []string) string {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		switch arg {
		case "--home", "--config", "--registry":
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}
