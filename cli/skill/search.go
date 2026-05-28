package skill

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/cli/command"
)

func (c cmd) runSearch(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill search", run.Program+" skill search <query> [flags]", "Search skills on ClawHub.")
	limit := fs.Int("limit", 20, "maximum number of results")
	if err := command.ParseFlexible(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("skill search requires a query")
	}
	query := strings.TrimSpace(strings.Join(rest, " "))
	if query == "" {
		return fmt.Errorf("skill search requires a query")
	}

	svc, err := newService(globals, run)
	if err != nil {
		return err
	}
	items, err := svc.Search(ctx, query, *limit)
	if err != nil {
		return err
	}
	return renderSearchResults(globals.Output, run.Stdout, items)
}
