package db

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/puddle/v2"
)

func TestIsUnavailable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection failure 08006", &pgconn.PgError{Code: "08006"}, true},
		{"too many connections 53300", &pgconn.PgError{Code: "53300"}, true},
		{"admin shutdown 57P01", &pgconn.PgError{Code: "57P01"}, true},
		{"cannot connect now 57P03", &pgconn.PgError{Code: "57P03"}, true},
		{"unique violation 23505", &pgconn.PgError{Code: "23505"}, false},
		{"check violation 23514", &pgconn.PgError{Code: "23514"}, false},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped deadline", fmt.Errorf("query: %w", context.DeadlineExceeded), true},
		{"connect error", &pgconn.ConnectError{}, true},
		{"closed pool", fmt.Errorf("acquire: %w", puddle.ErrClosedPool), true},
		{"other", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnavailable(tt.err); got != tt.want {
				t.Errorf("IsUnavailable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
