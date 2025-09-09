package providers

import (
	"clarity-ai/internal/domain/models"
	"context"
	"fmt"
	"log"
)

type AIProviderManager struct {
	openai AIProvider
	claude AIProvider
	gemini AIProvider
	groq   AIProvider
}

func NewAIProviderManager() *AIProviderManager {
	return &AIProviderManager{
		openai: NewOpenAIProvider(),
		claude: NewClaudeProvider(),
		gemini: NewGeminiProvider(),
		groq:   NewGroqProvider(),
	}
}

func (m *AIProviderManager) TranscribeAudio(ctx context.Context, audioFile string) (string, error) {
	// if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" && strings.TrimSpace(openaiKey) != "" {
	// 	log.Printf("🔊 Attempting transcription with OpenAI Whisper API")
	// 	transcript, err := m.openai.TranscribeAudio(ctx, audioFile)
	// 	if err == nil && len(strings.TrimSpace(transcript)) > 0 {
	// 		log.Printf("✅ OpenAI Whisper API transcription successful")
	// 		return transcript, nil
	// 	}
	// 	log.Printf("⚠️ OpenAI Whisper API failed: %v", err)
	// }

	// Fallback to local Whisper
	log.Printf("🔄 Falling back to local Whisper CLI")
	transcript, err := TranscribeWithLocalWhisper(ctx, audioFile)
	if err != nil {
		return "", fmt.Errorf("both OpenAI API and local Whisper failed. API error: OpenAI unavailable, Local error: %w", err)
	}

	return transcript, nil
}

func (m *AIProviderManager) GenerateSummary(ctx context.Context, provider models.AIProvider, transcript string) (string, error) {
	aiProvider, err := m.getProvider(provider)
	if err != nil {
		return "", err
	}
	return aiProvider.GenerateSummary(ctx, transcript)
}

func (m *AIProviderManager) GenerateTopics(ctx context.Context, provider models.AIProvider, transcript, startTime, endTime string) ([]*models.Topic, error) {
	aiProvider, err := m.getProvider(provider)
	if err != nil {
		return nil, err
	}
	return aiProvider.GenerateTopics(ctx, transcript, startTime, endTime)
}

func (m *AIProviderManager) GenerateQuestions(ctx context.Context, provider models.AIProvider, transcript string, topics []*models.Topic, startTime, endTime string) ([]*models.Question, error) {
	aiProvider, err := m.getProvider(provider)
	if err != nil {
		return nil, err
	}
	return aiProvider.GenerateQuestions(ctx, transcript, topics, startTime, endTime)
}

func (m *AIProviderManager) GenerateInsights(ctx context.Context, provider models.AIProvider, transcript, startTime, endTime string) ([]*models.KeyInsight, error) {
	aiProvider, err := m.getProvider(provider)
	if err != nil {
		return nil, err
	}
	return aiProvider.GenerateInsights(ctx, transcript, startTime, endTime)
}

func (m *AIProviderManager) getProvider(provider models.AIProvider) (AIProvider, error) {
	switch provider {
	case models.AIProviderOpenAI:
		return m.openai, nil
	case models.AIProviderClaude:
		return m.claude, nil
	case models.AIProviderGemini:
		return m.gemini, nil
	case models.AIProviderFree:
		return m.groq, nil
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", provider)
	}
}
