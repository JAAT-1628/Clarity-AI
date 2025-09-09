package handlers

import (
	pb "clarity-ai/api/proto/billing"
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/services"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/subscription"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PaymentHandler struct {
	pb.UnimplementedBillingServiceServer
	service services.PaymentService
	logger  *slog.Logger
}

func NewPaymentHandler(service services.PaymentService, logger *slog.Logger) *PaymentHandler {
	return &PaymentHandler{
		service: service,
		logger:  logger,
	}
}

// gRPC Handlers
func (h *PaymentHandler) CreateCheckoutSession(ctx context.Context, req *pb.CreateCheckoutSessionRequest) (*pb.CreateCheckoutSessionResponse, error) {
	if req.GetUserId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	priceID := strings.TrimSpace(req.GetPriceId())
	if priceID == "" {
		return nil, status.Error(codes.InvalidArgument, "price_id is required")
	}

	// Determine plan from price ID
	var plan models.UserPlan
	for planType, pID := range services.PlanToPriceID {
		if pID == priceID {
			plan = planType
			break
		}
	}

	if plan == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid price_id")
	}

	url, sessionID, err := h.service.CreateCheckoutSession(ctx, req.GetUserId(), plan, req.GetSuccessUrl(), req.GetCancelUrl())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create checkout session: %v", err)
	}

	return &pb.CreateCheckoutSessionResponse{
		CheckoutUrl: url,
		SessionId:   sessionID,
	}, nil
}

