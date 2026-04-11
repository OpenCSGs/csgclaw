package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"csgclaw/cli"
)

func main() {
	// step 1.0 Start from a thin entrypoint and hand off almost everything to cli.App.
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}
}

func run(args []string) error {
	// step 1.1 Build the CLI application object, which owns command dispatch and shared dependencies.
	app := cli.New()
	return executeWithSignalContext(args, app.Execute)
}

func executeWithSignalContext(args []string, execFn func(context.Context, []string) error) error {
	// step 1.2 Give every command a cancellation context so serve/log streaming can shut down cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execFn(ctx, args)
}
