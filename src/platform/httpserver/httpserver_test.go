package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (p fakePinger) Ping(context.Context) error { return p.err }

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		pingErr    error
		wantStatus int
		wantBody   map[string]string
	}{
		{"liveness", "/healthz", errors.New("db down"), http.StatusOK, map[string]string{"status": "ok", "version": "test"}},
		{"readiness ok", "/readyz", nil, http.StatusOK, map[string]string{"status": "ok", "database": "ok"}},
		{"readiness db down", "/readyz", errors.New("db down"), http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "database": "unreachable"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			RegisterHealth(mux, fakePinger{err: tt.pingErr}, "test")

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			for k, v := range tt.wantBody {
				if body[k] != v {
					t.Errorf("body[%q] = %q, want %q", k, body[k], v)
				}
			}
		})
	}
}

func TestRecoverReturns500(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), Recover(logger), LogRequests(logger))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
