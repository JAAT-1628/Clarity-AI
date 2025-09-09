package main

import (
	"clarity-ai/internal/config"
	"clarity-ai/internal/infrastructure/database"
	"clarity-ai/internal/infrastructure/queue"
	videoprocessor "clarity-ai/internal/workers/videoProcessor"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgresConnection(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Host + ":" + cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	videoRepo := database.NewVideoAnalysisRepository(db)
	regularQueue := queue.NewRedisQueue(redisClient)

	streamingQueue := queue.NewStreamingRedisQueue(redisClient)
	processor := videoprocessor.NewStreamingProcessor(videoRepo, regularQueue, streamingQueue)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		processor.Run(ctx)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	cancel()
	time.Sleep(2 * time.Second)
	log.Println("Video Worker stopped cleanly.")
}
