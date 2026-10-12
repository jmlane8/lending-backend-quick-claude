package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const scope = "loan-applications:submit"

type fixture struct {
	key      *ecdsa.PrivateKey
	verifier *Verifier
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{key: key, verifier: NewVerifier(&key.PublicKey, "dev", "lending-api")}
}

func (f fixture) token(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":   "dev",
		"aud":   "lending-api",
		"sub":   "user-42",
		"scope": "profile " + scope,
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	if mutate != nil {
		mutate(claims)
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodES256, claims).SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func serve(f fixture, authz string) (*httptest.ResponseRecorder, string) {
	var actor string
	h := Middleware(f.verifier, scope)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, _ = ActorFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodPost, "/v1/loan-applications", nil)
	if authz != "" {
		r.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w, actor
}

func TestMiddlewareAcceptsValidToken(t *testing.T) {
	f := newFixture(t)
	w, actor := serve(f, "Bearer "+f.token(t, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if actor != "user-42" {
		t.Errorf("actor = %q", actor)
	}
}

func TestMiddlewareRejects(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)

	tests := []struct {
		name     string
		authz    string
		status   int
		wantType string
	}{
		{"missing header", "", 401, "unauthenticated"},
		{"not bearer", "Basic abc", 401, "unauthenticated"},
		{"garbage token", "Bearer not.a.jwt", 401, "unauthenticated"},
		{"bad signature", "Bearer " + other.token(t, nil), 401, "unauthenticated"},
		{"expired", "Bearer " + f.token(t, func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }), 401, "unauthenticated"},
		{"no exp", "Bearer " + f.token(t, func(c jwt.MapClaims) { delete(c, "exp") }), 401, "unauthenticated"},
		{"wrong issuer", "Bearer " + f.token(t, func(c jwt.MapClaims) { c["iss"] = "evil" }), 401, "unauthenticated"},
		{"wrong audience", "Bearer " + f.token(t, func(c jwt.MapClaims) { c["aud"] = "other-api" }), 401, "unauthenticated"},
		{"empty sub", "Bearer " + f.token(t, func(c jwt.MapClaims) { c["sub"] = "" }), 401, "unauthenticated"},
		{"missing scope", "Bearer " + f.token(t, func(c jwt.MapClaims) { c["scope"] = "profile" }), 403, "forbidden"},
		{"scope prefix only", "Bearer " + f.token(t, func(c jwt.MapClaims) { c["scope"] = "loan-applications:submit-x" }), 403, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, _ := serve(f, tt.authz)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d", w.Code, tt.status)
			}
			if !strings.Contains(w.Body.String(), tt.wantType) {
				t.Errorf("body %s does not contain %s", w.Body.String(), tt.wantType)
			}
			if tt.status == 401 && w.Header().Get("WWW-Authenticate") == "" {
				t.Error("missing WWW-Authenticate header")
			}
		})
	}
}

func TestMiddlewareRejectsHS256AlgConfusion(t *testing.T) {
	f := newFixture(t)
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "dev", "aud": "lending-api", "sub": "x", "scope": scope, "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := serve(f, "Bearer "+tok); w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
