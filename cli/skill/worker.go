package skill

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/identity"
	skillapi "csgclaw/internal/skill"
)

func workerSkillsRoot() (string, bool) {
	id := identity.CanonicalAgentID(os.Getenv("CSGCLAW_CALLER_AGENT_ID"))
	if id == "" || id == identity.ManagerAgentID {
		return "", false
	}
	root := skillapi.RuntimeSkillsRoot()
	return root, filepath.IsAbs(root)
}

func sameDirectory(a, b string) bool {
	first, err := os.Stat(a)
	if err != nil {
		return false
	}
	second, err := os.Stat(b)
	return err == nil && os.SameFile(first, second)
}

func enableWorkerSkill(ctx context.Context, run *command.Context, globals command.GlobalOptions, name string) error {
	id := strings.TrimSpace(os.Getenv("CSGCLAW_CALLER_AGENT_ID"))
	if id == "" || globals.Token == "" {
		return fmt.Errorf("skill enable requires an authenticated CSGClaw worker")
	}
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid installed skill name %q", name)
	}
	path := "/api/v1/agents/" + url.PathEscape(id) + "/skills/" + url.PathEscape(name) + "/ensure-enabled"
	err := run.APIClient(globals).DoNoContent(ctx, http.MethodPost, path)
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == http.StatusNotFound && apiErr.Code != "skill_not_found", apiErr.Status == http.StatusMethodNotAllowed:
			return fmt.Errorf("server does not support worker skill enablement: %w", err)
		case apiErr.Status == http.StatusForbidden:
			return fmt.Errorf("check the current worker credential and server support for skill enablement: %w", err)
		}
	}
	return err
}

func (c cmd) runEnable(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill enable", run.Program+" skill enable <name>", "Enable an installed worker skill without restarting the worker.")
	if err := command.ParseFlexible(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("skill enable requires one installed skill name")
	}
	if err := enableWorkerSkill(ctx, run, globals, fs.Arg(0)); err != nil {
		return err
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{Command: "skill", Action: "enable", Status: "enabled", ID: fs.Arg(0)})
}

func (c cmd) runImport(run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill import", run.Program+" skill import <directory> [--force]", "Install the contents of a skill directory into the current worker, without enabling it.")
	force := fs.Bool("force", false, "replace an existing skill")
	if err := command.ParseFlexible(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("skill import requires one skill directory")
	}
	root, ok := workerSkillsRoot()
	if !ok {
		return fmt.Errorf("skill import requires a CSGClaw worker home")
	}
	result, err := skillapi.ImportDirectory(root, fs.Arg(0), *force)
	if err != nil {
		return err
	}
	return renderInstallResult(globals.Output, run.Stdout, result)
}
