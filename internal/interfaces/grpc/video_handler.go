package handler

import (
	pb "clarity-ai/api/proto/video"
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/services"
	"context"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type VideoAnalysisHandler struct {
	pb.UnimplementedVideoAnalyzerServiceServer
	videoService services.VideoAnalysisService
}

func NewVideoAnalysisHandler(videoService services.VideoAnalysisService) *VideoAnalysisHandler {
	return &VideoAnalysisHandler{
		videoService: videoService,
	}
}

func (h *VideoAnalysisHandler) StartVideoAnalysis(ctx context.Context, req *pb.StartVideoAnalysisRequest) (*pb.StartVideoAnalysisResponse, error) {
	if req.UserId == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid userId")
	}
	if req.VideoUrl == "" {
		return nil, status.Error(codes.InvalidArgument, "video url cannot be empty")
	}
	if req.VideoTitle == "" {
		return nil, status.Error(codes.InvalidArgument, "video title cannot be empty")
	}

	serviceReq := &services.StartVideoAnalysisRequest{
		UserID:     req.UserId,
		VideoURL:   req.VideoUrl,
		VideoTitle: req.VideoTitle,
		AIProvider: convertProtoAIProvider(req.AiProvider),
		Options:    convertProtoProcessingOptions(req.Options),
	}

	response, err := h.videoService.StartVideoAnalysis(ctx, serviceReq)
	if err != nil {
		return nil, h.handleServiceError(err)
	}

	return &pb.StartVideoAnalysisResponse{
		AnalysisId:               response.AnalysisID,
		Status:                   convertAnalysisStatusToProto(response.Status),
		Message:                  response.Message,
		EstimatedChunks:          int32(response.EstimatedChunks),
		EstimatedDurationMinutes: int32(response.EstimatedDurationMins),
		StreamUrl:                response.StreamURL,
	}, nil
}

func (h *VideoAnalysisHandler) GetAnalysisStatus(ctx context.Context, req *pb.GetAnalysisStatusRequest) (*pb.GetAnalysisStatusResponse, error) {
	if req.AnalysisId == "" {
		return nil, status.Error(codes.InvalidArgument, "analysis id is required")
	}

	statusResp, err := h.videoService.GetAnalysisStatus(ctx, req.AnalysisId)
	if err != nil {
		return nil, h.handleServiceError(err)
	}

	return &pb.GetAnalysisStatusResponse{
		AnalysisId:          statusResp.AnalysisID,
		Status:              convertAnalysisStatusToProto(statusResp.Status),
		TotalChunks:         int32(statusResp.TotalChunks),
		CompletedChunks:     int32(statusResp.CompletedChunks),
		FailedChunks:        int32(statusResp.FailedChunks),
		ProgressPercentage:  int32(statusResp.ProgressPercent),
		CurrentStage:        statusResp.CurrentStage,
		EstimatedCompletion: statusResp.EstimatedCompletion,
		ChunkProgress:       convertChunkProgressToProto(statusResp.ChunkProgress),
	}, nil
}

// func (h *VideoAnalysisHandler) StreamAnalysisResults(req *pb.StreamAnalysisResultsRequest, stream pb.VideoAnalysisService_StreamAnalysisResultsServer) error {
// 	if req.AnalysisId == "" {
// 		return status.Error(codes.InvalidArgument, "analysis ID is required")
// 	}

// 	return status.Error(codes.Unimplemented, "real-time streaming not yet implemented - use GetAnalysisStatus for polling")
// }

func (h *VideoAnalysisHandler) GetCompleteAnalysis(ctx context.Context, req *pb.GetCompleteAnalysisRequest) (*pb.GetCompleteAnalysisResponse, error) {
	if req.AnalysisId == "" {
		return nil, status.Error(codes.InvalidArgument, "analysis ID is required")
	}

	userID := h.getUserIDFromContext(ctx)

	analysis, err := h.videoService.GetCompleteAnalysis(ctx, req.AnalysisId, userID)
	if err != nil {
		return nil, h.handleServiceError(err)
	}

	return &pb.GetCompleteAnalysisResponse{
		Analysis: convertVideoAnalysisToProto(analysis),
	}, nil
}

func (h *VideoAnalysisHandler) GetAnalysisChunk(ctx context.Context, req *pb.GetAnalysisChunkRequest) (*pb.GetAnalysisChunkResponse, error) {
	if req.AnalysisId == "" || req.ChunkId == "" {
		return nil, status.Error(codes.InvalidArgument, "analysis ID and chunk ID are required")
	}

	userID := h.getUserIDFromContext(ctx)

	chunk, err := h.videoService.GetAnalysisChunk(ctx, req.AnalysisId, req.ChunkId, userID)
	if err != nil {
		return nil, h.handleServiceError(err)
	}

	return &pb.GetAnalysisChunkResponse{
		Chunk: convertVideoChunkToProto(chunk),
	}, nil
}

