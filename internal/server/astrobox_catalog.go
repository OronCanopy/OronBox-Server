package server

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zxor-org/OronBox-Server/internal/httpx"
	"github.com/zxor-org/OronBox-Server/internal/observability"
	"github.com/zxor-org/OronBox-Server/internal/store"
)

const astroBoxRaw = "https://raw.githubusercontent.com/"
const astroBoxCatalogBase = astroBoxRaw + "AstralSightStudios/AstroBox-Repo/refs/heads/main/"

var (
	astroBoxRawRequests = make(chan struct{}, 4)
	astroBoxRepoPart    = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	astroBoxCommit      = regexp.MustCompile(`^[a-fA-F0-9]{7,40}$`)
	astroBoxZeroWidth   = strings.NewReplacer("\u200b", "", "\u200c", "", "\u200d", "", "\u2060", "", "\ufeff", "")
)

type astroBoxCatalog struct {
	mu   sync.Mutex
	hash [sha256.Size]byte
	rows map[string]astroBoxIndexRow
}

type astroBoxIndexRow map[string]string

func (row astroBoxIndexRow) manifestPath() string {
	owner, repo, commit := row["repo_owner"], row["repo_name"], row["repo_commit_hash"]
	if !astroBoxRepoPart.MatchString(owner) || owner == "." || owner == ".." ||
		!astroBoxRepoPart.MatchString(repo) || repo == "." || repo == ".." || !astroBoxCommit.MatchString(commit) {
		return ""
	}
	file := "manifest_v2.json"
	if strings.HasPrefix(row["id"], "LegacyItem") {
		file = "manifest.json"
	}
	return owner + "/" + repo + "/" + commit + "/" + file
}

func (c *astroBoxCatalog) indexRows(body []byte) (map[string]astroBoxIndexRow, error) {
	hash := sha256.Sum256(body)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows != nil && c.hash == hash {
		return c.rows, nil
	}
	reader := csv.NewReader(strings.NewReader(astroBoxZeroWidth.Replace(string(body))))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}
	columns := make(map[string]bool, len(headers))
	for i, header := range headers {
		headers[i] = strings.TrimSpace(header)
		columns[headers[i]] = true
	}
	for _, name := range []string{"id", "name", "restype", "repo_owner", "repo_name", "repo_commit_hash"} {
		if !columns[name] {
			return nil, errors.New("invalid AstroBox index columns")
		}
	}
	rows := map[string]astroBoxIndexRow{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) < len(headers) {
			return nil, errors.New("incomplete AstroBox index row")
		}
		row := make(astroBoxIndexRow, len(headers))
		for i, header := range headers {
			row[header] = strings.TrimSpace(record[i])
		}
		if id := row["id"]; id != "" && id != "<placeholder>" {
			rows[id] = row
		}
	}
	c.hash, c.rows = hash, rows
	return rows, nil
}

func validateObjectField(field string) func([]byte) error {
	return func(body []byte) error {
		var object map[string]json.RawMessage
		var value map[string]json.RawMessage
		if json.Unmarshal(body, &object) != nil || json.Unmarshal(object[field], &value) != nil || value == nil {
			return errors.New("invalid AstroBox " + field)
		}
		return nil
	}
}

func (a *App) astroBoxMetadata(ctx context.Context, key, endpoint string, ttl time.Duration, validate func([]byte) error) (store.CatalogSnapshot, error) {
	return a.cachedCatalog(ctx, key, ttl, astroBoxRawRequests, func(ctx context.Context, etag string) (*http.Response, error) {
		req, err := catalogGet(ctx, endpoint, etag)
		if err != nil {
			return nil, err
		}
		return catalogHTTP.Do(req)
	}, validate)
}

func (a *App) astroBoxIndex(ctx context.Context) (store.CatalogSnapshot, error) {
	return a.astroBoxMetadata(ctx, "astrobox/index", astroBoxCatalogBase+"index_v2.csv", catalogRefreshInterval, func(body []byte) error {
		_, err := a.catalogs.astro.indexRows(body)
		return err
	})
}

func (a *App) astroBoxDevices(ctx context.Context) (store.CatalogSnapshot, error) {
	return a.astroBoxMetadata(ctx, "astrobox/devices", astroBoxCatalogBase+"devices_v2.json", catalogRefreshInterval, validateObjectField("xiaomi"))
}

func (a *App) handleAstroBoxIndex(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.astroBoxIndex(r.Context())
	if len(snapshot.Body) == 0 {
		astroBoxUnavailable(w, err)
		return
	}
	serveCatalog(w, r, "text/csv; charset=utf-8", snapshot.Body)
}

func (a *App) handleAstroBoxDevices(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.astroBoxDevices(r.Context())
	if len(snapshot.Body) == 0 {
		astroBoxUnavailable(w, err)
		return
	}
	serveCatalog(w, r, "application/json; charset=utf-8", snapshot.Body)
}

func (a *App) handleAstroBoxResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	index, err := a.astroBoxIndex(ctx)
	if len(index.Body) == 0 {
		astroBoxUnavailable(w, err)
		return
	}
	rows, parseErr := a.catalogs.astro.indexRows(index.Body)
	if parseErr != nil {
		astroBoxUnavailable(w, parseErr)
		return
	}
	item := rows[r.PathValue("id")]
	if item == nil {
		if err != nil {
			astroBoxUnavailable(w, err)
		} else {
			httpx.WriteJSON(w, http.StatusNotFound, httpx.ErrorBody("astrobox_resource_not_found", "AstroBox resource not found"))
		}
		return
	}
	path := item.manifestPath()
	if path == "" {
		astroBoxUnavailable(w, errors.New("invalid AstroBox resource revision"))
		return
	}
	manifest, err := a.astroBoxMetadata(ctx, "astrobox/manifest/"+path, astroBoxRaw+path, -1, validateObjectField("item"))
	if len(manifest.Body) == 0 {
		astroBoxUnavailable(w, err)
		return
	}
	body, _ := json.Marshal(struct {
		Item     astroBoxIndexRow `json:"item"`
		Manifest json.RawMessage  `json:"manifest"`
	}{item, manifest.Body})
	serveCatalog(w, r, "application/json; charset=utf-8", body)
}

func astroBoxUnavailable(w http.ResponseWriter, err error) {
	if err == nil {
		err = errors.New("AstroBox snapshot is unavailable")
	}
	httpx.WriteJSON(w, http.StatusServiceUnavailable, httpx.ErrorBody("astrobox_catalog_unavailable", err.Error()))
}

func (a *App) RunAstroBoxCatalogSync(ctx context.Context) {
	ticker := time.NewTicker(catalogRefreshInterval)
	defer ticker.Stop()
	log := observability.For("astrobox.catalog")
	for ctx.Err() == nil {
		index, err := a.astroBoxIndex(ctx)
		if err == nil {
			rows, parseErr := a.catalogs.astro.indexRows(index.Body)
			err = parseErr
			if err == nil {
				active := make([]string, 0, len(rows))
				for _, item := range rows {
					if path := item.manifestPath(); path != "" {
						active = append(active, "astrobox/manifest/"+path)
					}
				}
				err = a.store.PruneAstroBoxManifests(ctx, active)
			}
		}
		if err != nil && ctx.Err() == nil {
			log.Warn("AstroBox index sync failed", "last_success", index.CheckedAt, "error", err)
		}
		devices, err := a.astroBoxDevices(ctx)
		if err != nil && ctx.Err() == nil {
			log.Warn("AstroBox device list sync failed", "last_success", devices.CheckedAt, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
