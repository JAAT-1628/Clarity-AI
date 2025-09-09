package repositories

import (
	"clarity-ai/internal/domain/models"
	"context"
	"time"
)

type UserRepository interface {
	//create
	CreateUser(ctx context.Context, user *models.User) error

	//get
	GetUserID(ctx context.Context, id int64) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)

	//update
	UpdateUser(ctx context.Context, user *models.User) error
	UpdateApiUsage(ctx context.Context, id int64, usageCount int64) error

	//delete
	Delete(ctx context.Context, id int64) error

	UpdateUserAPIUsage(ctx context.Context, userID int64, count int64, resetDate time.Time) error
	IncrementAPIUsage(ctx context.Context, userID int64) error
}
