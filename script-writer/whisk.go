package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ASPECT_RATIO = "IMAGE_ASPECT_RATIO_LANDSCAPE" // Options: IMAGE_ASPECT_RATIO_LANDSCAPE, IMAGE_ASPECT_RATIO_PORTRAIT, IMAGE_ASPECT_RATIO_SQUARE
)

// HttpConfig holds all configuration for the HTTP client
type HttpConfig struct {
	URL               string            `json:"url"`
	AuthToken         string            `json:"auth_token"`
	Headers           map[string]string `json:"headers"`
	OutputDirectory   string            `json:"output_directory"`
	MaxConcurrency    int               `json:"max_concurrency"`
	Timeout           time.Duration     `json:"timeout"`
	SeedMode          string            `json:"seed_mode"` // "random" or "static"
	StaticSeed        int               `json:"static_seed"`
	RequestsPerMinute int               `json:"requests_per_minute"` // Rate limit: requests per minute
	RetryAttempts     int               `json:"retry_attempts"`      // Number of retry attempts for failed requests
	InitialRetryDelay time.Duration     `json:"initial_retry_delay"` // Initial delay for exponential backoff
	BackoffMultiplier float64           `json:"backoff_multiplier"`  // Multiplier for exponential backoff
	MaxRetryDelay     time.Duration     `json:"max_retry_delay"`     // Maximum delay between retries
	Tool              string            `json:"tool"`                // "whisk" or "imagefx"
}

// ClientContext represents the client context in the payload
type ClientContext struct {
	WorkflowID string `json:"workflowId"`
	Tool       string `json:"tool"`
	SessionID  string `json:"sessionId"`
}

// ImageModelSettings represents image model configuration
type ImageModelSettings struct {
	ImageModel  string `json:"imageModel"`
	AspectRatio string `json:"aspectRatio"`
}

// RequestPayload represents the complete request payload
type RequestPayload struct {
	ClientContext      ClientContext      `json:"clientContext"`
	ImageModelSettings ImageModelSettings `json:"imageModelSettings"`
	Seed               int                `json:"seed"`
	Prompt             string             `json:"prompt"`
	MediaCategory      string             `json:"mediaCategory"`
}

// GeneratedImage represents a single generated image in the response
type GeneratedImage struct {
	EncodedImage           string `json:"encodedImage"`
	Seed                   int    `json:"seed"`
	MediaGenerationID      string `json:"mediaGenerationId"`
	Prompt                 string `json:"prompt"`
	ImageModel             string `json:"imageModel"`
	IsMaskEditedImage      bool   `json:"isMaskEditedImage,omitempty"`
	ModelNameType          string `json:"modelNameType,omitempty"`
	WorkflowID             string `json:"workflowId,omitempty"`
	FingerprintLogRecordID string `json:"fingerprintLogRecordId,omitempty"`
}

// ImagePanel represents an image panel in the response
type ImagePanel struct {
	Prompt          string           `json:"prompt"`
	GeneratedImages []GeneratedImage `json:"generatedImages"`
}

// APIResponse represents the complete API response
type APIResponse struct {
	ImagePanels []ImagePanel `json:"imagePanels"`
	WorkflowID  string       `json:"workflowId"`
}

// ErrorInfo represents the error info in API error responses
type ErrorInfo struct {
	Type   string `json:"@type"`
	Reason string `json:"reason"`
}

// APIError represents the complete API error response
type APIError struct {
	Error struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Status  string      `json:"status"`
		Details []ErrorInfo `json:"details"`
	} `json:"error"`
}
type ImageFXUserInput struct {
	CandidatesCount int      `json:"candidatesCount"`
	Prompts         []string `json:"prompts"`
	Seed            int      `json:"seed"`
}

type ImageFXModelInput struct {
	ModelNameType string `json:"modelNameType"`
}

type ImageFXPayload struct {
	UserInput     ImageFXUserInput  `json:"userInput"`
	ClientContext ClientContext     `json:"clientContext"`
	ModelInput    ImageFXModelInput `json:"modelInput"`
	AspectRatio   string            `json:"aspectRatio"`
}

