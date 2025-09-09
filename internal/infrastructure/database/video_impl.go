package database

import (
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type videoAnalysisRepository struct {
	db *PostgresDB
}

func NewVideoAnalysisRepository(db *PostgresDB) repositories.VideoAnalysisRepository {
	return &videoAnalysisRepository{db: db}
}

func (r *videoAnalysisRepository) CreateAnalysis(ctx context.Context, analysis *models.VideoAnalysis) error {
	if analysis.ID == "" {
		analysis.ID = uuid.New().String()
	}

	query := `INSERT INTO video_analyses (
		id, user_id, video_url, video_title, status, ai_provider,
		total_chunks, total_duration_minutes, processing_options, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`

	optionsJSON, _ := json.Marshal(analysis.ProcessingOptions)
	metadataJSON, _ := json.Marshal(analysis.Metadata)

	err := r.db.QueryRowContext(ctx, query,
		analysis.ID,
		analysis.UserID,
		analysis.VideoURL,
		analysis.VideoTitle,
		analysis.Status,
		analysis.AIProvider,
		analysis.TotalChunks,
		analysis.TotalDurationMins,
		optionsJSON,
		metadataJSON,
	).Scan(&analysis.CreatedAt, &analysis.UpdatedAt)

	return err
}

func (r *videoAnalysisRepository) GetAnalysisByID(ctx context.Context, id string) (*models.VideoAnalysis, error) {
	query := `SELECT * FROM video_analyses WHERE id = $1`

	analysis := &models.VideoAnalysis{}
	var optionsJSON, metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&analysis.ID,
		&analysis.UserID,
		&analysis.VideoURL,
		&analysis.VideoTitle,
		&analysis.Status,
		&analysis.AIProvider,
		&analysis.TotalChunks,
		&analysis.CompletedChunks,
		&analysis.FailedChunks,
		&analysis.TotalDurationMins,
		&optionsJSON,
		&metadataJSON,
		&analysis.CreatedAt,
		&analysis.UpdatedAt,
		&analysis.CompletedAt,
	)

	if err != nil {
		return nil, err
	}

	json.Unmarshal(optionsJSON, &analysis.ProcessingOptions)
	json.Unmarshal(metadataJSON, &analysis.Metadata)

	return analysis, nil
}

func (r *videoAnalysisRepository) GetAnalysisByUserID(ctx context.Context, userID int64, limit, offset int) ([]*models.VideoAnalysis, error) {
	query := `SELECT id, user_id, video_url, video_title, status, ai_provider,
               total_chunks, completed_chunks, failed_chunks, total_duration_minutes,
               processing_options, metadata, created_at, updated_at, completed_at
        FROM video_analyses 
        WHERE user_id = $1 
        ORDER BY created_at DESC 
        LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var analyses []*models.VideoAnalysis
	for rows.Next() {
		analysis := &models.VideoAnalysis{}
		var optionsJSON, metadataJSON []byte

		err := rows.Scan(
			&analysis.ID, &analysis.UserID, &analysis.VideoURL, &analysis.VideoTitle,
			&analysis.Status, &analysis.AIProvider, &analysis.TotalChunks,
			&analysis.CompletedChunks, &analysis.FailedChunks, &analysis.TotalDurationMins,
			&optionsJSON, &metadataJSON, &analysis.CreatedAt, &analysis.UpdatedAt,
			&analysis.CompletedAt,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(optionsJSON, &analysis.ProcessingOptions)
		json.Unmarshal(metadataJSON, &analysis.Metadata)
		analyses = append(analyses, analysis)
	}

	return analyses, nil
}

func (r *videoAnalysisRepository) UpdateAnalysisStatus(ctx context.Context, id string, status models.AnalysisStatus) error {
	query := `UPDATE video_analyses 
        SET status = $2, updated_at = CURRENT_TIMESTAMP
        WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, status)
	return err
}

func (r *videoAnalysisRepository) UpdateAnalysisMetadata(ctx context.Context, id string, metadata map[string]interface{}) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `UPDATE video_analyses 
        SET metadata = $2, updated_at = CURRENT_TIMESTAMP
        WHERE id = $1`

	_, err = r.db.ExecContext(ctx, query, id, metadataJSON)
	return err
}

