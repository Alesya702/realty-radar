// Package app is the composition root: it loads configuration, creates
// infrastructure and wires modules together. It contains no business logic.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/stdlib"

	"realtyradar/src/modules/core"
	"realtyradar/src/platform/config"
	"realtyradar/src/platform/database"
	"realtyradar/src/platform/httpserver"
	"realtyradar/src/platform/logging"
	"realtyradar/src/platform/migrate"
	"realtyradar/src/web"
)

// Serve starts the HTTP server and blocks until ctx is cancelled.
func Serve(ctx context.Context, version string) error {
	cfg, logger, err := bootstrap(os.Stdout)
	if err != nil {
		return err
	}

	pool, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	nav := core.Nav()
	coreModule, err := core.New(pool, logger, nav)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	httpserver.RegisterHealth(mux, pool, version)
	web.Register(mux, web.Info{Version: version, Env: string(cfg.Env), Nav: nav})
	// Business modules register their routes here as they are implemented
	// (see docs/architecture/modules.md).
	coreModule.RegisterRoutes(mux)

	handler := httpserver.Chain(mux, httpserver.Recover(logger), httpserver.LogRequests(logger))
	srv := httpserver.New(cfg.HTTP, handler)

	logger.Info("starting http server", "addr", cfg.HTTP.Addr, "env", cfg.Env, "version", version)
	return httpserver.Run(ctx, srv, cfg.HTTP.ShutdownTimeout, logger)
}

// Migrate applies, rolls back or reports database migrations.
func Migrate(ctx context.Context, command string) error {
	switch command {
	case "up", "down", "status":
	default:
		return fmt.Errorf("migrate: unknown subcommand %q (expected up, down or status)", command)
	}

	cfg, logger, err := bootstrap(os.Stderr)
	if err != nil {
		return err
	}

	pool, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	return runMigrations(ctx, db, command, logger)
}

func runMigrations(ctx context.Context, db *sql.DB, command string, logger *slog.Logger) error {
	provider, err := migrate.NewProvider(db)
	if err != nil {
		return err
	}

	switch command {
	case "up":
		results, err := provider.Up(ctx)
		if err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		if len(results) == 0 {
			logger.Info("no pending migrations")
		}
		for _, r := range results {
			logger.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
		}
	case "down":
		r, err := provider.Down(ctx)
		if err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		logger.Info("migration rolled back", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
		for _, s := range statuses {
			fmt.Printf("%-16d %-10s %s\n", s.Source.Version, s.State, s.Source.Path)
		}
	}
	return nil
}

func bootstrap(logOutput io.Writer) (config.Config, *slog.Logger, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, logging.New(cfg.Log, logOutput), nil
}
