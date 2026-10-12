package loanapplication

import (
	"bytes"
	"strings"
	"testing"
)

func TestRequestHash(t *testing.T) {
	a, err := RequestHash([]byte(`{"loan":{"amount":100,"tenureMonths":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 32 {
		t.Errorf("hash length = %d, want 32 (SHA-256)", len(a))
	}
	b, err := RequestHash([]byte("{\n  \"loan\": { \"amount\": 100, \"tenureMonths\": 3 }\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("whitespace-only difference changed the hash")
	}
	c, err := RequestHash([]byte(`{"loan":{"amount":200,"tenureMonths":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, c) {
		t.Error("different amount produced the same hash")
	}
	if _, err := RequestHash([]byte(`{not json`)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestValidIdempotencyKey(t *testing.T) {
	tests := map[string]bool{
		"0192a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b": true,
		"a":                                    true,
		strings.Repeat("k", 255):               true,
		"!~":                                   true,
		"":                                     false,
		strings.Repeat("k", 256):               false,
		"has space":                            false,
		"tab\there":                            false,
		"ключ":                                 false,
	}
	for key, want := range tests {
		if got := ValidIdempotencyKey(key); got != want {
			t.Errorf("ValidIdempotencyKey(%q) = %v, want %v", key, got, want)
		}
	}
}
