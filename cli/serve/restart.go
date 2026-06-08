package serve

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/upgrade"
)

type internalRestartCmd struct{}

func NewInternalRestartCmd() command.Command {
	return internalRestartCmd{}
}

func (internalRestartCmd) Name() string {
	return "_restart"
}

func (internalRestartCmd) Summary() string {
	return "Internal config restart helper."
}

func (internalRestartCmd) Hidden() bool {
	return true
}

func (c internalRestartCmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("_restart", run.Program+" _restart [flags]", c.Summary())
	fs.Usage = func() {
		restartUsage(run, fs)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("_restart does not accept positional arguments")
	}

	artifacts := upgrade.RestartArtifactsFromEnv()
	fail := func(err error) error {
		err = explainRestartError(run.Program, err)
		if recordErr := artifacts.RecordFailure(err); recordErr != nil {
			return fmt.Errorf("%w\nAlso failed to record restart helper status: %v", err, recordErr)
		}
		return err
	}

	restarted, err := upgrade.RestartDaemonFromExecutable(ctx, upgrade.RestartOptions{
		ConfigPath: globals.Config,
	})
	if err != nil {
		return fail(err)
	}
	if !restarted.DaemonWasRunning {
		message := fmt.Sprintf("Config saved. Stop the running server and run `%s serve` again.", run.Program)
		if recordErr := artifacts.RecordManualRestartRequired(message); recordErr != nil {
			return fmt.Errorf("record manual restart status: %w", recordErr)
		}
	} else {
		_ = artifacts.ClearStatus()
	}
	return renderRestartResult(globals.Output, run.Stdout, restarted, run.Program)
}

func restartUsage(run *command.Context, fs *flag.FlagSet) {
	fmt.Fprintln(run.Stderr, "Restart the local CSGClaw daemon after config changes.")
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Usage:")
	fmt.Fprintf(run.Stderr, "  %s _restart [flags]\n", run.Program)
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Flags:")
	fs.PrintDefaults()
}

func explainRestartError(program string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "read pid file"), strings.Contains(msg, "parse pid file"), strings.Contains(msg, "stop running daemon"), strings.Contains(msg, "restart daemon"):
		return fmt.Errorf("%w\nRestart manually with `%s stop` and `%s serve -d`.", err, program, program)
	default:
		return err
	}
}

type restartResultView struct {
	PIDPath       string `json:"pid_path,omitempty"`
	DaemonRunning bool   `json:"daemon_running"`
	Restarted     bool   `json:"restarted"`
	ManualRestart bool   `json:"manual_restart_required"`
	Message       string `json:"message,omitempty"`
}

func renderRestartResult(output string, w io.Writer, restarted upgrade.RestartResult, program string) error {
	output, err := command.NormalizeOutput(output)
	if err != nil {
		return err
	}
	message := "Config saved."
	if restarted.Restarted {
		message = "Config saved and service restarted."
	} else if !restarted.DaemonWasRunning {
		message = fmt.Sprintf("%s\nNo daemon detected; stop the running server and run `%s serve` again.", message, program)
	}
	view := restartResultView{
		PIDPath:       restarted.PIDPath,
		DaemonRunning: restarted.DaemonWasRunning,
		Restarted:     restarted.Restarted,
		ManualRestart: restarted.DaemonWasRunning == false,
		Message:       message,
	}
	if output == "json" {
		return command.WriteJSON(w, view)
	}
	_, err = fmt.Fprintln(w, message)
	return err
}
