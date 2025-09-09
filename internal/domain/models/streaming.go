package models

import "time"

type StreamEventType string

const (
	EventChunkStarted       StreamEventType = "chunk_started"
	EventAudioExtracted     StreamEventType = "audio_extracted"
	EventTranscriptReady    StreamEventType = "transcript_ready"
	EventSummaryGenerated   StreamEventType = "summary_generated"
	EventTopicsGenerated    StreamEventType = "topics_generated"
	EventQuestionsGenerated StreamEventType = "questions_generated"
	EventInsightsGenerated  StreamEventType = "insights_generated"
	EventChunkCompleted     StreamEventType = "chunk_completed"
	EventChunkFailed        StreamEventType = "chunk_failed"

	EventAnalysisStarted   StreamEventType = "analysis_started"
	EventAnalysisCompleted StreamEventType = "analysis_completed"
	EventAnalysisFailed    StreamEventType = "analysis_failed"
	EventAnalysisProgress  StreamEventType = "analysis_progress"
)

type StreamMessage struct {
	ID          string          `json:"id"`
	AnalysisID  string          `json:"analysis_id"`
	ChunkID     string          `json:"chunk_id,omitempty"`
	ChunkNumber int             `json:"chunk_number,omitempty"`
	EventType   StreamEventType `json:"event_type"`
	Timestamp   time.Time       `json:"timestamp"`
	Sequence    int64           `json:"sequence"`
	Data        interface{}     `json:"data,omitempty"`
	Progress    *ProgressData   `json:"progress,omitempty"`
	Error       *ErrorData      `json:"error,omitempty"`
}

type ProgressData struct {
	ChunkProgress     int    `json:"chunk_progress"`
	OverallProgress   int    `json:"overall_progress"`
	CurrentStage      string `json:"current_stage"`
	CompletedChunks   int    `json:"completed_chunks"`
	TotalChunks       int    `json:"total_chunks"`
	EstimatedTimeLeft string `json:"estimated_time_left,omitempty"`
}

type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Stage   string `json:"stage"`
}

type TranscriptData struct {
	ChunkID     string `json:"chunk_id"`
	ChunkNumber int    `json:"chunk_number"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Text        string `json:"text"`
	WordCount   int    `json:"word_count"`
}

type SummaryData struct {
	ChunkID     string `json:"chunk_id"`
	ChunkNumber int    `json:"chunk_number"`
	Summary     string `json:"summary"`
}

type TopicsData struct {
	ChunkID     string   `json:"chunk_id"`
	ChunkNumber int      `json:"chunk_number"`
	Topics      []*Topic `json:"topics"`
}

type QuestionsData struct {
	ChunkID     string      `json:"chunk_id"`
	ChunkNumber int         `json:"chunk_number"`
	Questions   []*Question `json:"questions"`
}

type InsightsData struct {
	ChunkID     string        `json:"chunk_id"`
	ChunkNumber int           `json:"chunk_number"`
	Insights    []*KeyInsight `json:"insights"`
}

type AnalysisMetadata struct {
	TotalWords             int            `json:"total_words"`
	TotalTopics            int            `json:"total_topics"`
	TotalQuestions         int            `json:"total_questions"`
	TotalInsights          int            `json:"total_insights"`
	DetectedSubjects       []string       `json:"detected_subjects"`
	DifficultyDistribution map[string]int `json:"difficulty_distribution"`
	ProcessingDuration     time.Duration  `json:"processing_duration"`
}

type StreamSubscription struct {
	ID         string
	AnalysisID string
	UserID     int64
	Channel    chan *StreamMessage
	Connected  time.Time
	LastSeen   time.Time
}

type StreamingService interface {
	Subscribe(analysisID string, userID int64) (*StreamSubscription, error)
	Unsubscribe(subscriptionID string) error
	PublishMessage(msg *StreamMessage) error
	GetActiveSubscriptions(analysisID string) []*StreamSubscription
}
