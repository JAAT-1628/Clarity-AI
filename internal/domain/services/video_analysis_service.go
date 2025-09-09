package services

import (
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
	"clarity-ai/internal/infrastructure/queue"
	"context"
	"fmt"
	"log"
	"math"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type VideoAnalysisService interface {
	StartVideoAnalysis(ctx context.Context, req *StartVideoAnalysisRequest) (*StartVideoAnalysisResponse, error)
	GetAnalysisStatus(ctx context.Context, analysisID string) (*AnalysisStatusResponse, error)
	GetCompleteAnalysis(ctx context.Context, analysisID string, userID int64) (*models.VideoAnalysis, error)
	GetAnalysisChunk(ctx context.Context, analysisID, chunkID string, userID int64) (*models.VideoChunk, error)
	GetUserAnalyses(ctx context.Context, userID int64, limit, offset int) ([]*models.VideoAnalysis, error)
	CancelAnalysis(ctx context.Context, analysisID string, userID int64) error
}

type StartVideoAnalysisRequest struct {
	UserID     int64              `json:"user_id"`
	VideoURL   string             `json:"video_url"`
	VideoTitle string             `json:"video_title"`
	AIProvider models.AIProvider  `json:"ai_provider"`
	Options    *ProcessingOptions `json:"options"`
}

type ProcessingOptions struct {
	ChunkDurationMinutes int      `json:"chunk_duration_minutes"`
	EnableRealTimeStream bool     `json:"enable_real_time_stream"`
	AnalysisTypes        []string `json:"analysis_types"`
	Language             string   `json:"language"`
}

type StartVideoAnalysisResponse struct {
	AnalysisID            string                `json:"analysis_id"`
	Status                models.AnalysisStatus `json:"status"`
	Message               string                `json:"message"`
	EstimatedChunks       int                   `json:"estimated_chunks"`
	EstimatedDurationMins int                   `json:"estimated_duration_minutes"`
	StreamURL             string                `json:"stream_url"`
}

type AnalysisStatusResponse struct {
	AnalysisID          string                `json:"analysis_id"`
	Status              models.AnalysisStatus `json:"status"`
	TotalChunks         int                   `json:"total_chunks"`
	CompletedChunks     int                   `json:"completed_chunks"`
	FailedChunks        int                   `json:"failed_chunks"`
	ProgressPercent     int                   `json:"progress_percentage"`
	CurrentStage        string                `json:"current_stage"`
	EstimatedCompletion string                `json:"estimated_completion"`
	ChunkProgress       []*ChunkProgressInfo  `json:"chunk_progress"`
	StartedAt           time.Time             `json:"started_at"`
	LastUpdated         time.Time             `json:"last_updated"`
}

type ChunkProgressInfo struct {
	ChunkNumber       int                `json:"chunk_number"`
	Status            models.ChunkStatus `json:"status"`
	Stage             string             `json:"stage"`
	ProgressPercent   int                `json:"progress_percentage"`
	StartTime         string             `json:"start_time"`
	EndTime           string             `json:"end_time"`
	EstimatedTimeLeft string             `json:"estimated_time_left"`
}

type videoAnalysisService struct {
	videoRepo repositories.VideoAnalysisRepository
	userRepo  repositories.UserRepository
	queue     *queue.RedisQueue
	baseURL   string
}

func NewVideoAnalysisService(videoRepo repositories.VideoAnalysisRepository, userRepo repositories.UserRepository, queue *queue.RedisQueue, baseURL string) VideoAnalysisService {
	return &videoAnalysisService{
		videoRepo: videoRepo,
		userRepo:  userRepo,
		queue:     queue,
		baseURL:   baseURL,
	}
}

func (s *videoAnalysisService) StartVideoAnalysis(ctx context.Context, req *StartVideoAnalysisRequest) (*StartVideoAnalysisResponse, error) {
	user, err := s.userRepo.GetUserID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	if err := s.validateVideoURL(req.VideoURL); err != nil {
		return nil, fmt.Errorf("invalid video URL: %w", err)
	}

	if err := s.validateAIProvider(user.Plan, req.AIProvider); err != nil {
		return nil, fmt.Errorf("AI provider %s not available for %s plan. Upgrade your plan to access advanced AI features", req.AIProvider, user.Plan)
	}

	if err := s.checkUserLimits(ctx, user); err != nil {
		return nil, fmt.Errorf("usage limit exceeded: %w", err)
	}

	if req.Options == nil {
		req.Options = s.getDefaultProcessingOptions()
	}
	s.validateAndSetDefaults(req.Options)

	estimatedDuration, err := s.estimateVideoDuration(req.VideoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze video: %w", err)
	}

	chunkDuration := req.Options.ChunkDurationMinutes
	estimatedChunks := (estimatedDuration + chunkDuration - 1) / chunkDuration

	if err := s.validateVideoLengthForPlan(user.Plan, estimatedDuration); err != nil {
		return nil, err
	}

	analysis := &models.VideoAnalysis{
		UserID:            req.UserID,
		VideoURL:          req.VideoURL,
		VideoTitle:        req.VideoTitle,
		Status:            models.AnalysisStatusPending,
		AIProvider:        req.AIProvider,
		TotalChunks:       estimatedChunks,
		TotalDurationMins: estimatedDuration,
		ProcessingOptions: map[string]interface{}{
			"chunk_duration_minutes":  chunkDuration,
			"enable_real_time_stream": req.Options.EnableRealTimeStream,
			"analysis_types":          req.Options.AnalysisTypes,
			"language":                req.Options.Language,
		},
		Metadata: map[string]interface{}{
			"estimated_duration": estimatedDuration,
			"chunk_count":        estimatedChunks,
			"user_plan":          string(user.Plan),
			"ai_provider":        string(req.AIProvider),
		},
	}

	err = s.videoRepo.CreateAnalysis(ctx, analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to create analysis: %w", err)
	}

	err = s.createVideoChunks(ctx, analysis.ID, estimatedChunks, chunkDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunks: %w", err)
	}

	err = s.queueVideoProcessingJobs(ctx, analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to queue processing jobs: %w", err)
	}

	s.videoRepo.UpdateAnalysisStatus(ctx, analysis.ID, models.AnalysisStatusProcessing)

	s.updateUserAPIUsage(ctx, req.UserID)

	return &StartVideoAnalysisResponse{
		AnalysisID:            analysis.ID,
		Status:                models.AnalysisStatusProcessing,
		Message:               fmt.Sprintf("Video analysis started successfully! Processing %d chunks (~%d minutes each)", estimatedChunks, chunkDuration),
		EstimatedChunks:       estimatedChunks,
		EstimatedDurationMins: estimatedDuration,
		StreamURL:             fmt.Sprintf("ws://%s/api/v1/video/analysis/%s/stream", s.baseURL, analysis.ID),
	}, nil
}

func (s *videoAnalysisService) GetAnalysisStatus(ctx context.Context, analysisID string) (*AnalysisStatusResponse, error) {
	analysis, err := s.videoRepo.GetAnalysisByID(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("analysis not found: %w", err)
	}

	chunks, err := s.videoRepo.GetChunksByAnalysisID(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}

	progressPercent := 0
	if analysis.TotalChunks > 0 {
		progressPercent = (analysis.CompletedChunks * 100) / analysis.TotalChunks
	}

	estimatedCompletion := s.calculateEstimatedCompletion(chunks)
	chunkProgress := make([]*ChunkProgressInfo, len(chunks))
	for i, chunk := range chunks {
		chunkProgress[i] = &ChunkProgressInfo{
			ChunkNumber:       chunk.ChunkNumber,
			Status:            chunk.Status,
			Stage:             chunk.ProcessingStage,
			ProgressPercent:   chunk.ProgressPercentage,
			StartTime:         chunk.StartTime,
			EndTime:           chunk.EndTime,
			EstimatedTimeLeft: s.estimateChunkTimeLeft(chunk),
		}
	}

	return &AnalysisStatusResponse{
		AnalysisID:          analysis.ID,
		Status:              analysis.Status,
		TotalChunks:         analysis.TotalChunks,
		CompletedChunks:     analysis.CompletedChunks,
		FailedChunks:        analysis.FailedChunks,
		ProgressPercent:     progressPercent,
		CurrentStage:        s.getCurrentStage(analysis),
		EstimatedCompletion: estimatedCompletion,
		ChunkProgress:       chunkProgress,
		StartedAt:           analysis.CreatedAt,
		LastUpdated:         analysis.UpdatedAt,
	}, nil
}

func (s *videoAnalysisService) GetCompleteAnalysis(ctx context.Context, analysisID string, userID int64) (*models.VideoAnalysis, error) {
	analysis, err := s.videoRepo.GetAnalysisByID(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("analysis not found: %w", err)
	}

	if userID > 0 && analysis.UserID != userID {
		return nil, fmt.Errorf("unauthorized: analysis belongs to different user")
	}

	chunks, err := s.videoRepo.GetChunksByAnalysisID(ctx, analysisID)
	if err != nil {
		return nil, err
	}

	for _, chunk := range chunks {
		chunk.Topics, _ = s.videoRepo.GetTopicsByChunkID(ctx, chunk.ID)
		chunk.Questions, _ = s.videoRepo.GetQuestionsByChunkID(ctx, chunk.ID)
		chunk.KeyInsights, _ = s.videoRepo.GetInsightsByChunkID(ctx, chunk.ID)
	}

	analysis.Chunks = chunks
	return analysis, nil
}

func (s *videoAnalysisService) GetAnalysisChunk(ctx context.Context, analysisID, chunkID string, userID int64) (*models.VideoChunk, error) {
	analysis, err := s.videoRepo.GetAnalysisByID(ctx, analysisID)
	if err != nil {
		return nil, fmt.Errorf("analysis not found: %w", err)
	}

	if userID > 0 && analysis.UserID != userID {
		return nil, fmt.Errorf("unauthorized: analysis belongs to different user")
	}

	chunk, err := s.videoRepo.GetChunk(ctx, analysisID, chunkID)
	if err != nil {
		return nil, fmt.Errorf("chunk not found: %w", err)
	}

	return chunk, nil
}

func (s *videoAnalysisService) GetUserAnalyses(ctx context.Context, userID int64, limit, offset int) ([]*models.VideoAnalysis, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	return s.videoRepo.GetAnalysisByUserID(ctx, userID, limit, offset)
}

func (s *videoAnalysisService) CancelAnalysis(ctx context.Context, analysisID string, userID int64) error {
	analysis, err := s.videoRepo.GetAnalysisByID(ctx, analysisID)
	if err != nil {
		return fmt.Errorf("analysis not found: %w", err)
	}

	if analysis.UserID != userID {
		return fmt.Errorf("unauthorized: analysis belongs to different user")
	}

	if analysis.Status == models.AnalysisStatusCompleted {
		return fmt.Errorf("cannot cancel completed analysis")
	}

	return s.videoRepo.UpdateAnalysisStatus(ctx, analysisID, models.AnalysisStatusCancelled)
}

// - helper methods
func (s *videoAnalysisService) validateVideoURL(videoURL string) error {
	parsedURL, err := url.Parse(videoURL)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must use HTTP or HTTPS")
	}

	supportedDomains := []string{
		"youtube.com", "www.youtube.com", "youtu.be",
		"vimeo.com", "www.vimeo.com",
		"drive.google.com",
		"dropbox.com", "www.dropbox.com",
	}

	host := strings.ToLower(parsedURL.Host)
	for _, domain := range supportedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return nil
		}
	}

	return fmt.Errorf("unsupported video platform. Supported platforms: YouTube, Vimeo, Google Drive, Dropbox")
}

