package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDB Models
type Channel struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ChannelName  string             `bson:"channel_name" json:"channel_name"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
	TotalScripts int                `bson:"total_scripts" json:"total_scripts"`
	Settings     ChannelSettings    `bson:"settings" json:"settings"`
}

type ChannelSettings struct {
	DefaultSectionCount     int  `bson:"default_section_count" json:"default_section_count"`
	PreferredVisualGuidance bool `bson:"preferred_visual_guidance" json:"preferred_visual_guidance"`
}

type ScriptGeneration struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ChannelName       string             `bson:"channel_name" json:"channel_name"`
	Topic             string             `bson:"topic" json:"topic"`
	Status            string             `bson:"status" json:"status"` // "pending", "processing", "completed", "failed"
	GenerateVisuals   bool               `bson:"generate_visuals" json:"generate_visuals"`
	OutputFolder      string             `bson:"output_folder" json:"output_folder"`
	OutputFilename    string             `bson:"output_filename" json:"output_filename"`
	MetaTagFilename   string             `bson:"metatag_filename" json:"metatag_filename"`
	CreatedAt         time.Time          `bson:"created_at" json:"created_at"`
	StartedAt         *time.Time         `bson:"started_at,omitempty" json:"started_at,omitempty"`
	CompletedAt       *time.Time         `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
	ErrorMessage      string             `bson:"error_message,omitempty" json:"error_message,omitempty"`
	ProcessingTime    float64            `bson:"processing_time_seconds,omitempty" json:"processing_time_seconds,omitempty"`
	SectionsGenerated int                `bson:"sections_generated,omitempty" json:"sections_generated,omitempty"`
	CurrentSection    int                `bson:"current_section,omitempty" json:"current_section,omitempty"`
}

// API Request/Response structures
type ScriptRequest struct {
	Topic           string `json:"topic"`
	GenerateVisuals bool   `json:"generate_visuals"`
	ChannelName     string `json:"channel_name"`
}

