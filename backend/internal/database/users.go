package database

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pfaka/iot-dashboard/internal/models"
)

func (db *DB) CreateUser(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (email, name, picture, google_id, is_admin)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	return db.Pool.QueryRow(ctx, query,
		user.Email, user.Name, user.Picture, user.GoogleID, user.IsAdmin,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (db *DB) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, name, picture, google_id, is_admin, created_at, updated_at
		FROM users WHERE id = $1
	`
	user := &models.User{}
	err := db.Pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.Picture,
		&user.GoogleID, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (db *DB) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, name, picture, google_id, is_admin, created_at, updated_at
		FROM users WHERE email = $1
	`
	user := &models.User{}
	err := db.Pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.Picture,
		&user.GoogleID, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (db *DB) GetUserByGoogleID(ctx context.Context, googleID string) (*models.User, error) {
	query := `
		SELECT id, email, name, picture, google_id, is_admin, created_at, updated_at
		FROM users WHERE google_id = $1
	`
	user := &models.User{}
	err := db.Pool.QueryRow(ctx, query, googleID).Scan(
		&user.ID, &user.Email, &user.Name, &user.Picture,
		&user.GoogleID, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (db *DB) UpsertUserByGoogleID(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (email, name, picture, google_id, is_admin)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (google_id) DO UPDATE SET
			email = EXCLUDED.email,
			name = EXCLUDED.name,
			picture = EXCLUDED.picture,
			updated_at = NOW()
		RETURNING id, is_admin, created_at, updated_at
	`
	return db.Pool.QueryRow(ctx, query,
		user.Email, user.Name, user.Picture, user.GoogleID, user.IsAdmin,
	).Scan(&user.ID, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt)
}

func (db *DB) UpdateUser(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users SET name = $2, picture = $3, is_admin = $4, updated_at = $5
		WHERE id = $1
	`
	_, err := db.Pool.Exec(ctx, query, user.ID, user.Name, user.Picture, user.IsAdmin, time.Now())
	return err
}

func (db *DB) GetAllUsers(ctx context.Context) ([]models.User, error) {
	query := `
		SELECT id, email, name, picture, google_id, is_admin, created_at, updated_at
		FROM users ORDER BY created_at DESC
	`
	rows, err := db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(
			&user.ID, &user.Email, &user.Name, &user.Picture,
			&user.GoogleID, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (db *DB) GetUsersCount(ctx context.Context) (int64, error) {
	var count int64
	err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (db *DB) DeleteUser(ctx context.Context, id uuid.UUID) error {
	// First delete user's devices and their metrics
	_, err := db.Pool.Exec(ctx, `
		DELETE FROM metrics WHERE device_id IN (SELECT id FROM devices WHERE user_id = $1)
	`, id)
	if err != nil {
		return err
	}

	_, err = db.Pool.Exec(ctx, "DELETE FROM devices WHERE user_id = $1", id)
	if err != nil {
		return err
	}

	_, err = db.Pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	return err
}

func (db *DB) SetUserAdmin(ctx context.Context, id uuid.UUID, isAdmin bool) error {
	query := `UPDATE users SET is_admin = $2, updated_at = NOW() WHERE id = $1`
	_, err := db.Pool.Exec(ctx, query, id, isAdmin)
	return err
}
