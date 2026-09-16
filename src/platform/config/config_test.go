package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func TestFromLookupDefaults(t *testing.T) {
	cfg, err := FromLookup(lookupFrom(map[string]string{
		"RR_DATABASE_URL": "postgres://localhost/realtyradar",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Env != EnvLocal {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvLocal)
	}
	if cfg.Log.Level != slog.LevelInfo || cfg.Log.Format != "text" {
		t.Errorf("Log = %+v, want info/text", cfg.Log)
	}
	// The default binds the server to the loopback interface: the MVP has no
	// authentication, so it must not be reachable from the network (A-02).
	if cfg.HTTP.Addr != "127.0.0.1:8080" || cfg.HTTP.ShutdownTimeout != 10*time.Second {
		t.Errorf("HTTP = %+v, want 127.0.0.1:8080/10s", cfg.HTTP)
	}
	if cfg.Database.MaxConns != 10 {
		t.Errorf("Database.MaxConns = %d, want 10", cfg.Database.MaxConns)
	}
}

func TestFromLookupOverrides(t *testing.T) {
	cfg, err := FromLookup(lookupFrom(map[string]string{
		"RR_ENV":                   "production",
		"RR_LOG_LEVEL":             "debug",
		"RR_LOG_FORMAT":            "json",
		"RR_HTTP_ADDR":             "127.0.0.1:9000",
		"RR_HTTP_SHUTDOWN_TIMEOUT": "3s",
		"RR_DATABASE_URL":          "postgres://db/realtyradar",
		"RR_DATABASE_MAX_CONNS":    "25",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Env != EnvProduction || cfg.Log.Level != slog.LevelDebug || cfg.Log.Format != "json" {
		t.Errorf("unexpected env/log: %+v", cfg)
	}
	if cfg.HTTP.Addr != "127.0.0.1:9000" || cfg.HTTP.ShutdownTimeout != 3*time.Second {
		t.Errorf("unexpected HTTP config: %+v", cfg.HTTP)
	}
	if cfg.Database.URL != "postgres://db/realtyradar" || cfg.Database.MaxConns != 25 {
		t.Errorf("unexpected database config: %+v", cfg.Database)
	}
}

func TestFromLookupReportsAllErrors(t *testing.T) {
	_, err := FromLookup(lookupFrom(map[string]string{
		"RR_ENV":                "staging",
		"RR_LOG_FORMAT":         "xml",
		"RR_DATABASE_MAX_CONNS": "0",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"RR_ENV", "RR_LOG_FORMAT", "RR_DATABASE_URL", "RR_DATABASE_MAX_CONNS"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}
