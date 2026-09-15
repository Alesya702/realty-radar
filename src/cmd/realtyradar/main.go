// Command realtyradar is the single entry point of the Realty Radar application.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"realtyradar/src/app"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `Realty Radar

Usage:
  realtyradar serve                      start the HTTP server
  realtyradar migrate <up|down|status>   manage database migrations
  realtyradar demo                       load the demo dataset (fictional projects and lots)
  realtyradar version                    print version
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// .env is a local development convenience; variables already set in the environment win.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("command is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "serve":
		return app.Serve(ctx, version)
	case "migrate":
		if len(args) != 2 {
			return errors.New("migrate: expected exactly one subcommand: up, down or status")
		}
		return app.Migrate(ctx, args[1])
	case "demo":
		return app.SeedDemo(ctx)
	case "version":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}