func (s *videoAnalysisService) validateAIProvider(userPlan models.UserPlan, aiProvider models.AIProvider) error {
	planStr := strings.TrimSpace(strings.ToLower(string(userPlan)))

	var realPlan string
	switch {
	case strings.Contains(planStr, "free"):
		realPlan = "free"
	case strings.Contains(planStr, "premium"):
		realPlan = "premium"
	case strings.Contains(planStr, "pro"):
		realPlan = "pro"
	default:
		return fmt.Errorf("invalid user plan: %s", userPlan)
	}

	var realProvider string
	switch aiProvider {
	case models.AIProviderFree:
		realProvider = "free"
	case models.AIProviderGemini:
		realProvider = "gemini"
	case models.AIProviderOpenAI:
		realProvider = "openai"
	case models.AIProviderClaude:
		realProvider = "claude"
	default:
		return fmt.Errorf("invalid AI provider: %s", aiProvider)
	}

	switch realPlan {
	case "free":
		if realProvider != "free" {
			return fmt.Errorf("AI provider %s not available for FREE plan. Upgrade your plan to access advanced AI features", realProvider)
		}
	case "premium":
		allowedProviders := []string{"free", "gemini", "openai", "claude"}
		if !containsString(allowedProviders, realProvider) {
			return fmt.Errorf("AI provider %s not available for PREMIUM plan", realProvider)
		}
	case "pro":
		allowedProviders := []string{"free", "gemini", "openai", "claude"}
		if !containsString(allowedProviders, realProvider) {
			return fmt.Errorf("AI provider %s is not supported", realProvider)
		}
	}

	return nil
}

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (s *videoAnalysisService) checkUserLimits(ctx context.Context, user *models.User) error {
	log.Printf("🔍 DEBUG: Checking limits for user %d, plan: %s", user.ID, user.Plan)

	planStr := strings.TrimSpace(strings.ToLower(string(user.Plan)))

	var dailyLimit int64
	switch planStr {
	case "free":
		dailyLimit = 2
	case "premium":
		dailyLimit = 10
	case "pro":
		dailyLimit = 50
	default:
		log.Printf("❌ Unknown user plan (normalized): '%s' raw:'%s'", planStr, user.Plan)
		return fmt.Errorf("invalid user plan: %s", user.Plan)
	}

	log.Printf("🔍 DEBUG: Daily limit for %s plan: %d", planStr, dailyLimit)

	now := time.Now()
	if user.APIUsageResetDate.IsZero() || user.APIUsageResetDate.Before(now) {
		log.Printf("🔄 DEBUG: Resetting usage count for user %d", user.ID)
		user.APIUsageCount = 0
		user.APIUsageResetDate = now.Add(24 * time.Hour)
		if err := s.userRepo.UpdateUserAPIUsage(ctx, user.ID, user.APIUsageCount, user.APIUsageResetDate); err != nil {
			log.Printf("❌ Failed to reset user API usage: %v", err)
		}
	}

	log.Printf("🔍 DEBUG: Current usage: %d/%d", user.APIUsageCount, dailyLimit)
	if user.APIUsageCount >= dailyLimit {
		return fmt.Errorf("daily video analysis limit reached (%d/%d). Upgrade your plan or try again tomorrow", user.APIUsageCount, dailyLimit)
	}

	return nil

}