func (h *PaymentHandler) GetSubscription(ctx context.Context, req *pb.GetSubscriptionRequest) (*pb.GetSubscriptionResponse, error) {
	if req.UserId == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	sub, err := h.service.GetSubscription(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	var currentPeriodEnd int64
	if sub.CurrentPeriodEnd != nil {
		currentPeriodEnd = sub.CurrentPeriodEnd.Unix()
	}

	return &pb.GetSubscriptionResponse{
		Status:           string(sub.Status),
		Plan:             string(sub.Plan),
		CurrentPeriodEnd: currentPeriodEnd,
	}, nil
}

func (h *PaymentHandler) CancelSubscription(ctx context.Context, req *pb.CancelSubscriptionRequest) (*pb.CancelSubscriptionResponse, error) {
	if req.UserId == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	if err := h.service.CancelSubscription(ctx, req.UserId, req.CancelAtPeriodEnd); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.CancelSubscriptionResponse{
		Success: true,
		Message: "subscription cancelled successfully",
	}, nil
}

func (h *PaymentHandler) HandleWebhook(ctx context.Context, req *pb.HandleWebhookRequest) (*pb.HandleWebhookResponse, error) {
	// Since we're avoiding webhooks, just return accepted
	return &pb.HandleWebhookResponse{Accepted: true}, nil
}

// HTTP Handlers
func (h *PaymentHandler) CreateCheckoutHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     int64  `json:"user_id"`
		PriceID    string `json:"price_id"`
		SuccessURL string `json:"success_url"`
		CancelURL  string `json:"cancel_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid request", err)
		return
	}

	if req.UserID <= 0 {
		h.sendError(w, http.StatusBadRequest, "user_id is required", nil)
		return
	}

	if req.PriceID == "" {
		h.sendError(w, http.StatusBadRequest, "price_id is required", nil)
		return
	}

	// Determine plan from price ID
	var plan models.UserPlan
	for planType, pID := range services.PlanToPriceID {
		if pID == req.PriceID {
			plan = planType
			break
		}
	}

	if plan == "" {
		h.sendError(w, http.StatusBadRequest, "invalid price_id", nil)
		return
	}

	url, sessionID, err := h.service.CreateCheckoutSession(r.Context(), req.UserID, plan, req.SuccessURL, req.CancelURL)
	if err != nil {
		h.logError("failed to create checkout session", err, "user_id", req.UserID)
		h.sendError(w, http.StatusInternalServerError, "failed to create checkout session", err)
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]string{
		"checkout_url": url,
		"session_id":   sessionID,
	})
}

func (h *PaymentHandler) GetSubscriptionHTTP(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	if userIDStr == "" {
		h.sendError(w, http.StatusBadRequest, "user_id is required", nil)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid user_id", err)
		return
	}

	sub, err := h.service.GetSubscription(r.Context(), userID)
	if err != nil {
		h.sendError(w, http.StatusNotFound, "subscription not found", err)
		return
	}

	h.sendJSON(w, http.StatusOK, sub)
}

// Success handler that creates subscription after payment
func (h *PaymentHandler) HandleCheckoutSuccess(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		h.sendError(w, http.StatusBadRequest, "session_id is required", nil)
		return
	}

	// Get the checkout session from Stripe
	sess, err := session.Get(sessionID, nil)
	if err != nil {
		h.logError("failed to get checkout session", err, "session_id", sessionID)
		h.sendError(w, http.StatusInternalServerError, "failed to verify payment", err)
		return
	}

	// Extract user ID from metadata
	userIDStr, exists := sess.Metadata["user_id"]
	if !exists {
		h.sendError(w, http.StatusBadRequest, "user_id not found in session", nil)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid user_id", err)
		return
	}

	// Create subscription if payment was successful
	if sess.PaymentStatus == "paid" && sess.Subscription != nil {
		if err := h.createSubscriptionFromSession(r.Context(), userID, sess); err != nil {
			h.logError("failed to create subscription", err, "user_id", userID)
			// Don't fail the request - payment was successful
		}
	}

	// Return success response
	h.sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "success",
		"session_id": sessionID,
		"message":    "Payment completed successfully",
		"user_id":    userID,
	})
}

// Helper function to create subscription from successful checkout
func (h *PaymentHandler) createSubscriptionFromSession(ctx context.Context, userID int64, sess *stripe.CheckoutSession) error {
	// Get subscription details from Stripe
	stripeSub, err := subscription.Get(sess.Subscription.ID, nil)
	if err != nil {
		return fmt.Errorf("failed to get subscription from Stripe: %w", err)
	}

	// Extract plan from metadata
	planStr, exists := sess.Metadata["plan"]
	if !exists {
		return fmt.Errorf("plan not found in session metadata")
	}
	plan := models.UserPlan(planStr)

	// Create subscription in database
	sub := &models.Subscription{
		UserID:               userID,
		StripeCustomerID:     &sess.Customer.ID,
		StripeSubscriptionID: &stripeSub.ID,
		Plan:                 plan,
		Status:               models.SubscriptionStatus(stripeSub.Status),
		CancelAtPeriodEnd:    stripeSub.CancelAtPeriodEnd,
	}

	if stripeSub.CurrentPeriodEnd > 0 {
		end := time.Unix(stripeSub.CurrentPeriodEnd, 0)
		sub.CurrentPeriodEnd = &end
	}

	// Save to database - need to access the repository through service
	stripeService, ok := h.service.(*services.StripeService)
	if !ok {
		return fmt.Errorf("invalid service type")
	}

	if err := stripeService.CreateSubscriptionRecord(ctx, sub); err != nil {
		return fmt.Errorf("failed to save subscription: %w", err)
	}

	// Update user plan if subscription is active
	if stripeSub.Status == "active" || stripeSub.Status == "trialing" {
		if err := stripeService.UpdateUserPlan(ctx, userID, plan); err != nil {
			h.logError("failed to update user plan", err, "user_id", userID)
		}
	}

	return nil
}

func (h *PaymentHandler) sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *PaymentHandler) sendError(w http.ResponseWriter, status int, message string, err error) {
	response := map[string]string{"error": message}
	if err != nil {
		response["details"] = err.Error()
	}
	h.sendJSON(w, status, response)
}

func (h *PaymentHandler) logError(msg string, err error, args ...interface{}) {
	if h.logger != nil {
		allArgs := append([]interface{}{"error", err}, args...)
		h.logger.Error(msg, allArgs...)
	}
}
