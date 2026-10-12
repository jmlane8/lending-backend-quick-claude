package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRequestIDKeepsValidIncoming(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", "abc-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if seen != "abc-123" || w.Header().Get("X-Request-ID") != "abc-123" {
		t.Errorf("seen=%q header=%q", seen, w.Header().Get("X-Request-ID"))
	}
}

func TestRequestIDGeneratesWhenMissingOrInvalid(t *testing.T) {
	for _, incoming := range []string{"", strings.Repeat("a", 129), "bad id with spaces"} {
		var seen string
		h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seen = RequestIDFrom(r.Context())
		}))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if incoming != "" {
			r.Header.Set("X-Request-ID", incoming)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if seen == "" || seen == incoming || len(seen) != 36 {
			t.Errorf("incoming %q: generated id %q is not a UUID", incoming, seen)
		}
		if w.Header().Get("X-Request-ID") != seen {
			t.Errorf("response header %q != %q", w.Header().Get("X-Request-ID"), seen)
		}
	}
}

func TestLoggingEmitsOneLineWithoutBody(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /things", func(w http.ResponseWriter, r *http.Request) {
		LoggerFrom(r.Context()).Info("inside handler")
		w.WriteHeader(http.StatusCreated)
	})
	h := RequestID(Logging(logger)(mux))

	r := httptest.NewRequest(http.MethodPost, "/things", strings.NewReader(`{"secret":"Budi Santoso"}`))
	h.ServeHTTP(httptest.NewRecorder(), r)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines (handler + access), got %d: %s", len(lines), buf.String())
	}
	if strings.Contains(buf.String(), "Budi Santoso") {
		t.Error("request body leaked into logs")
	}
	var handlerLine, access map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &handlerLine); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &access); err != nil {
		t.Fatal(err)
	}
	if handlerLine["request_id"] == nil || handlerLine["request_id"] != access["request_id"] {
		t.Errorf("request_id not propagated: %v vs %v", handlerLine["request_id"], access["request_id"])
	}
	for _, k := range []string{"method", "route", "status", "duration_ms"} {
		if _, ok := access[k]; !ok {
			t.Errorf("access log missing %q: %v", k, access)
		}
	}
	if access["route"] != "POST /things" || access["status"] != float64(201) {
		t.Errorf("route/status = %v/%v", access["route"], access["status"])
	}
}

func TestRecoverWritesProblem(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	h := Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "internal-error") {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestMetricsRecordsRouteAndCodeClass(t *testing.T) {
	reg := prometheus.NewRegistry()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := Metrics(reg)(mux)

	for range 3 {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))

	expected := `
# HELP http_requests_total Total HTTP requests by route and status class.
# TYPE http_requests_total counter
http_requests_total{code="2xx",route="GET /ok"} 3
http_requests_total{code="4xx",route="unmatched"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "http_requests_total"); err != nil {
		t.Error(err)
	}
	if n, err := testutil.GatherAndCount(reg, "http_request_duration_seconds"); err != nil || n != 2 {
		t.Errorf("duration histogram series = %d, err %v", n, err)
	}
}
