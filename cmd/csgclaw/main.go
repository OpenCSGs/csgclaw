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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			// Restore default signal handling before starting cleanup so a
			// second Ctrl+C can terminate a blocked shutdown.
			signal.Stop(signals)
			cancel()
		case <-ctx.Done():
		}
	}()
	return execFn(ctx, args)
}
