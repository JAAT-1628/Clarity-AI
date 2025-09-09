package main

import (
	pb "clarity-ai/api/proto/video"
	"clarity-ai/internal/config"
	"clarity-ai/internal/domain/services"
	"clarity-ai/internal/infrastructure/database"
	"clarity-ai/internal/infrastructure/queue"
	handler "clarity-ai/internal/interfaces/grpc"
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgresConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Database connection test failed: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Host + ":" + cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	videoRepo := database.NewVideoAnalysisRepository(db)
	userRepo := database.NewUserRepository(db)
	jobQueue := queue.NewRedisQueue(redisClient)

	baseGatewayURL := "localhost:" + cfg.Server.Port
	videoService := services.NewVideoAnalysisService(videoRepo, userRepo, jobQueue, baseGatewayURL)

	listener, err := net.Listen("tcp", ":"+cfg.Server.VideoGRPCPort)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", cfg.Server.VideoGRPCPort, err)
	}

	grpcServer := grpc.NewServer()
	videoHandler := handler.NewVideoAnalysisHandler(videoService)
	pb.RegisterVideoAnalyzerServiceServer(grpcServer, videoHandler)

	if cfg.Server.Environment == "development" {
		reflection.Register(grpcServer)
	}

	go func() {
		log.Printf("Video Service listening on gRPC port :%s ...", cfg.Server.VideoGRPCPort)
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	grpcServer.GracefulStop()
	log.Println("Video Service stopped cleanly.")
}