func (s *videoAnalysisService) updateUserAPIUsage(ctx context.Context, userID int64) {
	err := s.userRepo.IncrementAPIUsage(ctx, userID)
	if err != nil {
		log.Printf("❌ Failed to update user API usage: %v", err)
	}
}

func (s *videoAnalysisService) validateVideoLengthForPlan(plan models.UserPlan, durationMinutes int) error {
	var maxDuration int
	switch plan {
	case models.PlanFree:
		maxDuration = 60
	case models.PlanPremium:
		maxDuration = 240
	case models.PlanPro:
		maxDuration = 720 // 12 hours max
	}

	if durationMinutes > maxDuration {
		return fmt.Errorf("video duration (%d minutes) exceeds plan limit (%d minutes). Upgrade your plan for longer videos", durationMinutes, maxDuration)
	}

	return nil
}

func (s *videoAnalysisService) getDefaultProcessingOptions() *ProcessingOptions {
	return &ProcessingOptions{
		ChunkDurationMinutes: 15,
		EnableRealTimeStream: true,
		AnalysisTypes:        []string{"transcript", "summary", "topics", "questions", "insights"},
		Language:             "en",
	}
}

func (s *videoAnalysisService) validateAndSetDefaults(options *ProcessingOptions) {
	if options.ChunkDurationMinutes <= 0 || options.ChunkDurationMinutes > 30 {
		options.ChunkDurationMinutes = 15
	}

	if len(options.AnalysisTypes) == 0 {
		options.AnalysisTypes = []string{"all"}
	}

	if options.Language == "" {
		options.Language = "en"
	}
}

