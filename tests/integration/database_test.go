//go:build integration

// Package integration_test contains tests that need a real PostgreSQL.
// Run with: RR_TEST_DATABASE_URL=... go test -tags integration ./tests/integration/...
// The database must be dedicated to tests: migrations are rolled back to zero.
package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"realtyradar/src/platform/config"
	"realtyradar/src/platform/database"
	"realtyradar/src/platform/httpserver"
	"realtyradar/src/platform/migrate"
)

func connect(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("RR_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("RR_TEST_DATABASE_URL is not set; it must point to a dedicated test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, config.DatabaseConfig{URL: url, MaxConns: 4})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrationsApplyAndRollBack(t *testing.T) {
	ctx := context.Background()
	pool := connect(t)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })

	provider, err := migrate.NewProvider(db)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	t.Cleanup(func() {
		if _, err := provider.DownTo(ctx, 0); err != nil {
			t.Errorf("roll back migrations: %v", err)
		}
	})

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	assertAllApplied(t, ctx, provider)

	var coreSchemaExists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'core')").Scan(&coreSchemaExists); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	if !coreSchemaExists {
		t.Error("schema core does not exist after migrations")
	}

	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("up after full rollback: %v", err)
	}
	assertAllApplied(t, ctx, provider)
}

func assertAllApplied(t *testing.T, ctx context.Context, provider *goose.Provider) {
	t.Helper()
	statuses, err := provider.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("no migrations found")
	}
	for _, s := range statuses {
		if s.State != goose.StateApplied {
			t.Errorf("migration %s is %s, want applied", s.Source.Path, s.State)
		}
	}
}

func TestReadinessWithRealDatabase(t *testing.T) {
	pool := connect(t)

	mux := http.NewServeMux()
	httpserver.RegisterHealth(mux, pool, "integration")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /readyz status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
