package database

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

// ==================== Access Devices ====================

func (db *DB) CreateAccessDevice(ctx context.Context, device *models.AccessDevice) error {
	query := `
		INSERT INTO access_devices (device_id, secret_key, name)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	return db.Pool.QueryRow(ctx, query, device.DeviceID, device.SecretKey, device.Name).
		Scan(&device.ID, &device.CreatedAt)
}

func (db *DB) GetAccessDeviceByID(ctx context.Context, id uuid.UUID) (*models.AccessDevice, error) {
	query := `
		SELECT id, device_id, secret_key, COALESCE(name, ''), created_at
		FROM access_devices WHERE id = $1
	`
	device := &models.AccessDevice{}
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&device.ID, &device.DeviceID, &device.SecretKey, &device.Name, &device.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (db *DB) GetAccessDeviceByDeviceID(ctx context.Context, deviceID string) (*models.AccessDevice, error) {
	query := `
		SELECT id, device_id, secret_key, COALESCE(name, ''), created_at
		FROM access_devices WHERE device_id = $1
	`
	device := &models.AccessDevice{}
	err := db.Pool.QueryRow(ctx, query, deviceID).Scan(
		&device.ID, &device.DeviceID, &device.SecretKey, &device.Name, &device.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (db *DB) GetAccessDeviceByCredentials(ctx context.Context, deviceID, secretKey string) (*models.AccessDevice, error) {
	query := `
		SELECT id, device_id, secret_key, COALESCE(name, ''), created_at
		FROM access_devices WHERE device_id = $1 AND secret_key = $2
	`
	device := &models.AccessDevice{}
	err := db.Pool.QueryRow(ctx, query, deviceID, secretKey).Scan(
		&device.ID, &device.DeviceID, &device.SecretKey, &device.Name, &device.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (db *DB) GetAllAccessDevices(ctx context.Context) ([]models.AccessDevice, error) {
	query := `
		SELECT id, device_id, secret_key, COALESCE(name, ''), created_at
		FROM access_devices ORDER BY created_at DESC
	`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.AccessDevice
	for rows.Next() {
		var device models.AccessDevice
		if err := rows.Scan(
			&device.ID, &device.DeviceID, &device.SecretKey, &device.Name, &device.CreatedAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func (db *DB) DeleteAccessDevice(ctx context.Context, id uuid.UUID) error {
	_, err := db.Pool.Exec(ctx, "DELETE FROM access_devices WHERE id = $1", id)
	return err
}

// ==================== Cards ====================

func (db *DB) CreateCard(ctx context.Context, card *models.Card) error {
	query := `
		INSERT INTO cards (card_uid, status)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at
	`
	return db.Pool.QueryRow(ctx, query, card.CardUID, card.Status).
		Scan(&card.ID, &card.CreatedAt, &card.UpdatedAt)
}

func (db *DB) GetCardByID(ctx context.Context, id uuid.UUID) (*models.Card, error) {
	query := `
		SELECT id, card_uid, status, created_at, updated_at
		FROM cards WHERE id = $1
	`
	card := &models.Card{}
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&card.ID, &card.CardUID, &card.Status, &card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Load linked devices
	devices, err := db.GetCardDevices(ctx, card.ID)
	if err != nil {
		return nil, err
	}
	card.Devices = devices

	return card, nil
}

func (db *DB) GetCardByUID(ctx context.Context, cardUID string) (*models.Card, error) {
	query := `
		SELECT id, card_uid, status, created_at, updated_at
		FROM cards WHERE card_uid = $1
	`
	card := &models.Card{}
	err := db.Pool.QueryRow(ctx, query, cardUID).Scan(
		&card.ID, &card.CardUID, &card.Status, &card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Load linked devices
	devices, err := db.GetCardDevices(ctx, card.ID)
	if err != nil {
		return nil, err
	}
	card.Devices = devices

	return card, nil
}

func (db *DB) GetAllCards(ctx context.Context) ([]models.Card, error) {
	query := `
		SELECT id, card_uid, status, created_at, updated_at
		FROM cards ORDER BY updated_at DESC
	`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		var card models.Card
		if err := rows.Scan(
			&card.ID, &card.CardUID, &card.Status, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, err
		}
		// Load linked devices for each card
		devices, _ := db.GetCardDevices(ctx, card.ID)
		card.Devices = devices
		cards = append(cards, card)
	}
	return cards, nil
}

func (db *DB) GetCardsByStatus(ctx context.Context, status string) ([]models.Card, error) {
	query := `
		SELECT id, card_uid, status, created_at, updated_at
		FROM cards WHERE status = $1 ORDER BY updated_at DESC
	`
	rows, err := db.Pool.Query(ctx, query, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		var card models.Card
		if err := rows.Scan(
			&card.ID, &card.CardUID, &card.Status, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, err
		}
		devices, _ := db.GetCardDevices(ctx, card.ID)
		card.Devices = devices
		cards = append(cards, card)
	}
	return cards, nil
}

func (db *DB) UpdateCardStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE cards SET status = $2, updated_at = $3 WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, id, status, time.Now())
	return err
}

func (db *DB) DeleteCard(ctx context.Context, id uuid.UUID) error {
	_, err := db.Pool.Exec(ctx, "DELETE FROM cards WHERE id = $1", id)
	return err
}

// ==================== Card-Device Links ====================

func (db *DB) LinkCardToDevice(ctx context.Context, cardID, deviceID uuid.UUID) error {
	query := `
		INSERT INTO card_devices (card_id, device_id)
		VALUES ($1, $2)
		ON CONFLICT (card_id, device_id) DO NOTHING
	`
	_, err := db.Pool.Exec(ctx, query, cardID, deviceID)
	return err
}

func (db *DB) UnlinkCardFromDevice(ctx context.Context, cardID, deviceID uuid.UUID) error {
	query := `DELETE FROM card_devices WHERE card_id = $1 AND device_id = $2`
	_, err := db.Pool.Exec(ctx, query, cardID, deviceID)
	return err
}

func (db *DB) GetCardDevices(ctx context.Context, cardID uuid.UUID) ([]models.DeviceBrief, error) {
	query := `
		SELECT ad.id, ad.device_id, COALESCE(ad.name, ad.device_id)
		FROM access_devices ad
		INNER JOIN card_devices cd ON cd.device_id = ad.id
		WHERE cd.card_id = $1
	`
	rows, err := db.Pool.Query(ctx, query, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.DeviceBrief
	for rows.Next() {
		var device models.DeviceBrief
		if err := rows.Scan(&device.ID, &device.DeviceID, &device.Name); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func (db *DB) IsCardLinkedToDevice(ctx context.Context, cardID, deviceID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM card_devices WHERE card_id = $1 AND device_id = $2)`
	var exists bool
	err := db.Pool.QueryRow(ctx, query, cardID, deviceID).Scan(&exists)
	return exists, err
}

