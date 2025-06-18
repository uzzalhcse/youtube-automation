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

// Global services and database
var (
	templateService        *TemplateService
	geminiService          *GeminiService
	scriptService          *ScriptService
	mongoClient            *mongo.Client
	database               *mongo.Database
	channelsCollection     *mongo.Collection
	scriptsCollection      *mongo.Collection
	scriptChunksCollection *mongo.Collection
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
	http.HandleFunc("/scripts-chunks/", getScriptChunksHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/channels/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/scripts") {
			getChannelScriptsHandler(w, r)
		} else {
			getChannelInfoHandler(w, r)
		}
	})
	// Start server
	port := getPort()
	fmt.Printf("=== Wisderly YouTube Script Generator API ===\n")
	fmt.Printf("Server starting on port %s\n", port)
	fmt.Printf("MongoDB connected: %s\n", getMongoURI())
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST /generate-script           - Generate YouTube script\n")
	fmt.Printf("  GET  /scripts/{id}              - Get script status\n")
	fmt.Printf("  GET  /scripts-chunks/{id}       - Get script chunks\n")
	fmt.Printf("  GET  /channels/{name}/scripts   - Get channel scripts\n")
	fmt.Printf("  GET  /channels/{name}           - Get channel info\n")
	fmt.Printf("  GET  /health                    - Health check\n")
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
	scriptChunksCollection = database.Collection("script_chunks")

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
			Keys: bson.D{{"channel_id", 1}, {"created_at", -1}},
		},
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
	// Index for script_chunks
	_, err = scriptChunksCollection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{"script_id", 1}, {"chunk_index", 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{"script_id", 1}},
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

// Get all scripts for a channel
func getChannelScriptsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	channelName := strings.TrimPrefix(r.URL.Path, "/channels/")
	channelName = strings.TrimSuffix(channelName, "/scripts")

	if channelName == "" {
		respondWithError(w, http.StatusBadRequest, "Channel name is required")
		return
	}

	cursor, err := scriptsCollection.Find(
		context.Background(),
		bson.M{"channel_name": channelName},
		options.Find().SetSort(bson.D{{"created_at", -1}}),
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}
	defer cursor.Close(context.Background())

	var scripts []ScriptGeneration
	if err = cursor.All(context.Background(), &scripts); err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding scripts: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(scripts)
}

// Get channel info with script count
func getChannelInfoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	channelName := strings.TrimPrefix(r.URL.Path, "/channels/")

	if channelName == "" {
		respondWithError(w, http.StatusBadRequest, "Channel name is required")
		return
	}

	var channel Channel
	err := channelsCollection.FindOne(context.Background(), bson.M{"channel_name": channelName}).Decode(&channel)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Channel not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(channel)
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
	var channel Channel
	err := channelsCollection.FindOne(context.Background(), bson.M{"channel_name": strings.TrimSpace(req.ChannelName)}).Decode(&channel)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Create channel if it doesn't exist
			newChannel := Channel{
				ChannelName:  strings.TrimSpace(req.ChannelName),
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
				TotalScripts: 0,
				Settings: ChannelSettings{
					DefaultSectionCount:     defaultSectionCount,
					PreferredVisualGuidance: false,
				},
			}
			result, err := channelsCollection.InsertOne(context.Background(), newChannel)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create channel: %v", err))
				return
			}
			channel.ID = result.InsertedID.(primitive.ObjectID)
			channel.ChannelName = newChannel.ChannelName
		} else {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
			return
		}
	}

	// Create script generation record in MongoDB
	scriptGen := &ScriptGeneration{
		ChannelID:       channel.ID,
		ChannelName:     strings.TrimSpace(req.ChannelName),
		Topic:           strings.TrimSpace(req.Topic),
		Status:          StatusPending,
		GenerateVisuals: req.GenerateVisuals,
		CreatedAt:       time.Now(),
		OutlinePoints:   []OutlinePoint{},
		ImagePrompts:    []ImagePrompt{},
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
	config, err := createScriptConfig(scriptGen, channel.Settings.DefaultSectionCount)
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
	err := scriptService.GenerateCompleteScript(config, scriptID)

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
func getScriptChunksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract script ID from URL path (/scripts-chunks/{scriptID})
	path := strings.TrimPrefix(r.URL.Path, "/scripts-chunks/")
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

	// Find chunks in database
	cursor, err := scriptChunksCollection.Find(
		context.Background(),
		bson.M{"script_id": objectID},
		options.Find().SetSort(bson.D{{"chunk_index", 1}}),
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}
	defer cursor.Close(context.Background())

	var chunks []ScriptChunk
	if err = cursor.All(context.Background(), &chunks); err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding chunks: %v", err))
		return
	}

	// Return response with chunks
	response := map[string]interface{}{
		"script_id":    path,
		"total_chunks": len(chunks),
		"chunks":       chunks,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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

func createScriptConfig(scriptGen *ScriptGeneration, sectionCount int) (*ScriptConfig, error) {

	config := &ScriptConfig{
		Topic:                scriptGen.Topic,
		GenerateVisuals:      scriptGen.GenerateVisuals,
		ChannelName:          scriptGen.ChannelName, // This should be replaced with dynamic channel name
		OutputFolder:         sanitizeFilename(scriptGen.Topic),
		OutputFilename:       fmt.Sprintf("script_%d.txt", time.Now().Unix()),
		MetaTagFilename:      fmt.Sprintf("metatag_%d.txt", time.Now().Unix()),
		SectionCount:         sectionCount,
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