func (s *videoAnalysisService) estimateVideoDuration(videoURL string) (int, error) {
	cmd := exec.Command("yt-dlp", "--print", "duration", "--no-playlist", videoURL)
	output, err := cmd.Output()
	if err != nil {
		return 100, nil
	}

	durationStr := strings.TrimSpace(string(output))
	durationSeconds, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 100, nil
	}

	return int(durationSeconds/60) + 1, nil
}

func (s *videoAnalysisService) createVideoChunks(ctx context.Context, analysisID string, estimatedDuration, requestedChunkDuration int) error {
	var chunkDuration int
	if estimatedDuration <= 8 {
		chunkDuration = 1
	} else if estimatedDuration <= 30 {
		chunkDuration = 2
	} else {
		chunkDuration = 3
	}

	if requestedChunkDuration > 0 && requestedChunkDuration <= chunkDuration {
		chunkDuration = requestedChunkDuration
	}

	numChunks := int(math.Ceil(float64(estimatedDuration) / float64(chunkDuration)))

	for i := 1; i <= numChunks; i++ {
		startMinutes := (i - 1) * chunkDuration
		endMinutes := i * chunkDuration
		if endMinutes > estimatedDuration {
			endMinutes = estimatedDuration
		}
		chunk := &models.VideoChunk{
			AnalysisID:         analysisID,
			ChunkNumber:        i,
			StartTime:          s.minutesToTimestamp(startMinutes),
			EndTime:            s.minutesToTimestamp(endMinutes),
			Status:             models.ChunkStatusPending,
			ProcessingStage:    "queued",
			ProgressPercentage: 0,
		}

		err := s.videoRepo.CreateChunk(ctx, chunk)
		if err != nil {
			return fmt.Errorf("failed to create chunk %d: %w", i, err)
		}
	}
	return nil
}