// -
// helper functions
func (h *VideoAnalysisHandler) getUserIDFromContext(ctx context.Context) int64 {
	return 0
}

func (h *VideoAnalysisHandler) handleServiceError(err error) error {
	errMsg := err.Error()

	if strings.Contains(errMsg, "not found") {
		return status.Error(codes.NotFound, errMsg)
	}
	if strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "belongs to different user") {
		return status.Error(codes.PermissionDenied, errMsg)
	}
	if strings.Contains(errMsg, "not available") || strings.Contains(errMsg, "plan") {
		return status.Error(codes.PermissionDenied, errMsg)
	}
	if strings.Contains(errMsg, "limit exceeded") || strings.Contains(errMsg, "usage limit") {
		return status.Error(codes.ResourceExhausted, errMsg)
	}
	if strings.Contains(errMsg, "invalid") {
		return status.Error(codes.InvalidArgument, errMsg)
	}

	return status.Error(codes.Internal, errMsg)
}

// Proto conversion functions
func convertProtoAIProvider(provider pb.AIProvider) models.AIProvider {
	switch provider {
	case pb.AIProvider_AI_PROVIDER_FREE:
		return models.AIProviderFree
	case pb.AIProvider_AI_PROVIDER_OPENAI:
		return models.AIProviderOpenAI
	case pb.AIProvider_AI_PROVIDER_CLAUDE:
		return models.AIProviderClaude
	case pb.AIProvider_AI_PROVIDER_GEMINI:
		return models.AIProviderGemini
	default:
		return models.AIProviderFree
	}
}

func convertProtoProcessingOptions(options *pb.ProcessingOptions) *services.ProcessingOptions {
	if options == nil {
		return &services.ProcessingOptions{
			ChunkDurationMinutes: 15,
			EnableRealTimeStream: true,
			AnalysisTypes:        []string{"all"},
			Language:             "en",
		}
	}

	analysisTypes := make([]string, len(options.AnalysisTypes))
	for i, t := range options.AnalysisTypes {
		analysisTypes[i] = strings.ToLower(t.String())
	}

	chunkDuration := int(options.ChunkDurationMinutes)
	if chunkDuration <= 0 {
		chunkDuration = 15
	}

	return &services.ProcessingOptions{
		ChunkDurationMinutes: chunkDuration,
		EnableRealTimeStream: options.EnableRealTimeStreaming,
		AnalysisTypes:        analysisTypes,
		Language:             options.Language,
	}
}

func convertAnalysisStatusToProto(status models.AnalysisStatus) pb.AnalysisStatus {
	switch status {
	case models.AnalysisStatusPending:
		return pb.AnalysisStatus_ANALYSIS_STATUS_PENDING
	case models.AnalysisStatusProcessing:
		return pb.AnalysisStatus_ANALYSIS_STATUS_PROCESSING
	case models.AnalysisStatusStreaming:
		return pb.AnalysisStatus_ANALYSIS_STATUS_STREAMING
	case models.AnalysisStatusCompleted:
		return pb.AnalysisStatus_ANALYSIS_STATUS_COMPLETED
	case models.AnalysisStatusFailed:
		return pb.AnalysisStatus_ANALYSIS_STATUS_FAILED
	case models.AnalysisStatusCancelled:
		return pb.AnalysisStatus_ANALYSIS_STATUS_CANCELLED
	default:
		return pb.AnalysisStatus_ANALYSIS_STATUS_PENDING
	}
}

func convertChunkStatusToProto(status models.ChunkStatus) pb.ChunkStatus {
	switch status {
	case models.ChunkStatusPending:
		return pb.ChunkStatus_CHUNK_STATUS_PENDING
	case models.ChunkStatusExtractingAudio:
		return pb.ChunkStatus_CHUNK_STATUS_EXTRACTING_AUDIO
	case models.ChunkStatusTranscribing:
		return pb.ChunkStatus_CHUNK_STATUS_TRANSCRIBING
	case models.ChunkStatusAnalyzing:
		return pb.ChunkStatus_CHUNK_STATUS_ANALYZING
	case models.ChunkStatusCompleted:
		return pb.ChunkStatus_CHUNK_STATUS_COMPLETED
	case models.ChunkStatusFailed:
		return pb.ChunkStatus_CHUNK_STATUS_FAILED
	default:
		return pb.ChunkStatus_CHUNK_STATUS_PENDING
	}
}