type ScriptResponse struct {
	Success         bool   `json:"success"`
	ScriptID        string `json:"script_id,omitempty"`
	Message         string `json:"message,omitempty"`
	Status          string `json:"status,omitempty"`
	Topic           string `json:"topic,omitempty"`
	ChannelName     string `json:"channel_name,omitempty"`
	OutputFolder    string `json:"output_folder,omitempty"`
	OutputFilename  string `json:"output_filename,omitempty"`
	MetaTagFilename string `json:"metatag_filename,omitempty"`
	GeneratedAt     string `json:"generated_at,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Global services and database
var (
	templateService    *TemplateService
	geminiService      *GeminiService
	scriptService      *ScriptService
	mongoClient        *mongo.Client
	database           *mongo.Database
	channelsCollection *mongo.Collection
	scriptsCollection  *mongo.Collection
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

func main() {
	// Initialize MongoDB connection
	if err := initializeMongoDB(); err != nil {
		log.Fatalf("Failed to initialize MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	// Initialize services
	if err := initializeServices(); err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// Setup HTTP routes
	http.HandleFunc("/generate-script", generateScriptHandler)
	http.HandleFunc("/scripts/", getScriptStatusHandler)
	http.HandleFunc("/health", healthHandler)

	// Start server
	port := getPort()
	fmt.Printf("=== Wisderly YouTube Script Generator API ===\n")
	fmt.Printf("Server starting on port %s\n", port)
	fmt.Printf("MongoDB connected: %s\n", getMongoURI())
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST /generate-script    - Generate YouTube script\n")
	fmt.Printf("  GET  /scripts/{id}       - Get script status\n")
	fmt.Printf("  GET  /health            - Health check\n")
	fmt.Println(strings.Repeat("=", 50))

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initializeMongoDB() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to MongoDB
	mongoURI := getMongoURI()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Test connection
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	// Initialize global variables
	mongoClient = client
	database = client.Database(getMongoDB())
	channelsCollection = database.Collection("channels")
	scriptsCollection = database.Collection("script_generations")

	// Create indexes
	if err := createIndexes(); err != nil {
		return fmt.Errorf("failed to create indexes: %v", err)
	}

	fmt.Println("✓ MongoDB connected successfully")
	return nil
}

func createIndexes() error {
	ctx := context.Background()

	// Index for script_generations
	_, err := scriptsCollection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{"channel_name", 1}, {"created_at", -1}},
		},
		{
			Keys: bson.D{{"status", 1}, {"created_at", -1}},
		},
	})
	if err != nil {
		return err
	}

	// Index for channels (unique channel_name)
	_, err = channelsCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{"channel_name", 1}},
		Options: options.Index().SetUnique(true),
	})

	return err
}

func initializeServices() error {
	// Get API key
	apiKey := getAPIKey()
	if apiKey == "" {
		return fmt.Errorf("API key not found. Set GEMINI_API_KEY environment variable")
	}

	// Initialize services (same as original logic)
	templateService = NewTemplateService()
	geminiService = NewGeminiService(apiKey)
	scriptService = NewScriptService(templateService, geminiService)

	// Load templates
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

	// Validate request
	if strings.TrimSpace(req.Topic) == "" {
		respondWithError(w, http.StatusBadRequest, "Topic cannot be empty")
		return
	}
	if strings.TrimSpace(req.ChannelName) == "" {
		respondWithError(w, http.StatusBadRequest, "Channel name cannot be empty")
		return
	}

	// Create script generation record in MongoDB
	scriptGen := &ScriptGeneration{
		ChannelName:     strings.TrimSpace(req.ChannelName),
		Topic:           strings.TrimSpace(req.Topic),
		Status:          StatusPending,
		GenerateVisuals: req.GenerateVisuals,
		CreatedAt:       time.Now(),
	}

	// Insert into database
	result, err := scriptsCollection.InsertOne(context.Background(), scriptGen)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create script record: %v", err))
		return
	}

	scriptID := result.InsertedID.(primitive.ObjectID)
	scriptGen.ID = scriptID

	// Create script config
	config, err := createScriptConfig(scriptGen)
	if err != nil {
		updateScriptStatus(scriptID, StatusFailed, fmt.Sprintf("Error creating config: %v", err))
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating config: %v", err))
		return
	}

	// Update script generation record with file details
	updateData := bson.M{
		"output_folder":    config.OutputFolder,
		"output_filename":  config.OutputFilename,
		"metatag_filename": config.MetaTagFilename,
		"status":           StatusProcessing,
		"started_at":       time.Now(),
	}
	_, err = scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": scriptID},
		bson.M{"$set": updateData},
	)
	if err != nil {
		log.Printf("Failed to update script record: %v", err)
	}

	// Process script generation in goroutine
	go func() {
		processScriptGeneration(scriptID, config)
	}()

	// Ensure or update channel record
	go func() {
		ensureChannelExists(req.ChannelName)
	}()

	// Return immediate response
	response := ScriptResponse{
		Success:         true,
		ScriptID:        scriptID.Hex(),
		Message:         "Script generation started",
		Status:          StatusProcessing,
		Topic:           req.Topic,
		ChannelName:     req.ChannelName,
		OutputFolder:    config.OutputFolder,
		OutputFilename:  config.OutputFilename,
		MetaTagFilename: config.MetaTagFilename,
		GeneratedAt:     time.Now().Format(time.RFC3339),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("✓ Script generation started for channel: %s | topic: %s | ID: %s",
		req.ChannelName, req.Topic, scriptID.Hex())
}

func processScriptGeneration(scriptID primitive.ObjectID, config *ScriptConfig) {
	startTime := time.Now()

	// Generate script (same logic as original)
	err := scriptService.GenerateCompleteScript(config)

	processingTime := time.Since(startTime).Seconds()

	if err != nil {
		// Update with failure
		updateData := bson.M{
			"status":                  StatusFailed,
			"error_message":           err.Error(),
			"processing_time_seconds": processingTime,
			"completed_at":            time.Now(),
		}
		scriptsCollection.UpdateOne(
			context.Background(),
			bson.M{"_id": scriptID},
			bson.M{"$set": updateData},
		)
		log.Printf("❌ Script generation failed for ID: %s | Error: %v", scriptID.Hex(), err)
		return
	}

	// Update with success
	updateData := bson.M{
		"status":                  StatusCompleted,
		"processing_time_seconds": processingTime,
		"completed_at":            time.Now(),
		"sections_generated":      config.SectionCount,
	}
	_, updateErr := scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": scriptID},
		bson.M{"$set": updateData},
	)
	if updateErr != nil {
		log.Printf("Failed to update completed script: %v", updateErr)
	}

	// Update channel statistics
	updateChannelStats(config.ChannelName, true)

	log.Printf("✅ Script generation completed for ID: %s | Time: %.2fs",
		scriptID.Hex(), processingTime)
}

func getScriptStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract script ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/scripts/")
	if path == "" {
		respondWithError(w, http.StatusBadRequest, "Script ID is required")
		return
	}

	// Convert to ObjectID
	objectID, err := primitive.ObjectIDFromHex(path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid script ID format")
		return
	}

	// Find script in database
	var script ScriptGeneration
	err = scriptsCollection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&script)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(script)
}

func ensureChannelExists(channelName string) {
	ctx := context.Background()

	// Check if channel exists
	var existingChannel Channel
	err := channelsCollection.FindOne(ctx, bson.M{"channel_name": channelName}).Decode(&existingChannel)

	if err == mongo.ErrNoDocuments {
		// Create new channel
		newChannel := Channel{
			ChannelName:  channelName,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			TotalScripts: 0,
			Settings: ChannelSettings{
				DefaultSectionCount:     defaultSectionCount,
				PreferredVisualGuidance: false,
			},
		}

		_, err := channelsCollection.InsertOne(ctx, newChannel)
		if err != nil {
			log.Printf("Failed to create channel %s: %v", channelName, err)
		} else {
			log.Printf("✓ Created new channel: %s", channelName)
		}
	}
}

func updateChannelStats(channelName string, success bool) {
	if success {
		_, err := channelsCollection.UpdateOne(
			context.Background(),
			bson.M{"channel_name": channelName},
			bson.M{
				"$inc": bson.M{"total_scripts": 1},
				"$set": bson.M{"updated_at": time.Now()},
			},
		)
		if err != nil {
			log.Printf("Failed to update channel stats for %s: %v", channelName, err)
		}
	}
}

func updateScriptStatus(scriptID primitive.ObjectID, status, errorMsg string) {
	updateData := bson.M{"status": status}
	if errorMsg != "" {
		updateData["error_message"] = errorMsg
	}

	scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": scriptID},
		bson.M{"$set": updateData},
	)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Test MongoDB connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mongoStatus := "healthy"
	if err := mongoClient.Ping(ctx, nil); err != nil {
		mongoStatus = "unhealthy: " + err.Error()
	}

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "Wisderly YouTube Script Generator API",
		"mongodb":   mongoStatus,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

func createScriptConfig(scriptGen *ScriptGeneration) (*ScriptConfig, error) {

	config := &ScriptConfig{
		Topic:                scriptGen.Topic,
		GenerateVisuals:      scriptGen.GenerateVisuals,
		ChannelName:          channelName, // This should be replaced with dynamic channel name
		OutputFolder:         sanitizeFilename(scriptGen.Topic),
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
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		return apiKey
	}
	return "AIzaSyDykpGH35C4BRC_V3OK-GoHAIJ97RfwvMc"
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8085"
}

func getMongoURI() string {
	if uri := os.Getenv("MONGODB_URI"); uri != "" {
		return uri
	}
	return "mongodb://localhost:27017"
}

func getMongoDB() string {
	if db := os.Getenv("MONGODB_DATABASE"); db != "" {
		return db
	}
	return "youtube_automation"
}

// Preserved original utility function
func sanitizeFilename(topic string) string {
	replacements := []string{" ", "_", "/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_"}

	sanitized := topic
	for i := 0; i < len(replacements); i += 2 {
		sanitized = strings.ReplaceAll(sanitized, replacements[i], replacements[i+1])
	}

	if len(sanitized) > 80 {
		sanitized = sanitized[:80]
	}

	return sanitized
}