func (s *videoAnalysisService) queueVideoProcessingJobs(ctx context.Context, analysis *models.VideoAnalysis) error {
	chunks, err := s.videoRepo.GetChunksByAnalysisID(ctx, analysis.ID)
	if err != nil {
		return err
	}

	for _, chunk := range chunks {
		job := queue.VideoProcessingJob{
			AnalysisID:  analysis.ID,
			ChunkID:     chunk.ID,
			ChunkNumber: chunk.ChunkNumber,
			VideoURL:    analysis.VideoURL,
			StartTime:   chunk.StartTime,
			EndTime:     chunk.EndTime,
			UserID:      analysis.UserID,
			AIProvider:  string(analysis.AIProvider),
			Options:     analysis.ProcessingOptions,
		}

		err = s.queue.EnqueueVideoChunk(ctx, job)
		if err != nil {
			return fmt.Errorf("failed to queue chunk %d: %w", chunk.ChunkNumber, err)
		}
	}

	return nil
}

func (s *videoAnalysisService) getCurrentStage(analysis *models.VideoAnalysis) string {
	switch analysis.Status {
	case models.AnalysisStatusPending:
		return "Preparing video for processing..."
	case models.AnalysisStatusProcessing:
		return fmt.Sprintf("Processing chunks (%d/%d completed)", analysis.CompletedChunks, analysis.TotalChunks)
	case models.AnalysisStatusStreaming:
		return fmt.Sprintf("Results available! (%d/%d chunks ready)", analysis.CompletedChunks, analysis.TotalChunks)
	case models.AnalysisStatusCompleted:
		return "Analysis complete - all results ready!"
	case models.AnalysisStatusFailed:
		return "Analysis failed - please try again"
	case models.AnalysisStatusCancelled:
		return "Analysis cancelled by user"
	default:
		return "Processing..."
	}
}

