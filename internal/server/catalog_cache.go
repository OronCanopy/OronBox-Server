package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zxor-org/OronBox-Server/internal/store"
	"golang.org/x/sync/singleflight"
)

const catalogRefreshInterval = 15 * time.Minute

type catalogCache struct {
	refresh singleflight.Group
	github  githubReleaseClient
	astro   astroBoxCatalog
}

var catalogHTTP = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

var errCatalogRetry = errors.New("upstream catalog retry delayed")

type catalogRequest func(context.Context, string) (*http.Response, error)

// A zero TTL revalidates the snapshot, and a negative TTL keeps immutable content.
func (a *App) cachedCatalog(ctx context.Context, key string, ttl time.Duration, slots chan struct{}, fetch catalogRequest, validate func([]byte) error) (store.CatalogSnapshot, error) {
	read := func(ctx context.Context) (store.CatalogSnapshot, bool, error) {
		value, err := a.store.CatalogSnapshot(ctx, key)
		if err != nil {
			return value, false, err
		}
		if len(value.Body) > 0 && (ttl < 0 || time.Since(value.CheckedAt) < ttl) {
			return value, false, nil
		}
		if value.RetryAt.Valid && time.Now().Before(value.RetryAt.Time) {
			return value, false, errCatalogRetry
		}
		return value, true, nil
	}
	value, refresh, err := read(ctx)
	if !refresh {
		return value, err
	}
	result := a.catalogs.refresh.DoChan(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
		defer cancel()
		previous, refresh, err := read(ctx)
		if !refresh {
			return previous, err
		}
		if slots != nil {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return previous, ctx.Err()
			}
		}
		next, err := fetchCatalogSnapshot(ctx, previous, fetch, validate)
		if err != nil {
			if saveErr := a.store.DelayCatalogRefresh(ctx, key, time.Now().Add(time.Minute)); saveErr != nil {
				err = errors.Join(err, saveErr)
			}
			return previous, err
		}
		if err := a.store.SaveCatalogSnapshot(ctx, key, next); err != nil {
			return previous, err
		}
		return next, nil
	})
	select {
	case <-ctx.Done():
		return value, ctx.Err()
	case result := <-result:
		return result.Val.(store.CatalogSnapshot), result.Err
	}
}

func fetchCatalogSnapshot(ctx context.Context, previous store.CatalogSnapshot, fetch catalogRequest, validate func([]byte) error) (store.CatalogSnapshot, error) {
	res, err := fetch(ctx, previous.ETag)
	if err != nil {
		return store.CatalogSnapshot{}, err
	}
	defer res.Body.Close()
	now := time.Now().UTC()
	if res.StatusCode == http.StatusNotModified && len(previous.Body) > 0 {
		previous.CheckedAt = now
		return previous, nil
	}
	if res.StatusCode != http.StatusOK {
		return store.CatalogSnapshot{}, fmt.Errorf("upstream catalog returned HTTP %d", res.StatusCode)
	}
	const maxSize = 8 << 20
	body, err := io.ReadAll(io.LimitReader(res.Body, maxSize+1))
	if err != nil {
		return store.CatalogSnapshot{}, err
	}
	if len(body) == 0 || len(body) > maxSize {
		return store.CatalogSnapshot{}, errors.New("invalid upstream catalog size")
	}
	if err := validate(body); err != nil {
		return store.CatalogSnapshot{}, err
	}
	return store.CatalogSnapshot{
		Body: body, ETag: res.Header.Get("ETag"), CheckedAt: now,
		HasNext: bytes.Contains([]byte(res.Header.Get("Link")), []byte(`rel="next"`)),
	}, nil
}

func catalogGet(ctx context.Context, endpoint, etag string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err == nil && etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	return req, err
}

func serveCatalog(w http.ResponseWriter, r *http.Request, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(body)))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}
