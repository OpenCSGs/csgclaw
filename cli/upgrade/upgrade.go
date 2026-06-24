package upgrade

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/upgrade"
	appversion "csgclaw/internal/version"
)

type cmd struct{}

var newUpgradeClient = func(run *command.Context) upgrade.Client {
	return upgrade.Client{
		HTTPClient: run.HTTPClient,
		GOOS:       currentGOOS,
		GOARCH:     runtime.GOARCH,
	}
}

var (
	currentGOOS     = runtime.GOOS
	installPrepared = func(client upgrade.Client, prepared upgrade.PreparedBundle) (upgrade.InstalledBundle, error) {
		return client.InstallPrepared(prepared)
	}
	stopDaemonFromExecutable  = upgrade.StopDaemonFromExecutable
	startDaemonFromExecutable = upgrade.StartDaemonFromExecutable
	startInstalledDaemon      = upgrade.StartInstalledDaemon
)

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "upgrade"
}

func (cmd) Summary() string {
	return "Check for, download, and install a newer CSGClaw release."
}

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("upgrade", run.Program+" upgrade [flags]", c.Summary())
	checkOnly := fs.Bool("check", false, "check for updates without downloading or installing")
	noRestart := fs.Bool("no-restart", false, "install without restarting the local service")
	fs.Usage = func() {
		usage(run, fs)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("upgrade does not accept positional arguments")
	}

	applyArtifacts := upgrade.ApplyArtifactsFromEnv()
	fail := func(err error) error {
		err = explainError(run.Program, err)
		if recordErr := applyArtifacts.RecordFailure(err); recordErr != nil {
			return fmt.Errorf("%w\nAlso failed to record upgrade helper status: %v", err, recordErr)
		}
		return err
	}

	client := newUpgradeClient(run)
	result, err := client.Check(ctx, appversion.Current())
	if err != nil {
		return fail(err)
	}
	if *checkOnly || !result.UpdateAvailable {
		_ = applyArtifacts.ClearStatus()
		return renderResult(globals.Output, run.Stdout, result)
	}
	if support := client.AutoUpgradeSupport(appversion.Current()); !support.Supported {
		return fail(upgrade.ErrNotOfficialBundle)
	}
	if result.Asset == nil {
		return fail(fmt.Errorf("matched release asset is required for installation"))
	}

	prepared, err := client.PrepareRelease(ctx, *result.Asset, "")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(prepared.WorkDir)

	var stopped upgrade.RestartResult
	if currentGOOS == "windows" && !*noRestart {
		stopped, err = stopDaemonFromExecutable(ctx)
		if err != nil {
			return fail(err)
		}
	}

	installed, err := installPrepared(client, prepared)
	if err != nil {
		if currentGOOS == "windows" && stopped.DaemonWasRunning {
			if restartErr := startDaemonFromExecutable(ctx, upgrade.RestartOptions{ConfigPath: globals.Config}); restartErr != nil {
				return fail(fmt.Errorf("%w\nAlso failed to restart daemon that was running before upgrade: %v", err, restartErr))
			}
		}
		return fail(err)
	}
	if *noRestart {
		_ = applyArtifacts.ClearStatus()
		return renderInstallResult(globals.Output, run.Stdout, result, installed, upgrade.RestartResult{}, run.Program, true)
	}

	restarted := upgrade.RestartResult{}
	if currentGOOS == "windows" {
		restarted = stopped
		if stopped.DaemonWasRunning {
			restarted, err = startInstalledDaemon(ctx, installed, upgrade.RestartOptions{
				ConfigPath: globals.Config,
			})
			if err != nil {
				return fail(err)
			}
		}
	} else {
		restarted, err = client.RestartIfRunning(ctx, installed, upgrade.RestartOptions{
			ConfigPath: globals.Config,
		})
		if err != nil {
			return fail(err)
		}
	}
	if !*noRestart && !restarted.DaemonWasRunning {
		if recordErr := applyArtifacts.RecordManualRestartRequired("manual restart required"); recordErr != nil {
			return fmt.Errorf("record manual restart status: %w", recordErr)
		}
	} else {
		_ = applyArtifacts.ClearStatus()
	}
	return renderInstallResult(globals.Output, run.Stdout, result, installed, restarted, run.Program, false)
}

func usage(run *command.Context, fs *flag.FlagSet) {
	fmt.Fprintln(run.Stderr, "Check for, download, and install a newer CSGClaw release.")
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Usage:")
	fmt.Fprintf(run.Stderr, "  %s upgrade [flags]\n", run.Program)
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Behavior:")
	fmt.Fprintln(run.Stderr, "  By default, upgrade checks the latest release, installs the matching official bundle,")
	fmt.Fprintln(run.Stderr, "  and restarts the local daemon when it is running with the default PID path.")
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Examples:")
	fmt.Fprintf(run.Stderr, "  %s upgrade --check\n", run.Program)
	fmt.Fprintf(run.Stderr, "  %s upgrade\n", run.Program)
	fmt.Fprintf(run.Stderr, "  %s upgrade --no-restart\n", run.Program)
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Common failure cases:")
	fmt.Fprintln(run.Stderr, "  Automatic install requires an official release bundle marker and layout (<install-root>/bin/csgclaw[.exe]).")
	fmt.Fprintln(run.Stderr, "  Automatic restart only supports the default PID path (~/.csgclaw/server.pid).")
	fmt.Fprintln(run.Stderr)
	fmt.Fprintln(run.Stderr, "Flags:")
	fs.PrintDefaults()
}

