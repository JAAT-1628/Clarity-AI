package queue

import (
	"clarity-ai/internal/domain/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type StreamingRedisQueue struct {
	*RedisQueue
	subscriptions map[string]*models.StreamSubscription
	mutex         sync.RWMutex
}

func NewStreamingRedisQueue(redisClient *redis.Client) *StreamingRedisQueue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Redis connection validation failed: %v", err)
	}

	return &StreamingRedisQueue{
		RedisQueue:    NewRedisQueue(redisClient),
		subscriptions: make(map[string]*models.StreamSubscription),
	}
}

func (q *StreamingRedisQueue) PublishStreamingUpdate(ctx context.Context, msg *models.StreamMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	seqKey := fmt.Sprintf("analysis_seq:%s", msg.AnalysisID)
	seq, err := q.client.Incr(ctx, seqKey).Result()
	if err != nil {
		return fmt.Errorf("failed to increment sequence: %w", err)
	}
	msg.Sequence = seq

	msgData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal streaming message: %w", err)
	}

	channels := []string{
		fmt.Sprintf("analysis_stream:%s", msg.AnalysisID),
		"analysis_stream:global",
	}

	if msg.ChunkID != "" {
		channels = append(channels, fmt.Sprintf("chunk_stream:%s", msg.ChunkID))
	}

	publishSuccess := 0
	publishErrors := []error{}

	for _, channel := range channels {
		if err := q.client.Publish(ctx, channel, msgData).Err(); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("channel %s: %w", channel, err))
			continue
		}
		publishSuccess++
	}

	streamKey := fmt.Sprintf("stream:analysis:%s", msg.AnalysisID)
	fields := map[string]interface{}{
		"event_type":   string(msg.EventType),
		"chunk_id":     msg.ChunkID,
		"chunk_number": msg.ChunkNumber,
		"timestamp":    msg.Timestamp.Unix(),
		"sequence":     msg.Sequence,
		"data":         string(msgData),
	}

	_, err = q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		ID:     "*",
		Values: fields,
		MaxLen: 1000,
	}).Result()
	if err != nil {
		publishErrors = append(publishErrors, fmt.Errorf("stream %s: %w", streamKey, err))
	} else {
		log.Printf("Added %s to Redis stream %s", msg.EventType, streamKey)
	}

	if publishSuccess == 0 && len(publishErrors) > 0 {
		return fmt.Errorf("failed to publish to any channel: %v", publishErrors)
	}

	return nil
}

func (q *StreamingRedisQueue) SubscribeToAnalysisStream(ctx context.Context, analysisID string, userID int64) (*models.StreamSubscription, error) {
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.client.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	subscription := &models.StreamSubscription{
		ID:         uuid.New().String(),
		AnalysisID: analysisID,
		UserID:     userID,
		Channel:    make(chan *models.StreamMessage, 100),
		Connected:  time.Now(),
		LastSeen:   time.Now(),
	}

	q.mutex.Lock()
	q.subscriptions[subscription.ID] = subscription
	q.mutex.Unlock()

	streamKey := fmt.Sprintf("stream:analysis:%s", analysisID)
	lastID := "$"

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in stream subscriber for %s: %v", analysisID, r)
			}
			close(subscription.Channel)
			q.mutex.Lock()
			delete(q.subscriptions, subscription.ID)
			q.mutex.Unlock()
			log.Printf("Closed stream subscription %s for analysis %s", subscription.ID, analysisID)
		}()

		backoff := time.Second

		for {
			if ctx.Err() != nil {
				log.Printf("Context cancelled for subscription %s (analysis %s)", subscription.ID, analysisID)
				return
			}

			res, err := q.client.XRead(ctx, &redis.XReadArgs{
				Streams: []string{streamKey, lastID},
				Block:   5 * time.Second,
				Count:   100,
			}).Result()

			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue
				}

				if ctx.Err() != nil {
					log.Printf("🔄 Subscription context closed for %s: %v", analysisID, ctx.Err())
					return
				}

				time.Sleep(backoff)
				if backoff < 5*time.Second {
					backoff *= 2
				}
				continue
			}

			backoff = time.Second

			for _, stream := range res {
				if stream.Stream != streamKey {
					continue
				}
				for _, xmsg := range stream.Messages {
					lastID = xmsg.ID

					raw, ok := xmsg.Values["data"].(string)
					if !ok || raw == "" {
						var msg models.StreamMessage
						msg.AnalysisID = analysisID
						if et, ok := xmsg.Values["event_type"].(string); ok {
							msg.EventType = models.StreamEventType(et)
						}
						if chunkID, ok := xmsg.Values["chunk_id"].(string); ok {
							msg.ChunkID = chunkID
						}
						if cn, ok := xmsg.Values["chunk_number"].(string); ok {
							if n, err := strconv.Atoi(cn); err == nil {
								msg.ChunkNumber = n
							}
						}
						if tsStr, ok := xmsg.Values["timestamp"].(string); ok {
							if unix, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
								msg.Timestamp = time.Unix(unix, 0)
							}
						}
						select {
						case subscription.Channel <- &msg:
							subscription.LastSeen = time.Now()
						case <-time.After(5 * time.Second):
							log.Printf("Subscriber %s slow, dropping fallback message", subscription.ID)
						case <-ctx.Done():
							return
						}
						continue
					}

					var streamMsg models.StreamMessage
					if err := json.Unmarshal([]byte(raw), &streamMsg); err != nil {
						log.Printf("❌ Failed to unmarshal stream message for %s: %v", analysisID, err)
						continue
					}

					select {
					case subscription.Channel <- &streamMsg:
						subscription.LastSeen = time.Now()
					case <-time.After(5 * time.Second):
						log.Printf("Subscriber %s slow, dropping %s message", subscription.ID, streamMsg.EventType)
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return subscription, nil
}

func (q *StreamingRedisQueue) GetStreamHistory(ctx context.Context, analysisID string, count int64) ([]*models.StreamMessage, error) {
	streamKey := fmt.Sprintf("stream:analysis:%s", analysisID) // Fixed colon missing
	result, err := q.client.XRevRange(ctx, streamKey, "+", "-").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get stream history: %w", err)
	}

	messages := make([]*models.StreamMessage, 0, len(result))
	for i, xMsg := range result {
		if int64(i) >= count && count > 0 {
			break
		}

		if dataField, ok := xMsg.Values["data"].(string); ok {
			var streamMsg models.StreamMessage
			if err := json.Unmarshal([]byte(dataField), &streamMsg); err == nil {
				messages = append(messages, &streamMsg)
			}
		}
	}

	return messages, nil
}

