package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"web-search/internal/config"
	"web-search/internal/observability"
	"web-search/internal/root"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	err := run(ctx, os.Args[1:], os.Stderr)

	stop()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("web-search", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", "config.yaml", "YAML configuration file")
	transport := flags.String("transport", "", "Override transport: http or stdio")

	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	if err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}

	if *transport != "" {
		cfg.Server.Transport = *transport
	}

	err = cfg.Validate()
	if err != nil {
		return err
	}

	logger, closer, err := observability.New(cfg.Logging, stderr)
	if err != nil {
		return err
	}
	defer closer.Close()

	app, err := root.New(cfg, logger)
	if err != nil {
		return err
	}

	err = app.Run(ctx)
	if err != nil && ctx.Err() == nil {
		logger.ErrorContext(ctx, "server.failed", "error", err.Error())
		return err
	}

	logger.InfoContext(ctx, "server.stopped")

	return nil
}