func explainError(program string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not installed from an official csgclaw bundle"):
		return fmt.Errorf("%w\nThis installation can check versions but cannot replace files automatically. Migrate it safely with:\n  curl -fsSL https://csgclaw.opencsg.com/install.sh | bash\nThe installer keeps the managed bundle under ~/.local/lib/csgclaw and replaces only ~/.local/bin/csgclaw with a symlink. You can also run `%s upgrade --check` for version check only.", err, program)
	case strings.Contains(msg, "downloaded size mismatch"), strings.Contains(msg, "downloaded sha256 mismatch"):
		return fmt.Errorf("%w\nThe downloaded archive did not match the published metadata. Retry later or report the broken release.", err)
	case strings.Contains(msg, "release bundle is missing"), strings.Contains(msg, "release archive contains unsupported entry"):
		return fmt.Errorf("%w\nThe published release bundle is invalid. Retry later or report the broken release.", err)
	case strings.Contains(msg, "read pid file"), strings.Contains(msg, "parse pid file"), strings.Contains(msg, "stop running daemon"), strings.Contains(msg, "restart daemon"):
		return fmt.Errorf("%w\nRun `%s upgrade --no-restart`, then restart manually with `%s stop` and `%s serve --daemon`.", err, program, program, program)
	default:
		return err
	}
}

func renderResult(output string, w io.Writer, result upgrade.CheckResult) error {
	output, err := command.NormalizeOutput(output)
	if err != nil {
		return err
	}
	if output == "json" {
		return command.WriteJSON(w, result)
	}
	assetName := "-"
	if result.Asset != nil && result.Asset.Name != "" {
		assetName = result.Asset.Name
	}
	update := "no"
	if result.UpdateAvailable {
		update = "yes"
	}
	_, err = fmt.Fprintf(w, "Current version: %s\nLatest version:  %s\nUpdate available: %s\nAsset: %s\n", result.CurrentVersion, result.LatestVersion, update, assetName)
	return err
}

type installResult struct {
	CurrentVersion  string                `json:"current_version"`
	LatestVersion   string                `json:"latest_version"`
	UpdateAvailable bool                  `json:"update_available"`
	Asset           *upgrade.ReleaseAsset `json:"asset,omitempty"`
	Status          string                `json:"status"`
	InstallRoot     string                `json:"install_root,omitempty"`
	PIDPath         string                `json:"pid_path,omitempty"`
	DaemonRunning   bool                  `json:"daemon_running"`
	Restarted       bool                  `json:"restarted"`
	RestartSkipped  bool                  `json:"restart_skipped"`
	Message         string                `json:"message,omitempty"`
}

func renderInstallResult(output string, w io.Writer, check upgrade.CheckResult, installed upgrade.InstalledBundle, restarted upgrade.RestartResult, program string, restartSkipped bool) error {
	output, err := command.NormalizeOutput(output)
	if err != nil {
		return err
	}
	status := "installed"
	message := fmt.Sprintf("Upgrade completed: %s", check.LatestVersion)
	if restarted.Restarted {
		status = "restarted"
	}
	if !restartSkipped && !restarted.DaemonWasRunning {
		message = fmt.Sprintf("%s\nNo running service detected", message)
	}
	if output == "json" {
		return command.WriteJSON(w, installResult{
			CurrentVersion:  check.CurrentVersion,
			LatestVersion:   check.LatestVersion,
			UpdateAvailable: check.UpdateAvailable,
			Asset:           check.Asset,
			Status:          status,
			InstallRoot:     installed.InstallRoot,
			PIDPath:         restarted.PIDPath,
			DaemonRunning:   restarted.DaemonWasRunning,
			Restarted:       restarted.Restarted,
			RestartSkipped:  restartSkipped,
			Message:         message,
		})
	}
	if err := renderResult(output, w, check); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Installing new bundle"); err != nil {
		return err
	}
	switch {
	case restartSkipped:
		if _, err := fmt.Fprintf(w, "%s\nRestart skipped\nRun `%s stop` and `%s serve --daemon` to apply the new version\n", message, program, program); err != nil {
			return err
		}
	case restarted.Restarted:
		if _, err := fmt.Fprintf(w, "Restarting service\n%s\n", message); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(w, "%s\nNo running service detected\n", message); err != nil {
			return err
		}
	}
	return nil
}
