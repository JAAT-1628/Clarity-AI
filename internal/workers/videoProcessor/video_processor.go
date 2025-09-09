package videoprocessor

import (
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
	"clarity-ai/internal/infrastructure/queue"
	"clarity-ai/internal/workers/videoProcessor/providers"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Processor struct {
	videoRepo       repositories.VideoAnalysisRepository
	queue           *queue.RedisQueue
	streamingQueue  *queue.StreamingRedisQueue
	aiProvider      *providers.AIProviderManager
	tempDir         string
	downloadMutex   sync.Map
	enableStreaming bool
}

func NewProcessor(vRepo repositories.VideoAnalysisRepository, q *queue.RedisQueue) *Processor {
	tempDir := "/tmp/video-processing"
	os.MkdirAll(tempDir, 0755)
	return &Processor{
		videoRepo:       vRepo,
		queue:           q,
		streamingQueue:  nil,
		aiProvider:      providers.NewAIProviderManager(),
		tempDir:         tempDir,
		enableStreaming: false,
	}
}

func NewStreamingProcessor(vRepo repositories.VideoAnalysisRepository, q *queue.RedisQueue, sq *queue.StreamingRedisQueue) *Processor {
	tempDir := "/tmp/video-processing"
	os.MkdirAll(tempDir, 0755)
	return &Processor{
		videoRepo:       vRepo,
		queue:           q,
		streamingQueue:  sq,
		aiProvider:      providers.NewAIProviderManager(),
		tempDir:         tempDir,
		enableStreaming: true,
	}
}

func (p *Processor) Run(ctx context.Context) {
	if err := p.validateSystemRequirements(); err != nil {
		log.Fatalf("System requirements not met: %v", err)
		return
	}

	if p.enableStreaming {
		go p.cleanupRoutine(ctx)
	}

	workerCount := 6
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			p.workerLoop(ctx, workerID)
		}(i + 1)
	}

	<-ctx.Done()
	log.Println("🛑 Shutting down workers...")
	wg.Wait()
	log.Println("✅ All workers stopped")
}

func (p *Processor) workerLoop(ctx context.Context, workerID int) {
	log.Printf("🔥 Worker %d ready for processing", workerID)
	for {
		select {
		case <-ctx.Done():
			log.Printf("🔴 Worker %d shutting down", workerID)
			return
		default:
			job, err := p.queue.DequeueVideoChunck(ctx)
			if err != nil {
				if !strings.Contains(err.Error(), "redis: nil") {
					log.Printf("❌ Worker %d queue error: %v", workerID, err)
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}

			if job != nil {
				start := time.Now()
				log.Printf("⚡ Worker %d: Processing chunk %d of analysis %s",
					workerID, job.ChunkNumber, job.AnalysisID)

				p.processVideoChunk(ctx, job)

				log.Printf("✅ Worker %d: Completed chunk %d in %v",
					workerID, job.ChunkNumber, time.Since(start))
			}
		}
	}
}

func (p *Processor) processVideoChunk(ctx context.Context, job *queue.VideoProcessingJob) {
	p.publishStreamingEvent(ctx, job, models.EventChunkStarted, 0, "Starting chunk processing", nil)

	if err := p.extractAudio(ctx, job); err != nil {
		p.handleError(ctx, job, "audio_extraction", err)
		return
	}

	transcript, err := p.speechToText(ctx, job)
	if err != nil {
		p.handleError(ctx, job, "speech_to_text", err)
		return
	}

	if err := p.generateAIAnalysis(ctx, job, transcript); err != nil {
		p.handleError(ctx, job, "ai_analysis", err)
		return
	}

	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusCompleted, "completed", 100)
	p.publishStreamingEvent(ctx, job, models.EventChunkCompleted, 100, "Chunk completed successfully", nil)

	p.updateAnalysisProgress(ctx, job.AnalysisID)
	p.publishProgress(ctx, job, "completed", 100)
	p.cleanupTempFile(job)
}

