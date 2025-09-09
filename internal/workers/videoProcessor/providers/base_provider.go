package providers

import (
	"clarity-ai/internal/domain/models"
	"context"
)

type AIProvider interface {
	GenerateSummary(ctx context.Context, transcript string) (string, error)
	GenerateTopics(ctx context.Context, transcript, startTime, endTime string) ([]*models.Topic, error)
	GenerateQuestions(ctx context.Context, transcript string, topics []*models.Topic, startTime, endTime string) ([]*models.Question, error)
	GenerateInsights(ctx context.Context, transcript, startTime, endTime string) ([]*models.KeyInsight, error)
	TranscribeAudio(ctx context.Context, audioFile string) (string, error)
}

type APIClient interface {
	SendRequest(ctx context.Context, prompt string, maxTokens int) (string, error)
	GetName() string
}

type BaseProvider struct {
	client  APIClient
	prompts *PromptTemplates
	parser  *ResponseParser
}

func NewBaseProvider(client APIClient) *BaseProvider {
	return &BaseProvider{
		client:  client,
		prompts: NewPromptTemplates(),
		parser:  NewResponseParser(),
	}
}

func (p *BaseProvider) GenerateSummary(ctx context.Context, transcript string) (string, error) {
	prompt := p.prompts.BuildSummaryPrompt(transcript)
	response, err := p.client.SendRequest(ctx, prompt, 150)
	if err != nil {
		return "", err
	}
	return p.parser.ParseSummary(response), nil
}

func (p *BaseProvider) GenerateTopics(ctx context.Context, transcript, startTime, endTime string) ([]*models.Topic, error) {
	prompt := p.prompts.BuildKeyTopicsPrompt(transcript, startTime, endTime)
	response, err := p.client.SendRequest(ctx, prompt, 300)
	if err != nil {
		return nil, err
	}
	return p.parser.ParseTopics(response, startTime, endTime)
}

func (p *BaseProvider) GenerateQuestions(ctx context.Context, transcript string, topics []*models.Topic, startTime, endTime string) ([]*models.Question, error) {
	prompt := p.prompts.BuildKeyQuestionsPrompt(transcript, topics, startTime, endTime)
	response, err := p.client.SendRequest(ctx, prompt, 400)
	if err != nil {
		return nil, err
	}
	return p.parser.ParseQuestions(response, topics, startTime)
}

func (p *BaseProvider) GenerateInsights(ctx context.Context, transcript, startTime, endTime string) ([]*models.KeyInsight, error) {
	prompt := p.prompts.BuildInsightsPrompt(transcript, startTime, endTime)
	response, err := p.client.SendRequest(ctx, prompt, 600)
	if err != nil {
		return nil, err
	}
	return p.parser.ParseInsights(response, startTime)
}
