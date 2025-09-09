package main

import (
	pb "clarity-ai/api/proto/user"
	"clarity-ai/internal/config"
	"clarity-ai/internal/domain/services"
	"clarity-ai/internal/infrastructure/database"
	"clarity-ai/internal/interfaces/grpc/user"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	userRepo := database.NewUserRepository(db)
	jwtService := services.NewJWTService(cfg.JWT.Secret, time.Duration(cfg.JWT.Expiration)*time.Second)
	authService := services.NewAuthService(userRepo, jwtService)

	lis, err := net.Listen("tcp", ":"+cfg.Server.GRPCPort)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", cfg.Server.GRPCPort, err)
	}

	grpcServer := grpc.NewServer()

	userHandler := user.NewUserServiceHandler(authService, userRepo)
	pb.RegisterUserServiceServer(grpcServer, userHandler)

	if cfg.Server.Environment == "development" {
		reflection.Register(grpcServer)
	}

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	grpcServer.GracefulStop()
	log.Println("✅ User Service stopped")
}
