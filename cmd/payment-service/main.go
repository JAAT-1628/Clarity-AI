package main

import (
	billingPb "clarity-ai/api/proto/billing"
	"clarity-ai/internal/config"
	"clarity-ai/internal/domain/services"
	"clarity-ai/internal/infrastructure/database"
	handlers "clarity-ai/internal/interfaces/grpc/payment"
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stripe/stripe-go/v79"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg := config.Load()

	stripe.Key = cfg.Billing.StripeSecret
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := database.NewPostgresConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	subscriptionRepo := database.NewSubscriptionRepository(db.DB)
	userRepo := database.NewUserRepository(db)

	paymentService := services.NewStripeService(subscriptionRepo, userRepo, logger)
	paymentHandler := handlers.NewPaymentHandler(paymentService, logger)

	go func() {
		if err := startGRPCServer(cfg.Billing.BillingGRPCPort, paymentHandler); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	time.Sleep(2 * time.Second)
	startHTTPServer(cfg.Server.Port, paymentHandler)
}

func startGRPCServer(port string, handler *handlers.PaymentHandler) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer()
	billingPb.RegisterBillingServiceServer(grpcServer, handler)
	reflection.Register(grpcServer)

	return grpcServer.Serve(lis)
}

func startHTTPServer(httpPort string, handler *handlers.PaymentHandler) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/billing/checkout", handler.CreateCheckoutHTTP)
	mux.HandleFunc("/api/billing/subscription", handler.GetSubscriptionHTTP)
	mux.HandleFunc("/success", handler.HandleCheckoutSuccess)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy", "service": "payment-service"}`))
	})

	server := &http.Server{
		Addr:    ":" + httpPort,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("✅ Payment Service stopped")
}
