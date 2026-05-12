package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const RequiredMigrationVersion = "019"

const latestMigrationVersionQuery = `
	select coalesce(max(version), '')
	from supabase_migrations.schema_migrations
`

func VerifyMigrationVersion(ctx context.Context, pool *pgxpool.Pool) error {
	var latest string
	if err := pool.QueryRow(ctx, latestMigrationVersionQuery).Scan(&latest); err != nil {
		return fmt.Errorf("query migration history: %w", err)
	}
	if latest != RequiredMigrationVersion {
		return fmt.Errorf("database migration version mismatch: api requires %s, database has %s", RequiredMigrationVersion, latest)
	}
	return nil
}
