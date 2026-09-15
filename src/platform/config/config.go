// Package config loads application configuration from environment variables.
//
// All variables use the RR_ prefix. The full list with descriptions lives in
// .env.example; keep both in sync.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment is the deployment environment the application runs in.
type Environment string

const (
	EnvLocal      Environment = "local"
	EnvTest       Environment = "test"
	EnvProduction Environment = "production"
)

// Config is the complete application configuration.
type Config struct {
	Env      Environment
	Log      LogConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level  slog.Level
	Format string // "text" or "json"
}

// HTTPConfig configures the HTTP server.
type HTTPConfig struct {
	Addr            string
	ShutdownTimeout time.Duration
}

// DatabaseConfig configures the PostgreSQL connection pool.
type DatabaseConfig struct {
	URL      string
	MaxConns int32
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return FromLookup(os.LookupEnv)
}

// FromLookup reads configuration using the given lookup function
// (os.LookupEnv in production, a map in tests). All problems are reported at once.
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	var errs []error
	get := func(key, fallback string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return fallback
	}

	var cfg Config

	cfg.Env = Environment(get("RR_ENV", string(EnvLocal)))
	switch cfg.Env {
	case EnvLocal, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("RR_ENV: unsupported value %q (expected local, test or production)", cfg.Env))
	}

	if err := cfg.Log.Level.UnmarshalText([]byte(get("RR_LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("RR_LOG_LEVEL: %w", err))
	}
	cfg.Log.Format = get("RR_LOG_FORMAT", "text")
	if cfg.Log.Format != "text" && cfg.Log.Format != "json" {
		errs = append(errs, fmt.Errorf("RR_LOG_FORMAT: unsupported value %q (expected text or json)", cfg.Log.Format))
	}

	cfg.HTTP.Addr = get("RR_HTTP_ADDR", ":8080")
	timeout, err := time.ParseDuration(get("RR_HTTP_SHUTDOWN_TIMEOUT", "10s"))
	if err != nil || timeout <= 0 {
		errs = append(errs, errors.New("RR_HTTP_SHUTDOWN_TIMEOUT: expected a positive duration such as 10s"))
	}
	cfg.HTTP.ShutdownTimeout = timeout

	cfg.Database.URL = get("RR_DATABASE_URL", "")
	if cfg.Database.URL == "" {
		errs = append(errs, errors.New("RR_DATABASE_URL: required"))
	}
	maxConns, err := strconv.ParseInt(get("RR_DATABASE_MAX_CONNS", "10"), 10, 32)
	if err != nil || maxConns <= 0 {
		errs = append(errs, errors.New("RR_DATABASE_MAX_CONNS: expected a positive integer"))
	}
	cfg.Database.MaxConns = int32(maxConns)

	return cfg, errors.Join(errs...)
}
