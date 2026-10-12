package auth

import (
	"crypto"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jmlane8/lending-backend-quick-claude/internal/httpx"
)

// Claims are the verified token claims this service uses.
type Claims struct {
	Subject string
	Scopes  []string
}

// Verifier validates bearer tokens signed with a configured public key.
type Verifier struct {
	key    crypto.PublicKey
	parser *jwt.Parser
}

// NewVerifier builds a verifier that accepts ES256/ES384/RS256 tokens issued by
// issuer for audience, with 30s clock leeway and a mandatory expiry.
func NewVerifier(key crypto.PublicKey, issuer, audience string) *Verifier {
	return &Verifier{
		key: key,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{"ES256", "ES384", "RS256"}),
			jwt.WithIssuer(issuer),
			jwt.WithAudience(audience),
			jwt.WithLeeway(30*time.Second),
			jwt.WithExpirationRequired(),
		),
	}
}

type tokenClaims struct {
	jwt.RegisteredClaims
	Scope string `json:"scope"`
}

// Verify checks the token signature and registered claims.
func (v *Verifier) Verify(token string) (Claims, error) {
	var tc tokenClaims
	if _, err := v.parser.ParseWithClaims(token, &tc, func(*jwt.Token) (any, error) { return v.key, nil }); err != nil {
		return Claims{}, fmt.Errorf("verify token: %w", err)
	}
	if tc.Subject == "" {
		return Claims{}, errors.New("verify token: empty subject")
	}
	return Claims{Subject: tc.Subject, Scopes: strings.Fields(tc.Scope)}, nil
}

// Middleware rejects requests without a valid bearer token (401) or without
// requiredScope (403), and stores the token subject as the actor.
func Middleware(v *Verifier, requiredScope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				unauthenticated(w, r)
				return
			}
			claims, err := v.Verify(token)
			if err != nil {
				httpx.LoggerFrom(r.Context()).Info("rejected bearer token", "reason", err.Error())
				unauthenticated(w, r)
				return
			}
			if !slices.Contains(claims.Scopes, requiredScope) {
				httpx.WriteProblem(w, r, httpx.Problem{
					Status: http.StatusForbidden,
					Type:   "forbidden",
					Title:  "Forbidden",
					Detail: "The token lacks the required scope " + requiredScope + ".",
				})
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), claims.Subject)))
		})
	}
}

func unauthenticated(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="lending-api"`)
	httpx.WriteProblem(w, r, httpx.Problem{
		Status: http.StatusUnauthorized,
		Type:   "unauthenticated",
		Title:  "Authentication required",
		Detail: "A valid bearer token is required.",
	})
}
