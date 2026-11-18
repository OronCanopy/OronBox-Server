package resource

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/json"
	"testing"
)

func TestAnalyzeVelaWatchface(t *testing.T) {
	payload := make([]byte, 0x40)
	copy(payload, []byte{0x5a, 0xa5, 0x34, 0x12})
	copy(payload[0x28:], "123456789")
	analysis, err := Analyze(payload)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Platform != VelaOS || analysis.Kind != Watchface || analysis.PackageID != "123456789" {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
}

func TestAnalyzeVelaWatchfaceGeneratesStablePayloadID(t *testing.T) {
	payload := make([]byte, 0x40)
	copy(payload, []byte{0x5a, 0xa5, 0x34, 0x12})
	analysis, err := Analyze(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.PackageID) < 9 || len(analysis.PackageID) > 12 {
		t.Fatalf("unexpected generated id %q", analysis.PackageID)
	}
	if got := watchfaceID(analysis.Payload); got != analysis.PackageID {
		t.Fatalf("payload id %q does not match analysis id %q", got, analysis.PackageID)
	}
}

func TestAnalyzeVelaQuickApp(t *testing.T) {
	payload := zipWith(t, map[string]any{"manifest.json": map[string]any{"package": "org.zxor.demo", "name": "Demo", "versionName": "1.0"}})
	analysis, err := Analyze(payload)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Platform != VelaOS || analysis.Kind != QuickApp || analysis.PackageID != "org.zxor.demo" {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
}

func TestAnalyzeCanopusPackage(t *testing.T) {
	const target = "xiaomi-band-10-3.1.28"
	const moduleID = "hello"
	manifest := map[string]any{
		"schema": 1, "module_id": moduleID, "canopus_abi": "2",
		"lifecycle": "removable", "version": "0.1.0", "version_code": 7,
		"artifacts": []any{map[string]any{
			"target_id": target,
			"path":      "artifacts/module.elf",
		}},
	}
	payload := tarWith(t, map[string][]byte{
		"manifest.json":        mustJSON(t, manifest),
		"artifacts/module.elf": {0x7f, 'E', 'L', 'F'},
	})
	analysis, err := Analyze(payload)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Platform != VelaOS || analysis.Kind != Canopus || analysis.PackageID != moduleID || analysis.Version != "0.1.0" || analysis.VersionCode == nil || *analysis.VersionCode != 7 {
		t.Fatalf("unexpected Canopus analysis: %#v", analysis)
	}
}

func TestAnalyzeZeppApp(t *testing.T) {
	payload := zipWith(t, map[string]any{"app.json": map[string]any{"app": map[string]any{"appType": "app", "appId": "0xf9467", "appName": "Anynews", "version": map[string]any{"name": "1.0"}}}})
	analysis, err := Analyze(payload)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Platform != ZeppOS || analysis.Kind != ZeppApp || analysis.PackageID != "1021031" {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
}

func zipWith(t *testing.T, files map[string]any) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, value := range files {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func tarWith(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	archive := tar.NewWriter(&output)
	for name, value := range files {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(value))}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestVersionCodeUsesWatchManifestAndPreservesZero(t *testing.T) {
	payload := zipWith(t, map[string]any{
		"manifest.json":       map[string]any{"package": "org.demo.app", "versionName": "1.0", "versionCode": 1},
		"manifest-watch.json": map[string]any{"versionName": "2.0", "versionCode": 2},
	})
	analysis, err := Analyze(payload)
	if err != nil || analysis.Version != "2.0" || analysis.VersionCode == nil || *analysis.VersionCode != 2 {
		t.Fatal(analysis, err)
	}
	payload = make([]byte, 0x40)
	copy(payload, []byte{0x5a, 0xa5, 0x34, 0x12})
	copy(payload[0x28:], "123456789")
	analysis, err = Analyze(payload)
	if err != nil || analysis.VersionCode == nil || *analysis.VersionCode != 0 {
		t.Fatal(analysis, err)
	}
	payload[4], payload[5], payload[6] = 3, 2, 1
	analysis, err = Analyze(payload)
	if err != nil || analysis.VersionCode == nil || *analysis.VersionCode != 66051 {
		t.Fatal(analysis, err)
	}
}