func (p *Processor) extractAudio(ctx context.Context, job *queue.VideoProcessingJob) error {
	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusExtractingAudio, "downloading_video", 5)
	p.publishProgress(ctx, job, "downloading_video", 5)

	videoFile, err := p.downloadVideo(ctx, job.VideoURL, job.AnalysisID)
	if err != nil {
		return fmt.Errorf("failed to download video: %w", err)
	}

	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusExtractingAudio, "extracting_audio", 20)
	p.publishProgress(ctx, job, "extracting_audio", 20)

	audioFile := filepath.Join(p.tempDir, fmt.Sprintf("chunk_%s_%d.wav", job.AnalysisID, job.ChunkNumber))
	startSeconds, err := p.timeToSeconds(job.StartTime)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}

	endSeconds, err := p.timeToSeconds(job.EndTime)
	if err != nil {
		return fmt.Errorf("invalid end time: %w", err)
	}

	duration := endSeconds - startSeconds
	log.Printf("🎵 Worker extracting audio: Start=%ds, Duration=%ds", startSeconds, duration)

	cmdFfmpeg := exec.CommandContext(ctx,
		"ffmpeg",
		"-i", videoFile,
		"-ss", fmt.Sprintf("%d", startSeconds),
		"-t", fmt.Sprintf("%d", duration),
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"-y",
		"-v", "quiet",
		"-threads", "2",
		audioFile,
	)

	output, err := cmdFfmpeg.CombinedOutput()
	if err != nil {
		log.Printf("❌ FFmpeg error: %v, output: %s", err, string(output))
		return fmt.Errorf("audio extraction failed: %w", err)
	}

	if _, err := os.Stat(audioFile); os.IsNotExist(err) {
		return fmt.Errorf("audio file was not created: %s", audioFile)
	}

	job.Options["audio_file"] = audioFile

	p.publishStreamingEvent(ctx, job, models.EventAudioExtracted, 25, "Audio extracted successfully", nil)

	log.Printf("✅ Audio extraction successful: %s", audioFile)
	return nil
}

func (p *Processor) speechToText(ctx context.Context, job *queue.VideoProcessingJob) (string, error) {
	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusTranscribing, "transcribing", 50)
	p.publishProgress(ctx, job, "transcribing", 50)

	audioFile := job.Options["audio_file"].(string)
	transcript, err := p.aiProvider.TranscribeAudio(ctx, audioFile)
	if err != nil {
		return "", fmt.Errorf("speech-to-text failed: %w", err)
	}

	err = p.videoRepo.UpdateChunkContent(ctx, job.ChunkID, transcript, "")
	if err != nil {
		log.Printf("❌ Failed to save transcript: %v", err)
	}

	p.publishContentUpdate(ctx, job, models.EventTranscriptReady, transcript)

	log.Printf("✅ Transcript ready for chunk %d (%d words)", job.ChunkNumber, len(strings.Fields(transcript)))
	return transcript, nil
}

