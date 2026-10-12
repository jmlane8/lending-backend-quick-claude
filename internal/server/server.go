// Package server assembles the HTTP handler: routes, health checks, metrics,
// and the middleware chain shared by every feature.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
)

// Deps are the server's collaborators.
type Deps struct {
	Logger   *slog.Logger
	Registry *prometheus.Registry
	// Ping checks database connectivity for /readyz.
	Ping func(context.Context) error
	// PoolStat exposes connection pool statistics as metrics (nil-safe).
	PoolStat func() *pgxpool.Stat
	// Routes register feature endpoints.
	Routes []func(*http.ServeMux)
}

// New builds the root handler. Middleware order (outermost first):
// RequestID → Logging → Metrics → Recover → mux, so panics are logged and counted as 500s.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.Ping(r.Context()); err != nil {
			httpx.LoggerFrom(r.Context()).Warn("readiness check failed", "error", err.Error())
			httpx.WriteProblem(w, r, httpx.Unavailable())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	d.Registry.MustRegister(newPoolCollector(d.PoolStat))
	mux.Handle("GET /metrics", promhttp.HandlerFor(d.Registry, promhttp.HandlerOpts{}))

	for _, register := range d.Routes {
		register(mux)
	}

	h := problemFallback(mux)
	h = httpx.Recover(d.Logger)(h)
	h = httpx.Metrics(d.Registry)(h)
	h = httpx.Logging(d.Logger)(h)
	return httpx.RequestID(h)
}

// problemFallback turns the mux's plain-text 404/405 responses into problem details.
func problemFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		switch rec.Code {
		case http.StatusNotFound:
			httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusNotFound, Type: "not-found", Title: "Not found"})
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", rec.Header().Get("Allow"))
			httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusMethodNotAllowed, Type: "method-not-allowed", Title: "Method not allowed"})
		default: // e.g. trailing-slash redirects
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// poolCollector exports pgxpool statistics (constitution Principle V).
type poolCollector struct {
	stat                                                    func() *pgxpool.Stat
	acquired, idle, total, max, acquireCount, acquireDurSec *prometheus.Desc
}

func newPoolCollector(stat func() *pgxpool.Stat) *poolCollector {
	d := func(name, help string) *prometheus.Desc { return prometheus.NewDesc(name, help, nil, nil) }
	return &poolCollector{
		stat:          stat,
		acquired:      d("db_pool_acquired_conns", "Connections currently checked out of the pool."),
		idle:          d("db_pool_idle_conns", "Idle connections in the pool."),
		total:         d("db_pool_total_conns", "Total connections in the pool."),
		max:           d("db_pool_max_conns", "Maximum pool size."),
		acquireCount:  d("db_pool_acquire_count_total", "Successful connection acquisitions."),
		acquireDurSec: d("db_pool_acquire_duration_seconds_total", "Total time spent waiting to acquire connections."),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.acquired, c.idle, c.total, c.max, c.acquireCount, c.acquireDurSec} {
		ch <- d
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	var acquired, idle, total, maxConns, count, durSec float64
	if c.stat != nil {
		if s := c.stat(); s != nil {
			acquired, idle, total = float64(s.AcquiredConns()), float64(s.IdleConns()), float64(s.TotalConns())
			maxConns, count, durSec = float64(s.MaxConns()), float64(s.AcquireCount()), s.AcquireDuration().Seconds()
		}
	}
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, acquired)
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, idle)
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, total)
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, maxConns)
	ch <- prometheus.MustNewConstMetric(c.acquireCount, prometheus.CounterValue, count)
	ch <- prometheus.MustNewConstMetric(c.acquireDurSec, prometheus.CounterValue, durSec)
}
