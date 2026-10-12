package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func testPublicKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func validEnv(t *testing.T) map[string]string {
	return map[string]string{
		"DATABASE_URL":        "postgres://u:p@localhost:5432/db",
		"APP_CURRENCY":        "IDR",
		"APP_TIMEZONE":        "Asia/Jakarta",
		"AUTH_JWT_ISSUER":     "dev",
		"AUTH_JWT_AUDIENCE":   "lending-api",
		"AUTH_JWT_PUBLIC_KEY": testPublicKeyPEM(t),
	}
}

func getenv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadValidWithDefaults(t *testing.T) {
	cfg, err := Load(getenv(validEnv(t)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.ErrorTypeBaseURL != "https://errors.lending.example/" {
		t.Errorf("ErrorTypeBaseURL = %q", cfg.ErrorTypeBaseURL)
	}
	if cfg.ShutdownTimeout != 20*time.Second {
		t.Errorf("ShutdownTimeout = %v", cfg.ShutdownTimeout)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns = %d", cfg.DBMaxConns)
	}
	if cfg.Currency != "IDR" || cfg.Location.String() != "Asia/Jakarta" {
		t.Errorf("Currency/Location = %q/%v", cfg.Currency, cfg.Location)
	}
	if cfg.JWTPublicKey == nil {
		t.Error("JWTPublicKey is nil")
	}
}

func TestLoadOverrides(t *testing.T) {
	env := validEnv(t)
	env["HTTP_ADDR"] = ":9090"
	env["SHUTDOWN_TIMEOUT"] = "5s"
	env["DB_MAX_CONNS"] = "25"
	env["ERROR_TYPE_BASE_URL"] = "https://errors.example.com/"
	cfg, err := Load(getenv(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.ShutdownTimeout != 5*time.Second || cfg.DBMaxConns != 25 ||
		cfg.ErrorTypeBaseURL != "https://errors.example.com/" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr []string
	}{
		{"missing database url", func(m map[string]string) { delete(m, "DATABASE_URL") }, []string{"DATABASE_URL"}},
		{"missing currency", func(m map[string]string) { delete(m, "APP_CURRENCY") }, []string{"APP_CURRENCY"}},
		{"lowercase currency", func(m map[string]string) { m["APP_CURRENCY"] = "idr" }, []string{"APP_CURRENCY"}},
		{"missing timezone", func(m map[string]string) { delete(m, "APP_TIMEZONE") }, []string{"APP_TIMEZONE"}},
		{"bad timezone", func(m map[string]string) { m["APP_TIMEZONE"] = "Mars/Olympus" }, []string{"APP_TIMEZONE"}},
		{"missing issuer", func(m map[string]string) { delete(m, "AUTH_JWT_ISSUER") }, []string{"AUTH_JWT_ISSUER"}},
		{"missing audience", func(m map[string]string) { delete(m, "AUTH_JWT_AUDIENCE") }, []string{"AUTH_JWT_AUDIENCE"}},
		{"missing key", func(m map[string]string) { delete(m, "AUTH_JWT_PUBLIC_KEY") }, []string{"AUTH_JWT_PUBLIC_KEY"}},
		{"bad key", func(m map[string]string) { m["AUTH_JWT_PUBLIC_KEY"] = "not a pem" }, []string{"AUTH_JWT_PUBLIC_KEY"}},
		{"bad shutdown timeout", func(m map[string]string) { m["SHUTDOWN_TIMEOUT"] = "soon" }, []string{"SHUTDOWN_TIMEOUT"}},
		{"bad max conns", func(m map[string]string) { m["DB_MAX_CONNS"] = "0" }, []string{"DB_MAX_CONNS"}},
		{
			"reports all errors at once",
			func(m map[string]string) { delete(m, "DATABASE_URL"); delete(m, "APP_CURRENCY") },
			[]string{"DATABASE_URL", "APP_CURRENCY"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv(t)
			tt.mutate(env)
			_, err := Load(getenv(env))
			if err == nil {
				t.Fatal("expected error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %s", err, want)
				}
			}
		})
	}
}