func isUnsafeGenerationError(body []byte) bool {
	var apiError APIError
	if err := json.Unmarshal(body, &apiError); err != nil {
		return false
	}

	for _, detail := range apiError.Error.Details {
		if detail.Reason == "PUBLIC_ERROR_UNSAFE_GENERATION" {
			return true
		}
	}
	return false
}

// RateLimiter manages request rate limiting with proper time-based control
type RateLimiter struct {
	requestsPerMinute int
	requests          []time.Time
	mu                sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	return &RateLimiter{
		requestsPerMinute: requestsPerMinute,
		requests:          make([]time.Time, 0, requestsPerMinute),
	}
}

// Wait blocks until a request can be made within the rate limit
func (rl *RateLimiter) Wait() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Remove requests older than 1 minute
	cutoff := now.Add(-time.Minute)
	validRequests := make([]time.Time, 0, len(rl.requests))
	for _, reqTime := range rl.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	rl.requests = validRequests

	// If we've hit the rate limit, wait until the oldest request expires
	if len(rl.requests) >= rl.requestsPerMinute {
		oldestRequest := rl.requests[0]
		waitUntil := oldestRequest.Add(time.Minute)
		waitDuration := waitUntil.Sub(now)

		if waitDuration > 0 {
			fmt.Printf("Rate limit reached (%d/%d requests in last minute), waiting %v...\n",
				len(rl.requests), rl.requestsPerMinute, waitDuration.Round(time.Second))
			rl.mu.Unlock()
			time.Sleep(waitDuration)
			rl.mu.Lock()

			// Clean up expired requests after waiting
			now = time.Now()
			cutoff = now.Add(-time.Minute)
			validRequests = make([]time.Time, 0, len(rl.requests))
			for _, reqTime := range rl.requests {
				if reqTime.After(cutoff) {
					validRequests = append(validRequests, reqTime)
				}
			}
			rl.requests = validRequests
		}
	}

	// Record this request
	rl.requests = append(rl.requests, now)
}

// GetCurrentUsage returns current rate limit usage
func (rl *RateLimiter) GetCurrentUsage() (int, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)
	count := 0
	for _, reqTime := range rl.requests {
		if reqTime.After(cutoff) {
			count++
		}
	}
	return count, rl.requestsPerMinute
}

// HTTPClient wraps the configuration and provides methods for making requests
type HTTPClient struct {
	config      HttpConfig
	httpClient  *http.Client
	rng         *rand.Rand
	rateLimiter *RateLimiter
}

func NewHTTPClient(config HttpConfig) *HTTPClient {
	var rateLimiter *RateLimiter
	if config.RequestsPerMinute > 0 {
		rateLimiter = NewRateLimiter(config.RequestsPerMinute)
	}

	return &HTTPClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
		rateLimiter: rateLimiter,
	}
}

