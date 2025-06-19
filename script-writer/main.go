package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"youtube_automation/elevenlabs"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Global services and database
var (
	templateService        *TemplateService
	geminiService          *GeminiService
	database               *mongo.Database
	channelsCollection     *mongo.Collection
	scriptsCollection      *mongo.Collection
	scriptAudiosCollection *mongo.Collection
	scriptSrtCollection    *mongo.Collection
	chunkVisualsCollection *mongo.Collection
	videoStatusCollection  *mongo.Collection
	templatesCollection    *mongo.Collection
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

type YtAutomation struct {
	mongoClient      *mongo.Client
	templateService  *TemplateService
	geminiService    *GeminiService
	outlineParser    *OutlineParser
	elevenLabsClient *elevenlabs.ElevenLabsClient
	client           *http.Client
	googleHttpClient *HTTPClient
}

func NewYtAutomation(mongoClient *mongo.Client, templateService *TemplateService, geminiService *GeminiService, config HttpConfig) *YtAutomation {
	var rateLimiter *RateLimiter
	if config.RequestsPerMinute > 0 {
		rateLimiter = NewRateLimiter(config.RequestsPerMinute)
	}
	return &YtAutomation{
		mongoClient:     mongoClient,
		templateService: templateService,
		geminiService:   geminiService,
		outlineParser:   NewOutlineParser(),
		elevenLabsClient: elevenlabs.NewElevenLabsClient(os.Getenv("ELEVENLABS_API_KEY"), &elevenlabs.Proxy{
			Server:   os.Getenv("PROXY_SERVER"),
			Username: os.Getenv("PROXY_USERNAME"),
			Password: os.Getenv("PROXY_PASSWORD"),
		}),
		client: &http.Client{Timeout: timeout},
		googleHttpClient: &HTTPClient{
			config: config,
			httpClient: &http.Client{
				Timeout: config.Timeout,
			},
			rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
			rateLimiter: rateLimiter,
		},
	}
}
func main() {
	// Load environment variables
	err := LoadEnvironmentVariables()
	if err != nil {
		// Use fmt.Printf since logger is not set up yet
		fmt.Printf("Failed to load environment variables: %v\n", err)
		os.Exit(1)
	}
	httpConfig := LoadConfigFromEnv()
	// Validate required configuration
	if httpConfig.AuthToken == "" {
		log.Fatal("API_AUTH_TOKEN is required. Please set it in your .env file or environment variables.")
	}
	// Validate tool selection
	if httpConfig.Tool != "whisk" && httpConfig.Tool != "imagefx" {
		log.Fatalf("Invalid tool: %s. Must be 'whisk' or 'imagefx'", httpConfig.Tool)
	}

	fmt.Printf("Using tool: %s\n", httpConfig.Tool)
	// Initialize services (same as original logic)
	templateService = NewTemplateService()
	geminiService = NewGeminiService(os.Getenv("GEMINI_API_KEY"))

	// Initialize MongoDB connection
	mClient, err := initializeMongoDB()
	if err != nil {
		log.Fatalf("Failed to initialize MongoDB: %v", err)
	}

	yt := NewYtAutomation(mClient, templateService, geminiService, httpConfig)

	defer yt.mongoClient.Disconnect(context.Background())

	// Setup HTTP routes
	http.HandleFunc("/generate-script", yt.generateScriptHandler) // step 1
	http.HandleFunc("/scripts/", yt.getScriptStatusHandler)
	http.HandleFunc("/generate-audio/", yt.generateAudioHandler)                     // step 2
	http.HandleFunc("/generate-subtitle/", yt.generateSubtitleHandler)               // step 3
	http.HandleFunc("/generate-visual-prompt/", yt.generateVisualPromptHandler)      // step 4
	http.HandleFunc("/generate-visual-images/", yt.generateVisualImagePromptHandler) // step 5
	http.HandleFunc("/generate-video/", yt.generateVideoHandler)                     // step 6
	http.HandleFunc("/scripts-chunks/", yt.getScriptAudiosHandler)
	http.HandleFunc("/health", yt.healthHandler)
	http.HandleFunc("/channels/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/scripts") {
			yt.getChannelScriptsHandler(w, r)
		} else {
			yt.getChannelInfoHandler(w, r)
		}
	})
	http.HandleFunc("/templates", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			yt.getTemplatesHandler(w, r)
		case "POST":
			yt.createTemplateHandler(w, r)
		default:
			respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	})
	http.HandleFunc("/templates/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PUT":
			yt.updateTemplateHandler(w, r)
		case "DELETE":
			yt.deleteTemplateHandler(w, r)
		default:
			respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
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
	fmt.Printf("  GET/POST /templates              - Manage templates\n")
	fmt.Printf("  PUT/DELETE /templates/{id}       - Update/Delete template\n")
	fmt.Println(strings.Repeat("=", 50))

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
func LoadEnvironmentVariables() error {
	err := godotenv.Load()
	if err != nil {
		return fmt.Errorf("error loading .env file: %w", err)
	}

	slog.Debug("Environment variables loaded successfully")
	return nil
}

func initializeMongoDB() (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to MongoDB
	mongoURI := getMongoURI()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Test connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	// Initialize global variables
	database = client.Database(getMongoDB())
	channelsCollection = database.Collection("channels")
	scriptsCollection = database.Collection("scripts")
	scriptAudiosCollection = database.Collection("script_audios")
	scriptSrtCollection = database.Collection("script_srt")
	chunkVisualsCollection = database.Collection("chunk_visuals")
	videoStatusCollection = database.Collection("script_videos")
	templatesCollection = database.Collection("templates")

	// Create indexes
	if err := createIndexes(); err != nil {
		return nil, fmt.Errorf("failed to create indexes: %v", err)
	}
	// Add template indexes in createIndexes
	if err := createTemplateIndexes(); err != nil {
		return nil, fmt.Errorf("failed to create template indexes: %v", err)
	}

	// Initialize default templates if none exist
	if err := initializeDefaultTemplates(); err != nil {
		return nil, fmt.Errorf("failed to initialize default templates: %v", err)
	}
	fmt.Println("✓ MongoDB connected successfully")
	return client, nil
}
func createTemplateIndexes() error {
	ctx := context.Background()

	_, err := templatesCollection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{"name", 1}, {"type", 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{"type", 1}, {"is_active", 1}},
		},
	})
	return err
}
func initializeDefaultTemplates() error {
	ctx := context.Background()

	// Check if templates exist
	count, err := templatesCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}

	if count > 0 {
		fmt.Println("✓ Templates already exist in database")
		return nil
	}

	// Default templates content
	defaultTemplates := []Template{
		{
			Name:        "default_outline",
			Type:        "outline",
			Content:     `Create a comprehensive outline for a YouTube video about [TOPIC]. Structure it with clear, engaging bullet points that will guide script writing.`,
			Description: "Default outline template for YouTube videos",
			IsActive:    true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Version:     1,
		},
		{
			Name:        "default_script",
			Type:        "script",
			Content:     `Write a compelling YouTube script section based on the provided outline point. Make it engaging and informative for the target audience.`,
			Description: "Default script generation template",
			IsActive:    true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Version:     1,
		},
		{
			Name:        "default_hook_intro",
			Type:        "hook_intro",
			Content:     `Create an engaging hook and introduction for a YouTube video. Start with a compelling question or statement that grabs attention immediately.`,
			Description: "Default hook and introduction template",
			IsActive:    true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Version:     1,
		},
		{
			Name:        "default_meta_tag",
			Type:        "meta_tag",
			Content:     `Generate SEO-friendly description, tags, and thumbnail statement for the YouTube video based on the content.`,
			Description: "Default meta tag and description template",
			IsActive:    true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Version:     1,
		},
		{
			Name:        "default_visual_guidance",
			Type:        "visual_guidance",
			Content:     `Create visual guidance descriptions for video editing based on the script content. Focus on senior-friendly aesthetics.`,
			Description: "Default visual guidance template",
			IsActive:    true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Version:     1,
		},
	}

	var docs []interface{}
	for _, template := range defaultTemplates {
		docs = append(docs, template)
	}

	_, err = templatesCollection.InsertMany(ctx, docs)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Initialized %d default templates\n", len(defaultTemplates))
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
	// Index for script_audios
	_, err = scriptAudiosCollection.Indexes().CreateMany(ctx, []mongo.IndexModel{
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
	// Index for script_srt
	_, err = scriptSrtCollection.Indexes().CreateMany(ctx, []mongo.IndexModel{
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

// Get all scripts for a channel
func (yt *YtAutomation) getChannelScriptsHandler(w http.ResponseWriter, r *http.Request) {
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

	var scripts []Script
	if err = cursor.All(context.Background(), &scripts); err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding scripts: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(scripts)
}

// Get channel info with script count
func (yt *YtAutomation) getChannelInfoHandler(w http.ResponseWriter, r *http.Request) {
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

func (yt *YtAutomation) generateScriptHandler(w http.ResponseWriter, r *http.Request) {
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
					WordLimitForHookIntro:   200,
					VisualImageMultiplier:   visualImageMultiplier,
					WordLimitPerSection:     500,
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
	scriptGen := &Script{
		ChannelID:       channel.ID,
		ChannelName:     strings.TrimSpace(req.ChannelName),
		Topic:           strings.TrimSpace(req.Topic),
		Status:          StatusPending,
		GenerateVisuals: req.GenerateVisuals,
		CreatedAt:       time.Now(),
		OutlinePoints:   []OutlinePoint{},
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
	config, err := createScriptConfig(scriptGen, channel)
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
		yt.processScriptGeneration(scriptID, config)
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

func (yt *YtAutomation) processScriptGeneration(scriptID primitive.ObjectID, config *ScriptConfig) {
	startTime := time.Now()

	// Generate script (same logic as original)
	err := yt.GenerateCompleteScript(config, scriptID)

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
		"sections_generated":      config.channel.Settings.DefaultSectionCount,
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
	updateChannelStats(config.channel.ChannelName, true)

	log.Printf("✅ Script generation completed for ID: %s | Time: %.2fs",
		scriptID.Hex(), processingTime)
}

func (yt *YtAutomation) getScriptStatusHandler(w http.ResponseWriter, r *http.Request) {
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
	var script Script
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
func (yt *YtAutomation) getScriptAudiosHandler(w http.ResponseWriter, r *http.Request) {
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
	cursor, err := scriptAudiosCollection.Find(
		context.Background(),
		bson.M{"script_id": objectID},
		options.Find().SetSort(bson.D{{"chunk_index", 1}}),
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}
	defer cursor.Close(context.Background())

	var chunks []ScriptAudio
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
func (yt *YtAutomation) generateAudioHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract script ID from URL path (/generate-audio/{scriptID})
	path := strings.TrimPrefix(r.URL.Path, "/generate-audio/")
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

	var script Script
	err = scriptsCollection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&script)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	// Check if chunks already exist for this script
	existingCount, err := scriptAudiosCollection.CountDocuments(
		context.Background(),
		bson.M{"script_id": objectID},
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error checking existing chunks: %v", err))
		return
	}
	if err := os.MkdirAll(filepath.Join("assets", "audio"), 0755); err != nil {
		log.Printf("Warning: Failed to create audio directory: %v", err)
	}
	var savedChunks []ScriptAudio
	chunks := splitTextByCharLimit(script.FullScript, splitVoiceByCharLimit)

	if existingCount > 0 {
		// Chunks already exist, fetch them instead of creating new ones
		fmt.Printf("ℹ Script chunks already exist for script %s, fetching existing chunks\n", path)

		findOptions := options.Find().SetSort(bson.M{"chunk_index": 1})
		cursor, err := scriptAudiosCollection.Find(
			context.Background(),
			bson.M{"script_id": objectID},
			findOptions,
		)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error fetching existing chunks: %v", err))
			return
		}
		defer cursor.Close(context.Background())

		if err = cursor.All(context.Background(), &savedChunks); err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding existing chunks: %v", err))
			return
		}
	} else {
		// Create new chunks
		var chunkDocs []interface{}

		for i, chunk := range chunks {
			chunkDoc := ScriptAudio{
				ScriptID:   script.ID,
				ChunkIndex: i + 1,
				Content:    chunk,
				CharCount:  len(chunk),
				HasVisual:  false,
				CreatedAt:  time.Now(),
			}
			chunkDocs = append(chunkDocs, chunkDoc)
		}

		// Insert all chunks in batch
		if len(chunkDocs) > 0 {
			result, err := scriptAudiosCollection.InsertMany(context.Background(), chunkDocs)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save script chunks: %v", err))
				return // Added return here to prevent continuing after error
			}

			// Prepare saved chunks with IDs for visual generation
			for i, insertedID := range result.InsertedIDs {
				chunk := chunkDocs[i].(ScriptAudio)
				chunk.ID = insertedID.(primitive.ObjectID)
				savedChunks = append(savedChunks, chunk)
			}

			fmt.Printf("✓ Saved %d script chunks to database\n", len(chunkDocs))
		}
	}

	// Generate voice over using the current chunks (whether new or existing)
	if err := yt.generateVoiceOver(script, savedChunks); err != nil {
		fmt.Printf("Warning: Failed to generate audio for chunks: %v\n", err)
	}

	// Return response with chunks
	data := map[string]interface{}{
		"script_id":    path,
		"total_chunks": len(chunks),
	}

	// Remove the duplicate WriteHeader call - json.NewEncoder will call it
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Voice generation completed",
		"data":    data,
	})
}
func (yt *YtAutomation) generateSubtitleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract script ID from URL path (/generate-subtitle/{scriptID})
	path := strings.TrimPrefix(r.URL.Path, "/generate-subtitle/")
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

	var script Script
	err = scriptsCollection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&script)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	srt, err := yt.GenerateSRT(TranscriptPayload{
		AudioPath: script.FullAudioFile,
		Language:  "en",
		OutputSrt: true,
	})
	if err != nil {
		fmt.Printf("Warning: Failed to generate audio for chunks: %v\n", err)
	}
	_, err = scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": script.ID},
		bson.M{"$set": bson.M{"srt": srt}},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to save chunk audio file path: %v\n", err)
	}

	// Split the script into chunks
	chunks, err := splitSRTByDuration(srt, 1*time.Minute)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to split SRT into chunks: %v", err))
		return
	}
	// Prepare chunk documents for batch insert
	var chunkDocs []interface{}
	var savedChunks []ScriptSrt

	for i, chunk := range chunks {
		chunkDoc := ScriptSrt{
			ScriptID:   script.ID,
			ChunkIndex: i + 1,
			Content:    chunk,
			CharCount:  len(chunk),
			CreatedAt:  time.Now(),
		}
		chunkDocs = append(chunkDocs, chunkDoc)
	}

	// Insert all chunks in batch
	if len(chunkDocs) > 0 {
		result, err := scriptSrtCollection.InsertMany(context.Background(), chunkDocs)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save script chunks: %w", err))
		}

		// Prepare saved chunks with IDs for visual generation
		for i, insertedID := range result.InsertedIDs {
			chunk := chunkDocs[i].(ScriptSrt)
			chunk.ID = insertedID.(primitive.ObjectID)
			savedChunks = append(savedChunks, chunk)
		}

		fmt.Printf("✓ Saved %d script srt chunks to database\n", len(chunkDocs))
	}

	// Return response with chunks
	data := map[string]interface{}{
		"script_id":    path,
		"total_chunks": len(chunks),
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Subtitle generation completed",
		"data":    data,
	})
}
func (yt *YtAutomation) generateVisualPromptHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract script ID from URL path (/generate-visual/{scriptID})
	path := strings.TrimPrefix(r.URL.Path, "/generate-visual-prompt/")
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
	var script Script
	err = scriptsCollection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&script)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	// Check if chunks already exist for this script
	existingCount, err := scriptAudiosCollection.CountDocuments(
		context.Background(),
		bson.M{"script_id": objectID},
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error checking existing chunks: %v", err))
		return
	}

	var scriptSrtChunks []ScriptSrt

	if existingCount > 0 {

		findOptions := options.Find().SetSort(bson.M{"chunk_index": 1})
		cursor, err := scriptSrtCollection.Find(
			context.Background(),
			bson.M{"script_id": objectID},
			findOptions,
		)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error fetching existing chunks: %v", err))
			return
		}
		defer cursor.Close(context.Background())

		if err = cursor.All(context.Background(), &scriptSrtChunks); err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding existing chunks: %v", err))
			return
		}
	}
	go func() {
		if err := yt.generateVisualPromptForChunks(objectID, scriptSrtChunks); err != nil {
			fmt.Printf("Warning: Failed to generate visuals for chunks: %v\n", err)
		}
	}()

	// TODO: Implement visual generation logic, Generate visual from raw script or srt. yt.generateVisualPromptForChunks()
	// Return response with chunks
	data := map[string]interface{}{
		"script_id": path,
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Visual Prompt generation InProgress",
		"data":    data,
	})
}
func (yt *YtAutomation) generateVisualImagePromptHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract script ID from URL path (/generate-visual/{scriptID})
	path := strings.TrimPrefix(r.URL.Path, "/generate-visual-images/")
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
	var script Script
	err = scriptsCollection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&script)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	// Check if chunks already exist for this script
	existingCount, err := chunkVisualsCollection.CountDocuments(
		context.Background(),
		bson.M{"script_id": objectID},
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error checking existing chunks: %v", err))
		return
	}

	var chunkVisuals []ChunkVisual

	if existingCount > 0 {

		findOptions := options.Find().SetSort(bson.M{"chunk_index": 1})
		cursor, err := chunkVisualsCollection.Find(
			context.Background(),
			bson.M{"script_id": objectID},
			findOptions,
		)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error fetching existing chunks: %v", err))
			return
		}
		defer cursor.Close(context.Background())

		if err = cursor.All(context.Background(), &chunkVisuals); err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding existing chunks: %v", err))
			return
		}
	}
	go func() {
		if err := yt.generateVisualImagePromptForChunks(objectID, chunkVisuals); err != nil {
			fmt.Printf("Warning: Failed to generate visuals for chunks: %v\n", err)
		}
	}()
	// Return response with chunks
	data := map[string]interface{}{
		"script_id": path,
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Visual generation InProgress",
		"data":    data,
	})
}

