package repositories

import (
	"clarity-ai/internal/domain/models"
	"context"
)

type SubscriptionRepository interface {
	GetByUserID(ctx context.Context, userID int64) (*models.Subscription, error)
	GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (*models.Subscription, error)
	Create(ctx context.Context, sub *models.Subscription) error
	Update(ctx context.Context, sub *models.Subscription) error
}
