package schema

import (
	"context"
	"database/sql"
)

const moduleKindSupportV20 = `
ALTER TABLE resources DROP CONSTRAINT IF EXISTS resources_kind_check;
ALTER TABLE resources ADD CONSTRAINT resources_kind_check CHECK (kind IN ('quickapp','watchface','module'));
ALTER TABLE resource_collections DROP CONSTRAINT IF EXISTS resource_collections_kind_check;
ALTER TABLE resource_collections ADD CONSTRAINT resource_collections_kind_check CHECK (kind IN ('quickapp','watchface','module'));
`

func migrateSchemaV20(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, moduleKindSupportV20); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version)VALUES(20)`)
	return err
}