func convertVideoAnalysisToProto(analysis *models.VideoAnalysis) *pb.VideoAnalysis {
	protoAnalysis := &pb.VideoAnalysis{
		Id:                   analysis.ID,
		UserId:               analysis.UserID,
		VideoUrl:             analysis.VideoURL,
		VideoTitle:           analysis.VideoTitle,
		Status:               convertAnalysisStatusToProto(analysis.Status),
		AiProvider:           convertAIProviderToProto(analysis.AIProvider),
		TotalChunks:          int32(analysis.TotalChunks),
		CompletedChunks:      int32(analysis.CompletedChunks),
		TotalDurationMinutes: int32(analysis.TotalDurationMins),
		CreatedAt:            timestamppb.New(analysis.CreatedAt),
		UpdatedAt:            timestamppb.New(analysis.UpdatedAt),
	}

	if analysis.CompletedAt != nil {
		protoAnalysis.CompletedAt = timestamppb.New(*analysis.CompletedAt)
	}

	for _, chunk := range analysis.Chunks {
		protoAnalysis.Chunks = append(protoAnalysis.Chunks, convertVideoChunkToProto(chunk))
	}

	if analysis.Metadata != nil {
		protoAnalysis.Metadata = convertAnalysisMetadata(analysis.Metadata)
	}

	return protoAnalysis
}

func convertVideoChunkToProto(chunk *models.VideoChunk) *pb.VideoChunk {
	if chunk == nil {
		return nil
	}

	protoChunk := &pb.VideoChunk{
		ChunkId:     chunk.ID,
		ChunkNumber: int32(chunk.ChunkNumber),
		StartTime:   chunk.StartTime,
		EndTime:     chunk.EndTime,
		Transcript:  safeStringDeref(chunk.Transcript),
		Summary:     safeStringDeref(chunk.Summary),
	}

	if chunk.Topics != nil {
		for _, topic := range chunk.Topics {
			if topic != nil {
				protoChunk.Topics = append(protoChunk.Topics, convertTopicToProto(topic))
			}
		}
	}

	if chunk.Questions != nil {
		for _, question := range chunk.Questions {
			if question != nil {
				protoChunk.Questions = append(protoChunk.Questions, convertQuestionToProto(question))
			}
		}
	}

	if chunk.KeyInsights != nil {
		for _, insight := range chunk.KeyInsights {
			if insight != nil {
				protoChunk.KeyInsights = append(protoChunk.KeyInsights, convertInsightToProto(insight))
			}
		}
	}

	return protoChunk
}