func (p *Processor) generateAIAnalysis(ctx context.Context, job *queue.VideoProcessingJob, transcript string) error {
	provider := models.AIProvider(job.AIProvider)

	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusAnalyzing, "generating_summary", 60)
	p.publishStreamingEvent(ctx, job, models.EventAnalysisProgress, 60, "generating_summary", nil)

	summary, err := p.aiProvider.GenerateSummary(ctx, provider, transcript)
	if err != nil {
		return fmt.Errorf("failed to generate summary: %w", err)
	}

	err = p.videoRepo.UpdateChunkContent(ctx, job.ChunkID, transcript, summary)
	if err != nil {
		log.Printf("❌ Failed to save summary: %v", err)
	}
	p.publishContentUpdate(ctx, job, models.EventSummaryGenerated, summary)
	log.Printf("📝 Summary generated for chunk %d", job.ChunkNumber)

	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusAnalyzing, "generating_topics", 70)
	p.publishStreamingEvent(ctx, job, models.EventAnalysisProgress, 70, "generating_topics", nil)

	topics, err := p.aiProvider.GenerateTopics(ctx, provider, transcript, job.StartTime, job.EndTime)
	if err != nil {
		return fmt.Errorf("failed to generate topics: %w", err)
	}

	for _, topic := range topics {
		topic.ChunkID = job.ChunkID
		topic.AnalysisID = job.AnalysisID
		if err := p.videoRepo.CreateTopic(ctx, topic); err != nil {
			log.Printf("❌ Failed to save topic: %v", err)
		}
	}
	p.publishContentUpdate(ctx, job, models.EventTopicsGenerated, topics)
	log.Printf("🎯 Topics generated for chunk %d (%d topics)", job.ChunkNumber, len(topics))

	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusAnalyzing, "generating_questions", 80)
	p.publishStreamingEvent(ctx, job, models.EventAnalysisProgress, 80, "generating_questions", nil)

	questions, err := p.aiProvider.GenerateQuestions(ctx, provider, transcript, topics, job.StartTime, job.EndTime)
	if err != nil {
		return fmt.Errorf("failed to generate questions: %w", err)
	}

	for _, question := range questions {
		question.ChunkID = job.ChunkID
		question.AnalysisID = job.AnalysisID
		if err := p.videoRepo.CreateQuestion(ctx, question); err != nil {
			log.Printf("❌ Failed to save question: %v", err)
		}
	}
	p.publishContentUpdate(ctx, job, models.EventQuestionsGenerated, questions)
	log.Printf("❓ Questions generated for chunk %d (%d questions)", job.ChunkNumber, len(questions))

	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusAnalyzing, "generating_insights", 90)
	p.publishStreamingEvent(ctx, job, models.EventAnalysisProgress, 90, "generating_insights", nil)

	insights, err := p.aiProvider.GenerateInsights(ctx, provider, transcript, job.StartTime, job.EndTime)
	if err != nil {
		return fmt.Errorf("failed to generate insights: %w", err)
	}

	for _, insight := range insights {
		insight.ChunkID = job.ChunkID
		insight.AnalysisID = job.AnalysisID
		if err := p.videoRepo.CreateKeyInsight(ctx, insight); err != nil {
			log.Printf("❌ Failed to save insight: %v", err)
		}
	}
	p.publishContentUpdate(ctx, job, models.EventInsightsGenerated, insights)
	log.Printf("💡 Insights generated for chunk %d (%d insights)", job.ChunkNumber, len(insights))

	return nil
}

func (p *Processor) publishStreamingEvent(ctx context.Context, job *queue.VideoProcessingJob, eventType models.StreamEventType, progress int, stage string, data interface{}) {
	if !p.enableStreaming || p.streamingQueue == nil {
		return
	}

	p.streamingQueue.PublishStreamingUpdate(ctx, &models.StreamMessage{
		AnalysisID:  job.AnalysisID,
		ChunkID:     job.ChunkID,
		ChunkNumber: job.ChunkNumber,
		EventType:   eventType,
		Progress: &models.ProgressData{
			ChunkProgress: progress,
			CurrentStage:  stage,
		},
		Data: data,
	})
}

func (p *Processor) publishContentUpdate(ctx context.Context, job *queue.VideoProcessingJob, eventType models.StreamEventType, content interface{}) {
	if !p.enableStreaming || p.streamingQueue == nil {
		return
	}

	switch eventType {
	case models.EventTranscriptReady:
		if transcript, ok := content.(string); ok {
			p.streamingQueue.PublishTranscriptUpdate(ctx, job.AnalysisID, job.ChunkID, job.ChunkNumber, job.StartTime, job.EndTime, transcript)
		}
	case models.EventSummaryGenerated:
		if summary, ok := content.(string); ok {
			p.streamingQueue.PublishSummaryUpdate(ctx, job.AnalysisID, job.ChunkID, job.ChunkNumber, summary)
		}
	case models.EventTopicsGenerated:
		if topics, ok := content.([]*models.Topic); ok {
			p.streamingQueue.PublishTopicsUpdate(ctx, job.AnalysisID, job.ChunkID, job.ChunkNumber, topics)
		}
	case models.EventQuestionsGenerated:
		if questions, ok := content.([]*models.Question); ok {
			p.streamingQueue.PublishQuestionsUpdate(ctx, job.AnalysisID, job.ChunkID, job.ChunkNumber, questions)
		}
	case models.EventInsightsGenerated:
		if insights, ok := content.([]*models.KeyInsight); ok {
			p.streamingQueue.PublishInsightsUpdate(ctx, job.AnalysisID, job.ChunkID, job.ChunkNumber, insights)
		}
	}
}

