package schema

import (
	"context"
	"database/sql"
)

const canopusKindMigrationV21 = `
ALTER TABLE resources DROP CONSTRAINT IF EXISTS resources_kind_check;
ALTER TABLE resources ADD CONSTRAINT resources_kind_check CHECK (kind IN ('quickapp','watchface','canopus'));
ALTER TABLE resource_collections DROP CONSTRAINT IF EXISTS resource_collections_kind_check;
ALTER TABLE resource_collections ADD CONSTRAINT resource_collections_kind_check CHECK (kind IN ('quickapp','watchface','canopus'));
`

func migrateSchemaV21(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, canopusKindMigrationV21); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version)VALUES(21)`)
	return err
}
