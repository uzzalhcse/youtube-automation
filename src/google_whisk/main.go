package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ASPECT_RATIO = "IMAGE_ASPECT_RATIO_LANDSCAPE" // Options: IMAGE_ASPECT_RATIO_LANDSCAPE, IMAGE_ASPECT_RATIO_PORTRAIT, IMAGE_ASPECT_RATIO_SQUARE
)

// Config holds all configuration for the HTTP client
type Config struct {
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

// isUnsafeGenerationError checks if the error is due to unsafe content policy violation
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
	config      Config
	httpClient  *http.Client
	rng         *rand.Rand
	rateLimiter *RateLimiter
}

// NewHTTPClient creates a new HTTP client with the given configuration
func NewHTTPClient(config Config) *HTTPClient {
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

// MakeRequest makes a single HTTP POST request with the given payload, with retry logic
// MakeRequest makes a single HTTP POST request with the given payload, with retry logic
func (c *HTTPClient) MakeRequest(payload interface{}) (*APIResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		// Apply rate limiting if configured
		if c.rateLimiter != nil {
			c.rateLimiter.Wait()
			current, max := c.rateLimiter.GetCurrentUsage()
			fmt.Printf("Rate limit usage: %d/%d requests in last minute\n", current, max)
		}

		// Determine URL based on tool configuration
		url := c.config.URL
		if c.config.Tool == "imagefx" {
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

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", c.config.AuthToken)

		// Add custom headers
		for key, value := range c.config.Headers {
			req.Header.Set(key, value)
		}

		// Make the request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to make request: %w", err)
			if attempt < c.config.RetryAttempts {
				c.waitWithBackoff(attempt)
				continue
			}
			return nil, lastErr
		}
		defer resp.Body.Close()

		// Read response body first to check for unsafe generation errors
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			if attempt < c.config.RetryAttempts {
				c.waitWithBackoff(attempt)
				continue
			}
			return nil, lastErr
		}

		// Handle rate limiting (429) and server errors (5xx) with retry
		if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
			lastErr = fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))

			if attempt < c.config.RetryAttempts {
				fmt.Printf("Request failed with status %d, retrying in %v (attempt %d/%d)...\n",
					resp.StatusCode, c.calculateBackoffDelay(attempt), attempt+1, c.config.RetryAttempts+1)
				c.waitWithBackoff(attempt)
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
func (c *HTTPClient) calculateBackoffDelay(attempt int) time.Duration {
	delay := time.Duration(float64(c.config.InitialRetryDelay) * math.Pow(c.config.BackoffMultiplier, float64(attempt)))
	if delay > c.config.MaxRetryDelay {
		delay = c.config.MaxRetryDelay
	}
	return delay
}

// waitWithBackoff waits with exponential backoff
func (c *HTTPClient) waitWithBackoff(attempt int) {
	delay := c.calculateBackoffDelay(attempt)
	time.Sleep(delay)
}

// SaveImage decodes base64 image data and saves it as a JPG file
func (c *HTTPClient) SaveImage(encodedImage, filename string) error {
	// Decode base64 image
	imageData, err := base64.StdEncoding.DecodeString(encodedImage)
	if err != nil {
		return fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// Ensure output directory exists
	err = os.MkdirAll(c.config.OutputDirectory, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create full file path
	fullPath := filepath.Join(c.config.OutputDirectory, filename)

	// Write image to file
	err = os.WriteFile(fullPath, imageData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	fmt.Printf("Image saved: %s\n", fullPath)
	return nil
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
func (c *HTTPClient) ProcessResponse(response *APIResponse, requestID string) error {
	for panelIdx, panel := range response.ImagePanels {
		for imgIdx, img := range panel.GeneratedImages {
			filename := fmt.Sprintf("%s_panel_%d_image_%d_seed_%d.jpg",
				requestID, panelIdx, imgIdx, img.Seed)

			err := c.SaveImage(img.EncodedImage, filename)
			if err != nil {
				return fmt.Errorf("failed to save image %s: %w", filename, err)
			}
		}
	}
	return nil
}

// RequestJob represents a single request job for concurrent processing
type RequestJob struct {
	ID      string
	Payload interface{} // Changed from RequestPayload to interface{}
}

// JobResult represents the result of a job execution
type JobResult struct {
	ID      string
	Success bool
	Error   error
	Skipped bool // true if skipped due to content policy violation
}

// MakeConcurrentRequests makes multiple requests concurrently with proper rate limiting
func (c *HTTPClient) MakeConcurrentRequests(jobs []RequestJob) error {
	// Create a semaphore to limit concurrency
	semaphore := make(chan struct{}, c.config.MaxConcurrency)
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

			// Get seed for logging based on payload type
			var seed int
			switch payload := j.Payload.(type) {
			case RequestPayload:
				seed = payload.Seed
			case ImageFXPayload:
				seed = payload.UserInput.Seed
			default:
				seed = 0 // fallback
			}

			fmt.Printf("Starting request %s with seed %d\n", j.ID, seed)

			result := JobResult{ID: j.ID}

			// Make the request (with built-in rate limiting and retries)
			response, err := c.MakeRequest(j.Payload)
			if err != nil {
				// Check if it's a content policy violation
				if strings.Contains(err.Error(), "content policy violation") {
					result.Skipped = true
					result.Error = err
					fmt.Printf("Skipping request %s due to content policy violation\n", j.ID)
				} else {
					result.Success = false
					result.Error = err
				}

				mu.Lock()
				results = append(results, result)
				completed++
				fmt.Printf("Request %s failed/skipped (%d/%d): %v\n", j.ID, completed, len(jobs), err)
				mu.Unlock()
				return
			}

			// Process and save images
			err = c.ProcessResponse(response, j.ID)
			if err != nil {
				result.Success = false
				result.Error = fmt.Errorf("processing %s failed: %w", j.ID, err)

				mu.Lock()
				results = append(results, result)
				completed++
				fmt.Printf("Request %s processing failed (%d/%d): %v\n", j.ID, completed, len(jobs), err)
				mu.Unlock()
				return
			}

			result.Success = true
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
func (c *HTTPClient) GenerateSeed() int {
	if c.config.SeedMode == "static" {
		return c.config.StaticSeed
	}
	// Generate random seed between 1 and 999999
	return c.rng.Intn(999999) + 1
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

// ParsePromptsFromFile reads and parses prompts from a text file
func ParsePromptsFromFile(filename string) ([]string, error) {
	// Read the file content
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts file: %w", err)
	}

	// Convert to string and clean up
	text := strings.TrimSpace(string(content))

	// Parse prompts using regex to match the pattern: Prompt X: "content"
	// (?s) flag enables dot to match newlines, .*? is non-greedy to stop at first closing quote
	re := regexp.MustCompile(`(?s)Prompt\s+\d+:\s*"(.*?)"`)
	matches := re.FindAllStringSubmatch(text, -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("no prompts found in file %s", filename)
	}

	// Extract the prompt content (capture group 1)
	var prompts []string
	for _, match := range matches {
		if len(match) > 1 {
			prompts = append(prompts, match[1])
		}
	}

	return prompts, nil
}

// CreateJobsFromPrompts creates request jobs from a list of prompts
// CreateJobsFromPrompts creates request jobs from a list of prompts
func (c *HTTPClient) CreateJobsFromPrompts(prompts []string, options map[string]interface{}) []RequestJob {
	var jobs []RequestJob

	for i, prompt := range prompts {
		// Create options for this specific job
		jobOptions := make(map[string]interface{})

		// Copy global options
		for k, v := range options {
			jobOptions[k] = v
		}

		// Set the prompt and seed
		jobOptions["prompt"] = prompt
		jobOptions["seed"] = c.GenerateSeed()

		// Create the job based on the tool type
		var payload interface{}
		if c.config.Tool == "imagefx" {
			payload = CreateImageFXPayload(jobOptions)
		} else {
			// Default to whisk
			payload = CreateWhiskPayload(jobOptions)
		}

		// Create the job
		job := RequestJob{
			ID:      fmt.Sprintf("prompt_%d", i+1),
			Payload: payload,
		}

		jobs = append(jobs, job)
	}

	return jobs
}

// LoadEnv loads environment variables from a .env file
func LoadEnv(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Warning: %s file not found, using default values\n", filename)
			return nil
		}
		return fmt.Errorf("failed to open %s: %w", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		os.Setenv(key, value)
	}

	return scanner.Err()
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
func LoadConfigFromEnv() Config {
	return Config{
		URL:       GetEnv("API_URL", "https://aisandbox-pa.googleapis.com/v1/whisk:generateImage"),
		AuthToken: GetEnv("API_AUTH_TOKEN", ""),
		Headers: map[string]string{
			"User-Agent": GetEnv("USER_AGENT", "Go HTTP Client"),
			"Accept":     GetEnv("ACCEPT_HEADER", "application/json"),
		},
		OutputDirectory:   GetEnv("OUTPUT_DIRECTORY", "./generated_images"),
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

func main() {
	// Load environment variables from .env file
	err := LoadEnv(".env")
	if err != nil {
		log.Printf("Warning: Failed to load .env file: %v", err)
	}

	// Load configuration from environment variables
	config := LoadConfigFromEnv()

	// Validate required configuration
	if config.AuthToken == "" {
		log.Fatal("API_AUTH_TOKEN is required. Please set it in your .env file or environment variables.")
	}
	// Validate tool selection
	if config.Tool != "whisk" && config.Tool != "imagefx" {
		log.Fatalf("Invalid tool: %s. Must be 'whisk' or 'imagefx'", config.Tool)
	}

	fmt.Printf("Using tool: %s\n", config.Tool)
	// Create HTTP client
	client := NewHTTPClient(config)

	// Parse prompts from file
	promptsFile := GetEnv("PROMPTS_FILE", "image_prompts.txt")
	fmt.Printf("Reading prompts from %s...\n", promptsFile)
	prompts, err := ParsePromptsFromFile(promptsFile)
	if err != nil {
		log.Fatalf("Failed to parse prompts: %v", err)
	}

	fmt.Printf("Found %d prompts\n", len(prompts))
	for i, prompt := range prompts {
		fmt.Printf("Prompt %d: %.80s...\n", i+1, prompt)
	}

	// Global options for all prompts
	globalOptions := map[string]interface{}{
		"imageModel":  GetEnv("IMAGE_MODEL", "IMAGEN_3_5"),
		"aspectRatio": ASPECT_RATIO,
	}

	// Create jobs from prompts
	jobs := client.CreateJobsFromPrompts(prompts, globalOptions)

	// Make concurrent requests with rate limiting and retries
	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("- Rate limit: %d requests per minute\n", config.RequestsPerMinute)
	fmt.Printf("- Max concurrency: %d\n", config.MaxConcurrency)
	fmt.Printf("- Retry attempts: %d\n", config.RetryAttempts)
	fmt.Printf("- Output directory: %s\n", config.OutputDirectory)
	fmt.Printf("\nMaking %d requests with rate limiting and retries...\n", len(jobs))

	startTime := time.Now()
	err = client.MakeConcurrentRequests(jobs)
	if err != nil {
		log.Printf("Some requests encountered critical errors: %v", err)
		// Don't exit fatally - let the program complete and show summary
	}

	duration := time.Since(startTime)
	fmt.Printf("\nExecution completed in %v!\n", duration)
}

// LoadConfigFromFile loads configuration from a JSON file
func LoadConfigFromFile(filename string) (Config, error) {
	var config Config

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
func SaveConfigToFile(config Config, filename string) error {
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