func (p *Processor) updateAnalysisProgress(ctx context.Context, analysisID string) {
	chunks, err := p.videoRepo.GetChunksByAnalysisID(ctx, analysisID)
	if err != nil {
		log.Printf("failed to get chunks for progress update: %v", err)
		return
	}

	completedCount := 0
	failedCount := 0
	totalCount := len(chunks)
	for _, chunk := range chunks {
		switch chunk.Status {
		case models.ChunkStatusCompleted:
			completedCount++
		case models.ChunkStatusFailed:
			failedCount++
		}
	}

	overallProgress := 0
	if totalCount > 0 {
		overallProgress = (completedCount * 100) / totalCount
	}

	var newStatus models.AnalysisStatus
	var eventType models.StreamEventType

	if completedCount == totalCount && totalCount > 0 {
		newStatus = models.AnalysisStatusCompleted
		eventType = models.EventAnalysisCompleted
		now := time.Now()
		p.videoRepo.UpdateAnalysisCompletedAt(ctx, analysisID, &now)
		p.calculateAndUpdateMetadata(ctx, analysisID, chunks)
		p.cleanupVideoFile(analysisID)
	} else if failedCount == totalCount && totalCount > 0 {
		newStatus = models.AnalysisStatusFailed
		eventType = models.EventAnalysisFailed
		p.cleanupVideoFile(analysisID)
	} else if completedCount > 0 {
		newStatus = models.AnalysisStatusStreaming
		eventType = models.EventAnalysisProgress
	} else {
		newStatus = models.AnalysisStatusProcessing
		eventType = models.EventAnalysisProgress
	}

	err = p.videoRepo.UpdateAnalysisStatus(ctx, analysisID, newStatus)
	if err != nil {
		log.Printf("failed to update analysis status: %v", err)
		return
	}

	err = p.videoRepo.UpdateAnalysisChunkCounts(ctx, analysisID, completedCount, failedCount)
	if err != nil {
		log.Printf("failed to update chunk counts: %v", err)
	}

	if p.enableStreaming && p.streamingQueue != nil {
		p.streamingQueue.PublishStreamingUpdate(ctx, &models.StreamMessage{
			AnalysisID: analysisID,
			EventType:  eventType,
			Progress: &models.ProgressData{
				OverallProgress: overallProgress,
				CompletedChunks: completedCount,
				TotalChunks:     totalCount,
				CurrentStage:    p.getCurrentStage(newStatus, completedCount, totalCount),
			},
		})
	}

	log.Printf("Analysis progress: %d/%d chunks completed, %d failed", completedCount, totalCount, failedCount)
}

func (p *Processor) getCurrentStage(status models.AnalysisStatus, completed, total int) string {
	switch status {
	case models.AnalysisStatusCompleted:
		return "Analysis complete - all results ready!"
	case models.AnalysisStatusFailed:
		return "Analysis failed - please try again"
	case models.AnalysisStatusStreaming:
		return fmt.Sprintf("Results streaming live! (%d/%d chunks ready)", completed, total)
	default:
		return fmt.Sprintf("Processing chunks... (%d/%d completed)", completed, total)
	}
}

func (p *Processor) handleError(ctx context.Context, job *queue.VideoProcessingJob, stage string, err error) {
	log.Printf("❌ Error in %s for chunk %d: %v", stage, job.ChunkNumber, err)
	p.videoRepo.UpdateChunkStatus(ctx, job.ChunkID, models.ChunkStatusFailed, stage+"_failed", 0)

	if p.enableStreaming && p.streamingQueue != nil {
		p.streamingQueue.PublishErrorUpdate(ctx, job.AnalysisID, job.ChunkID, job.ChunkNumber, &models.ErrorData{
			Code:    "processing_failed",
			Message: err.Error(),
			Stage:   stage,
		})
	}

	p.publishProgress(ctx, job, stage+"_failed", 0)
	p.updateAnalysisProgress(ctx, job.AnalysisID)
	p.cleanupTempFile(job)
}