func safeStringDeref(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func convertTopicToProto(topic *models.Topic) *pb.Topic {
	return &pb.Topic{
		Id:              topic.ID,
		Title:           topic.Title,
		Description:     topic.Description,
		KeyPoints:       topic.KeyPoints,
		TimestampStart:  topic.TimestampStart,
		TimestampEnd:    topic.TimestampEnd,
		WhyImportant:    topic.WhyImportant,
		RelatedConcepts: topic.RelatedConcepts,
		Difficulty:      convertTopicDifficultyToProto(topic.Difficulty),
	}
}

func convertQuestionToProto(question *models.Question) *pb.Question {
	protoQuestion := &pb.Question{
		Id:                 question.ID,
		Question:           question.Question,
		Answer:             question.Answer,
		Difficulty:         convertQuestionDifficultyToProto(question.Difficulty),
		Type:               convertQuestionTypeToProto(question.Type),
		TimestampReference: question.TimestampReference,
	}

	if question.TopicID != nil {
		protoQuestion.TopicId = *question.TopicID
	}

	return protoQuestion
}

func convertInsightToProto(insight *models.KeyInsight) *pb.KeyInsight {
	return &pb.KeyInsight{
		Id:          insight.ID,
		Insight:     insight.Insight,
		Explanation: insight.Explanation,
		Timestamp:   insight.Timestamp,
		Type:        convertInsightTypeToProto(insight.Type),
	}
}

func convertChunkProgressToProto(chunkProgress []*services.ChunkProgressInfo) []*pb.ChunkProgress {
	protoProgress := make([]*pb.ChunkProgress, len(chunkProgress))

	for i, progress := range chunkProgress {
		protoProgress[i] = &pb.ChunkProgress{
			ChunkNumber:        int32(progress.ChunkNumber),
			Status:             convertChunkStatusToProto(progress.Status),
			Stage:              progress.Stage,
			ProgressPercentage: int32(progress.ProgressPercent),
		}
	}

	return protoProgress
}

func convertAIProviderToProto(provider models.AIProvider) pb.AIProvider {
	switch provider {
	case models.AIProviderFree:
		return pb.AIProvider_AI_PROVIDER_FREE
	case models.AIProviderOpenAI:
		return pb.AIProvider_AI_PROVIDER_OPENAI
	case models.AIProviderClaude:
		return pb.AIProvider_AI_PROVIDER_CLAUDE
	case models.AIProviderGemini:
		return pb.AIProvider_AI_PROVIDER_GEMINI
	default:
		return pb.AIProvider_AI_PROVIDER_FREE
	}
}

func convertTopicDifficultyToProto(difficulty string) pb.TopicDifficulty {
	switch strings.ToLower(difficulty) {
	case "beginner":
		return pb.TopicDifficulty_TOPIC_DIFFICULTY_BEGINNER
	case "intermediate":
		return pb.TopicDifficulty_TOPIC_DIFFICULTY_INTERMEDIATE
	case "advanced":
		return pb.TopicDifficulty_TOPIC_DIFFICULTY_ADVANCED
	case "expert":
		return pb.TopicDifficulty_TOPIC_DIFFICULTY_EXPERT
	default:
		return pb.TopicDifficulty_TOPIC_DIFFICULTY_BEGINNER
	}
}

func convertQuestionDifficultyToProto(difficulty string) pb.QuestionDifficulty {
	switch strings.ToLower(difficulty) {
	case "easy":
		return pb.QuestionDifficulty_QUESTION_DIFFICULTY_EASY
	case "medium":
		return pb.QuestionDifficulty_QUESTION_DIFFICULTY_MEDIUM
	case "hard":
		return pb.QuestionDifficulty_QUESTION_DIFFICULTY_HARD
	default:
		return pb.QuestionDifficulty_QUESTION_DIFFICULTY_EASY
	}
}

func convertQuestionTypeToProto(qType string) pb.QuestionType {
	switch strings.ToLower(qType) {
	case "multiple_choice":
		return pb.QuestionType_QUESTION_TYPE_MULTIPLE_CHOICE
	case "true_false":
		return pb.QuestionType_QUESTION_TYPE_TRUE_FALSE
	case "short_answer":
		return pb.QuestionType_QUESTION_TYPE_SHORT_ANSWER
	case "essay":
		return pb.QuestionType_QUESTION_TYPE_ESSAY
	case "coding":
		return pb.QuestionType_QUESTION_TYPE_CODING
	default:
		return pb.QuestionType_QUESTION_TYPE_MULTIPLE_CHOICE
	}
}

func convertInsightTypeToProto(iType string) pb.InsightType {
	switch strings.ToLower(iType) {
	case "key_concept":
		return pb.InsightType_INSIGHT_TYPE_KEY_CONCEPT
	case "important_formula":
		return pb.InsightType_INSIGHT_TYPE_IMPORTANT_FORMULA
	case "practical_tip":
		return pb.InsightType_INSIGHT_TYPE_PRACTICAL_TIP
	case "common_mistake":
		return pb.InsightType_INSIGHT_TYPE_COMMON_MISTAKE
	case "best_practice":
		return pb.InsightType_INSIGHT_TYPE_BEST_PRACTICE
	default:
		return pb.InsightType_INSIGHT_TYPE_KEY_CONCEPT
	}
}

func convertAnalysisMetadata(metadata map[string]interface{}) *pb.AnalysisMetadata {
	if metadata == nil {
		return &pb.AnalysisMetadata{
			TotalWords:       0,
			TotalTopics:      0,
			TotalQuestions:   0,
			TotalInsights:    0,
			DominantLanguage: "en",
			DetectedSubjects: []string{},
			DifficultyDistribution: &pb.DifficultyDistribution{
				EasyTopics:   0,
				MediumTopics: 0,
				HardTopics:   0,
			},
		}
	}

	return &pb.AnalysisMetadata{
		TotalWords:       getInt64FromMap(metadata, "totalWords"),
		TotalTopics:      getInt64FromMap(metadata, "totalTopics"),
		TotalQuestions:   getInt64FromMap(metadata, "totalQuestions"),
		TotalInsights:    getInt64FromMap(metadata, "totalInsights"),
		DominantLanguage: getStringFromMap(metadata, "dominantLanguage"),
		DetectedSubjects: getStringSliceFromMap(metadata, "detectedSubjects"),
		DifficultyDistribution: &pb.DifficultyDistribution{
			EasyTopics:   int32(getIntFromMap(metadata, "easyTopics")),
			MediumTopics: int32(getIntFromMap(metadata, "mediumTopics")),
			HardTopics:   int32(getIntFromMap(metadata, "hardTopics")),
		},
	}
}

// -
// Utility functions
func getInt64FromMap(m map[string]interface{}, key string) int64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func getIntFromMap(m map[string]interface{}, key string) int {
	return int(getInt64FromMap(m, key))
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return ""
}

func getStringSliceFromMap(m map[string]interface{}, key string) []string {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case []string:
			return v
		case []interface{}:
			result := make([]string, len(v))
			for i, item := range v {
				if str, ok := item.(string); ok {
					result[i] = str
				}
			}
			return result
		}
	}
	return []string{}
}
