package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type GroqProvider struct {
	*BaseProvider
	client *GroqClient
}

type GroqClient struct {
	apiKey  string
	baseURL string
}

func NewGroqProvider() *GroqProvider {
	client := &GroqClient{
		apiKey:  os.Getenv("GROQ_API_KEY"),
		baseURL: "https://api.groq.com/openai/v1",
	}

	return &GroqProvider{
		BaseProvider: NewBaseProvider(client),
		client:       client,
	}
}

func (c *GroqClient) SendRequest(ctx context.Context, prompt string, maxTokens int) (string, error) {
	requestBody := map[string]interface{}{
		"model": "llama-3.3-70b-versatile",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  maxTokens,
		"temperature": 0.4,
		"top_p":       0.8,
	}

	return c.makeAPICall(ctx, "/chat/completions", requestBody)
}

func (c *GroqClient) GetName() string {
	return "Groq"
}

func (p *GroqProvider) TranscribeAudio(ctx context.Context, audioFile string) (string, error) {
	openaiProvider := NewOpenAIProvider()
	return openaiProvider.TranscribeAudio(ctx, audioFile)
}

func (c *GroqClient) makeAPICall(ctx context.Context, endpoint string, requestBody map[string]interface{}) (string, error) {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse Groq response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("groq API error: %s (%s)", response.Error.Message, response.Error.Type)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response from Groq")
	}

	return response.Choices[0].Message.Content, nil
}
