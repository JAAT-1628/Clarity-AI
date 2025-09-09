package models

import (
	"time"

	"github.com/google/uuid"
)

type SubscriptionStatus string

const (
	StatusActive     SubscriptionStatus = "active"
	StatusCanceled   SubscriptionStatus = "canceled"
	StatusIncomplete SubscriptionStatus = "incomplete"
	StatusPastDue    SubscriptionStatus = "past_due"
	StatusTrialing   SubscriptionStatus = "trialing"
	StatusUnpaid     SubscriptionStatus = "unpaid"
	StatusInactive   SubscriptionStatus = "inactive"
)

type Subscription struct {
	ID                   uuid.UUID          `json:"id" db:"id"`
	UserID               int64              `json:"user_id" db:"user_id"`
	StripeCustomerID     *string            `json:"stripe_customer_id" db:"stripe_customer_id"`
	StripeSubscriptionID *string            `json:"stripe_subscription_id" db:"stripe_subscription_id"`
	Plan                 UserPlan           `json:"plan" db:"plan"`
	Status               SubscriptionStatus `json:"status" db:"status"`
	CurrentPeriodEnd     *time.Time         `json:"current_period_end" db:"current_period_end"`
	CancelAtPeriodEnd    bool               `json:"cancel_at_period_end" db:"cancel_at_period_end"`
	CreatedAt            time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at" db:"updated_at"`
}
