package creator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type XiaomiCloudDevice struct {
	PdID    int      `json:"pd_id"`
	Name    string   `json:"name"`
	Product string   `json:"product"`
	Feature []string `json:"feature"`
	Alias   string   `json:"alias,omitempty"`
}

type xiaomiApiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Result  struct {
		List []struct {
			PdID    int             `json:"pd_id"`
			PdName  string          `json:"pd_name"`
			Name    string          `json:"name"`
			Model   string          `json:"model"`
			Alias   string          `json:"alias"`
			Feature json.RawMessage `json:"feature"`
		} `json:"list"`
	} `json:"result"`
}

// FetchXiaomiCloudDevices fetches the full official wearable device list from Xiaomi Health cloud API.
func FetchXiaomiCloudDevices(ctx context.Context) ([]XiaomiCloudDevice, error) {
	endpoint := "https://hlth.io.mi.com/app/v1/product/get_config_info_by_category?locale=zh_CN&data=%7B%22category%22:%22wearable%22,%22app_platform%22:0,%22last_modify_time%22:0%7D"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create xiaomi request: %w", err)
	}
	req.Header.Set("Cookie", "auth_key=rwelJuWBFJxmbMKD")
	req.Header.Set("User-Agent", "MiFitness/9.24.367 (Android)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch xiaomi cloud devices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xiaomi cloud devices http %d", resp.StatusCode)
	}

	var apiResp xiaomiApiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode xiaomi cloud devices response: %w", err)
	}

	var result []XiaomiCloudDevice
	for _, item := range apiResp.Result.List {
		if item.PdID == 0 && item.Model == "" {
			continue
		}
		name := item.PdName
		if name == "" {
			name = item.Name
		}
		var features []string
		if len(item.Feature) > 0 {
			// Feature could be an array of strings
			_ = json.Unmarshal(item.Feature, &features)
		}
		if features == nil {
			features = []string{}
		}

		result = append(result, XiaomiCloudDevice{
			PdID:    item.PdID,
			Name:    name,
			Product: item.Model,
			Feature: features,
			Alias:   item.Alias,
		})
	}

	return result, nil
}
