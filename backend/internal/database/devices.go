package database

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

func (db *DB) CreateDevice(ctx context.Context, device *models.Device) error {
	query := `
		INSERT INTO devices (user_id, name, token, dht_enabled, mesh_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	return db.Pool.QueryRow(ctx, query,
		device.UserID, device.Name, device.Token, device.DHTEnabled, device.MeshEnabled,
	).Scan(&device.ID, &device.CreatedAt, &device.UpdatedAt)
}

func (db *DB) GetDeviceByID(ctx context.Context, id uuid.UUID) (*models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   created_at, updated_at
		FROM devices WHERE id = $1
	`
	device := &models.Device{}
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Token,
		&device.ChipID, &device.MAC, &device.Platform, &device.Firmware,
		&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
		&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (db *DB) GetDeviceByToken(ctx context.Context, token string) (*models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   created_at, updated_at
		FROM devices WHERE token = $1
	`
	device := &models.Device{}
	err := db.Pool.QueryRow(ctx, query, token).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Token,
		&device.ChipID, &device.MAC, &device.Platform, &device.Firmware,
		&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
		&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (db *DB) GetDevicesByUserID(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   created_at, updated_at
		FROM devices WHERE user_id = $1 ORDER BY created_at DESC
	`
	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var device models.Device
		if err := rows.Scan(
			&device.ID, &device.UserID, &device.Name, &device.Token,
			&device.ChipID, &device.MAC, &device.Platform, &device.Firmware,
			&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
			&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
			&device.CreatedAt, &device.UpdatedAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func (db *DB) GetAllDevices(ctx context.Context) ([]models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   created_at, updated_at
		FROM devices ORDER BY created_at DESC
	`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var device models.Device
		if err := rows.Scan(
			&device.ID, &device.UserID, &device.Name, &device.Token,
			&device.ChipID, &device.MAC, &device.Platform, &device.Firmware,
			&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
			&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
			&device.CreatedAt, &device.UpdatedAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func (db *DB) UpdateAlertSettings(ctx context.Context, deviceID uuid.UUID, req *models.UpdateAlertSettingsRequest) error {
	query := `
		UPDATE devices SET
			alerts_enabled = COALESCE($2, alerts_enabled),
			alert_temp_min = COALESCE($3, alert_temp_min),
			alert_temp_max = COALESCE($4, alert_temp_max),
			alert_humidity_max = COALESCE($5, alert_humidity_max),
			updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Pool.Exec(ctx, query, deviceID, req.AlertsEnabled, req.AlertTempMin, req.AlertTempMax, req.AlertHumidityMax)
	return err
}

func (db *DB) UpdateDevice(ctx context.Context, device *models.Device) error {
	query := `
		UPDATE devices SET
			name = $2, chip_id = $3, mac = $4, platform = $5, firmware = $6,
			is_online = $7, last_seen = $8, dht_enabled = $9, mesh_enabled = $10,
			updated_at = $11
		WHERE id = $1
	`
	_, err := db.Pool.Exec(ctx, query,
		device.ID, device.Name, device.ChipID, device.MAC, device.Platform, device.Firmware,
		device.IsOnline, device.LastSeen, device.DHTEnabled, device.MeshEnabled, time.Now(),
	)
	return err
}

func (db *DB) UpdateDeviceOnline(ctx context.Context, deviceID uuid.UUID, isOnline bool) error {
	query := `UPDATE devices SET is_online = $2, last_seen = $3, updated_at = $3 WHERE id = $1`
	now := time.Now()
	_, err := db.Pool.Exec(ctx, query, deviceID, isOnline, now)
	return err
}

func (db *DB) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	_, err := db.Pool.Exec(ctx, "DELETE FROM devices WHERE id = $1", id)
	return err
}

func (db *DB) GetDevicesCount(ctx context.Context) (int64, error) {
	var count int64
	err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM devices").Scan(&count)
	return count, err
}

func (db *DB) GetOnlineDevicesCount(ctx context.Context) (int64, error) {
	var count int64
	err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM devices WHERE is_online = true").Scan(&count)
	return count, err
}

func (db *DB) GetDevicesCountByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM devices WHERE user_id = $1", userID).Scan(&count)
	return count, err
}

func (db *DB) GetOnlineDevicesCountByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM devices WHERE user_id = $1 AND is_online = true", userID).Scan(&count)
	return count, err
}

func (db *DB) MarkOfflineDevices(ctx context.Context, timeout time.Duration) error {
	query := `
		UPDATE devices SET is_online = false
		WHERE is_online = true AND last_seen < $1
	`
	_, err := db.Pool.Exec(ctx, query, time.Now().Add(-timeout))
	return err
}

