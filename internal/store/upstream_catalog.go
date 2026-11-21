package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type CatalogSnapshot struct {
	Body      []byte
	ETag      string
	HasNext   bool
	CheckedAt time.Time
	RetryAt   sql.NullTime
}

func (s *Store) CatalogSnapshot(ctx context.Context, key string) (CatalogSnapshot, error) {
	var snapshot CatalogSnapshot
	err := s.db.QueryRowContext(ctx, `SELECT body, etag, has_next, checked_at, retry_at FROM upstream_catalog_cache WHERE cache_key=$1`, key).
		Scan(&snapshot.Body, &snapshot.ETag, &snapshot.HasNext, &snapshot.CheckedAt, &snapshot.RetryAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return snapshot, err
}

func (s *Store) SaveCatalogSnapshot(ctx context.Context, key string, value CatalogSnapshot) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO upstream_catalog_cache(cache_key,body,etag,has_next,checked_at) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(cache_key) DO UPDATE SET body=excluded.body,etag=excluded.etag,has_next=excluded.has_next,checked_at=excluded.checked_at,retry_at=NULL`,
		key, value.Body, value.ETag, value.HasNext, value.CheckedAt)
	return err
}

func (s *Store) DelayCatalogRefresh(ctx context.Context, key string, retryAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO upstream_catalog_cache(cache_key,retry_at) VALUES($1,$2)
		ON CONFLICT(cache_key) DO UPDATE SET retry_at=excluded.retry_at`, key, retryAt)
	return err
}

func (s *Store) PruneAstroBoxManifests(ctx context.Context, active []string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM upstream_catalog_cache WHERE cache_key LIKE 'astrobox/manifest/%'
		AND GREATEST(checked_at,retry_at) < now() - interval '1 day' AND NOT(cache_key=ANY($1))`, active)
	return err
}
