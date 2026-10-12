// Command devtoken mints a short-lived ES256 JWT for local development,
// signed with dev/jwt.key (generate with `make dev-keys`).
package main

import (
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	keyPath := flag.String("key", "dev/jwt.key", "PEM-encoded ECDSA private key")
	sub := flag.String("sub", "dev-user", "subject (actor) claim")
	scope := flag.String("scope", "loan-applications:submit", "space-separated scopes")
	flag.Parse()

	if err := run(*keyPath, *sub, *scope); err != nil {
		fmt.Fprintln(os.Stderr, "devtoken:", err)
		os.Exit(1)
	}
}

func run(keyPath, sub, scope string) error {
	raw, err := os.ReadFile(keyPath) // #nosec G304 -- dev-only tool reading a developer-supplied path
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return fmt.Errorf("%s: no PEM block found", keyPath)
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse EC private key: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   envOr("AUTH_JWT_ISSUER", "dev"),
		"aud":   envOr("AUTH_JWT_AUDIENCE", "lending-api"),
		"sub":   sub,
		"scope": scope,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodES256, claims).SignedString(key)
	if err != nil {
		return fmt.Errorf("sign token: %w", err)
	}
	fmt.Println(signed)
	return nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
