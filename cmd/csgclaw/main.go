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
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}
}

func run(args []string) error {
	app := cli.New()
	return executeWithSignalContext(args, app.Execute)
}

func executeWithSignalContext(args []string, execFn func(context.Context, []string) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execFn(ctx, args)
}
