package schema

import (
	"context"
	"database/sql"
)

const deviceModelsSchemaV22 = `
CREATE TABLE IF NOT EXISTS device_models (
    pd_id INTEGER PRIMARY KEY,
    model TEXT NOT NULL,
    product TEXT NOT NULL,
    id TEXT NOT NULL,
    codename TEXT NOT NULL,
    name TEXT NOT NULL,
    vendor TEXT NOT NULL DEFAULT 'xiaomi',
    feature JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_device_models_model ON device_models(model);
CREATE INDEX IF NOT EXISTS idx_device_models_product ON device_models(product);
CREATE INDEX IF NOT EXISTS idx_device_models_codename ON device_models(codename);
`

func migrateSchemaV22(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, deviceModelsSchemaV22); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version)VALUES(22)`)
	return err
}
