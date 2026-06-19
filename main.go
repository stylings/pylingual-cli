package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cold/pylingual-cli/internal/api"
	"github.com/cold/pylingual-cli/internal/cli"
	"github.com/cold/pylingual-cli/internal/files"
	"github.com/cold/pylingual-cli/internal/runner"
	"github.com/cold/pylingual-cli/internal/ui"
)

func main() {
	cfg, err := cli.Parse(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			return
		}
		if errors.Is(err, cli.ErrUsage) {
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	discovery, err := files.Discover(cfg.Inputs, cfg.OutDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, warning := range discovery.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	if len(discovery.Jobs) == 0 {
		fmt.Fprintln(os.Stderr, "error: no .pyc files found")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := api.NewClient(api.Config{
		BaseURL: cfg.BaseURL,
		Timeout: cfg.Timeout,
	})
	run := runner.New(client, runner.Config{
		Concurrency:  cfg.Concurrency,
		PollInterval: cfg.PollInterval,
	})

	events := run.Start(ctx, discovery.Jobs)
	plain := cfg.Plain || os.Getenv("CI") != "" || !isTerminal(os.Stdout)

	var summary runner.Summary
	if plain {
		summary = ui.RunPlain(os.Stdout, discovery.Jobs, events)
	} else {
		summary, err = ui.RunRich(ctx, cancel, discovery.Jobs, events)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				os.Exit(130)
			}
			fmt.Fprintf(os.Stderr, "ui error: %v\n", err)
			os.Exit(1)
		}
	}

	if summary.Failed > 0 {
		os.Exit(1)
	}
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
