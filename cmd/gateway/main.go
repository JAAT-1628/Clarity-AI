package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	billingPb "clarity-ai/api/proto/billing"
	userPb "clarity-ai/api/proto/user"
	videoPb "clarity-ai/api/proto/video"
	"clarity-ai/internal/config"
	"clarity-ai/internal/infrastructure/queue"
	"clarity-ai/internal/interfaces/http/middleware"
	"clarity-ai/internal/websocket"
)

func main() {
	cfg := config.Load()

	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:            getRedisAddr(cfg),
		Password:        cfg.Redis.Password,
		DB:              cfg.Redis.DB,
		Network:         "tcp",
		DialTimeout:     10 * time.Second,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		PoolSize:        10,
		MinIdleConns:    5,
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	})
	defer redisClient.Close()

	streamingQueue := queue.NewStreamingRedisQueue(redisClient)
	wsHandler := websocket.NewWebSocketHandler(streamingQueue)

	router := gin.Default()
	router.Use(middleware.CORS())
	router.Use(middleware.RequestLogger())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "clarity-ai-gateway",
			"time":    time.Now(),
		})
	})

	ctx := context.Background()
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	if err := userPb.RegisterUserServiceHandlerFromEndpoint(ctx, mux, getUserServiceAddr(cfg), opts); err != nil {
		log.Fatalf("Failed to register User Service: %v", err)
	}

	if err := videoPb.RegisterVideoAnalyzerServiceHandlerFromEndpoint(ctx, mux, getVideoServiceAddr(cfg), opts); err != nil {
		log.Fatalf("Failed to register Video Service: %v", err)
	}

	if err := billingPb.RegisterBillingServiceHandlerFromEndpoint(ctx, mux, getBillingServiceAddr(cfg), opts); err != nil {
		log.Fatalf("Failed to register Billing Service: %v", err)
	}

	router.POST("/api/billing/webhook", func(c *gin.Context) {
		payload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
			return
		}

		headers := make(map[string]string, len(c.Request.Header))
		for k, v := range c.Request.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}

		conn, err := grpc.Dial(getBillingServiceAddr(cfg), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "billing service unavailable"})
			return
		}
		defer conn.Close()

		client := billingPb.NewBillingServiceClient(conn)
		gctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		resp, err := client.HandleWebhook(gctx, &billingPb.HandleWebhookRequest{
			RawBody: payload,
			Headers: headers,
		})

		if err != nil || !resp.GetAccepted() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "webhook not processed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	router.GET("/success", func(c *gin.Context) {
		sessionID := c.Query("session_id")
		if sessionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
			return
		}

		paymentServiceURL := getPaymentServiceHTTPAddr(cfg)
		successURL := fmt.Sprintf("%s/success?session_id=%s", paymentServiceURL, sessionID)

		resp, err := http.Get(successURL)
		if err != nil {
			log.Printf("Failed to call payment service success endpoint: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process payment completion"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			c.JSON(http.StatusOK, gin.H{
				"status":     "success",
				"message":    "Payment completed successfully",
				"session_id": sessionID,
			})
		} else {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Payment service returned error: %s", string(body))
			c.JSON(resp.StatusCode, gin.H{"error": "failed to process payment completion"})
		}
	})

	router.GET("/cancel", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "cancelled",
			"message": "Payment was cancelled",
		})
	})

	router.Any("/api/auth/*path", gin.WrapH(mux))

	streamingGroup := router.Group("/stream")
	streamingGroup.GET("/video/:analysis_id", gin.WrapH(wsHandler))
	streamingGroup.GET("/status", gin.WrapH(http.HandlerFunc(wsHandler.GetStatus)))
	streamingGroup.GET("/connections", gin.WrapH(http.HandlerFunc(wsHandler.GetAnalysisConnections)))

	apiGroup := router.Group("/api")
	apiGroup.Use(middleware.JWTAuthMiddleware(getUserServiceAddr(cfg)))

	apiGroup.POST("/billing/checkout", gin.WrapH(mux))
	apiGroup.GET("/billing/subscription", gin.WrapH(mux))
	apiGroup.POST("/billing/cancel", gin.WrapH(mux))

	apiGroup.Any("/user/*path", gin.WrapH(mux))
	apiGroup.Any("/video/*path", gin.WrapH(mux))

	srv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("✅ API Gateway stopped")
}

func getRedisAddr(cfg *config.Config) string {
	if cfg.Server.Environment == "development" {
		return cfg.Redis.Host + ":" + cfg.Redis.Port
	}
	return "redis:6379"
}

func getUserServiceAddr(cfg *config.Config) string {
	if cfg.Server.Environment == "development" {
		return "localhost:" + cfg.Server.GRPCPort
	}
	return "user-service:" + cfg.Server.GRPCPort
}

func getVideoServiceAddr(cfg *config.Config) string {
	if cfg.Server.Environment == "development" {
		return "localhost:" + cfg.Server.VideoGRPCPort
	}
	return "video-service:" + cfg.Server.VideoGRPCPort
}

func getBillingServiceAddr(cfg *config.Config) string {
	if cfg.Server.Environment == "development" {
		return "localhost:" + cfg.Billing.BillingGRPCPort
	}
	return "payments-service:" + cfg.Billing.BillingGRPCPort
}

func getPaymentServiceHTTPAddr(cfg *config.Config) string {
	if cfg.Server.Environment == "development" {
		return "localhost:" + cfg.Server.Port
	}
	return "payments-service:" + cfg.Server.Port
}
