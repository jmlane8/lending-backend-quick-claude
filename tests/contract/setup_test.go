//go:build integration

// Package contract exercises the real HTTP handler against PostgreSQL and checks
// requests and responses against api/openapi.yaml.
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jmlane8/lending-backend-quick-claude/internal/auth"
	"github.com/jmlane8/lending-backend-quick-claude/internal/db"
	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
	"github.com/jmlane8/lending-backend-quick-claude/internal/loanapplication"
	"github.com/jmlane8/lending-backend-quick-claude/internal/server"
	"github.com/jmlane8/lending-backend-quick-claude/tests/testutil"
)

const currency = "IDR"

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	srv    *httptest.Server
	signer *testutil.Signer
	router routers.Router
	logs   *bytes.Buffer
}

// newEnv builds the full handler (real PostgreSQL, real auth, fixed clock) and
// loads the OpenAPI contract.
func newEnv(t *testing.T) *env {
	t.Helper()
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	httpx.SetDisplayLocation(jakarta)
	t.Cleanup(func() { httpx.SetDisplayLocation(time.UTC) })

	pool := testutil.StartPostgres(t)
	signer := testutil.NewSigner(t)
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	reg := prometheus.NewRegistry()

	svc := loanapplication.NewService(pool, loanapplication.ServiceConfig{
		Currency: currency, Location: jakarta, Now: testutil.FixedClock(), Registerer: reg,
	})
	verifier := auth.NewVerifier(signer.PublicKey(), testutil.JWTIssuer, testutil.JWTAudience)
	handler := server.New(server.Deps{
		Logger:   logger,
		Registry: reg,
		Ping:     func(ctx context.Context) error { return db.Ping(ctx, pool) },
		PoolStat: pool.Stat,
		Routes:   []func(*http.ServeMux){loanapplication.NewHandler(svc, verifier).Register},
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(repoRoot(), "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("load contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("contract is invalid: %v", err)
	}
	doc.Servers = openapi3.Servers{{URL: srv.URL + "/v1"}}
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	return &env{t: t, pool: pool, srv: srv, signer: signer, router: router, logs: logs}
}

func example(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(), "dev", "examples", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

// set assigns a value at a dotted path, e.g. set(body, "loan.amount", 100).
func set(m map[string]any, path []string, v any) {
	for _, k := range path[:len(path)-1] {
		m = m[k].(map[string]any)
	}
	if v == nil {
		delete(m, path[len(path)-1])
		return
	}
	m[path[len(path)-1]] = v
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("response is not JSON: %s", r.body)
	}
	return m
}

type call struct {
	body            any    // marshaled to JSON unless rawBody is set
	rawBody         []byte // sent verbatim
	token           string // "" means a valid submit token; "-" means none
	idempotencyKey  string // "" means a fresh UUID; "-" means none
	validateRequest bool   // only for requests that are expected to conform
}

// submit POSTs to /v1/loan-applications and always validates the response against
// the contract (and the request too when validateRequest is set).
func (e *env) submit(c call) response {
	t := e.t
	t.Helper()
	payload := c.rawBody
	if payload == nil {
		var err error
		if payload, err = json.Marshal(c.body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/loan-applications", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	switch c.token {
	case "":
		req.Header.Set("Authorization", "Bearer "+e.signer.Token("officer-7", testutil.SubmitScope))
	case "-":
	default:
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	switch c.idempotencyKey {
	case "":
		req.Header.Set("Idempotency-Key", uuid.NewString())
	case "-":
	default:
		req.Header.Set("Idempotency-Key", c.idempotencyKey)
	}

	route, pathParams, err := e.router.FindRoute(req)
	if err != nil {
		t.Fatalf("contract has no route for request: %v", err)
	}
	reqInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options:    &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}
	if c.validateRequest {
		reqCopy := req.Clone(context.Background())
		reqCopy.Body = io.NopCloser(bytes.NewReader(payload))
		reqInput.Request = reqCopy
		if err := openapi3filter.ValidateRequest(context.Background(), reqInput); err != nil {
			t.Fatalf("request does not conform to contract: %v", err)
		}
		reqInput.Request = req
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	respInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: reqInput,
		Status:                 resp.StatusCode,
		Header:                 resp.Header,
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	respInput.SetBodyBytes(body)
	if err := openapi3filter.ValidateResponse(context.Background(), respInput); err != nil {
		t.Errorf("response %d does not conform to contract: %v\nbody: %s", resp.StatusCode, err, body)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: body}
}

func (e *env) countApplications() int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM loan_applications").Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}
