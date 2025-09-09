package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

type OpenAIProvider struct {
	*BaseProvider
	client *OpenAIClient
}

type OpenAIClient struct {
	apiKey  string
	baseURL string
}

func NewOpenAIProvider() *OpenAIProvider {
	client := &OpenAIClient{
		apiKey:  os.Getenv("OPENAI_API_KEY"),
		baseURL: "https://api.openai.com/v1",
	}

	provider := &OpenAIProvider{
		BaseProvider: NewBaseProvider(client),
		client:       client,
	}

	return provider
}

func (c *OpenAIClient) SendRequest(ctx context.Context, prompt string, maxTokens int) (string, error) {
	requestBody := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  maxTokens,
		"temperature": 0.7,
	}

	return c.makeAPICall(ctx, "/chat/completions", requestBody)
}

func (c *OpenAIClient) GetName() string {
	return "OpenAI"
}

func (p *OpenAIProvider) TranscribeAudio(ctx context.Context, audioFile string) (string, error) {
	return p.client.transcribeWithWhisper(ctx, audioFile)
}

func (c *OpenAIClient) makeAPICall(ctx context.Context, endpoint string, requestBody map[string]interface{}) (string, error) {
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
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}

	if response.Error != nil {
		return "", fmt.Errorf("OpenAI API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return response.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) transcribeWithWhisper(ctx context.Context, audioFile string) (string, error) {
	file, err := os.Open(audioFile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", audioFile)
	if err != nil {
		return "", err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return "", err
	}

	writer.WriteField("model", "whisper-1")
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

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

	var transcription struct {
		Text string `json:"text"`
	}

	if err := json.Unmarshal(body, &transcription); err != nil {
		return "", err
	}

	return transcription.Text, nil
}
