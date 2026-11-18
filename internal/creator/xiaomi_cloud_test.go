package creator

import (
	"encoding/json"
	"testing"
)

func TestXiaomiApiResponseParsing(t *testing.T) {
	sampleJSON := `{
		"code": 0,
		"message": "ok",
		"result": {
			"list": [
				{
					"pd_id": 28155,
					"pd_name": "Xiaomi Watch S5 eSIM 46mm",
					"model": "miwear.watch.p62lte",
					"alias": "p62",
					"feature": ["esim", "local_music", "watchface"]
				},
				{
					"pd_id": 27469,
					"name": "Xiaomi Watch S5 46mm",
					"model": "miwear.watch.p62",
					"alias": "p62",
					"feature": ["local_music", "watchface"]
				}
			]
		}
	}`

	var apiResp xiaomiApiResponse
	if err := json.Unmarshal([]byte(sampleJSON), &apiResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(apiResp.Result.List) != 2 {
		t.Fatalf("expected 2 items, got %d", len(apiResp.Result.List))
	}

	first := apiResp.Result.List[0]
	if first.PdID != 28155 || first.Model != "miwear.watch.p62lte" || first.PdName != "Xiaomi Watch S5 eSIM 46mm" {
		t.Fatalf("unexpected first item: %+v", first)
	}

	var features []string
	if err := json.Unmarshal(first.Feature, &features); err != nil {
		t.Fatalf("failed to unmarshal features: %v", err)
	}
	if len(features) != 3 || features[0] != "esim" {
		t.Fatalf("unexpected features: %v", features)
	}
}
