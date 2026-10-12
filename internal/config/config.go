// Package config loads and validates service configuration from the environment.
// Startup fails fast with every problem reported at once.
package config

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Config is the validated service configuration.
type Config struct {
	DatabaseURL      string
	HTTPAddr         string
	Currency         string
	Location         *time.Location
	JWTIssuer        string
	JWTAudience      string
	JWTPublicKey     crypto.PublicKey
	ErrorTypeBaseURL string
	ShutdownTimeout  time.Duration
	DBMaxConns       int32
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Load reads configuration through getenv (os.Getenv in production) and returns
// all validation failures joined into one error.
func Load(getenv func(string) string) (Config, error) {
	var (
		cfg  Config
		errs []error
	)

	required := func(name string) string {
		v := getenv(name)
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
		return v
	}
	withDefault := func(name, fallback string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return fallback
	}

	cfg.DatabaseURL = required("DATABASE_URL")
	cfg.JWTIssuer = required("AUTH_JWT_ISSUER")
	cfg.JWTAudience = required("AUTH_JWT_AUDIENCE")
	cfg.HTTPAddr = withDefault("HTTP_ADDR", ":8080")
	cfg.ErrorTypeBaseURL = withDefault("ERROR_TYPE_BASE_URL", "https://errors.lending.example/")

	if v := required("APP_CURRENCY"); v != "" {
		if !currencyPattern.MatchString(v) {
			errs = append(errs, fmt.Errorf("APP_CURRENCY must be an ISO 4217 code such as IDR, got %q", v))
		}
		cfg.Currency = v
	}

	if v := required("APP_TIMEZONE"); v != "" {
		loc, err := time.LoadLocation(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("APP_TIMEZONE: %w", err))
		}
		cfg.Location = loc
	}

	if v := required("AUTH_JWT_PUBLIC_KEY"); v != "" {
		key, err := parsePublicKey(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("AUTH_JWT_PUBLIC_KEY: %w", err))
		}
		cfg.JWTPublicKey = key
	}

	timeout, err := time.ParseDuration(withDefault("SHUTDOWN_TIMEOUT", "20s"))
	if err != nil || timeout <= 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT must be a positive duration such as 20s"))
	}
	cfg.ShutdownTimeout = timeout

	maxConns, err := strconv.ParseInt(withDefault("DB_MAX_CONNS", "10"), 10, 32)
	if err != nil || maxConns < 1 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS must be a positive integer"))
	}
	cfg.DBMaxConns = int32(maxConns) // #nosec G115 -- bounded by ParseInt bitSize 32

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}

func parsePublicKey(pemText string) (crypto.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("not a PEM-encoded public key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return key, nil
}