func (r *videoAnalysisRepository) UpdateAnalysisCompletedAt(ctx context.Context, analysisID string, completedAt *time.Time) error {
	query := `UPDATE video_analyses SET completed_at = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, completedAt, analysisID)
	return err
}

func (r *videoAnalysisRepository) CreateChunk(ctx context.Context, chunk *models.VideoChunk) error {
	if chunk.ID == "" {
		chunk.ID = uuid.New().String()
	}

	query := `INSERT INTO video_chunks (
        id, analysis_id, chunk_number, start_time, end_time, status,
        processing_stage, progress_percentage
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query,
		chunk.ID,
		chunk.AnalysisID,
		chunk.ChunkNumber,
		chunk.StartTime,
		chunk.EndTime,
		chunk.Status,
		chunk.ProcessingStage,
		chunk.ProgressPercentage,
	).Scan(&chunk.CreatedAt, &chunk.UpdatedAt)

	return err
}

func (r *videoAnalysisRepository) GetChunk(ctx context.Context, analysisID, chunkID string) (*models.VideoChunk, error) {
	query := `SELECT id, analysis_id, chunk_number, start_time, end_time, status,
		transcript, summary, processing_stage, progress_percentage,
		created_at, updated_at, completed_at
		FROM video_chunks
		WHERE analysis_id = $1 AND id = $2`

	chunk := &models.VideoChunk{}
	var transcript, summary sql.NullString

	err := r.db.QueryRowContext(ctx, query, analysisID, chunkID).Scan(
		&chunk.ID, &chunk.AnalysisID, &chunk.ChunkNumber, &chunk.StartTime,
		&chunk.EndTime, &chunk.Status, &transcript, &summary,
		&chunk.ProcessingStage, &chunk.ProgressPercentage, &chunk.CreatedAt,
		&chunk.UpdatedAt, &chunk.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get video chunk: %w", err)
	}

	if transcript.Valid {
		chunk.Transcript = &transcript.String
	} else {
		chunk.Transcript = nil
	}

	if summary.Valid {
		chunk.Summary = &summary.String
	} else {
		chunk.Summary = nil
	}

	chunk.Topics, _ = r.GetTopicsByChunkID(ctx, chunkID)
	chunk.Questions, _ = r.GetQuestionsByChunkID(ctx, chunkID)
	chunk.KeyInsights, _ = r.GetInsightsByChunkID(ctx, chunkID)

	return chunk, nil
}

func (r *videoAnalysisRepository) GetChunksByAnalysisID(ctx context.Context, analysisID string) ([]*models.VideoChunk, error) {
	query := `SELECT id, analysis_id, chunk_number, start_time, end_time, status,
		transcript, summary, processing_stage, progress_percentage, 
		created_at, updated_at, completed_at 
		FROM video_chunks WHERE analysis_id = $1 ORDER BY chunk_number`

	rows, err := r.db.QueryContext(ctx, query, analysisID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []*models.VideoChunk
	for rows.Next() {
		chunk := &models.VideoChunk{}
		var transcript, summary sql.NullString

		err := rows.Scan(
			&chunk.ID, &chunk.AnalysisID, &chunk.ChunkNumber,
			&chunk.StartTime, &chunk.EndTime, &chunk.Status,
			&transcript, &summary, &chunk.ProcessingStage,
			&chunk.ProgressPercentage, &chunk.CreatedAt,
			&chunk.UpdatedAt, &chunk.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan video chunk: %w", err)
		}

		if transcript.Valid {
			chunk.Transcript = &transcript.String
		} else {
			chunk.Transcript = nil
		}

		if summary.Valid {
			chunk.Summary = &summary.String
		} else {
			chunk.Summary = nil
		}

		chunks = append(chunks, chunk)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over video chunks: %w", err)
	}

	return chunks, nil
}

func (r *videoAnalysisRepository) UpdateChunkStatus(ctx context.Context, chunkID string, status models.ChunkStatus, stage string, progress int) error {
	query := `UPDATE video_chunks 
	SET status = $2, processing_stage = $3, progress_percentage = $4, updated_at = CURRENT_TIMESTAMP
	WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, chunkID, status, stage, progress)
	return err
}

func (r *videoAnalysisRepository) UpdateChunkContent(ctx context.Context, chunkID, transcript, summary string) error {
	query := `UPDATE video_chunks 
	SET transcript = $2, summary = $3, updated_at = CURRENT_TIMESTAMP
	WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, chunkID, transcript, summary)
	return err
}