func (yt *YtAutomation) MakeRequest(payload interface{}) (*APIResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= yt.googleHttpClient.config.RetryAttempts; attempt++ {
		// Get API key from database based on tool
		provider := "whisk"
		if yt.googleHttpClient.config.Tool == "imagefx" {
			provider = "imagefx" // or whatever provider name you use for imagefx
		}

		apiKey, err := yt.apiKeyManager.GetActiveKey(provider)
		if err != nil {
			return nil, fmt.Errorf("failed to get API key: %w", err)
		}

		// Apply rate limiting if configured
		if yt.googleHttpClient.rateLimiter != nil {
			yt.googleHttpClient.rateLimiter.Wait()
			current, max := yt.googleHttpClient.rateLimiter.GetCurrentUsage()
			fmt.Printf("Rate limit usage: %d/%d requests in last minute\n", current, max)
		}

		// Determine URL based on tool configuration
		url := yt.googleHttpClient.config.URL
		if yt.googleHttpClient.config.Tool == "imagefx" {
			url = "https://aisandbox-pa.googleapis.com/v1:runImageFx"
		}

		// Marshal payload to JSON
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}

		// Create HTTP request with the determined URL
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers - use API key from database
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", apiKey.KeyValue)

		// Add custom headers
		for key, value := range yt.googleHttpClient.config.Headers {
			req.Header.Set(key, value)
		}

		// Make the request
		resp, err := yt.googleHttpClient.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to make request: %w", err)
			if attempt < yt.googleHttpClient.config.RetryAttempts {
				yt.waitWithBackoff(attempt)
				continue
			}
			return nil, lastErr
		}
		defer resp.Body.Close()

		// Read response body first to check for unsafe generation errors
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			if attempt < yt.googleHttpClient.config.RetryAttempts {
				yt.waitWithBackoff(attempt)
				continue
			}
			return nil, lastErr
		}

		// Log API key usage for successful requests
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("Successfully used API key: %s (Provider: %s)\n",
				apiKey.ID.Hex()[:8]+"...", apiKey.Provider)
		}

		// Handle rate limiting (429) and server errors (5xx) with retry
		// Flag the current API key as problematic and try with a new one
		if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
			lastErr = fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))

			// Flag the current API key as problematic
			flagErr := yt.apiKeyManager.FlagKeyAsProblematic(apiKey.ID, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
			if flagErr != nil {
				log.Printf("Warning: Failed to flag API key as problematic: %v", flagErr)
			} else {
				fmt.Printf("Flagged API key %s as problematic due to %d error\n",
					apiKey.ID.Hex()[:8]+"...", resp.StatusCode)
			}

			if attempt < yt.googleHttpClient.config.RetryAttempts {
				fmt.Printf("Request failed with status %d, flagging API key and retrying with new key (attempt %d/%d)...\n",
					resp.StatusCode, attempt+1, yt.googleHttpClient.config.RetryAttempts+1)
				yt.waitWithBackoff(attempt)
				continue
			}
			return nil, lastErr
		}

		// Handle content policy violations (400 with unsafe generation error) - don't retry
		if resp.StatusCode == 400 && isUnsafeGenerationError(body) {
			return nil, fmt.Errorf("content policy violation - prompt contains unsafe content: %s", string(body))
		}

		// Check for other error status codes (don't retry client errors except 429)
		if resp.StatusCode != http.StatusOK {
			// For other 4xx errors that might indicate API key issues, flag the key
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 400 {
				flagErr := yt.apiKeyManager.FlagKeyAsProblematic(apiKey.ID, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
				if flagErr != nil {
					log.Printf("Warning: Failed to flag API key as problematic: %v", flagErr)
				} else {
					fmt.Printf("Flagged API key %s as problematic due to %d error\n",
						apiKey.ID.Hex()[:8]+"...", resp.StatusCode)
				}
			}
			return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse successful response
		var apiResponse APIResponse
		err = json.Unmarshal(body, &apiResponse)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		return &apiResponse, nil
	}

	return nil, fmt.Errorf("max retry attempts exceeded: %w", lastErr)
}

// calculateBackoffDelay calculates the delay for exponential backoff
func (yt *YtAutomation) calculateBackoffDelay(attempt int) time.Duration {
	delay := time.Duration(float64(yt.googleHttpClient.config.InitialRetryDelay) * math.Pow(yt.googleHttpClient.config.BackoffMultiplier, float64(attempt)))
	if delay > yt.googleHttpClient.config.MaxRetryDelay {
		delay = yt.googleHttpClient.config.MaxRetryDelay
	}
	return delay
}

// waitWithBackoff waits with exponential backoff
func (yt *YtAutomation) waitWithBackoff(attempt int) {
	delay := yt.calculateBackoffDelay(attempt)
	time.Sleep(delay)
}

