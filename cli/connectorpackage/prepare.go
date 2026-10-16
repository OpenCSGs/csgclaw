package connectorpackage

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apps"
)

type prepareCmd struct{}

func NewPrepareCmd() command.Command { return prepareCmd{} }
func (prepareCmd) Name() string      { return "_prepare-connector" }
func (prepareCmd) Summary() string   { return "Prepare a built-in CLI connector package for an image." }
func (prepareCmd) Hidden() bool      { return true }

func (c prepareCmd) Run(ctx context.Context, run *command.Context, args []string, _ command.GlobalOptions) error {
	fs := run.NewFlagSet(c.Name(), run.Program+" "+c.Name()+" --id ID --directory DIR", c.Summary())
	id := fs.String("id", "", "Built-in CLI connector ID")
	directory := fs.String("directory", "", "Preinstalled packages directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || strings.TrimSpace(*id) == "" || strings.TrimSpace(*directory) == "" {
		return fmt.Errorf("%s requires --id and --directory", c.Name())
	}
	pkg, err := apps.PrepareCLIPackage(ctx, *id, *directory)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(run.Stdout, pkg.Executable)
	return err
}
