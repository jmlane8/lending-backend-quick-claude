package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

type requestIDKey struct{}

// RequestIDFrom returns the request's correlation ID.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func validRequestID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7E {
			return false
		}
	}
	return true
}

// RequestID keeps a valid incoming X-Request-ID or generates a UUIDv7, stores it in
// the context, and echoes it in the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			if v7, err := uuid.NewV7(); err == nil {
				id = v7.String()
			} else {
				id = uuid.NewString()
			}
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// statusRecorder captures the response status for logging and metrics.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (s *statusRecorder) code() int {
	if s.status == 0 {
		return http.StatusOK
	}
	return s.status
}

func routeOf(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	return r.Pattern
}

// Logging attaches a request-scoped logger to the context and writes one access log
// line per request. Request and response bodies are never logged.
func Logging(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			logger := base.With("request_id", RequestIDFrom(r.Context()))
			rec := &statusRecorder{ResponseWriter: w}
			ctx := WithLogger(r.Context(), logger)
			r = r.WithContext(ctx)

			next.ServeHTTP(rec, r)

			logger.LogAttrs(ctx, slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("route", routeOf(r)),
				slog.Int("status", rec.code()),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			)
		})
	}
}

// Recover converts a panic into a 500 problem and logs it.
func Recover(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
						panic(v)
					}
					logger := base
					if l, ok := r.Context().Value(loggerKey{}).(*slog.Logger); ok {
						logger = l
					}
					logger.Error("panic serving request", slog.Any("panic", v))
					WriteProblem(w, r, InternalError())
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Metrics records request duration and count by route pattern and status class.
func Metrics(reg prometheus.Registerer) func(http.Handler) http.Handler {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration by route and status class.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "code"})
	total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by route and status class.",
	}, []string{"route", "code"})
	reg.MustRegister(duration, total)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			route, code := routeOf(r), strconv.Itoa(rec.code()/100)+"xx"
			duration.WithLabelValues(route, code).Observe(time.Since(start).Seconds())
			total.WithLabelValues(route, code).Inc()
		})
	}
}
