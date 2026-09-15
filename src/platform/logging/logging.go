// Package logging builds the application's structured logger (log/slog).
package logging

import (
	"io"
	"log/slog"

	"realtyradar/src/platform/config"
)

// New returns a logger writing to w in the configured format and level.
func New(cfg config.LogConfig, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.Level}
	if cfg.Format == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
