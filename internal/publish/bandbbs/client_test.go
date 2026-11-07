package bandbbs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTagLineFitsBandBBSCharacterLimit(t *testing.T) {
	value := truncateRunes(strings.Repeat("测", 101), 100)
	if got := len([]rune(value)); got != 100 {
		t.Fatalf("tag line length = %d, want 100", got)
	}
	if got := truncateRunes("short", 100); got != "short" {
		t.Fatalf("short tag line = %q", got)
	}
}

func TestFindMatchingVersionRequiresVersionAndFilename(t *testing.T) {
	versions := []versionSummary{
		{ID: 11, VersionString: "1.0", Files: []struct {
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
		}{{Filename: "old.rpk", Size: 10}}},
		{ID: 12, VersionString: "1.0", Files: []struct {
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
		}{{Filename: "new.rpk", Size: 20}}},
	}
	if got := findMatchingVersion(versions, "1.0", "new.rpk", 20); got == nil || got.ID != 12 {
		t.Fatalf("matched version = %#v, want version 12", got)
	}
	if got := findMatchingVersion(versions, "1.0", "missing.rpk", 20); got != nil {
		t.Fatalf("matched version = %#v, want no match", got)
	}
	if got := findMatchingVersion(versions, "", "new.rpk", 20); got != nil {
		t.Fatalf("matched version = %#v, want no match without version string", got)
	}
}

func TestFindMatchingUpdateRequiresExactContent(t *testing.T) {
	updates := []updateSummary{
		{ID: 21, Title: "更新", Message: "修复问题"},
	}
	if got := findMatchingUpdate(updates, "更新", "修复问题"); got == nil || got.ID != 21 {
		t.Fatalf("matched update = %#v, want update 21", got)
	}
	if got := findMatchingUpdate(updates, "更新", "其他内容"); got != nil {
		t.Fatalf("matched update = %#v, want no match", got)
	}
}

