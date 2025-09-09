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

type GeminiProvider struct {
	*BaseProvider
	client *GeminiClient
}

type GeminiClient struct {
	apiKey  string
	baseURL string
}

func NewGeminiProvider() *GeminiProvider {
	client := &GeminiClient{
		apiKey:  os.Getenv("GOOGLE_API_KEY"),
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
	}

	return &GeminiProvider{
		BaseProvider: NewBaseProvider(client),
		client:       client,
	}
}

func (c *GeminiClient) SendRequest(ctx context.Context, prompt string, maxTokens int) (string, error) {
	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": maxTokens,
			"temperature":     0.7,
		},
	}

	return c.makeAPICall(ctx, "/models/gemini-1.5-pro:generateContent", requestBody)
}

func (c *GeminiClient) GetName() string {
	return "Gemini"
}

func (p *GeminiProvider) TranscribeAudio(ctx context.Context, audioFile string) (string, error) {
	openaiProvider := NewOpenAIProvider()
	return openaiProvider.TranscribeAudio(ctx, audioFile)
}

func (c *GeminiClient) makeAPICall(ctx context.Context, endpoint string, requestBody map[string]interface{}) (string, error) {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s%s?key=%s", c.baseURL, endpoint, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

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
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("gemini API error %d: %s", response.Error.Code, response.Error.Message)
	}

	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from Gemini")
	}

	return response.Candidates[0].Content.Parts[0].Text, nil
}
