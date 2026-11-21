package schema

import (
	"context"
	"database/sql"
)

const upstreamCatalogSchemaV23 = `
CREATE TABLE IF NOT EXISTS upstream_catalog_cache (
    cache_key TEXT PRIMARY KEY,
    body BYTEA,
    etag TEXT NOT NULL DEFAULT '',
    has_next BOOLEAN NOT NULL DEFAULT false,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    retry_at TIMESTAMPTZ
);`

func migrateSchemaV23(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, upstreamCatalogSchemaV23); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version)VALUES(23)`)
	return err
}
