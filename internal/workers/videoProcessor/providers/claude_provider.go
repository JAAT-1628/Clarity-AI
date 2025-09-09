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

type ClaudeProvider struct {
	*BaseProvider
	client *ClaudeClient
}

type ClaudeClient struct {
	apiKey  string
	baseURL string
}

func NewClaudeProvider() *ClaudeProvider {
	client := &ClaudeClient{
		apiKey:  os.Getenv("ANTHROPIC_API_KEY"),
		baseURL: "https://api.anthropic.com/v1",
	}

	return &ClaudeProvider{
		BaseProvider: NewBaseProvider(client),
		client:       client,
	}
}

func (c *ClaudeClient) SendRequest(ctx context.Context, prompt string, maxToken int) (string, error) {
	requestBody := map[string]interface{}{
		"model":      "claude-3-sonnet-20240229",
		"max_tokens": maxToken,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	return c.makeAPICall(ctx, "/messages", requestBody)
}

func (c *ClaudeClient) GetName() string {
	return "Claude"
}

func (p *ClaudeProvider) TranscribeAudio(ctx context.Context, audioFile string) (string, error) {
	openaiProvider := NewOpenAIProvider()
	return openaiProvider.TranscribeAudio(ctx, audioFile)
}

func (c *ClaudeClient) makeAPICall(ctx context.Context, endpoint string, requestBody map[string]interface{}) (string, error) {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
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
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}

	if response.Error != nil {
		return "", fmt.Errorf("claude API error: %s", response.Error.Message)
	}

	if len(response.Content) == 0 {
		return "", fmt.Errorf("no response from Claude")
	}

	return response.Content[0].Text, nil
}
