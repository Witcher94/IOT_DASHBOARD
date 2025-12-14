package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

func (db *DB) CreateDevice(ctx context.Context, device *models.Device) error {
	deviceType := device.DeviceType
	if deviceType == "" {
		deviceType = models.DeviceTypeSimple
	}
	query := `
		INSERT INTO devices (user_id, name, token, dht_enabled, mesh_enabled, device_type, gateway_id, mesh_node_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`
	return db.Pool.QueryRow(ctx, query,
		device.UserID, device.Name, device.Token, device.DHTEnabled, device.MeshEnabled,
		deviceType, device.GatewayID, device.MeshNodeID,
	).Scan(&device.ID, &device.CreatedAt, &device.UpdatedAt)
}

func (db *DB) GetDeviceByID(ctx context.Context, id uuid.UUID) (*models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, pending_chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   COALESCE(device_type, 'simple_device'), gateway_id, mesh_node_id,
			   created_at, updated_at
		FROM devices WHERE id = $1
	`
	device := &models.Device{}
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Token,
		&device.ChipID, &device.PendingChipID, &device.MAC, &device.Platform, &device.Firmware,
		&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
		&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
		&device.DeviceType, &device.GatewayID, &device.MeshNodeID,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (db *DB) GetDeviceByToken(ctx context.Context, token string) (*models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, pending_chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   COALESCE(device_type, 'simple_device'), gateway_id, mesh_node_id,
			   created_at, updated_at
		FROM devices WHERE token = $1
	`
	device := &models.Device{}
	err := db.Pool.QueryRow(ctx, query, token).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Token,
		&device.ChipID, &device.PendingChipID, &device.MAC, &device.Platform, &device.Firmware,
		&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
		&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
		&device.DeviceType, &device.GatewayID, &device.MeshNodeID,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (db *DB) GetDevicesByUserID(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, pending_chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   COALESCE(device_type, 'simple_device'), gateway_id, mesh_node_id,
			   created_at, updated_at
		FROM devices WHERE user_id = $1 ORDER BY device_type DESC, created_at DESC
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
			&device.ChipID, &device.PendingChipID, &device.MAC, &device.Platform, &device.Firmware,
			&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
			&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
			&device.DeviceType, &device.GatewayID, &device.MeshNodeID,
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
		SELECT id, user_id, name, token, chip_id, pending_chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   COALESCE(device_type, 'simple_device'), gateway_id, mesh_node_id,
			   created_at, updated_at
		FROM devices ORDER BY device_type DESC, created_at DESC
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
			&device.ChipID, &device.PendingChipID, &device.MAC, &device.Platform, &device.Firmware,
			&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
			&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
			&device.DeviceType, &device.GatewayID, &device.MeshNodeID,
			&device.CreatedAt, &device.UpdatedAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// GetMeshNodesByGatewayID returns all mesh nodes belonging to a gateway
func (db *DB) GetMeshNodesByGatewayID(ctx context.Context, gatewayID uuid.UUID) ([]models.Device, error) {
	query := `
		SELECT id, user_id, name, token, chip_id, pending_chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   COALESCE(device_type, 'mesh_node'), gateway_id, mesh_node_id,
			   created_at, updated_at
		FROM devices WHERE gateway_id = $1 ORDER BY mesh_node_id
	`
	rows, err := db.Pool.Query(ctx, query, gatewayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var device models.Device
		if err := rows.Scan(
			&device.ID, &device.UserID, &device.Name, &device.Token,
			&device.ChipID, &device.PendingChipID, &device.MAC, &device.Platform, &device.Firmware,
			&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
			&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
			&device.DeviceType, &device.GatewayID, &device.MeshNodeID,
			&device.CreatedAt, &device.UpdatedAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// GetOrCreateMeshNode finds or creates a mesh node by mesh_node_id and gateway_id
func (db *DB) GetOrCreateMeshNode(ctx context.Context, gatewayID uuid.UUID, meshNodeID uint32, nodeName string) (*models.Device, error) {
	// First try to find existing
	query := `
		SELECT id, user_id, name, token, chip_id, pending_chip_id, mac, platform, firmware,
			   is_online, last_seen, dht_enabled, mesh_enabled,
			   COALESCE(alerts_enabled, true), alert_temp_min, alert_temp_max, alert_humidity_max,
			   COALESCE(device_type, 'mesh_node'), gateway_id, mesh_node_id,
			   created_at, updated_at
		FROM devices WHERE gateway_id = $1 AND mesh_node_id = $2
	`
	device := &models.Device{}
	err := db.Pool.QueryRow(ctx, query, gatewayID, meshNodeID).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Token,
		&device.ChipID, &device.PendingChipID, &device.MAC, &device.Platform, &device.Firmware,
		&device.IsOnline, &device.LastSeen, &device.DHTEnabled, &device.MeshEnabled,
		&device.AlertsEnabled, &device.AlertTempMin, &device.AlertTempMax, &device.AlertHumidityMax,
		&device.DeviceType, &device.GatewayID, &device.MeshNodeID,
		&device.CreatedAt, &device.UpdatedAt,
	)
	if err == nil {
		return device, nil
	}

	// Get gateway to inherit user_id
	gateway, err := db.GetDeviceByID(ctx, gatewayID)
	if err != nil {
		return nil, err
	}

	// Create new mesh node
	device = &models.Device{
		UserID:      gateway.UserID,
		Name:        nodeName,
		Token:       uuid.New().String() + uuid.New().String()[:32], // 64 char token
		DeviceType:  models.DeviceTypeMeshNode,
		GatewayID:   &gatewayID,
		MeshNodeID:  &meshNodeID,
		DHTEnabled:  true,
		MeshEnabled: true,
	}
	if err := db.CreateDevice(ctx, device); err != nil {
		return nil, err
	}

	return device, nil
}

// UpdateMeshNodeMetrics updates metrics for a mesh node
func (db *DB) UpdateMeshNodeMetrics(ctx context.Context, deviceID uuid.UUID, chipID, mac, platform, firmware string) error {
	query := `
		UPDATE devices SET
			chip_id = $2, mac = $3, platform = $4, firmware = $5,
			is_online = true, last_seen = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Pool.Exec(ctx, query, deviceID, chipID, mac, platform, firmware)
	return err
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
			name = $2, token = $3, chip_id = $4, mac = $5, platform = $6, firmware = $7,
			is_online = $8, last_seen = $9, dht_enabled = $10, mesh_enabled = $11,
			updated_at = $12
		WHERE id = $1
	`
	_, err := db.Pool.Exec(ctx, query,
		device.ID, device.Name, device.Token, device.ChipID, device.MAC, device.Platform, device.Firmware,
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

func (db *DB) UpdateDeviceType(ctx context.Context, deviceID uuid.UUID, deviceType string) error {
	query := `UPDATE devices SET device_type = $1 WHERE id = $2`
	_, err := db.Pool.Exec(ctx, query, deviceType, deviceID)
	return err
}

// SetDeviceChipID sets the chip_id for a device (hardware lock for clone protection)
// This is typically called once on first connection from the device
func (db *DB) SetDeviceChipID(ctx context.Context, deviceID uuid.UUID, chipID string) error {
	query := `UPDATE devices SET chip_id = $2, updated_at = $3 WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, deviceID, chipID, time.Now())
	return err
}

// ClearDeviceChipID clears the chip_id for a device (allows re-binding to new hardware)
// This should only be called by admins when replacing hardware
func (db *DB) ClearDeviceChipID(ctx context.Context, deviceID uuid.UUID) error {
	query := `UPDATE devices SET chip_id = NULL, pending_chip_id = NULL, updated_at = $2 WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, deviceID, time.Now())
	return err
}

// SetPendingChipID sets the pending_chip_id for a device (awaiting user confirmation)
func (db *DB) SetPendingChipID(ctx context.Context, deviceID uuid.UUID, chipID string) error {
	query := `UPDATE devices SET pending_chip_id = $2, updated_at = $3 WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, deviceID, chipID, time.Now())
	return err
}

// ConfirmChipID moves pending_chip_id to chip_id (user confirmed the hardware)
func (db *DB) ConfirmChipID(ctx context.Context, deviceID uuid.UUID) error {
	query := `UPDATE devices SET chip_id = pending_chip_id, pending_chip_id = NULL, updated_at = $2 WHERE id = $1 AND pending_chip_id IS NOT NULL`
	result, err := db.Pool.Exec(ctx, query, deviceID, time.Now())
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no pending chip_id to confirm")
	}
	return nil
}

// RejectPendingChipID clears the pending_chip_id (user rejected the hardware)
func (db *DB) RejectPendingChipID(ctx context.Context, deviceID uuid.UUID) error {
	query := `UPDATE devices SET pending_chip_id = NULL, updated_at = $2 WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, deviceID, time.Now())
	return err
}