func (q *StreamingRedisQueue) PublishTranscriptUpdate(ctx context.Context, analysisID, chunkID string, chunkNumber int, startTime, endTime, transcript string) error {
	msg := &models.StreamMessage{
		AnalysisID:  analysisID,
		ChunkID:     chunkID,
		ChunkNumber: chunkNumber,
		EventType:   models.EventTranscriptReady,
		Data: &models.TranscriptData{
			ChunkID:     chunkID,
			ChunkNumber: chunkNumber,
			StartTime:   startTime,
			EndTime:     endTime,
			Text:        transcript,
			WordCount:   len(strings.Fields(transcript)),
		},
	}
	return q.PublishStreamingUpdate(ctx, msg)
}

func (q *StreamingRedisQueue) PublishSummaryUpdate(ctx context.Context, analysisID, chunkID string, chunkNumber int, summary string) error {
	msg := &models.StreamMessage{
		AnalysisID:  analysisID,
		ChunkID:     chunkID,
		ChunkNumber: chunkNumber,
		EventType:   models.EventSummaryGenerated,
		Data: &models.SummaryData{
			ChunkID:     chunkID,
			ChunkNumber: chunkNumber,
			Summary:     summary,
		},
	}
	return q.PublishStreamingUpdate(ctx, msg)
}

func (q *StreamingRedisQueue) PublishTopicsUpdate(ctx context.Context, analysisID, chunkID string, chunkNumber int, topics []*models.Topic) error {
	msg := &models.StreamMessage{
		AnalysisID:  analysisID,
		ChunkID:     chunkID,
		ChunkNumber: chunkNumber,
		EventType:   models.EventTopicsGenerated,
		Data: &models.TopicsData{
			ChunkID:     chunkID,
			ChunkNumber: chunkNumber,
			Topics:      topics,
		},
	}
	return q.PublishStreamingUpdate(ctx, msg)
}

func (q *StreamingRedisQueue) PublishQuestionsUpdate(ctx context.Context, analysisID, chunkID string, chunkNumber int, questions []*models.Question) error {
	msg := &models.StreamMessage{
		AnalysisID:  analysisID,
		ChunkID:     chunkID,
		ChunkNumber: chunkNumber,
		EventType:   models.EventQuestionsGenerated,
		Data: &models.QuestionsData{
			ChunkID:     chunkID,
			ChunkNumber: chunkNumber,
			Questions:   questions,
		},
	}
	return q.PublishStreamingUpdate(ctx, msg)
}

func (q *StreamingRedisQueue) PublishInsightsUpdate(ctx context.Context, analysisID, chunkID string, chunkNumber int, insights []*models.KeyInsight) error {
	msg := &models.StreamMessage{
		AnalysisID:  analysisID,
		ChunkID:     chunkID,
		ChunkNumber: chunkNumber,
		EventType:   models.EventInsightsGenerated,
		Data: &models.InsightsData{
			ChunkID:     chunkID,
			ChunkNumber: chunkNumber,
			Insights:    insights,
		},
	}
	return q.PublishStreamingUpdate(ctx, msg)
}

func (q *StreamingRedisQueue) PublishProgressUpdate(ctx context.Context, analysisID, chunkID string, chunkNumber int, progress *models.ProgressData) error {
	msg := &models.StreamMessage{
		AnalysisID:  analysisID,
		ChunkID:     chunkID,
		ChunkNumber: chunkNumber,
		EventType:   models.EventAnalysisProgress,
		Progress:    progress,
	}
	return q.PublishStreamingUpdate(ctx, msg)
}

func (q *StreamingRedisQueue) PublishErrorUpdate(ctx context.Context, analysisID, chunkID string, chunkNumber int, errorData *models.ErrorData) error {
	msg := &models.StreamMessage{
		AnalysisID:  analysisID,
		ChunkID:     chunkID,
		ChunkNumber: chunkNumber,
		EventType:   models.EventChunkFailed,
		Error:       errorData,
	}
	return q.PublishStreamingUpdate(ctx, msg)
}

func (q *StreamingRedisQueue) CleanupSubscriptions() {
	cutoff := time.Now().Add(-30 * time.Minute)
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for id, sub := range q.subscriptions {
		if sub.LastSeen.Before(cutoff) {
			close(sub.Channel)
			delete(q.subscriptions, id)
			log.Printf("🧹 Cleaned up inactive subscription %s", id)
		}
	}
}

func (q *StreamingRedisQueue) GetActiveSubscriptionCount(analysisID string) int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	count := 0
	for _, sub := range q.subscriptions {
		if sub.AnalysisID == analysisID {
			count++
		}
	}
	return count
}
