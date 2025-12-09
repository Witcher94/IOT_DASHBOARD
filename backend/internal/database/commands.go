package database

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

func (db *DB) CreateCommand(ctx context.Context, cmd *models.Command) error {
	query := `
		INSERT INTO commands (device_id, command, params, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`
	return db.Pool.QueryRow(ctx, query,
		cmd.DeviceID, cmd.Command, cmd.Params, "pending",
	).Scan(&cmd.ID, &cmd.CreatedAt)
}

func (db *DB) GetPendingCommand(ctx context.Context, deviceID uuid.UUID) (*models.Command, error) {
	query := `
		SELECT id, device_id, command, params, status, created_at, sent_at, acked_at
		FROM commands
		WHERE device_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
		LIMIT 1
	`
	cmd := &models.Command{}
	err := db.Pool.QueryRow(ctx, query, deviceID).Scan(
		&cmd.ID, &cmd.DeviceID, &cmd.Command, &cmd.Params,
		&cmd.Status, &cmd.CreatedAt, &cmd.SentAt, &cmd.AckedAt,
	)
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func (db *DB) MarkCommandSent(ctx context.Context, commandID uuid.UUID) error {
	query := `UPDATE commands SET status = 'sent', sent_at = $2 WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, commandID, time.Now())
	return err
}

func (db *DB) AcknowledgeCommand(ctx context.Context, commandID uuid.UUID, status string) error {
	query := `UPDATE commands SET status = $2, acked_at = $3 WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, commandID, status, time.Now())
	return err
}

func (db *DB) GetCommandsByDeviceID(ctx context.Context, deviceID uuid.UUID, limit int) ([]models.Command, error) {
	query := `
		SELECT id, device_id, command, params, status, created_at, sent_at, acked_at
		FROM commands WHERE device_id = $1
		ORDER BY created_at DESC LIMIT $2
	`
	rows, err := db.Pool.Query(ctx, query, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []models.Command
	for rows.Next() {
		var cmd models.Command
		if err := rows.Scan(
			&cmd.ID, &cmd.DeviceID, &cmd.Command, &cmd.Params,
			&cmd.Status, &cmd.CreatedAt, &cmd.SentAt, &cmd.AckedAt,
		); err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}
	return commands, nil
}

func (db *DB) GetCommandByID(ctx context.Context, id uuid.UUID) (*models.Command, error) {
	query := `
		SELECT id, device_id, command, params, status, created_at, sent_at, acked_at
		FROM commands WHERE id = $1
	`
	cmd := &models.Command{}
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&cmd.ID, &cmd.DeviceID, &cmd.Command, &cmd.Params,
		&cmd.Status, &cmd.CreatedAt, &cmd.SentAt, &cmd.AckedAt,
	)
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func (db *DB) DeleteCommand(ctx context.Context, id uuid.UUID) error {
	_, err := db.Pool.Exec(ctx, "DELETE FROM commands WHERE id = $1", id)
	return err
}

