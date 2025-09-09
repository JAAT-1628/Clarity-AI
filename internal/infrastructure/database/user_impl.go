package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
)

type userRepository struct {
	db *PostgresDB
}

func NewUserRepository(db *PostgresDB) repositories.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) CreateUser(ctx context.Context, user *models.User) error {
	query := `INSERT INTO users (username, email, password, plan, api_usage_count, api_usage_reset_date) 
              VALUES ($1, $2, $3, $4, $5, $6) 
              RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(
		ctx, query,
		user.Username,
		user.Email,
		user.Password,
		user.Plan,
		user.APIUsageCount,
		user.APIUsageResetDate,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *userRepository) GetUserID(ctx context.Context, id int64) (*models.User, error) {
	var user models.User
	query := `SELECT id, username, email, password, plan, api_usage_count, 
              api_usage_reset_date, created_at, updated_at 
              FROM users WHERE id = $1`

	err := r.db.GetContext(ctx, &user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user with id %d not found", id)
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	return &user, nil
}

func (r *userRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	query := `SELECT id, username, email, password, plan, api_usage_count, 
              api_usage_reset_date, created_at, updated_at 
              FROM users WHERE email = $1`

	err := r.db.GetContext(ctx, &user, query, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user with email %s not found", email)
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return &user, nil
}

func (r *userRepository) UpdateUser(ctx context.Context, user *models.User) error {
	query := `UPDATE users SET username = $2, email = $3, password = $4, plan = $5, 
              api_usage_count = $6, api_usage_reset_date = $7 
              WHERE id = $1`

	result, err := r.db.ExecContext(
		ctx, query,
		user.ID,
		user.Username,
		user.Email,
		user.Password,
		user.Plan,
		user.APIUsageCount,
		user.APIUsageResetDate,
	)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user with id %d not found", user.ID)
	}
	return nil
}

func (r *userRepository) UpdateApiUsage(ctx context.Context, id int64, usageCount int64) error {
	query := `UPDATE users SET api_usage_count = $2, api_usage_reset_date = $3 WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, usageCount, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update API usage: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user with id %d not found", id)
	}
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM users WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user with id %d not found", id)
	}
	return nil
}

func (r *userRepository) UpdateUserAPIUsage(ctx context.Context, userID int64, count int64, resetDate time.Time) error {
	query := `UPDATE users SET api_usage_count = $1, api_usage_reset_date = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, count, resetDate, userID)
	return err
}

func (r *userRepository) IncrementAPIUsage(ctx context.Context, userID int64) error {
	query := `UPDATE users SET api_usage_count = api_usage_count + 1 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}
