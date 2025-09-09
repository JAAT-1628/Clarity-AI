package models

import (
	"time"
)

type VideoAnalysis struct {
	ID                string                 `json:"id" db:"id"`
	UserID            int64                  `json:"user_id" db:"user_id"`
	VideoURL          string                 `json:"video_url" db:"video_url"`
	VideoTitle        string                 `json:"video_title" db:"video_title"`
	Status            AnalysisStatus         `json:"status" db:"status"`
	AIProvider        AIProvider             `json:"ai_provider" db:"ai_provider"`
	TotalChunks       int                    `json:"total_chunks" db:"total_chunks"`
	CompletedChunks   int                    `json:"completed_chunks" db:"completed_chunks"`
	FailedChunks      int                    `json:"failed_chunks" db:"failed_chunks"`
	TotalDurationMins int                    `json:"total_duration_minutes" db:"total_duration_minutes"`
	ProcessingOptions map[string]interface{} `json:"processing_options" db:"processing_options"`
	Metadata          map[string]interface{} `json:"metadata" db:"metadata"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at" db:"updated_at"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty" db:"completed_at"`

	// Relationships (not in DB)
	Chunks []*VideoChunk `json:"chunks,omitempty"`
}

type VideoChunk struct {
	ID                 string      `json:"id" db:"id"`
	AnalysisID         string      `json:"analysis_id" db:"analysis_id"`
	ChunkNumber        int         `json:"chunk_number" db:"chunk_number"`
	StartTime          string      `json:"start_time" db:"start_time"`
	EndTime            string      `json:"end_time" db:"end_time"`
	Status             ChunkStatus `json:"status" db:"status"`
	Transcript         *string     `json:"transcript" db:"transcript"`
	Summary            *string     `json:"summary" db:"summary"`
	ProcessingStage    string      `json:"processing_stage" db:"processing_stage"`
	ProgressPercentage int         `json:"progress_percentage" db:"progress_percentage"`
	CreatedAt          time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at" db:"updated_at"`
	CompletedAt        *time.Time  `json:"completed_at,omitempty" db:"completed_at"`

	// Relationships (not in DB)
	Topics      []*Topic      `json:"topics,omitempty"`
	Questions   []*Question   `json:"questions,omitempty"`
	KeyInsights []*KeyInsight `json:"key_insights,omitempty"`
}

type Topic struct {
	ID              string    `json:"id" db:"id"`
	ChunkID         string    `json:"chunk_id" db:"chunk_id"`
	AnalysisID      string    `json:"analysis_id" db:"analysis_id"`
	Title           string    `json:"title" db:"title"`
	Description     string    `json:"description" db:"description"`
	KeyPoints       []string  `json:"key_points" db:"key_points"`
	TimestampStart  string    `json:"timestamp_start" db:"timestamp_start"`
	TimestampEnd    string    `json:"timestamp_end" db:"timestamp_end"`
	WhyImportant    string    `json:"why_important" db:"why_important"`
	RelatedConcepts []string  `json:"related_concepts" db:"related_concepts"`
	Difficulty      string    `json:"difficulty" db:"difficulty"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type Question struct {
	ID                 string    `json:"id" db:"id"`
	TopicID            *string   `json:"topic_id,omitempty" db:"topic_id"`
	ChunkID            string    `json:"chunk_id" db:"chunk_id"`
	AnalysisID         string    `json:"analysis_id" db:"analysis_id"`
	Question           string    `json:"question" db:"question"`
	Answer             string    `json:"answer" db:"answer"`
	Difficulty         string    `json:"difficulty" db:"difficulty"`
	Type               string    `json:"type" db:"type"`
	TimestampReference string    `json:"timestamp_reference" db:"timestamp_reference"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

type KeyInsight struct {
	ID          string    `json:"id" db:"id"`
	ChunkID     string    `json:"chunk_id" db:"chunk_id"`
	AnalysisID  string    `json:"analysis_id" db:"analysis_id"`
	Insight     string    `json:"insight" db:"insight"`
	Explanation string    `json:"explanation" db:"explanation"`
	Timestamp   string    `json:"timestamp" db:"timestamp"`
	Type        string    `json:"type" db:"type"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// Enums
type AnalysisStatus string

const (
	AnalysisStatusPending    AnalysisStatus = "pending"
	AnalysisStatusProcessing AnalysisStatus = "processing"
	AnalysisStatusStreaming  AnalysisStatus = "streaming"
	AnalysisStatusCompleted  AnalysisStatus = "completed"
	AnalysisStatusFailed     AnalysisStatus = "failed"
	AnalysisStatusCancelled  AnalysisStatus = "cancelled"
)

type ChunkStatus string

const (
	ChunkStatusPending         ChunkStatus = "pending"
	ChunkStatusExtractingAudio ChunkStatus = "extracting_audio"
	ChunkStatusTranscribing    ChunkStatus = "transcribing"
	ChunkStatusAnalyzing       ChunkStatus = "analyzing"
	ChunkStatusCompleted       ChunkStatus = "completed"
	ChunkStatusFailed          ChunkStatus = "failed"
)
