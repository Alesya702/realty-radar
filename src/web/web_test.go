package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexPage(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Info{Version: "v-test", Env: "test"})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Realty Radar") || !strings.Contains(body, "v-test") {
		t.Errorf("body does not contain title and version")
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Info{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
