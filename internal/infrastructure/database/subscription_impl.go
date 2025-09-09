package database

import (
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type subscriptionRepository struct {
	db *sqlx.DB
}

func NewSubscriptionRepository(db *sqlx.DB) repositories.SubscriptionRepository {
	return &subscriptionRepository{db: db}
}

func (r *subscriptionRepository) GetByUserID(ctx context.Context, userID int64) (*models.Subscription, error) {
	var sub models.Subscription
	query := `
		SELECT id, user_id, stripe_customer_id, stripe_subscription_id, plan, status,
		       current_period_end, cancel_at_period_end, created_at, updated_at
		FROM subscriptions 
		WHERE user_id = $1 
		ORDER BY created_at DESC 
		LIMIT 1`

	err := r.db.GetContext(ctx, &sub, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("subscription not found for user %d", userID)
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return &sub, nil
}

func (r *subscriptionRepository) GetByStripeSubscriptionID(ctx context.Context, stripeSubID string) (*models.Subscription, error) {
	var sub models.Subscription
	query := `
		SELECT id, user_id, stripe_customer_id, stripe_subscription_id, plan, status,
		       current_period_end, cancel_at_period_end, created_at, updated_at
		FROM subscriptions 
		WHERE stripe_subscription_id = $1`

	err := r.db.GetContext(ctx, &sub, query, stripeSubID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("subscription not found for stripe subscription %s", stripeSubID)
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return &sub, nil
}

func (r *subscriptionRepository) Create(ctx context.Context, sub *models.Subscription) error {
	if sub.ID == uuid.Nil {
		sub.ID = uuid.New()
	}

	query := `
		INSERT INTO subscriptions (id, user_id, stripe_customer_id, stripe_subscription_id,
		                          plan, status, current_period_end, cancel_at_period_end)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query, sub.ID, sub.UserID, sub.StripeCustomerID,
		sub.StripeSubscriptionID, sub.Plan, sub.Status, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd).Scan(&sub.CreatedAt, &sub.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	return nil
}

func (r *subscriptionRepository) Update(ctx context.Context, sub *models.Subscription) error {
	query := `
		UPDATE subscriptions 
		SET stripe_customer_id = $2, stripe_subscription_id = $3, plan = $4, 
		    status = $5, current_period_end = $6, cancel_at_period_end = $7,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.QueryRowContext(ctx, query, sub.ID, sub.StripeCustomerID,
		sub.StripeSubscriptionID, sub.Plan, sub.Status, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd).Scan(&sub.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}
