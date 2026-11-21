package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zxor-org/OronBox-Server/internal/config"
	"github.com/zxor-org/OronBox-Server/internal/store"
)

func TestFirmwareCatalogReusesPersistentSnapshot(t *testing.T) {
	db := importVerificationDB(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/AstralSightStudios/MiWearFirmwareArchives/releases" || r.URL.Query().Get("page") != "1" {
			t.Error("unexpected upstream request", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("credential sent to a different host")
		}
		switch calls.Add(1) {
		case 1:
			w.Header().Set("ETag", `"release"`)
			w.Header().Set("Link", `<https://api.github.com/example?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"id":1,"name":"miwear.watch.o66 v1.2.3"}]`)
		case 2:
			if r.Header.Get("If-None-Match") != `"release"` {
				t.Error("upstream ETag missing on cache revalidation")
			}
			w.WriteHeader(http.StatusNotModified)
		default:
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer upstream.Close()
	cfg := config.Config{GitHub: config.GitHubConfig{APIURL: upstream.URL, ReleaseToken: "test-only"}}
	app := &App{store: store.New(db), cfg: cfg}
	request := func(app *App, etag string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/firmware/xiaomi/catalog", nil)
		r.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		app.handleXiaomiFirmwareCatalog(w, r)
		return w
	}
	expire := func() {
		t.Helper()
		if _, err := db.Exec(`UPDATE upstream_catalog_cache SET checked_at=now()-interval '1 hour'`); err != nil {
			t.Fatal(err)
		}
	}
	first := request(app, "")
	if first.Code != 200 || first.Header().Get("ETag") == "" || first.Body.String() != `{"releases":[{"id":1,"name":"miwear.watch.o66 v1.2.3"}],"has_next":true}` {
		t.Fatalf("unexpected catalog response %d %s", first.Code, first.Body)
	}
	app = &App{store: store.New(db), cfg: cfg}
	if cached := request(app, first.Header().Get("ETag")); cached.Code != 304 || calls.Load() != 1 {
		t.Fatalf("restarted app did not use snapshot: HTTP %d, %d upstream requests", cached.Code, calls.Load())
	}
	expire()
	if revalidated := request(app, first.Header().Get("ETag")); revalidated.Code != 304 || calls.Load() != 2 {
		t.Fatalf("snapshot revalidation failed: HTTP %d, %d upstream requests", revalidated.Code, calls.Load())
	}
	expire()
	for range 2 {
		if stale := request(app, ""); stale.Code != 200 || stale.Body.String() != first.Body.String() {
			t.Fatalf("rate-limited upstream replaced the snapshot: HTTP %d %s", stale.Code, stale.Body)
		}
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("rate-limited requests were retried: %d upstream requests", n)
	}
}
