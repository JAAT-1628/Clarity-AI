package repositories

import (
	"clarity-ai/internal/domain/models"
	"context"
	"time"
)

type VideoAnalysisRepository interface {
	//analysis operaions
	CreateAnalysis(ctx context.Context, analysis *models.VideoAnalysis) error
	GetAnalysisByID(ctx context.Context, id string) (*models.VideoAnalysis, error)
	GetAnalysisByUserID(ctx context.Context, userID int64, limit, offset int) ([]*models.VideoAnalysis, error)
	UpdateAnalysisStatus(ctx context.Context, id string, status models.AnalysisStatus) error
	UpdateAnalysisMetadata(ctx context.Context, id string, metadata map[string]interface{}) error
	UpdateAnalysisCompletedAt(ctx context.Context, analysisID string, completedAt *time.Time) error
	// chunks operations
	CreateChunk(ctx context.Context, chunk *models.VideoChunk) error
	GetChunk(ctx context.Context, analysisID, chunkID string) (*models.VideoChunk, error)
	GetChunksByAnalysisID(ctx context.Context, analysisID string) ([]*models.VideoChunk, error)
	UpdateChunkStatus(ctx context.Context, chunkID string, status models.ChunkStatus, stage string, progress int) error
	UpdateChunkContent(ctx context.Context, chunkID string, transcript, summary string) error
	UpdateAnalysisChunkCounts(ctx context.Context, analysisID string, completedChunks, failedChunks int) error

	// topics, questions, insights
	CreateTopic(ctx context.Context, topic *models.Topic) error
	CreateQuestion(ctx context.Context, question *models.Question) error
	CreateKeyInsight(ctx context.Context, insight *models.KeyInsight) error
	GetTopicsByChunkID(ctx context.Context, chunkID string) ([]*models.Topic, error)
	GetQuestionsByChunkID(ctx context.Context, chunkID string) ([]*models.Question, error)
	GetInsightsByChunkID(ctx context.Context, chunkID string) ([]*models.KeyInsight, error)
}