func (s *videoAnalysisService) estimateChunkTimeLeft(chunk *models.VideoChunk) string {
	if chunk.Status == models.ChunkStatusCompleted || chunk.Status == models.ChunkStatusFailed {
		return "Complete"
	}

	remainingProgress := 100 - chunk.ProgressPercentage
	if remainingProgress <= 0 {
		return "Almost done..."
	}

	var estimatedMinutes int
	switch chunk.Status {
	case models.ChunkStatusExtractingAudio:
		estimatedMinutes = 2
	case models.ChunkStatusTranscribing:
		estimatedMinutes = 5
	case models.ChunkStatusAnalyzing:
		estimatedMinutes = 8
	default:
		estimatedMinutes = 10
	}

	adjustedMinutes := (estimatedMinutes * remainingProgress) / 100
	if adjustedMinutes <= 0 {
		adjustedMinutes = 1
	}

	return fmt.Sprintf("~%d min", adjustedMinutes)
}

func (s *videoAnalysisService) minutesToTimestamp(minutes int) string {
	hours := minutes / 60
	mins := minutes % 60
	return fmt.Sprintf("%02d:%02d:00", hours, mins)
}

func (s *videoAnalysisService) calculateEstimatedCompletion(chunks []*models.VideoChunk) string {
	if len(chunks) == 0 {
		return "Unknown"
	}

	completedCount := 0
	pendingCount := 0
	totalProcessingTime := time.Duration(0)

	for _, chunk := range chunks {
		switch chunk.Status {
		case models.ChunkStatusCompleted:
			completedCount++
			if !chunk.UpdatedAt.IsZero() && !chunk.CreatedAt.IsZero() {
				processingTime := chunk.UpdatedAt.Sub(chunk.CreatedAt)
				totalProcessingTime += processingTime
			}
		case models.ChunkStatusPending, models.ChunkStatusExtractingAudio, models.ChunkStatusTranscribing, models.ChunkStatusAnalyzing:
			pendingCount++
		}
	}

	if completedCount == len(chunks) && completedCount > 0 {
		return "Analysis completed"
	}

	if completedCount == 0 && pendingCount == 0 {
		return "Almost done!"
	}

	if completedCount > 0 && pendingCount > 0 {
		avgTimePerChunk := totalProcessingTime / time.Duration(completedCount)
		estimatedRemaining := avgTimePerChunk * time.Duration(pendingCount)
		if estimatedRemaining < time.Minute {
			return fmt.Sprintf("~%d seconds", int(estimatedRemaining.Seconds()))
		} else if estimatedRemaining < time.Hour {
			return fmt.Sprintf("~%d minutes", int(estimatedRemaining.Minutes()))
		} else {
			return fmt.Sprintf("~%d hours", int(estimatedRemaining.Hours()))
		}
	}

	return "Almost done!"
}
