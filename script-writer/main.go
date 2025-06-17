package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// API Request/Response structures
type ScriptRequest struct {
	Topic           string `json:"topic"`
	GenerateVisuals bool   `json:"generate_visuals"`
}

type ScriptResponse struct {
	Success         bool   `json:"success"`
	Message         string `json:"message,omitempty"`
	Topic           string `json:"topic,omitempty"`
	OutputFolder    string `json:"output_folder,omitempty"`
	OutputFilename  string `json:"output_filename,omitempty"`
	MetaTagFilename string `json:"metatag_filename,omitempty"`
	GeneratedAt     string `json:"generated_at,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Global services - initialized once
var (
	templateService *TemplateService
	geminiService   *GeminiService
	scriptService   *ScriptService
)

func main() {
	// Initialize services once at startup
	if err := initializeServices(); err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// Setup HTTP routes
	http.HandleFunc("/generate-script", generateScriptHandler)
	http.HandleFunc("/health", healthHandler)

	// Start server
	port := getPort()
	fmt.Printf("=== Wisderly YouTube Script Generator API ===\n")
	fmt.Printf("Server starting on port %s\n", port)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST /generate-script - Generate YouTube script\n")
	fmt.Printf("  GET  /health         - Health check\n")
	fmt.Println(strings.Repeat("=", 50))

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initializeServices() error {
	// Get API key - in production, use environment variable
	apiKey := getAPIKey()
	if apiKey == "" {
		return fmt.Errorf("API key not found. Set GEMINI_API_KEY environment variable or update the code")
	}

	// Initialize services (same as original logic)
	templateService = NewTemplateService()
	geminiService = NewGeminiService(apiKey)
	scriptService = NewScriptService(templateService, geminiService)

	// Load templates (same as original logic)
	if err := templateService.LoadAllTemplates(); err != nil {
		return fmt.Errorf("error loading templates: %v", err)
	}

	fmt.Println("✓ All templates loaded successfully")
	return nil
}

func generateScriptHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST method
	if r.Method != "POST" {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed. Use POST.")
		return
	}

	// Parse request body
	var req ScriptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	// Validate topic (same validation as original getUserInput)
	if strings.TrimSpace(req.Topic) == "" {
		respondWithError(w, http.StatusBadRequest, "Topic cannot be empty")
		return
	}

	// Create script config (same logic as original getUserInput)
	config, err := createScriptConfig(req.Topic, req.GenerateVisuals)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating config: %v", err))
		return
	}

	// Generate script (same logic as original main function)
	if err := scriptService.GenerateCompleteScript(config); err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error generating script: %v", err))
		return
	}

	// Return success response
	response := ScriptResponse{
		Success:         true,
		Message:         "Script generated successfully",
		Topic:           config.Topic,
		OutputFolder:    config.OutputFolder,
		OutputFilename:  config.OutputFilename,
		MetaTagFilename: config.MetaTagFilename,
		GeneratedAt:     time.Now().Format(time.RFC3339),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	// Log success (similar to original output)
	log.Printf("✓ Script generated for topic: %s | File: %s", config.Topic, config.OutputFilename)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "Wisderly YouTube Script Generator API",
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

func createScriptConfig(topic string, generateVisuals bool) (*ScriptConfig, error) {
	// Same logic as original getUserInput function
	topic = strings.TrimSpace(topic)

	config := &ScriptConfig{
		Topic:                topic,
		GenerateVisuals:      generateVisuals,
		ChannelName:          channelName,
		OutputFolder:         sanitizeFilename(topic),
		OutputFilename:       fmt.Sprintf("script_%d.txt", time.Now().Unix()),
		MetaTagFilename:      fmt.Sprintf("metatag_%d.txt", time.Now().Unix()),
		SectionCount:         defaultSectionCount,
		SleepBetweenSections: defaultSleepBetweenSections,
	}

	return config, nil
}

func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	response := ScriptResponse{
		Success: false,
		Error:   message,
	}
	json.NewEncoder(w).Encode(response)
	log.Printf("❌ Error: %s", message)
}

func getAPIKey() string {
	// Try environment variable first (production)
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		return apiKey
	}

	// Fallback to hardcoded (same as original)
	return "AIzaSyDykpGH35C4BRC_V3OK-GoHAIJ97RfwvMc"
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8085"
}

// Preserved original utility function
func sanitizeFilename(topic string) string {
	replacements := []string{" ", "_", "/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_"}

	sanitized := topic
	for i := 0; i < len(replacements); i += 2 {
		sanitized = strings.ReplaceAll(sanitized, replacements[i], replacements[i+1])
	}

	// Limit length
	if len(sanitized) > 80 {
		sanitized = sanitized[:80]
	}

	return sanitized
}

// Legacy CLI function - kept for backward compatibility
func runCLIMode() {
	fmt.Println("=== Wisderly YouTube Script Generator ===")
	fmt.Println("This tool will automatically generate a complete YouTube script.")
	fmt.Println()

	// Get user input
	config, err := getUserInput()
	if err != nil {
		fmt.Printf("Error getting user input: %v\n", err)
		return
	}

	// Generate script
	if err := scriptService.GenerateCompleteScript(config); err != nil {
		fmt.Printf("Error generating script: %v\n", err)
		return
	}

	fmt.Printf("\n🎉 Complete script saved to: %s\n", config.OutputFilename)
	fmt.Println(">>> End of script. Stay Healthy.")
}

func getUserInput() (*ScriptConfig, error) {
	reader := bufio.NewReader(os.Stdin)

	// Get topic from user
	fmt.Print("Enter your video topic: ")
	topic, _ := reader.ReadString('\n')
	topic = strings.TrimSpace(topic)

	if topic == "" {
		return nil, fmt.Errorf("topic cannot be empty")
	}

	// Ask for visual guidance preference
	fmt.Print("Generate Visual Guidance? (y/n): ")
	visualInput, _ := reader.ReadString('\n')
	generateVisuals := strings.TrimSpace(strings.ToLower(visualInput)) == "y"

	config := &ScriptConfig{
		Topic:                topic,
		GenerateVisuals:      generateVisuals,
		ChannelName:          channelName,
		OutputFolder:         sanitizeFilename(topic),
		OutputFilename:       fmt.Sprintf("script_%d.txt", time.Now().Unix()),
		MetaTagFilename:      fmt.Sprintf("metatag_%d.txt", time.Now().Unix()),
		SectionCount:         defaultSectionCount,
		SleepBetweenSections: defaultSleepBetweenSections,
	}

	fmt.Printf("\nGenerating script for topic: %s\n", topic)
	fmt.Printf("Visual Guidance: %t\n", generateVisuals)
	fmt.Printf("Output file: %s\n", config.OutputFilename)
	fmt.Println("\n" + strings.Repeat("=", 50))

	return config, nil
}
