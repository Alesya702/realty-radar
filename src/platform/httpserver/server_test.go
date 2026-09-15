package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"realtyradar/src/platform/config"
)

func TestRunShutsDownWhenContextIsCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.HTTPConfig{Addr: "127.0.0.1:0"}, http.NotFoundHandler())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, srv, time.Second, logger) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.HTTPConfig{Addr: "invalid-address"}, http.NotFoundHandler())

	if err := Run(context.Background(), srv, time.Second, logger); err == nil {
		t.Fatal("expected a listen error")
	}
}
