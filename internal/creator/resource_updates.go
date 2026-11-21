package creator

import (
	"context"
	"encoding/json"
)

type InstalledResourceVersion struct {
	Kind        string `json:"kind"`
	PackageID   string `json:"package_id"`
	VersionCode int64  `json:"version_code"`
}

type ResourceUpdate struct {
	ResourceID string `json:"resource_id"`
	FileID     string `json:"file_id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	PaidType   string `json:"paid_type"`
	InstalledResourceVersion
}

func (s *Service) ResourceUpdates(ctx context.Context, device string, installed []InstalledResourceVersion) ([]ResourceUpdate, error) {
	result := make([]ResourceUpdate, 0)
	if len(installed) == 0 {
		return result, nil
	}
	packages, err := json.Marshal(installed)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
WITH target AS (
    SELECT id FROM devices
    WHERE enabled AND (codename = $1 OR astrobox_id = $1)
    ORDER BY (codename = $1) DESC
    LIMIT 1
)
SELECT DISTINCT r.id::text, a.id::text, rr.name, a.package_version, a.blob_sha256,
       rr.paid_type, i.kind, a.package_id, v.version_code::bigint
FROM jsonb_to_recordset($2::jsonb) AS i(kind text, package_id text, version_code bigint)
JOIN revision_artifacts a ON a.package_id = i.package_id
JOIN resources r ON r.current_revision_id = a.revision_id AND r.kind = i.kind
JOIN resource_revisions rr ON rr.id = a.revision_id AND rr.resource_id = r.id
JOIN revision_artifact_devices b ON b.artifact_id = a.id
JOIN target d ON d.id = b.device_id
CROSS JOIN LATERAL (
    SELECT CASE WHEN a.analysis->>'version_code' ~ '^[0-9]{1,19}$'
                THEN (a.analysis->>'version_code')::numeric END AS version_code
) v
WHERE r.moderation_state = 'visible'
  AND v.version_code > i.version_code AND v.version_code <= 9223372036854775807
ORDER BY i.kind, a.package_id, v.version_code::bigint DESC, r.id::text, a.id::text`, device, string(packages))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ResourceUpdate
		if err := rows.Scan(&item.ResourceID, &item.FileID, &item.Name, &item.Version, &item.SHA256, &item.PaidType, &item.Kind, &item.PackageID, &item.VersionCode); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
