// Package db opens the PostgreSQL pool, runs migrations, and classifies database errors.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/puddle/v2"
)

// Open creates a connection pool and verifies connectivity.
func Open(ctx context.Context, databaseURL string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := Ping(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Ping checks connectivity with a 1 second timeout (used by /readyz).
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

// IsUnavailable reports whether err means the database is temporarily unreachable,
// so the client should retry (503) rather than treat it as a server bug (500).
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, puddle.ErrClosedPool) {
		return true
	}
	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "08") ||
			strings.HasPrefix(pgErr.Code, "53") ||
			pgErr.Code == "57P01" || pgErr.Code == "57P02" || pgErr.Code == "57P03"
	}
	return pgconn.SafeToRetry(err)
}