func (p *Processor) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if p.streamingQueue != nil {
				p.streamingQueue.CleanupSubscriptions()
			}
		}
	}
}

func (p *Processor) downloadVideo(ctx context.Context, videoURL, analysisID string) (string, error) {
	videoFile := filepath.Join(p.tempDir, fmt.Sprintf("video_%s.mp4", analysisID))
	if _, err := os.Stat(videoFile); err == nil {
		log.Printf("♻️ Using cached video file: %s", videoFile)
		return videoFile, nil
	}

	mutexInterface, _ := p.downloadMutex.LoadOrStore(videoURL, &sync.Mutex{})
	mutex := mutexInterface.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	if _, err := os.Stat(videoFile); err == nil {
		log.Printf("♻️ Using cached video file (after lock): %s", videoFile)
		return videoFile, nil
	}

	log.Printf("📥 Downloading video: %s", videoURL)
	tempVideoFile := videoFile + ".downloading"
	cmdDownload := exec.CommandContext(ctx,
		"yt-dlp",
		"-f", "best[ext=mp4]/best",
		"-o", tempVideoFile,
		"--no-playlist",
		"--no-warnings",
		"--quiet",
		videoURL,
	)

	output, err := cmdDownload.CombinedOutput()
	if err != nil {
		log.Printf("❌ yt-dlp error: %v", err)
		log.Printf("❌ yt-dlp output: %s", string(output))
		return "", fmt.Errorf("failed to download video: %w", err)
	}

	if err := os.Rename(tempVideoFile, videoFile); err != nil {
		log.Printf("❌ Failed to rename downloaded file: %v", err)
		return "", fmt.Errorf("failed to finalize video download: %w", err)
	}

	if _, err := os.Stat(videoFile); os.IsNotExist(err) {
		return "", fmt.Errorf("video download failed - file not created: %s", videoFile)
	}

	log.Printf("✅ Video downloaded successfully: %s", videoFile)
	return videoFile, nil
}

func (p *Processor) calculateAndUpdateMetadata(ctx context.Context, analysisID string, chunks []*models.VideoChunk) {
	totalWords := 0
	totalTopics := 0
	totalQuestions := 0
	totalInsights := 0
	allText := ""
	easyTopics := 0
	mediumTopics := 0
	hardTopics := 0

	for _, chunk := range chunks {
		if chunk.Transcript != nil && *chunk.Transcript != "" {
			words := len(strings.Fields(*chunk.Transcript))
			totalWords += words
			allText += " " + *chunk.Transcript
		}

		topics, _ := p.videoRepo.GetTopicsByChunkID(ctx, chunk.ID)
		questions, _ := p.videoRepo.GetQuestionsByChunkID(ctx, chunk.ID)
		insights, _ := p.videoRepo.GetInsightsByChunkID(ctx, chunk.ID)

		totalTopics += len(topics)
		totalQuestions += len(questions)
		totalInsights += len(insights)

		for _, topic := range topics {
			difficulty := strings.ToLower(topic.Difficulty)
			switch {
			case strings.Contains(difficulty, "beginner") || strings.Contains(difficulty, "easy") || difficulty == "easy":
				easyTopics++
			case strings.Contains(difficulty, "intermediate") || strings.Contains(difficulty, "medium") || difficulty == "medium":
				mediumTopics++
			case strings.Contains(difficulty, "advanced") || strings.Contains(difficulty, "expert") || strings.Contains(difficulty, "hard") || difficulty == "hard":
				hardTopics++
			default:
				mediumTopics++
			}
		}
	}

	detectedSubjects := p.detectSubjectsFromContent(allText)

	metadata := map[string]interface{}{
		"totalWords":       totalWords,
		"totalTopics":      totalTopics,
		"totalQuestions":   totalQuestions,
		"totalInsights":    totalInsights,
		"dominantLanguage": "en",
		"detectedSubjects": detectedSubjects,
		"difficultyDistribution": map[string]int{
			"easyTopics":   easyTopics,
			"mediumTopics": mediumTopics,
			"hardTopics":   hardTopics,
		},
	}

	err := p.videoRepo.UpdateAnalysisMetadata(ctx, analysisID, metadata)
	if err != nil {
		log.Printf("Failed to update metadata: %v", err)
	} else {
		log.Printf("✅ Metadata updated: %d words, %d topics (%d easy, %d medium, %d hard), %d questions, %d insights, subjects: %v",
			totalWords, totalTopics, easyTopics, mediumTopics, hardTopics, totalQuestions, totalInsights, detectedSubjects)
	}

	if p.enableStreaming && p.streamingQueue != nil {
		p.streamingQueue.PublishStreamingUpdate(ctx, &models.StreamMessage{
			AnalysisID: analysisID,
			EventType:  models.EventAnalysisCompleted,
			Data: &models.AnalysisMetadata{
				TotalWords:       totalWords,
				TotalTopics:      totalTopics,
				TotalQuestions:   totalQuestions,
				TotalInsights:    totalInsights,
				DetectedSubjects: detectedSubjects,
				DifficultyDistribution: map[string]int{
					"easy":   easyTopics,
					"medium": mediumTopics,
					"hard":   hardTopics,
				},
			},
		})
	}
}

