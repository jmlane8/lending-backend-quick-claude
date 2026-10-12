package server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

func newTestServer(pingErr error) http.Handler {
	return New(Deps{
		Logger:   slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		Registry: prometheus.NewRegistry(),
		Ping:     func(context.Context) error { return pingErr },
		PoolStat: func() *pgxpool.Stat { return nil },
	})
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestHealthz(t *testing.T) {
	w := do(newTestServer(nil), http.MethodGet, "/healthz")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Errorf("got %d %s", w.Code, w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	if w := do(newTestServer(nil), http.MethodGet, "/readyz"); w.Code != http.StatusOK {
		t.Errorf("ready: got %d", w.Code)
	}
	w := do(newTestServer(errors.New("db down")), http.MethodGet, "/readyz")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "temporarily-unavailable") {
		t.Errorf("not ready: got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "db down") {
		t.Error("internal error detail leaked")
	}
}

func TestMetricsEndpointIncludesHTTPAndPoolMetrics(t *testing.T) {
	h := newTestServer(nil)
	do(h, http.MethodGet, "/healthz")
	w := do(h, http.MethodGet, "/metrics")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	for _, name := range []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"db_pool_acquired_conns",
		"db_pool_idle_conns",
		"db_pool_total_conns",
		"db_pool_max_conns",
		"db_pool_acquire_count_total",
		"db_pool_acquire_duration_seconds_total",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics output missing %s", name)
		}
	}
}

func TestUnknownRouteAndMethodAreProblems(t *testing.T) {
	h := newTestServer(nil)
	w := do(h, http.MethodGet, "/nope")
	if w.Code != http.StatusNotFound || w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("404: got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	w = do(h, http.MethodPost, "/healthz")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("405: got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Allow") == "" {
		t.Error("405 missing Allow header")
	}
}

func TestRoutesAreRegistered(t *testing.T) {
	h := New(Deps{
		Logger:   slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		Registry: prometheus.NewRegistry(),
		Ping:     func(context.Context) error { return nil },
		PoolStat: func() *pgxpool.Stat { return nil },
		Routes: []func(*http.ServeMux){func(mux *http.ServeMux) {
			mux.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
		}},
	})
	if w := do(h, http.MethodGet, "/v1/ping"); w.Code != http.StatusTeapot {
		t.Errorf("got %d", w.Code)
	}
}
