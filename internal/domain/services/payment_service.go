package services

import (
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/customer"
	"github.com/stripe/stripe-go/v79/subscription"
)

var PlanToPriceID = map[models.UserPlan]string{
	models.PlanPremium: "price_1S247PGdis6BRvOZqfifSGRZ",
	models.PlanPro:     "price_1S246gGdis6BRvOZjNWeuMw9",
}

type PaymentService interface {
	CreateCheckoutSession(ctx context.Context, userID int64, plan models.UserPlan, successURL, cancelURL string) (string, string, error)
	GetSubscription(ctx context.Context, userID int64) (*models.Subscription, error)
	CancelSubscription(ctx context.Context, userID int64, cancelAtPeriodEnd bool) error
	SyncSubscription(ctx context.Context, userID int64) error
}

type StripeService struct {
	repo     repositories.SubscriptionRepository
	userRepo repositories.UserRepository
	logger   *slog.Logger
}

func NewStripeService(repo repositories.SubscriptionRepository, userRepo repositories.UserRepository, logger *slog.Logger) *StripeService {
	return &StripeService{
		repo:     repo,
		userRepo: userRepo,
		logger:   logger,
	}
}

func (s *StripeService) CreateCheckoutSession(ctx context.Context, userID int64, plan models.UserPlan, successURL, cancelURL string) (string, string, error) {
	priceID, exists := PlanToPriceID[plan]
	if !exists {
		return "", "", fmt.Errorf("invalid plan: %s", plan)
	}

	customerID, err := s.getOrCreateCustomer(ctx, userID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get customer: %w", err)
	}

	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer:   stripe.String(customerID),
		SuccessURL: stripe.String(successURL + "?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		Metadata: map[string]string{
			"user_id": strconv.FormatInt(userID, 10),
			"plan":    string(plan),
		},
	}

	sess, err := session.New(params)
	if err != nil {
		return "", "", fmt.Errorf("failed to create checkout session: %w", err)
	}

	return sess.URL, sess.ID, nil
}

func (s *StripeService) GetSubscription(ctx context.Context, userID int64) (*models.Subscription, error) {
	sub, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("subscription not found: %w", err)
	}

	if sub.StripeSubscriptionID != nil {
		if err := s.syncSubscriptionFromStripe(ctx, sub); err != nil {
			s.logError("failed to sync subscription", err, "user_id", userID)
		}
	}

	return sub, nil
}

func (s *StripeService) CancelSubscription(ctx context.Context, userID int64, cancelAtPeriodEnd bool) error {
	sub, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	if sub.StripeSubscriptionID == nil {
		return fmt.Errorf("no active Stripe subscription found")
	}

	var stripeSub *stripe.Subscription
	if cancelAtPeriodEnd {
		stripeSub, err = subscription.Update(*sub.StripeSubscriptionID, &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		})
	} else {
		stripeSub, err = subscription.Cancel(*sub.StripeSubscriptionID, nil)
	}

	if err != nil {
		return fmt.Errorf("failed to cancel subscription in Stripe: %w", err)
	}

	sub.Status = models.SubscriptionStatus(stripeSub.Status)
	sub.CancelAtPeriodEnd = stripeSub.CancelAtPeriodEnd
	if stripeSub.CurrentPeriodEnd > 0 {
		end := time.Unix(stripeSub.CurrentPeriodEnd, 0)
		sub.CurrentPeriodEnd = &end
	}

	if err := s.repo.Update(ctx, sub); err != nil {
		s.logError("failed to update subscription after cancellation", err)
	}

	if !cancelAtPeriodEnd {
		if err := s.UpdateUserPlan(ctx, userID, models.PlanFree); err != nil {
			s.logError("failed to update user plan", err, "user_id", userID)
		}
	}

	return nil
}

func (s *StripeService) SyncSubscription(ctx context.Context, userID int64) error {
	sub, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	return s.syncSubscriptionFromStripe(ctx, sub)
}

func (s *StripeService) CreateSubscriptionRecord(ctx context.Context, sub *models.Subscription) error {
	return s.repo.Create(ctx, sub)
}

func (s *StripeService) UpdateUserPlan(ctx context.Context, userID int64, plan models.UserPlan) error {
	user, err := s.userRepo.GetUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	user.Plan = plan
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user plan: %w", err)
	}

	return nil
}

func (s *StripeService) syncSubscriptionFromStripe(ctx context.Context, sub *models.Subscription) error {
	if sub.StripeSubscriptionID == nil {
		return nil
	}

	stripeSub, err := subscription.Get(*sub.StripeSubscriptionID, nil)
	if err != nil {
		return fmt.Errorf("failed to get subscription from Stripe: %w", err)
	}

	sub.Status = models.SubscriptionStatus(stripeSub.Status)
	sub.CancelAtPeriodEnd = stripeSub.CancelAtPeriodEnd

	if stripeSub.CurrentPeriodEnd > 0 {
		end := time.Unix(stripeSub.CurrentPeriodEnd, 0)
		sub.CurrentPeriodEnd = &end
	}

	if len(stripeSub.Items.Data) > 0 {
		priceID := stripeSub.Items.Data[0].Plan.ID
		for plan, pID := range PlanToPriceID {
			if pID == priceID {
				sub.Plan = plan
				break
			}
		}
	}

	if err := s.repo.Update(ctx, sub); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	switch stripeSub.Status {
	case "active", "trialing":
		if err := s.UpdateUserPlan(ctx, sub.UserID, sub.Plan); err != nil {
			s.logError("failed to update user plan", err, "user_id", sub.UserID)
		}
	case "canceled", "unpaid":
		if err := s.UpdateUserPlan(ctx, sub.UserID, models.PlanFree); err != nil {
			s.logError("failed to downgrade user plan", err, "user_id", sub.UserID)
		}
	}

	return nil
}

func (s *StripeService) getOrCreateCustomer(ctx context.Context, userID int64) (string, error) {
	if sub, err := s.repo.GetByUserID(ctx, userID); err == nil && sub.StripeCustomerID != nil {
		return *sub.StripeCustomerID, nil
	}

	user, err := s.userRepo.GetUserID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	params := &stripe.CustomerParams{
		Email: stripe.String(user.Email),
		Name:  stripe.String(user.Username),
		Metadata: map[string]string{
			"user_id": strconv.FormatInt(userID, 10),
		},
	}

	cust, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("failed to create Stripe customer: %w", err)
	}

	return cust.ID, nil
}

func (s *StripeService) logError(msg string, err error, args ...interface{}) {
	if s.logger != nil {
		allArgs := append([]interface{}{"error", err}, args...)
		s.logger.Error(msg, allArgs...)
	}
}
