package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func New(databaseURL string) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

// RunMigrations виконує міграції бази даних
func (db *DB) RunMigrations(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) UNIQUE NOT NULL,
			name VARCHAR(255) NOT NULL,
			picture TEXT,
			google_id VARCHAR(255) UNIQUE,
			is_admin BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			token VARCHAR(255) UNIQUE NOT NULL,
			chip_id VARCHAR(64),
			mac VARCHAR(32),
			platform VARCHAR(32),
			firmware VARCHAR(32),
			is_online BOOLEAN DEFAULT FALSE,
			last_seen TIMESTAMP WITH TIME ZONE,
			dht_enabled BOOLEAN DEFAULT TRUE,
			mesh_enabled BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS metrics (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			temperature DECIMAL(5,2),
			humidity DECIMAL(5,2),
			rssi INTEGER,
			free_heap BIGINT,
			wifi_scan JSONB,
			mesh_nodes JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS commands (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			command VARCHAR(64) NOT NULL,
			params JSONB,
			status VARCHAR(32) DEFAULT 'pending',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			sent_at TIMESTAMP WITH TIME ZONE,
			acked_at TIMESTAMP WITH TIME ZONE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_device_id ON metrics(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_created_at ON metrics(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_token ON devices(token)`,
		`CREATE INDEX IF NOT EXISTS idx_commands_device_id ON commands(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status)`,
		// Alert settings
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS alert_temp_min DECIMAL(5,2)`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS alert_temp_max DECIMAL(5,2) DEFAULT 40`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS alert_humidity_max DECIMAL(5,2) DEFAULT 90`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS alert_policy_id VARCHAR(255)`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS alerts_enabled BOOLEAN DEFAULT TRUE`,
		// Notification channel per user
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS notification_channel_id VARCHAR(255)`,
		// Device type and gateway support
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS device_type VARCHAR(32) DEFAULT 'simple_device'`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS gateway_id UUID REFERENCES devices(id) ON DELETE SET NULL`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS mesh_node_id BIGINT`,
		`CREATE INDEX IF NOT EXISTS idx_devices_gateway_id ON devices(gateway_id)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_device_type ON devices(device_type)`,
		// Device sharing
		`CREATE TABLE IF NOT EXISTS device_shares (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			shared_with_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			permission VARCHAR(20) DEFAULT 'view',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			UNIQUE(device_id, shared_with_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_device_shares_device_id ON device_shares(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_device_shares_shared_with ON device_shares(shared_with_id)`,
		// SKUD (Access Control) tables
		// Cards table for SKUD
		`CREATE TABLE IF NOT EXISTS cards (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			card_uid VARCHAR(64) UNIQUE NOT NULL,
			status VARCHAR(32) DEFAULT 'pending',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		// Card-Device links (now references devices table, not access_devices)
		`CREATE TABLE IF NOT EXISTS card_devices (
			card_id UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
			device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			PRIMARY KEY (card_id, device_id)
		)`,
		`CREATE TABLE IF NOT EXISTS access_logs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			device_id VARCHAR(64),
			card_uid VARCHAR(64),
			card_type VARCHAR(32),
			action VARCHAR(32) NOT NULL,
			status VARCHAR(32),
			allowed BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cards_card_uid ON cards(card_uid)`,
		`CREATE INDEX IF NOT EXISTS idx_cards_status ON cards(status)`,
		// Access logs indexes for fast filtering
		`CREATE INDEX IF NOT EXISTS idx_access_logs_created_at ON access_logs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_card_uid ON access_logs(card_uid)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_device_id ON access_logs(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_action ON access_logs(action)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_allowed ON access_logs(allowed)`,
		// Composite index for common filter combinations
		`CREATE INDEX IF NOT EXISTS idx_access_logs_allowed_created ON access_logs(allowed, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_action_created ON access_logs(action, created_at DESC)`,

		// Nonce table for replay attack protection
		`CREATE TABLE IF NOT EXISTS access_nonces (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			nonce VARCHAR(64) NOT NULL,
			timestamp BIGINT NOT NULL,
			created_at TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_access_nonces_device_nonce ON access_nonces(device_id, nonce)`,
		`CREATE INDEX IF NOT EXISTS idx_access_nonces_created_at ON access_nonces(created_at)`,
	}

	for _, migration := range migrations {
		if _, err := db.Pool.Exec(ctx, migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}
