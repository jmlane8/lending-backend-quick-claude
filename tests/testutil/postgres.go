// Package testutil provides shared fixtures for integration and contract tests:
// a disposable PostgreSQL 18 container, a JWT signer, and a fixed clock.
package testutil

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/jmlane8/lending-backend-quick-claude/internal/db"
)

var (
	pgOnce sync.Once
	pgURL  string
	pgErr  error
)

// PostgresURL starts one PostgreSQL 18 container per test binary (reaped by
// Testcontainers when the process exits), applies migrations, and returns its URL.
func PostgresURL(t testing.TB) string {
	t.Helper()
	pgOnce.Do(func() {
		ctx := context.Background()
		var c *tcpostgres.PostgresContainer
		c, pgErr = tcpostgres.Run(ctx, "postgres:18",
			tcpostgres.WithDatabase("lending"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			tcpostgres.BasicWaitStrategies(),
		)
		if pgErr != nil {
			return
		}
		pgURL, pgErr = c.ConnectionString(ctx, "sslmode=disable")
		if pgErr != nil {
			return
		}
		pgErr = db.Up(pgURL)
	})
	if pgErr != nil {
		t.Fatalf("start postgres: %v", pgErr)
	}
	return pgURL
}

// StartPostgres returns a pool to a migrated, empty database. Tables are truncated
// before the test, so tests in one package must not run in parallel with each other.
func StartPostgres(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, PostgresURL(t), 30)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	TruncateAll(t, pool)
	return pool
}

// TruncateAll empties every application table.
func TruncateAll(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		DO $$
		DECLARE r record;
		BEGIN
			FOR r IN SELECT tablename FROM pg_tables
			         WHERE schemaname = 'public' AND tablename <> 'schema_migrations'
			LOOP
				EXECUTE format('TRUNCATE TABLE %I CASCADE', r.tablename);
			END LOOP;
		END $$`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