func (r *videoAnalysisRepository) UpdateAnalysisChunkCounts(ctx context.Context, analysisID string, completedChunks, failedChunks int) error {
	query := `UPDATE video_analyses 
              SET completed_chunks = $2, failed_chunks = $3, updated_at = CURRENT_TIMESTAMP 
              WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, analysisID, completedChunks, failedChunks)
	return err
}

func (r *videoAnalysisRepository) CreateTopic(ctx context.Context, topic *models.Topic) error {
	if topic.ID == "" {
		topic.ID = uuid.New().String()
	}

	query := `INSERT INTO topics (
		id, chunk_id, analysis_id, title, description, key_points,
		timestamp_start, timestamp_end, why_important, related_concepts, difficulty
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at`

	keyPointsJSON, _ := json.Marshal(topic.KeyPoints)
	relatedConceptsJSON, _ := json.Marshal(topic.RelatedConcepts)

	err := r.db.QueryRowContext(ctx, query,
		topic.ID,
		topic.ChunkID,
		topic.AnalysisID,
		topic.Title,
		topic.Description,
		keyPointsJSON,
		topic.TimestampStart,
		topic.TimestampEnd,
		topic.WhyImportant,
		relatedConceptsJSON,
		topic.Difficulty,
	).Scan(&topic.CreatedAt)

	return err
}

func (r *videoAnalysisRepository) CreateQuestion(ctx context.Context, question *models.Question) error {
	if question.ID == "" {
		question.ID = uuid.New().String()
	}

	query := `INSERT INTO questions (
		id, topic_id, chunk_id, analysis_id, question, answer,
		difficulty, type, timestamp_reference
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`

	err := r.db.QueryRowContext(ctx, query,
		question.ID,
		question.TopicID,
		question.ChunkID,
		question.AnalysisID,
		question.Question,
		question.Answer,
		question.Difficulty,
		question.Type,
		question.TimestampReference,
	).Scan(&question.CreatedAt)

	return err
}

func (r *videoAnalysisRepository) CreateKeyInsight(ctx context.Context, insight *models.KeyInsight) error {
	if insight.ID == "" {
		insight.ID = uuid.New().String()
	}

	query := `INSERT INTO key_insights (
            id, chunk_id, analysis_id, insight, explanation, timestamp, type
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING created_at`

	err := r.db.QueryRowContext(ctx, query,
		insight.ID,
		insight.ChunkID,
		insight.AnalysisID,
		insight.Insight,
		insight.Explanation,
		insight.Timestamp,
		insight.Type,
	).Scan(&insight.CreatedAt)

	return err
}

func (r *videoAnalysisRepository) GetTopicsByChunkID(ctx context.Context, chunkID string) ([]*models.Topic, error) {
	query := `SELECT * FROM topics WHERE chunk_id = $1`

	rows, err := r.db.QueryContext(ctx, query, chunkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []*models.Topic
	for rows.Next() {
		topic := &models.Topic{}
		var keyPointsJSON, relatedConceptsJSON []byte
		err := rows.Scan(
			&topic.ID,
			&topic.ChunkID,
			&topic.AnalysisID,
			&topic.Title,
			&topic.Description,
			&keyPointsJSON,
			&topic.TimestampStart,
			&topic.TimestampEnd,
			&topic.WhyImportant,
			&relatedConceptsJSON,
			&topic.Difficulty,
			&topic.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(keyPointsJSON, &topic.KeyPoints)
		json.Unmarshal(relatedConceptsJSON, &topic.RelatedConcepts)
		topics = append(topics, topic)
	}

	return topics, nil
}

func (r *videoAnalysisRepository) GetQuestionsByChunkID(ctx context.Context, chunkID string) ([]*models.Question, error) {
	query := `SELECT id, topic_id, chunk_id, analysis_id, question, answer,
               difficulty, type, timestamp_reference, created_at
        FROM questions 
        WHERE chunk_id = $1
        ORDER BY created_at`

	rows, err := r.db.QueryContext(ctx, query, chunkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var questions []*models.Question
	for rows.Next() {
		question := &models.Question{}

		err := rows.Scan(
			&question.ID,
			&question.TopicID,
			&question.ChunkID,
			&question.AnalysisID,
			&question.Question,
			&question.Answer,
			&question.Difficulty,
			&question.Type,
			&question.TimestampReference,
			&question.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		questions = append(questions, question)
	}

	return questions, nil
}

func (r *videoAnalysisRepository) GetInsightsByChunkID(ctx context.Context, chunkID string) ([]*models.KeyInsight, error) {
	query := `SELECT id, chunk_id, analysis_id, insight, explanation, timestamp, type, created_at
        FROM key_insights 
        WHERE chunk_id = $1
        ORDER BY created_at`

	rows, err := r.db.QueryContext(ctx, query, chunkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var insights []*models.KeyInsight
	for rows.Next() {
		insight := &models.KeyInsight{}

		err := rows.Scan(
			&insight.ID,
			&insight.ChunkID,
			&insight.AnalysisID,
			&insight.Insight,
			&insight.Explanation,
			&insight.Timestamp,
			&insight.Type,
			&insight.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		insights = append(insights, insight)
	}

	return insights, nil
}
