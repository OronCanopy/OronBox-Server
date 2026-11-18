package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
)

type DeviceModel struct {
	PdID     int      `json:"pd_id"`
	Model    string   `json:"model"`
	Product  string   `json:"product"`
	ID       string   `json:"id"`
	Codename string   `json:"codename"`
	Name     string   `json:"name"`
	Vendor   string   `json:"vendor"`
	Feature  []string `json:"feature"`
}

type DeviceModelClientItem struct {
	Model    string   `json:"model"`
	Product  string   `json:"product"`
	ID       string   `json:"id"`
	Codename string   `json:"codename"`
	Name     string   `json:"name"`
	Vendor   string   `json:"vendor"`
	Feature  []string `json:"feature"`
}

func (s *Store) DeviceModelsMap(ctx context.Context) (map[string]DeviceModelClientItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT pd_id, model, product, id, codename, name, vendor, feature
FROM device_models
ORDER BY pd_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query device_models: %w", err)
	}
	defer rows.Close()

	result := make(map[string]DeviceModelClientItem)
	for rows.Next() {
		var pdID int
		var item DeviceModelClientItem
		var featureRaw []byte
		if err := rows.Scan(&pdID, &item.Model, &item.Product, &item.ID, &item.Codename, &item.Name, &item.Vendor, &featureRaw); err != nil {
			return nil, fmt.Errorf("scan device_model: %w", err)
		}
		item.Feature = []string{}
		if len(featureRaw) > 0 {
			_ = json.Unmarshal(featureRaw, &item.Feature)
		}
		result[strconv.Itoa(pdID)] = item
	}
	return result, rows.Err()
}

func (s *Store) AdminDeviceModels(ctx context.Context) ([]DeviceModel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT pd_id, model, product, id, codename, name, vendor, feature
FROM device_models
ORDER BY pd_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query admin device_models: %w", err)
	}
	defer rows.Close()

	var result []DeviceModel
	for rows.Next() {
		var item DeviceModel
		var featureRaw []byte
		if err := rows.Scan(&item.PdID, &item.Model, &item.Product, &item.ID, &item.Codename, &item.Name, &item.Vendor, &featureRaw); err != nil {
			return nil, fmt.Errorf("scan admin device_model: %w", err)
		}
		item.Feature = []string{}
		if len(featureRaw) > 0 {
			_ = json.Unmarshal(featureRaw, &item.Feature)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ImportDeviceModels(ctx context.Context, items []DeviceModel) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	deviceStmt, err := tx.PrepareContext(ctx, `
INSERT INTO device_models (pd_id, model, product, id, codename, name, vendor, feature, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (pd_id) DO UPDATE SET
    model = excluded.model,
    product = excluded.product,
    id = excluded.id,
    codename = excluded.codename,
    name = excluded.name,
    vendor = excluded.vendor,
    feature = excluded.feature,
    updated_at = now()`)
	if err != nil {
		return fmt.Errorf("prepare device_models statement: %w", err)
	}
	defer deviceStmt.Close()

	v1Stmt, err := tx.PrepareContext(ctx, `
INSERT INTO devices (id, codename, display_name, platform, astrobox_id, vendor, enabled, updated_at)
VALUES ($1, $2, $3, 'vela_os', $4, $5, true, now())
ON CONFLICT (codename) DO UPDATE SET
    display_name = excluded.display_name,
    astrobox_id = excluded.astrobox_id,
    vendor = excluded.vendor,
    enabled = true,
    updated_at = now()
WHERE devices.platform = 'vela_os'`)
	if err != nil {
		return fmt.Errorf("prepare devices sync statement: %w", err)
	}
	defer v1Stmt.Close()

	for _, item := range items {
		featBytes, err := json.Marshal(item.Feature)
		if err != nil {
			featBytes = []byte("[]")
		}
		vendor := item.Vendor
		if vendor == "" {
			vendor = "xiaomi"
		}
		if _, err := deviceStmt.ExecContext(ctx, item.PdID, item.Model, item.Product, item.ID, item.Codename, item.Name, vendor, featBytes); err != nil {
			return fmt.Errorf("upsert device_model pd_id=%d: %w", item.PdID, err)
		}

		// Also ensure a corresponding record in devices exists for resource/manifest bindings
		devUUID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://oronbox.org/devices/"+item.Codename))
		if _, err := v1Stmt.ExecContext(ctx, devUUID, item.Codename, item.Name, item.ID, vendor); err != nil {
			return fmt.Errorf("sync devices v1 for codename=%s: %w", item.Codename, err)
		}
	}

	return tx.Commit()
}
