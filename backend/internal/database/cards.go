package database

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

// ==================== Challenge Management (SKUD Challenge-Response) ====================

// CreateChallenge generates and stores a new challenge for a SKUD device
// Only one active challenge per device (replaces existing)
func (db *DB) CreateChallenge(ctx context.Context, deviceID uuid.UUID) (string, error) {
	// Generate random 32-char hex challenge
	challenge := generateRandomHex(32)

	// Upsert challenge (replace existing for this device)
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO access_challenges (device_id, challenge, expires_at)
		VALUES ($1, $2, NOW() + INTERVAL '30 seconds')
		ON CONFLICT (device_id) DO UPDATE SET
			challenge = EXCLUDED.challenge,
			created_at = NOW(),
			expires_at = NOW() + INTERVAL '30 seconds'
	`, deviceID, challenge)

	if err != nil {
		return "", err
	}

	return challenge, nil
}

// ValidateAndConsumeChallenge checks if the challenge is valid and removes it
// Returns error if challenge is invalid, expired, or already used
func (db *DB) ValidateAndConsumeChallenge(ctx context.Context, deviceID uuid.UUID, challenge string) error {
	// Try to delete the challenge if it matches and is not expired
	result, err := db.Pool.Exec(ctx, `
		DELETE FROM access_challenges
		WHERE device_id = $1 AND challenge = $2 AND expires_at > NOW()
	`, deviceID, challenge)

	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("invalid or expired challenge")
	}

	return nil
}

// CleanupExpiredChallenges removes expired challenges
func (db *DB) CleanupExpiredChallenges(ctx context.Context) error {
	_, err := db.Pool.Exec(ctx, `
		DELETE FROM access_challenges WHERE expires_at < NOW()
	`)
	return err
}

// generateRandomHex generates a random hex string of specified length
func generateRandomHex(length int) string {
	bytes := make([]byte, length/2)
	for i := range bytes {
		bytes[i] = byte(time.Now().UnixNano()&0xFF) ^ byte(i*17)
	}
	// Use crypto/rand for better randomness
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
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
	return db.GetCardsFiltered(ctx, "", nil)
}

func (db *DB) GetCardsByStatus(ctx context.Context, status string) ([]models.Card, error) {
	return db.GetCardsFiltered(ctx, status, nil)
}

// GetCardsFiltered returns cards with optional status and device filters
func (db *DB) GetCardsFiltered(ctx context.Context, status string, deviceID *uuid.UUID) ([]models.Card, error) {
	var query string
	var args []interface{}
	argNum := 1

	if deviceID != nil {
		// Filter cards linked to specific device
		query = `
			SELECT DISTINCT c.id, c.card_uid, c.status, c.created_at, c.updated_at
			FROM cards c
			INNER JOIN card_devices cd ON cd.card_id = c.id
			WHERE cd.device_id = $1
		`
		args = append(args, *deviceID)
		argNum++

		if status != "" {
			query += fmt.Sprintf(" AND c.status = $%d", argNum)
			args = append(args, status)
		}
		query += " ORDER BY c.updated_at DESC"
	} else {
		// All cards
		query = `SELECT id, card_uid, status, created_at, updated_at FROM cards`
		if status != "" {
			query += fmt.Sprintf(" WHERE status = $%d", argNum)
			args = append(args, status)
		}
		query += " ORDER BY updated_at DESC"
	}

	rows, err := db.Pool.Query(ctx, query, args...)
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
	// Now using devices table instead of access_devices
	query := `
		SELECT d.id, d.id::text, d.name
		FROM devices d
		INNER JOIN card_devices cd ON cd.device_id = d.id
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

