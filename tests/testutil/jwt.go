package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Test token settings shared by tests and the servers they build.
const (
	JWTIssuer   = "test-issuer"
	JWTAudience = "lending-api"
	SubmitScope = "loan-applications:submit"
)

// Signer mints ES256 tokens for tests.
type Signer struct {
	t   testing.TB
	key *ecdsa.PrivateKey
}

// NewSigner generates a fresh P-256 key pair.
func NewSigner(t testing.TB) *Signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &Signer{t: t, key: key}
}

// PublicKey returns the verification key.
func (s *Signer) PublicKey() *ecdsa.PublicKey { return &s.key.PublicKey }

// PublicKeyPEM returns the verification key as PEM (AUTH_JWT_PUBLIC_KEY format).
func (s *Signer) PublicKeyPEM() string {
	der, err := x509.MarshalPKIXPublicKey(&s.key.PublicKey)
	if err != nil {
		s.t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// Token returns a valid token for sub with the given space-separated scope.
func (s *Signer) Token(sub, scope string) string {
	tok, err := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss":   JWTIssuer,
		"aud":   JWTAudience,
		"sub":   sub,
		"scope": scope,
		"exp":   time.Now().Add(time.Hour).Unix(),
	}).SignedString(s.key)
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}