func (yt *YtAutomation) generateVideoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		respondWithError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	// Extract script ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/generate-video/")
	if path == "" {
		respondWithError(w, http.StatusBadRequest, "Script ID is required")
		return
	}

	// Validate ObjectID format
	scriptID, err := primitive.ObjectIDFromHex(path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid script ID format")
		return
	}

	// Fetch script from database
	script, err := yt.getScript(scriptID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	// Validate script status
	if script.Status != "completed" {
		respondWithError(w, http.StatusBadRequest, "Script must be completed before generating video")
		return
	}

	// Check for existing video generation in progress
	existingStatus, err := yt.getVideoGenerationStatus(scriptID)
	if err == nil && (existingStatus.Status == "pending" || existingStatus.Status == "processing") {
		respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"message":    "Video generation already in progress",
			"status":     existingStatus.Status,
			"process_id": existingStatus.ProcessID,
			"progress":   existingStatus.Progress,
		})
		return
	}

	// Fetch chunk visuals
	chunkVisuals, err := yt.getChunkVisuals(scriptID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error fetching chunk visuals: %v", err))
		return
	}

	if len(chunkVisuals) == 0 {
		respondWithError(w, http.StatusBadRequest, "No visual chunks found for this script")
		return
	}

	// Build video request payload
	videoRequest, err := yt.buildVideoRequest(script, chunkVisuals)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error building video request: %v", err))
		return
	}

	// Create video generation status record
	status := VideoGenerationStatus{
		ScriptID:    scriptID,
		Status:      "pending",
		RequestData: *videoRequest,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	statusID, err := yt.createVideoGenerationStatus(&status)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating status record: %v", err))
		return
	}

	// Start video generation asynchronously
	go func() {
		err := yt.generateVideoAsync(statusID, videoRequest)
		if err != nil {
			fmt.Printf("Error in async video generation: %v\n", err)
			yt.updateVideoGenerationStatus(statusID, VideoGenerationStatus{
				Status:    "failed",
				ErrorMsg:  err.Error(),
				UpdatedAt: time.Now(),
			})
		}
	}()

	// Return immediate response
	respondWithJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":   "Video generation started successfully",
		"status":    "pending",
		"script_id": scriptID.Hex(),
		"status_id": statusID.Hex(),
		"check_url": fmt.Sprintf("/video-status/%s", statusID.Hex()),
	})
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
				WordLimitForHookIntro:   200,
				VisualImageMultiplier:   visualImageMultiplier,
				WordLimitPerSection:     500,
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

func (yt *YtAutomation) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Test MongoDB connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mongoStatus := "healthy"
	if err := yt.mongoClient.Ping(ctx, nil); err != nil {
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

func createScriptConfig(scriptGen *Script, channel Channel) (*ScriptConfig, error) {

	config := &ScriptConfig{
		Topic:                scriptGen.Topic,
		GenerateVisuals:      scriptGen.GenerateVisuals,
		OutputFolder:         sanitizeFilename(scriptGen.Topic),
		OutputFilename:       fmt.Sprintf("script_%d.txt", time.Now().Unix()),
		MetaTagFilename:      fmt.Sprintf("metatag_%d.txt", time.Now().Unix()),
		channel:              channel,
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