func TestPublishExternalPurchaseUsesResourceMetadata(t *testing.T) {
	t.Parallel()
	var submitted url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/resources/" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		submitted = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resource":{"resource_id":7568,"view_url":"https://www.bandbbs.cn/resources/7568/"}}`))
	}))
	defer server.Close()

	price := 100.0
	var snap snapshot
	snap.Revision.Name = "测试付费资源"
	snap.Revision.Summary = "外部购买测试"
	snap.Revision.PurchaseLink = "https://ifdian.net/item/example"
	snap.Revision.PurchasePrice = &price
	snap.Revision.PurchaseCurrency = "CNY"
	rawSnapshot, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	rawConfig, err := json.Marshal(config{
		Agreement: true,
		Targets:   []target{{CategoryID: 95}},
	})
	if err != nil {
		t.Fatal(err)
	}

	client := New(server.URL, nil, nil)
	result, err := client.Publish(context.Background(), "token", nil, rawSnapshot, rawConfig)
	if err != nil {
		t.Fatal(err)
	}
	if result.Resources["95"].ResourceID != "7568" {
		t.Fatalf("publish result = %#v", result)
	}
	for key, want := range map[string]string{
		"resource_type":         "external_purchase",
		"external_purchase_url": "https://ifdian.net/item/example",
		"price":                 "100.00",
		"currency":              "CNY",
	} {
		if got := submitted.Get(key); got != want {
			t.Errorf("form %s = %q, want %q", key, got, want)
		}
	}
	if submitted.Has("version_attachment_key") {
		t.Fatal("external purchase unexpectedly submitted a version attachment")
	}
}

func TestBandBBSSelectsExactArtifactWithinSharedPackageID(t *testing.T) {
	var snap snapshot
	if err := json.Unmarshal([]byte(`{"artifacts":[{"blob_sha256":"aaa","package_id":"same.app"},{"blob_sha256":"bbb","package_id":"same.app"}]}`), &snap); err != nil {
		t.Fatal(err)
	}
	index, err := snap.artifactIndex(target{CategoryID: 12, PackageID: "same.app", ArtifactSHA256: "bbb"}, false)
	if err != nil || index != 1 {
		t.Fatalf("selected %d, error %v", index, err)
	}
	if _, err := snap.artifactIndex(target{CategoryID: 12, PackageID: "same.app"}, false); err == nil {
		t.Fatal("ambiguous category package selected silently")
	}
	if _, err := snap.artifactIndex(target{CategoryID: 12, PackageID: "same.app", ArtifactSHA256: "missing"}, false); err == nil {
		t.Fatal("missing bound artifact accepted")
	}
}

func TestBandBBSRelatedLinksAndPartialFailure(t *testing.T) {
	var description string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.Form.Get("resource_category_id") == "13" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"errors":[{"message":"category rejected"}]}`))
			return
		}
		description = r.Form.Get("description")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"resource":{"resource_id":345}}`))
	}))
	defer server.Close()
	client := New(server.URL, nil, nil)
	raw := []byte(`{"revision":{"name":"Test","summary":"Short","description":"Description","purchase_link":"https://example.com/buy","purchase_price":1,"links":[{"title":"Project [site]","url":"https://example.com/project"}]}}`)
	cfg := []byte(`{"agreement":true,"targets":[{"category_id":12},{"category_id":13}]}`)
	result, err := client.Publish(context.Background(), "token", nil, raw, cfg)
	if err == nil {
		t.Fatal("expected second category failure")
	}
	if result.Resources["12"].ResourceID != "345" || result.Resources["12"].URL != "https://www.bandbbs.cn/resources/345/" {
		t.Fatalf("lost completed category and URL: %#v", result)
	}
	if !strings.Contains(description, "[URL=https://example.com/project]Project &#91;site&#93;[/URL]") || !strings.Contains(description, "Description") {
		t.Fatalf("related link not rendered safely: %s", description)
	}
	unsafe := strings.Replace(string(raw), "https://example.com/project", "https://example.com/][IMG]", 1)
	if _, err := client.Publish(context.Background(), "token", nil, []byte(unsafe), cfg); err == nil {
		t.Fatal("unsafe link allowed before external writes")
	}
}

func TestBandBBSOverwritePreviousUpdateEditsInPlace(t *testing.T) {
	var (
		editedUpdateID string
		createdUpdate  bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resources/4201/updates":
			_, _ = w.Write([]byte(`{"updates":[{"resource_update_id":12900,"title":"旧标题","message":"旧内容","can_edit":true}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/resource-updates/12900/":
			_ = r.ParseForm()
			editedUpdateID = "12900"
			if r.PostForm.Get("title") != "新版本标题" || r.PostForm.Get("message") != "新版本说明" {
				t.Errorf("unexpected edit update body: %v", r.PostForm)
			}
			_, _ = w.Write([]byte(`{"success":true,"update":{"resource_update_id":12900}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/resource-updates/":
			createdUpdate = true
			_, _ = w.Write([]byte(`{"success":true,"update":{"resource_update_id":12999}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, nil, nil)

	// 1. 测试 overwritePrevious: true 且内容变更时原地编辑
	res, err := client.createUpdate(context.Background(), "token", "4201", "新版本标题", "新版本说明", false, true)
	if err != nil {
		t.Fatalf("createUpdate failed: %v", err)
	}
	if res.ID != "12900" {
		t.Errorf("update ID = %q, want 12900", res.ID)
	}
	if editedUpdateID != "12900" {
		t.Errorf("edited update ID = %q, want 12900", editedUpdateID)
	}
	if createdUpdate {
		t.Error("unexpectedly created a new update")
	}

	// 2. 测试 overwritePrevious: true 且内容已相同时幂等返回，不触发多余请求
	editedUpdateID = ""
	resSame, err := client.createUpdate(context.Background(), "token", "4201", "旧标题", "旧内容", false, true)
	if err != nil {
		t.Fatalf("createUpdate same failed: %v", err)
	}
	if resSame.ID != "12900" {
		t.Errorf("update ID = %q, want 12900", resSame.ID)
	}
	if editedUpdateID != "" {
		t.Errorf("should not call editUpdate when content is already identical")
	}

	// 3. 测试 overwritePrevious: false 时正常创建新 update
	resNew, err := client.createUpdate(context.Background(), "token", "4201", "新版本标题2", "新版本说明2", false, false)
	if err != nil {
		t.Fatalf("createUpdate new failed: %v", err)
	}
	if resNew.ID != "12999" {
		t.Errorf("new update ID = %q, want 12999", resNew.ID)
	}
	if !createdUpdate {
		t.Error("expected createUpdate to create a new update when overwritePrevious is false")
	}
}

func TestBandBBSOverwriteCleansResidualSameVersionStringWithoutDuplicateDelete(t *testing.T) {
	var deletedVersions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resources/4201/versions":
			// 包含最新版本 13500 (10.25)，以及一个更早的残留同名版本 13490 (10.25)，还有一个其他版本 13400 (9.0)
			_, _ = w.Write([]byte(`{"versions":[
				{"resource_version_id":13400,"version_string":"9.0"},
				{"resource_version_id":13490,"version_string":"10.25"},
				{"resource_version_id":13500,"version_string":"10.25"}
			]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/resource-versions/":
			_, _ = w.Write([]byte(`{"success":true,"version":{"resource_version_id":13510,"version_string":"10.25"}}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/resource-versions/"):
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			deletedVersions = append(deletedVersions, parts[len(parts)-1])
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, nil, nil)
	res, err := client.replaceVersion(context.Background(), "token", "4201", "10.25", "app.rpk", 100, "key", false, true)
	if err != nil {
		t.Fatalf("replaceVersion failed: %v", err)
	}
	if res.ID != "13510" {
		t.Fatalf("created ID = %s, want 13510", res.ID)
	}
	// 期望先删除了 latestVersion (13500)，随后清理了同名残留 (13490)，而 13400 (9.0) 不受影响，且 13500 不被重复删除
	if len(deletedVersions) != 2 || deletedVersions[0] != "13500" || deletedVersions[1] != "13490" {
		t.Fatalf("deleted versions = %v, want [13500 13490]", deletedVersions)
	}
}
