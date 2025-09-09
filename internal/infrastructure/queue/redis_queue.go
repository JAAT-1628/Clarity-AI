package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(redisClient *redis.Client) *RedisQueue {
	return &RedisQueue{client: redisClient}
}

type VideoProcessingJob struct {
	AnalysisID  string                 `json:"analysis_id"`
	ChunkID     string                 `json:"chunk_id"`
	ChunkNumber int                    `json:"chunk_number"`
	VideoURL    string                 `json:"video_url"`
	StartTime   string                 `json:"start_time"`
	EndTime     string                 `json:"end_time"`
	UserID      int64                  `json:"user_id"`
	AIProvider  string                 `json:"ai_provider"`
	Options     map[string]interface{} `json:"options"`
}

func (q *RedisQueue) EnqueueVideoChunk(ctx context.Context, job VideoProcessingJob) error {
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job %w", err)
	}

	return q.client.LPush(ctx, "video_processing_queue", jobData).Err()
}

func (q *RedisQueue) DequeueVideoChunck(ctx context.Context) (*VideoProcessingJob, error) {
	result, err := q.client.BRPop(ctx, 30*time.Second, "video_processing_queue").Result()
	if err != nil {
		return nil, err
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("invalid queue result")
	}

	var job VideoProcessingJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

func (q *RedisQueue) PublishChunkUpdate(ctx context.Context, analysisID string, update interface{}) error {
	updateData, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal update: %w", err)
	}

	channel := fmt.Sprintf("analysis_updates:%s", analysisID)
	return q.client.Publish(ctx, channel, updateData).Err()
}
