package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Config holds all configuration for the HTTP client
type Config struct {
	URL             string            `json:"url"`
	AuthToken       string            `json:"auth_token"`
	Headers         map[string]string `json:"headers"`
	OutputDirectory string            `json:"output_directory"`
	MaxConcurrency  int               `json:"max_concurrency"`
	Timeout         time.Duration     `json:"timeout"`
	SeedMode        string            `json:"seed_mode"` // "random" or "static"
	StaticSeed      int               `json:"static_seed"`
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
	EncodedImage      string `json:"encodedImage"`
	Seed              int    `json:"seed"`
	MediaGenerationID string `json:"mediaGenerationId"`
	Prompt            string `json:"prompt"`
	ImageModel        string `json:"imageModel"`
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

// HTTPClient wraps the configuration and provides methods for making requests
type HTTPClient struct {
	config     Config
	httpClient *http.Client
	rng        *rand.Rand
}

// NewHTTPClient creates a new HTTP client with the given configuration
func NewHTTPClient(config Config) *HTTPClient {
	return &HTTPClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// MakeRequest makes a single HTTP POST request with the given payload
func (c *HTTPClient) MakeRequest(payload RequestPayload) (*APIResponse, error) {
	// Marshal payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", c.config.URL, bytes.NewBuffer(jsonData))
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
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read and parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResponse APIResponse
	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &apiResponse, nil
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
	Payload RequestPayload
}

// MakeConcurrentRequests makes multiple requests concurrently
func (c *HTTPClient) MakeConcurrentRequests(jobs []RequestJob) error {
	// Create a semaphore to limit concurrency
	semaphore := make(chan struct{}, c.config.MaxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for _, job := range jobs {
		wg.Add(1)
		go func(j RequestJob) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			fmt.Printf("Starting request %s with seed %d\n", j.ID, j.Payload.Seed)

			// Make the request
			response, err := c.MakeRequest(j.Payload)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("request %s failed: %w", j.ID, err))
				mu.Unlock()
				return
			}

			// Process and save images
			err = c.ProcessResponse(response, j.ID)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("processing %s failed: %w", j.ID, err))
				mu.Unlock()
				return
			}

			fmt.Printf("Completed request %s\n", j.ID)
		}(job)
	}

	wg.Wait()

	if len(errors) > 0 {
		for _, err := range errors {
			log.Printf("Error: %v", err)
		}
		return fmt.Errorf("encountered %d errors during concurrent requests", len(errors))
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
			AspectRatio: "IMAGE_ASPECT_RATIO_LANDSCAPE",
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
	re := regexp.MustCompile(`Prompt\s+\d+:\s*"([^"]+)"`)
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

		// Create the job
		job := RequestJob{
			ID:      fmt.Sprintf("prompt_%d", i+1),
			Payload: CreateDefaultPayload(jobOptions),
		}

		jobs = append(jobs, job)
	}

	return jobs
}

func main() {
	// Configuration
	config := Config{
		URL:       "https://aisandbox-pa.googleapis.com/v1/whisk:generateImage",
		AuthToken: "Bearer ya29.a0AW4Xtxgia6SvOFRR3a0cFo8--bA7l0TWaKoiTe57uFRo8SJYlJ7xjpOea9NgZjb8s7mW0DUctXvv23BTKio-fXnGluFTLHz4Mk-sgDxdrWQIQ-cJGjr4egeT7_G6K5DedLuge7ilBmuhFhuOpAavf5B32SWYBOReQFWi55g3xIRohMfXJ724OCGxViGmjUoafRMAQskedy4cCik5UW5qcYCM728J4rWd_Bd0yG-b6YL1Q9XbYr2alwbP_Xr_ihBIqVJJoRea14x7xolnWqRkFhAx9P504FvmWb3HKsuviiav0UhEs1OePEFf8-0LAz5lIVMjEF3SoxGqTx6YrpA5SV_iovsnrTi1EsjCcGHohdxEvqQfrlw4t-thSbmPsMUZfOwmrXCMVxaPQHtXzFnEXtIg8Ixd5Twnhlqx6pHc-QaCgYKAXwSARMSFQHGX2MiuYUa90u3T2SoUnR3H5y7yA0433",
		Headers: map[string]string{
			"User-Agent": "Go HTTP Client",
			"Accept":     "application/json",
		},
		OutputDirectory: "./generated_images",
		MaxConcurrency:  5,
		Timeout:         60 * time.Second,
		SeedMode:        "random", // "random" or "static"
		StaticSeed:      12345,    // Used when SeedMode is "static"
	}

	// Create HTTP client
	client := NewHTTPClient(config)

	// Parse prompts from file
	fmt.Println("Reading prompts from prompts.txt...")
	prompts, err := ParsePromptsFromFile("image_prompts.txt")
	if err != nil {
		log.Fatalf("Failed to parse prompts: %v", err)
	}

	fmt.Printf("Found %d prompts\n", len(prompts))
	for i, prompt := range prompts {
		fmt.Printf("Prompt %d: %.80s...\n", i+1, prompt)
	}

	// Global options for all prompts
	globalOptions := map[string]interface{}{
		"imageModel":  "IMAGEN_3_5",
		"aspectRatio": "IMAGE_ASPECT_RATIO_LANDSCAPE",
	}

	// Create jobs from prompts
	jobs := client.CreateJobsFromPrompts(prompts, globalOptions)

	// Make concurrent requests
	fmt.Printf("\nMaking %d concurrent requests with %s seeds...\n", len(jobs), config.SeedMode)
	err = client.MakeConcurrentRequests(jobs)
	if err != nil {
		log.Fatalf("Concurrent requests failed: %v", err)
	}

	fmt.Println("All requests completed successfully!")
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
