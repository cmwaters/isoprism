package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a connection pool to the Supabase Postgres database.
//
// Supabase's connection pooler (PgBouncer) runs in Transaction mode, which is
// incompatible with pgx's default prepared-statement cache — PgBouncer doesn't
// maintain per-session state across transactions, so prepared statements
// disappear between requests and cause "prepared statement already exists" errors.
//
// Fix: switch to SimpleProtocol (plain text queries, no prepared statements).
// Slightly slower per query but correct and still fast enough for this workload.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to parse DSN: %w", err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to reach database: %w", err)
	}
	return pool, nil
}