func (yt *YtAutomation) SaveImage(encodedImage, filename string) (string, error) {
	// Decode base64 image
	imageData, err := base64.StdEncoding.DecodeString(encodedImage)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// Ensure output directory exists
	err = os.MkdirAll(yt.googleHttpClient.config.OutputDirectory, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create full file path
	fullPath := filepath.Join(yt.googleHttpClient.config.OutputDirectory, filename)

	// Write image to file
	err = os.WriteFile(fullPath, imageData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write image file: %w", err)
	}

	fmt.Printf("Image saved: %s\n", fullPath)
	return fullPath, nil
}

// Helper function to get prompt by job ID
// Helper function to get prompt by job ID
func getPromptByJobID(jobs []RequestJob, jobID string) string {
	for _, job := range jobs {
		if job.ID == jobID {
			// Handle different payload types
			switch payload := job.Payload.(type) {
			case RequestPayload:
				return payload.Prompt
			case ImageFXPayload:
				if len(payload.UserInput.Prompts) > 0 {
					return payload.UserInput.Prompts[0]
				}
			}
		}
	}
	return "Unknown prompt"
}

// ProcessResponse processes the API response and saves all images
func (yt *YtAutomation) ProcessResponse(response *APIResponse, job RequestJob) error {
	for _, panel := range response.ImagePanels {
		for _, img := range panel.GeneratedImages {
			// Format times by replacing colons and commas with underscores for filesystem safety
			startTime := strings.ReplaceAll(strings.ReplaceAll(job.chunkVisual.StartTime, ":", "_"), ",", "_")
			endTime := strings.ReplaceAll(strings.ReplaceAll(job.chunkVisual.EndTime, ":", "_"), ",", "_")

			filename := fmt.Sprintf("chunk_%d_prompt_%d_start_%s_end_%s_seed_%d.jpg",
				job.chunkVisual.ChunkIndex, job.chunkVisual.PromptIndex, startTime, endTime, img.Seed)

			fullPath, err := yt.SaveImage(img.EncodedImage, filename)
			if err != nil {
				return fmt.Errorf("failed to save image %s: %w", filename, err)
			}

			_, err = chunkVisualsCollection.UpdateOne(
				context.Background(),
				bson.M{"_id": job.chunkVisual.ID},
				bson.M{"$set": bson.M{"image_path": fullPath}},
			)
			if err != nil {
				fmt.Printf("Warning: Failed to update chunk visual image_path: %v\n", err)
			}
		}
	}
	return nil
}

// RequestJob represents a single request job for concurrent processing
type RequestJob struct {
	ID          string
	Payload     interface{} // Changed from RequestPayload to interface{}
	chunkVisual ChunkVisual
}

// JobResult represents the result of a job execution
type JobResult struct {
	ID      string
	Success bool
	Error   error
	Skipped bool // true if skipped due to content policy violation
}

// Add this function to sanitize prompts for content policy violations
func (yt *YtAutomation) SanitizePrompt(originalPrompt string) string {
	// Simple sanitization - remove potentially problematic words/phrases
	// You can expand this list based on your needs
	problematicWords := []string{
		"violence", "violent", "blood", "poison", "death", "kill", "murder",
		"nude", "naked", "sexual", "explicit", "adult", "porn", "nsfw",
		"weapon", "gun", "knife", "bomb", "explosive", "terrorist",
		"hate", "racism", "discrimination", "offensive",
	}

	sanitized := strings.ToLower(originalPrompt)
	for _, word := range problematicWords {
		sanitized = strings.ReplaceAll(sanitized, word, "")
	}

	// Clean up extra spaces and add safe content
	sanitized = strings.TrimSpace(strings.ReplaceAll(sanitized, "  ", " "))
	if sanitized == "" {
		sanitized = "a beautiful peaceful landscape"
	} else {
		sanitized = "a safe and family-friendly " + sanitized
	}

	return sanitized
}

// Modified MakeRequestWithRetry method that handles content policy violations
func (yt *YtAutomation) MakeRequestWithRetry(originalPayload interface{}, maxContentRetries int) (*APIResponse, error) {
	var lastErr error

	for contentRetry := 0; contentRetry <= maxContentRetries; contentRetry++ {
		// Create a copy of the payload for this attempt
		var payload interface{}

		if contentRetry > 0 {
			// Modify the prompt for retry attempts
			switch p := originalPayload.(type) {
			case RequestPayload:
				modifiedPayload := p
				if contentRetry == 1 {
					modifiedPayload.Prompt = yt.SanitizePrompt(p.Prompt)
				} else {
					// For subsequent retries, make it even more generic
					modifiedPayload.Prompt = fmt.Sprintf("a safe and peaceful artistic representation of %s", yt.SanitizePrompt(p.Prompt))
				}
				payload = modifiedPayload
				fmt.Printf("Content retry %d: Modified prompt to: %s\n", contentRetry, modifiedPayload.Prompt)

			case ImageFXPayload:
				modifiedPayload := p
				if len(p.UserInput.Prompts) > 0 {
					if contentRetry == 1 {
						modifiedPayload.UserInput.Prompts[0] = yt.SanitizePrompt(p.UserInput.Prompts[0])
					} else {
						modifiedPayload.UserInput.Prompts[0] = fmt.Sprintf("a safe and peaceful artistic representation of %s", yt.SanitizePrompt(p.UserInput.Prompts[0]))
					}
					payload = modifiedPayload
					fmt.Printf("Content retry %d: Modified prompt to: %s\n", contentRetry, modifiedPayload.UserInput.Prompts[0])
				}
			}
		} else {
			payload = originalPayload
		}

		// Use the existing MakeRequest method
		response, err := yt.MakeRequest(payload)
		if err != nil {
			// Check if it's a content policy violation
			if strings.Contains(err.Error(), "content policy violation") {
				lastErr = err
				if contentRetry < maxContentRetries {
					fmt.Printf("Content policy violation detected, retrying with sanitized prompt (attempt %d/%d)\n",
						contentRetry+1, maxContentRetries+1)
					continue
				}
			}
			// For other errors, return immediately
			return nil, err
		}

		return response, nil
	}

	// If we exhausted all content retries
	return nil, fmt.Errorf("content policy violation persists after %d sanitization attempts: %w", maxContentRetries+1, lastErr)
}

// MakeConcurrentRequests makes multiple requests concurrently with proper rate limiting
func (yt *YtAutomation) MakeConcurrentRequests(jobs []RequestJob) error {
	// Create a semaphore to limit concurrency
	semaphore := make(chan struct{}, yt.googleHttpClient.config.MaxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []JobResult
	var completed int

	for _, job := range jobs {
		wg.Add(1)
		// This is the section inside the goroutine that needs to be fixed
		go func(j RequestJob) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := JobResult{ID: j.ID}
			// Update status to processing
			yt.updateVisualChunkStatus(j.chunkVisual.ID, "processing")
			// Make the request (with built-in rate limiting and retries)
			response, err := yt.MakeRequestWithRetry(j.Payload, 3)
			if err != nil {
				// Check if it's still a content policy violation after retries
				if strings.Contains(err.Error(), "content policy violation") {
					result.Skipped = true
					result.Error = err
					yt.updateVisualChunkStatus(j.chunkVisual.ID, "skipped")
					fmt.Printf("Skipping request %s - content policy violation persists after sanitization attempts\n", j.ID)
				} else if strings.Contains(err.Error(), "no active API keys found") {
					// Handle case where no API keys are available
					result.Success = false
					result.Error = err
					yt.updateVisualChunkWithAPIKeyError(j.chunkVisual.ID, "No active API keys available")
					fmt.Printf("Request %s failed due to no available API keys\n", j.ID)
				} else {
					result.Success = false
					result.Error = err
					yt.updateVisualChunkStatus(j.chunkVisual.ID, "failed")
				}

				mu.Lock()
				results = append(results, result)
				completed++
				fmt.Printf("Request %s failed/skipped (%d/%d): %v\n", j.ID, completed, len(jobs), err)
				mu.Unlock()
				return
			}

			// Process and save images
			err = yt.ProcessResponse(response, j)
			if err != nil {
				result.Success = false
				result.Error = fmt.Errorf("processing %s failed: %w", j.ID, err)
				yt.updateVisualChunkStatus(j.chunkVisual.ID, "failed")

				mu.Lock()
				results = append(results, result)
				completed++
				fmt.Printf("Request %s processing failed (%d/%d): %v\n", j.ID, completed, len(jobs), err)
				mu.Unlock()
				return
			}

			result.Success = true
			yt.updateVisualChunkStatus(j.chunkVisual.ID, "completed")
			// Update processing info if available
			if payload, ok := j.Payload.(RequestPayload); ok {
				yt.updateVisualChunkWithProcessingInfo(j.chunkVisual.ID, payload.Seed, 1)
			} else if payload, ok := j.Payload.(ImageFXPayload); ok {
				yt.updateVisualChunkWithProcessingInfo(j.chunkVisual.ID, payload.UserInput.Seed, 1)
			}
			mu.Lock()
			results = append(results, result)
			completed++
			fmt.Printf("Completed request %s (%d/%d)\n", j.ID, completed, len(jobs))
			mu.Unlock()
		}(job)
	}

	wg.Wait()

	// Analyze results
	var successCount, failureCount, skippedCount int
	var criticalErrors []error

	for _, result := range results {
		switch {
		case result.Success:
			successCount++
		case result.Skipped:
			skippedCount++
			log.Printf("Skipped %s due to content policy violation", result.ID)
			log.Printf("Prompt was: %s", getPromptByJobID(jobs, result.ID))
		default:
			failureCount++
			criticalErrors = append(criticalErrors, fmt.Errorf("%s: %w", result.ID, result.Error))
			log.Printf("Failed %s: %v", result.ID, result.Error)
		}
	}

	// Print summary
	fmt.Printf("\n=== EXECUTION SUMMARY ===\n")
	fmt.Printf("Total requests: %d\n", len(jobs))
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Skipped (content policy): %d\n", skippedCount)
	fmt.Printf("Failed (other errors): %d\n", failureCount)

	// Only return error if there are critical failures (not content policy violations)
	if len(criticalErrors) > 0 {
		fmt.Printf("\nCritical errors encountered:\n")
		for _, err := range criticalErrors {
			fmt.Printf("- %v\n", err)
		}
		return fmt.Errorf("encountered %d critical errors during concurrent requests", len(criticalErrors))
	}

	if skippedCount > 0 {
		fmt.Printf("\nNote: %d requests were skipped due to content policy violations. This is normal and expected.\n", skippedCount)
	}

	return nil
}

// GenerateSeed generates a seed based on the configuration
func (yt *YtAutomation) GenerateSeed() int {
	if yt.googleHttpClient.config.SeedMode == "static" {
		return yt.googleHttpClient.config.StaticSeed
	}
	// Generate random seed between 1 and 999999
	return yt.googleHttpClient.rng.Intn(999999) + 1
}

// CreateDefaultPayload creates a default payload with customizable options
func CreateDefaultPayload(options map[string]interface{}) RequestPayload {
	payload := RequestPayload{
		ClientContext: ClientContext{
			WorkflowID: "c1adcfbd-0a10-4476-a265-ee421e26f7ba",
			Tool:       "BACKBONE",
			SessionID:  ";1748453848255",
		},
		ImageModelSettings: ImageModelSettings{
			ImageModel:  "IMAGEN_3_5",
			AspectRatio: ASPECT_RATIO,
		},
		Seed:          738224,
		Prompt:        "A beautiful landscape",
		MediaCategory: "MEDIA_CATEGORY_BOARD",
	}

	// Apply custom options
	if val, ok := options["workflowId"].(string); ok {
		payload.ClientContext.WorkflowID = val
	}
	if val, ok := options["tool"].(string); ok {
		payload.ClientContext.Tool = val
	}
	if val, ok := options["sessionId"].(string); ok {
		payload.ClientContext.SessionID = val
	}
	if val, ok := options["imageModel"].(string); ok {
		payload.ImageModelSettings.ImageModel = val
	}
	if val, ok := options["aspectRatio"].(string); ok {
		payload.ImageModelSettings.AspectRatio = val
	}
	if val, ok := options["seed"].(int); ok {
		payload.Seed = val
	}
	if val, ok := options["prompt"].(string); ok {
		payload.Prompt = val
	}
	if val, ok := options["mediaCategory"].(string); ok {
		payload.MediaCategory = val
	}

	return payload
}

func (yt *YtAutomation) CreateJobsFromPrompts(chunkVisuals []ChunkVisual, options map[string]interface{}) []RequestJob {
	var jobs []RequestJob

	for i, chunkVisual := range chunkVisuals {
		// Create options for this specific job
		jobOptions := make(map[string]interface{})

		// Copy global options
		for k, v := range options {
			jobOptions[k] = v
		}

		// Set the prompt and seed
		jobOptions["prompt"] = chunkVisual.Prompt
		jobOptions["seed"] = yt.GenerateSeed()

		// Create the job based on the tool type
		var payload interface{}
		if yt.googleHttpClient.config.Tool == "imagefx" {
			payload = CreateImageFXPayload(jobOptions)
		} else {
			// Default to whisk
			payload = CreateWhiskPayload(jobOptions)
		}

		// Create the job
		job := RequestJob{
			ID:          fmt.Sprintf("prompt_%d", i+1),
			Payload:     payload,
			chunkVisual: chunkVisual,
		}

		jobs = append(jobs, job)
	}

	return jobs
}

// GetEnv gets an environment variable with a default value
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvInt gets an environment variable as integer with a default value
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetEnvDuration gets an environment variable as duration with a default value
func GetEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// GetEnvFloat gets an environment variable as float64 with a default value
func GetEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() HttpConfig {
	return HttpConfig{
		URL:       GetEnv("API_URL", "https://aisandbox-pa.googleapis.com/v1/whisk:generateImage"),
		AuthToken: GetEnv("API_AUTH_TOKEN", ""),
		Headers: map[string]string{
			"User-Agent": GetEnv("USER_AGENT", "Go HTTP Client"),
			"Accept":     GetEnv("ACCEPT_HEADER", "application/json"),
		},
		OutputDirectory:   GetEnv("OUTPUT_DIRECTORY", "./assets/images/"),
		MaxConcurrency:    GetEnvInt("MAX_CONCURRENCY", 2),
		Timeout:           GetEnvDuration("TIMEOUT", 60*time.Second),
		SeedMode:          GetEnv("SEED_MODE", "random"),
		StaticSeed:        GetEnvInt("STATIC_SEED", 12345),
		RequestsPerMinute: GetEnvInt("REQUESTS_PER_MINUTE", 15),
		RetryAttempts:     GetEnvInt("RETRY_ATTEMPTS", 3),
		InitialRetryDelay: GetEnvDuration("INITIAL_RETRY_DELAY", 2*time.Second),
		BackoffMultiplier: GetEnvFloat("BACKOFF_MULTIPLIER", 2.0),
		MaxRetryDelay:     GetEnvDuration("MAX_RETRY_DELAY", 30*time.Second),
		Tool:              GetEnv("TOOL", "whisk"),
	}
}

// LoadConfigFromFile loads configuration from a JSON file
func LoadConfigFromFile(filename string) (HttpConfig, error) {
	var config HttpConfig

	file, err := os.Open(filename)
	if err != nil {
		return config, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	return config, err
}

// SaveConfigToFile saves configuration to a JSON file
func SaveConfigToFile(config HttpConfig, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(config)
}
func CreateImageFXPayload(options map[string]interface{}) ImageFXPayload {
	payload := ImageFXPayload{
		UserInput: ImageFXUserInput{
			CandidatesCount: 1,
			Prompts:         []string{"A beautiful landscape"},
			Seed:            738224,
		},
		ClientContext: ClientContext{
			SessionID: ";1749359707591",
			Tool:      "IMAGE_FX",
		},
		ModelInput: ImageFXModelInput{
			ModelNameType: "IMAGEN_3_1",
		},
		AspectRatio: ASPECT_RATIO,
	}

	// Apply options similar to your existing logic
	if val, ok := options["prompt"].(string); ok {
		payload.UserInput.Prompts = []string{val}
	}
	// ... handle other options

	return payload
}

// CreateWhiskPayload creates a Whisk-specific payload with customizable options
func CreateWhiskPayload(options map[string]interface{}) RequestPayload {
	payload := RequestPayload{
		ClientContext: ClientContext{
			WorkflowID: "c1adcfbd-0a10-4476-a265-ee421e26f7ba",
			Tool:       "BACKBONE",
			SessionID:  ";1748453848255",
		},
		ImageModelSettings: ImageModelSettings{
			ImageModel:  "IMAGEN_3_5",
			AspectRatio: ASPECT_RATIO,
		},
		Seed:          738224,
		Prompt:        "A beautiful landscape",
		MediaCategory: "MEDIA_CATEGORY_BOARD",
	}

	// Apply custom options
	if val, ok := options["workflowId"].(string); ok {
		payload.ClientContext.WorkflowID = val
	}
	if val, ok := options["tool"].(string); ok {
		payload.ClientContext.Tool = val
	}
	if val, ok := options["sessionId"].(string); ok {
		payload.ClientContext.SessionID = val
	}
	if val, ok := options["imageModel"].(string); ok {
		payload.ImageModelSettings.ImageModel = val
	}
	if val, ok := options["aspectRatio"].(string); ok {
		payload.ImageModelSettings.AspectRatio = val
	}
	if val, ok := options["seed"].(int); ok {
		payload.Seed = val
	}
	if val, ok := options["prompt"].(string); ok {
		payload.Prompt = val
	}
	if val, ok := options["mediaCategory"].(string); ok {
		payload.MediaCategory = val
	}

	return payload
}