func (p *Processor) detectSubjectsFromContent(text string) []string {
	if text == "" {
		return []string{"General Education"}
	}

	text = strings.ToLower(text)
	subjects := []string{}
	subjectKeywords := map[string][]string{
		"Programming": {"code", "algorithm", "function"},
		"Mathematics": {"math", "formula", "calculate"},
		"Science":     {"experiment", "biology", "chemistry"},
		"Business":    {"business", "marketing", "finance"},
		"History":     {"history", "war", "civilization"},
		"Language":    {"language", "grammar", "vocabulary"},
		"Art":         {"art", "design", "creative"},
		"Health":      {"health", "medical", "fitness"},
		"Education":   {"learning", "study", "student"},
	}

	for subject, keywords := range subjectKeywords {
		for _, keyword := range keywords {
			if strings.Contains(text, keyword) {
				subjects = append(subjects, subject)
				break
			}
		}
	}

	if len(subjects) == 0 {
		return []string{"General Education"}
	}

	return subjects
}

func (p *Processor) publishProgress(ctx context.Context, job *queue.VideoProcessingJob, stage string, progress int) {
	update := map[string]interface{}{
		"analysis_id":  job.AnalysisID,
		"chunk_id":     job.ChunkID,
		"chunk_number": job.ChunkNumber,
		"status":       stage,
		"progress":     progress,
		"timestamp":    time.Now().Unix(),
	}

	err := p.queue.PublishChunkUpdate(ctx, job.AnalysisID, update)
	if err != nil {
		log.Printf("Failed to publish progress update: %v", err)
	}
}

func (p *Processor) cleanupTempFile(job *queue.VideoProcessingJob) {
	if audioFile, ok := job.Options["audio_file"].(string); ok {
		os.Remove(audioFile)
		log.Printf("🗑️ Cleaned up audio file: %s", audioFile)
	}
}

func (p *Processor) cleanupVideoFile(analysisID string) {
	videoFile := filepath.Join(p.tempDir, fmt.Sprintf("video_%s.mp4", analysisID))
	if err := os.Remove(videoFile); err == nil {
		log.Printf("🗑️ Cleaned up video file: %s", videoFile)
	}
}

func (p *Processor) timeToSeconds(timeStr string) (int, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format: %s", timeStr)
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	seconds, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, err
	}

	return hours*3600 + minutes*60 + seconds, nil
}

func (p *Processor) validateSystemRequirements() error {
	cmd := exec.Command("yt-dlp", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp is not installed or not in PATH. Please install yt-dlp")
	}

	cmd = exec.Command("ffmpeg", "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg is not installed or not in PATH. Please install ffmpeg")
	}

	cmd = exec.Command("whisper", "--help")
	if err := cmd.Run(); err != nil {
		log.Printf("⚠️ Local Whisper CLI not found - will use OpenAI API for transcription")
	} else {
		log.Printf("✅ Local Whisper CLI found")
	}

	testFile := filepath.Join(p.tempDir, "test.tmp")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("temp directory is not writable: %s", p.tempDir)
	}

	os.Remove(testFile)
	log.Println("✅ System requirements validated")
	return nil
}
