// ai_service.go - NEW FILE
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AIService interface for different AI providers
type AIService interface {
	GenerateContentWithSystem(systemPrompt, userPrompt string) (string, error)
}

// AIProvider enum
type AIProvider string

const (
	ProviderGemini     AIProvider = "gemini"
	ProviderOpenRouter AIProvider = "openrouter"
)

// OpenRouterService handles OpenRouter API interactions
type OpenRouterService struct {
	apiKey string
	client *http.Client
	model  string
}

type OpenRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []OpenRouterMessage `json:"messages"`
	Stream      bool                `json:"stream"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
	TopP        float64             `json:"top_p,omitempty"`
}

type OpenRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterResponse struct {
	Choices []OpenRouterChoice `json:"choices"`
	Error   *OpenRouterError   `json:"error,omitempty"`
}

type OpenRouterChoice struct {
	Message OpenRouterMessage `json:"message"`
}

type OpenRouterError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// NewOpenRouterService creates a new OpenRouter service
func NewOpenRouterService(apiKey string) *OpenRouterService {
	return &OpenRouterService{
		apiKey: apiKey,
		client: &http.Client{Timeout: 300 * time.Second}, // 5 minutes for R1 models
		//model:  "meta-llama/llama-3.1-70b-instruct",
		model: "deepseek/deepseek-chat-v3-0324:free",
		//model: "deepseek/deepseek-r1-distill-llama-70b", // Free DeepSeek R1 model
		//model: "qwen/qwen3-30b-a3b-04-28:free",
	}
}

// GenerateContent implements AIService interface
func (o *OpenRouterService) GenerateContentWithSystem(systemPrompt, userPrompt string) (string, error) {
	return o.retryWithBackoffSystem(systemPrompt, userPrompt, 3)
}
func (o *OpenRouterService) callAPIWithSystem(systemPrompt, userPrompt string) (string, error) {

	if debugMode {
		fmt.Println(systemPrompt)
		fmt.Println(userPrompt)
	}
	requestBody := OpenRouterRequest{
		Model: o.model,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		Stream:      false,
		MaxTokens:   4000,
		Temperature: 0.7,
		TopP:        0.9,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshalling JSON: %w", err)
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("HTTP-Referer", "http://localhost:3000")
	req.Header.Set("X-Title", "YT Automation Service")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var openRouterResp OpenRouterResponse
	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	if openRouterResp.Error != nil {
		return "", fmt.Errorf("OpenRouter API error: %s", openRouterResp.Error.Message)
	}

	if len(openRouterResp.Choices) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return openRouterResp.Choices[0].Message.Content, nil
}
func (o *OpenRouterService) retryWithBackoffSystem(systemPrompt, userPrompt string, maxRetries int) (string, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := o.callAPIWithSystem(systemPrompt, userPrompt)
		if err == nil {
			return result, nil
		}

		lastErr = err

		backoffDuration := time.Duration(1<<attempt) * time.Second
		if isTimeoutError(err) {
			backoffDuration = time.Duration(10*(attempt+1)) * time.Second
			fmt.Printf("OpenRouter timeout (attempt %d/%d), retrying in %v: %v\n",
				attempt+1, maxRetries, backoffDuration, err)
		} else {
			fmt.Printf("OpenRouter API call failed (attempt %d/%d), retrying in %v: %v\n",
				attempt+1, maxRetries, backoffDuration, err)
		}

		if attempt < maxRetries-1 {
			time.Sleep(backoffDuration)
		}
	}

	return "", fmt.Errorf("API call failed after %d attempts, last error: %w", maxRetries, lastErr)
}

// Helper function to check if error is timeout-related
func isTimeoutError(err error) bool {
	return err != nil && (err.Error() == "context deadline exceeded" ||
		err.Error() == "Client.Timeout exceeded while awaiting headers" ||
		err.Error() == "context deadline exceeded (Client.Timeout or context cancellation while reading body)")
}

// AIServiceFactory creates the appropriate AI service based on configuration
func NewAIService(provider AIProvider, geminiKey, openRouterKey string) (AIService, error) {
	switch provider {
	case ProviderGemini:
		if geminiKey == "" {
			return nil, fmt.Errorf("Gemini API key is required")
		}
		return NewGeminiService(geminiKey), nil
	case ProviderOpenRouter:
		if openRouterKey == "" {
			return nil, fmt.Errorf("OpenRouter API key is required")
		}
		return NewOpenRouterService(openRouterKey), nil
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", provider)
	}
}