// ==================== Access Logs ====================

func (db *DB) CreateAccessLog(ctx context.Context, log *models.AccessLog) error {
	query := `
		INSERT INTO access_logs (device_id, card_uid, card_type, action, status, allowed)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`
	return db.Pool.QueryRow(ctx, query,
		log.DeviceID, log.CardUID, log.CardType, log.Action, log.Status, log.Allowed,
	).Scan(&log.ID, &log.CreatedAt)
}

// AccessLogFilter параметри фільтрації логів
type AccessLogFilter struct {
	Action   string // verify, register, card_status, card_delete
	Allowed  *bool  // true/false or nil for all
	CardUID  string // partial match
	DeviceID string // partial match
	Limit    int
}

func (db *DB) GetAccessLogs(ctx context.Context, limit int) ([]models.AccessLog, error) {
	return db.GetAccessLogsFiltered(ctx, AccessLogFilter{Limit: limit})
}

func (db *DB) GetAccessLogsFiltered(ctx context.Context, filter AccessLogFilter) ([]models.AccessLog, error) {
	// Build dynamic query with filters
	query := `
		SELECT id, COALESCE(device_id, ''), COALESCE(card_uid, ''), COALESCE(card_type, ''),
		       action, COALESCE(status, ''), allowed, created_at
		FROM access_logs
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.Action != "" {
		query += ` AND action = $` + strconv.Itoa(argNum)
		args = append(args, filter.Action)
		argNum++
	}

	if filter.Allowed != nil {
		query += ` AND allowed = $` + strconv.Itoa(argNum)
		args = append(args, *filter.Allowed)
		argNum++
	}

	if filter.CardUID != "" {
		query += ` AND card_uid ILIKE $` + strconv.Itoa(argNum)
		args = append(args, "%"+filter.CardUID+"%")
		argNum++
	}

	if filter.DeviceID != "" {
		query += ` AND device_id ILIKE $` + strconv.Itoa(argNum)
		args = append(args, "%"+filter.DeviceID+"%")
		argNum++
	}

	query += ` ORDER BY created_at DESC`

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += ` LIMIT $` + strconv.Itoa(argNum)
	args = append(args, limit)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.AccessLog
	for rows.Next() {
		var log models.AccessLog
		if err := rows.Scan(
			&log.ID, &log.DeviceID, &log.CardUID, &log.CardType,
			&log.Action, &log.Status, &log.Allowed, &log.CreatedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}


func (db *DB) GetAccessLogsByCardUID(ctx context.Context, cardUID string, limit int) ([]models.AccessLog, error) {
	query := `
		SELECT id, COALESCE(device_id, ''), COALESCE(card_uid, ''), COALESCE(card_type, ''),
		       action, COALESCE(status, ''), allowed, created_at
		FROM access_logs WHERE card_uid = $1 ORDER BY created_at DESC LIMIT $2
	`
	rows, err := db.Pool.Query(ctx, query, cardUID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.AccessLog
	for rows.Next() {
		var log models.AccessLog
		if err := rows.Scan(
			&log.ID, &log.DeviceID, &log.CardUID, &log.CardType,
			&log.Action, &log.Status, &log.Allowed, &log.CreatedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

