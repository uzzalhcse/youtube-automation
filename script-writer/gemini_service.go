package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent"
	timeout = 60 * time.Second
)

// GeminiService handles all Gemini API interactions
type GeminiService struct {
	apiKey string
	client *http.Client
}

// NewGeminiService creates a new Gemini service
func NewGeminiService(apiKey string) *GeminiService {
	return &GeminiService{
		apiKey: apiKey,
		client: &http.Client{Timeout: timeout},
	}
}
func (g *GeminiService) GenerateContentWithSystem(systemPrompt, userPrompt string) (string, error) {
	// For Gemini, we combine them as it doesn't have separate system/user roles like OpenAI
	combinedPrompt := systemPrompt + "\n\n" + userPrompt
	return g.GenerateContent(combinedPrompt)
}
func (g *GeminiService) GenerateContent(prompt string) (string, error) {
	return g.RetryWithExponentialBackoff(prompt, maxRetries)
}

func (g *GeminiService) callAPI(prompt string) (string, error) {
	requestBody := GeminiRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshalling JSON: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", baseURL, g.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WiserlyScriptGenerator/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}

// RetryWithExponentialBackoff implements retry logic for API calls
func (g *GeminiService) RetryWithExponentialBackoff(prompt string, maxRetries int) (string, error) {
	var lastErr error

	if debugMode {
		fmt.Println(prompt)
	}
	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := g.callAPI(prompt)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Exponential backoff: 1s, 2s, 4s, 8s...
		backoffDuration := time.Duration(1<<attempt) * time.Second
		fmt.Printf("API call failed (attempt %d/%d), retrying in %v: %v\n",
			attempt+1, maxRetries, backoffDuration, err)

		if attempt < maxRetries-1 {
			time.Sleep(backoffDuration)
		}
	}

	return "", fmt.Errorf("API call failed after %d attempts, last error: %w", maxRetries, lastErr)
}
