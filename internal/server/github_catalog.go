package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zxor-org/OronBox-Server/internal/httpx"
	"github.com/zxor-org/OronBox-Server/internal/observability"
	"github.com/zxor-org/OronBox-Server/internal/store"
)

type releaseRepository string

const (
	appReleases     releaseRepository = "OronCanopy/OronBox"
	xiaomiReleases  releaseRepository = "AstralSightStudios/MiWearFirmwareArchives"
	maxReleasePages                   = 50
)

type githubReleaseClient struct {
	mu           sync.Mutex
	blockedUntil time.Time
}

func (a *App) releasePage(ctx context.Context, repo releaseRepository, page int, ttl time.Duration) (store.CatalogSnapshot, error) {
	key := fmt.Sprintf("github/releases/%s/%d", repo, page)
	return a.cachedCatalog(ctx, key, ttl, nil, func(ctx context.Context, etag string) (*http.Response, error) {
		return a.fetchReleasePage(ctx, repo, page, etag)
	}, validateReleasePage)
}

func validateReleasePage(body []byte) error {
	var releases []struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(body, &releases) != nil || releases == nil {
		return errors.New("invalid GitHub Release list")
	}
	for _, release := range releases {
		if release.ID <= 0 {
			return errors.New("invalid GitHub Release ID")
		}
	}
	return nil
}

func (a *App) fetchReleasePage(ctx context.Context, repo releaseRepository, page int, etag string) (*http.Response, error) {
	if repo != appReleases && repo != xiaomiReleases {
		return nil, errors.New("unknown Release repository")
	}
	client := &a.catalogs.github
	client.mu.Lock()
	blocked := time.Now().Before(client.blockedUntil)
	client.mu.Unlock()
	if blocked {
		return nil, errCatalogRetry
	}
	endpoint := fmt.Sprintf("%s/repos/%s/releases?per_page=100&page=%d", strings.TrimRight(a.cfg.GitHub.APIURL, "/"), repo, page)
	req, err := catalogGet(ctx, endpoint, etag)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := a.cfg.GitHub.ReleaseToken; token != "" && req.URL.Scheme == "https" && req.URL.Host == "api.github.com" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := catalogHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if until := githubReleaseRetryAt(res, time.Now()); !until.IsZero() {
		client.mu.Lock()
		if client.blockedUntil.Before(until) {
			client.blockedUntil = until
		}
		client.mu.Unlock()
	}
	observability.For("github.releases").Debug("GitHub Release page requested", "repo", repo, "page", page,
		"status", res.StatusCode, "rate_limit_remaining", res.Header.Get("X-RateLimit-Remaining"))
	return res, nil
}

func githubReleaseRetryAt(res *http.Response, now time.Time) time.Time {
	if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusTooManyRequests {
		return time.Time{}
	}
	if raw := res.Header.Get("Retry-After"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return now.Add(time.Duration(seconds) * time.Second)
		}
		if when, err := http.ParseTime(raw); err == nil && when.After(now) {
			return when
		}
	}
	if res.Header.Get("X-RateLimit-Remaining") == "0" {
		if seconds, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && time.Unix(seconds, 0).After(now) {
			return time.Unix(seconds, 0)
		}
	}
	return now.Add(time.Minute)
}

func (a *App) handleXiaomiFirmwareCatalog(w http.ResponseWriter, r *http.Request) {
	page := 1
	if query := r.URL.Query().Get("page"); query != "" {
		value, err := strconv.Atoi(query)
		if err != nil || value < 1 || value > maxReleasePages {
			httpx.WriteJSON(w, http.StatusBadRequest, httpx.ErrorBody("invalid_page", "Invalid Release page"))
			return
		}
		page = value
	}
	value, err := a.releasePage(r.Context(), xiaomiReleases, page, catalogRefreshInterval)
	if len(value.Body) == 0 {
		if err == nil {
			err = errors.New("firmware snapshot is unavailable")
		}
		httpx.WriteJSON(w, http.StatusServiceUnavailable, httpx.ErrorBody("firmware_catalog_unavailable", err.Error()))
		return
	}
	if err != nil && !errors.Is(err, errCatalogRetry) {
		observability.For("github.releases").Warn("Serving cached Xiaomi firmware list", "page", page, "last_success", value.CheckedAt, "error", err)
	}
	body, _ := json.Marshal(struct {
		Releases json.RawMessage `json:"releases"`
		HasNext  bool            `json:"has_next"`
	}{bytes.TrimSpace(value.Body), value.HasNext && page < maxReleasePages})
	serveCatalog(w, r, "application/json; charset=utf-8", body)
}
