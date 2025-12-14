package database

import (
	"context"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

// CreateDeviceShare creates a new share for a device
func (db *DB) CreateDeviceShare(ctx context.Context, share *models.DeviceShare) error {
	query := `
		INSERT INTO device_shares (device_id, owner_id, shared_with_id, permission)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`
	return db.Pool.QueryRow(ctx, query,
		share.DeviceID, share.OwnerID, share.SharedWithID, share.Permission,
	).Scan(&share.ID, &share.CreatedAt)
}

// GetDeviceShares returns all shares for a device
func (db *DB) GetDeviceShares(ctx context.Context, deviceID uuid.UUID) ([]models.DeviceShare, error) {
	query := `
		SELECT ds.id, ds.device_id, ds.owner_id, ds.shared_with_id, ds.permission, ds.created_at,
		       u.name as shared_with_name, u.email as shared_with_email
		FROM device_shares ds
		JOIN users u ON u.id = ds.shared_with_id
		WHERE ds.device_id = $1
		ORDER BY ds.created_at DESC
	`
	rows, err := db.Pool.Query(ctx, query, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []models.DeviceShare
	for rows.Next() {
		var s models.DeviceShare
		if err := rows.Scan(&s.ID, &s.DeviceID, &s.OwnerID, &s.SharedWithID, &s.Permission, &s.CreatedAt,
			&s.SharedWithName, &s.SharedWithEmail); err != nil {
			return nil, err
		}
		shares = append(shares, s)
	}
	return shares, nil
}

// DeleteDeviceShare removes a share
func (db *DB) DeleteDeviceShare(ctx context.Context, deviceID, sharedWithID uuid.UUID) error {
	query := `DELETE FROM device_shares WHERE device_id = $1 AND shared_with_id = $2`
	_, err := db.Pool.Exec(ctx, query, deviceID, sharedWithID)
	return err
}

// GetSharedDevices returns devices shared with a user
func (db *DB) GetSharedDevices(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	query := `
		SELECT d.id, d.user_id, d.name, d.device_type, d.chip_id, d.mac, d.platform, d.firmware,
		       d.is_online, d.last_seen, d.dht_enabled, d.mesh_enabled, d.created_at, d.updated_at,
		       d.alerts_enabled, d.alert_temp_min, d.alert_temp_max, d.alert_humidity_max
		FROM devices d
		JOIN device_shares ds ON ds.device_id = d.id
		WHERE ds.shared_with_id = $1
		ORDER BY d.name
	`
	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.DeviceType, &d.ChipID, &d.MAC, &d.Platform, &d.Firmware,
			&d.IsOnline, &d.LastSeen, &d.DHTEnabled, &d.MeshEnabled, &d.CreatedAt, &d.UpdatedAt,
			&d.AlertsEnabled, &d.AlertTempMin, &d.AlertTempMax, &d.AlertHumidityMax); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// HasDeviceAccess checks if a user has access to a device (owner or shared)
func (db *DB) HasDeviceAccess(ctx context.Context, deviceID, userID uuid.UUID) (bool, string, error) {
	// Check if owner
	var ownerID uuid.UUID
	err := db.Pool.QueryRow(ctx, `SELECT user_id FROM devices WHERE id = $1`, deviceID).Scan(&ownerID)
	if err != nil {
		return false, "", err
	}
	if ownerID == userID {
		return true, "owner", nil
	}

	// Check if shared
	var permission string
	err = db.Pool.QueryRow(ctx,
		`SELECT permission FROM device_shares WHERE device_id = $1 AND shared_with_id = $2`,
		deviceID, userID,
	).Scan(&permission)
	if err != nil {
		return false, "", nil // No access
	}
	return true, permission, nil
}

