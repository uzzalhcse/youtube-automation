package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Enhanced VideoRequest with validation and better structure
type VideoRequest struct {
	Title      string         `json:"title" validate:"required,min=1,max=200"`
	Duration   int            `json:"duration" validate:"required,min=1,max=3600"`
	Width      int            `json:"width" validate:"required,min=480,max=4096"`
	Height     int            `json:"height" validate:"required,min=360,max=4096"`
	Background string         `json:"background" validate:"required"`
	Images     []ImageAsset   `json:"images" validate:"required,min=1"`
	Audio      AudioConfig    `json:"audio"`
	Subtitles  SubtitleConfig `json:"subtitles"`
	Scenes     []Scene        `json:"scenes"`
}

// VideoResponse represents the API response
type VideoResponse struct {
	Success   bool   `json:"success"`
	VideoID   string `json:"video_id,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
	ProcessID string `json:"process_id,omitempty"`
}

// VideoGenerationStatus tracks the video generation process
type VideoGenerationStatus struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ScriptID    primitive.ObjectID `bson:"script_id" json:"script_id"`
	VideoID     string             `bson:"video_id,omitempty" json:"video_id,omitempty"`
	ProcessID   string             `bson:"process_id,omitempty" json:"process_id,omitempty"`
	Status      string             `bson:"status" json:"status"` // "pending", "processing", "completed", "failed"
	VideoURL    string             `bson:"video_url,omitempty" json:"video_url,omitempty"`
	Progress    int                `bson:"progress" json:"progress"` // 0-100
	ErrorMsg    string             `bson:"error_msg,omitempty" json:"error_msg,omitempty"`
	RequestData VideoRequest       `bson:"request_data" json:"request_data"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
	CompletedAt *time.Time         `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
}

// Helper function to get script
func (yt *YtAutomation) getScript(scriptID primitive.ObjectID) (*Script, error) {
	var script Script
	err := scriptsCollection.FindOne(context.Background(), bson.M{"_id": scriptID}).Decode(&script)
	return &script, err
}

// Helper function to get chunk visuals
func (yt *YtAutomation) getChunkVisuals(scriptID primitive.ObjectID) ([]ChunkVisual, error) {
	var chunkVisuals []ChunkVisual

	findOptions := options.Find().SetSort(bson.M{"chunk_index": 1})
	cursor, err := chunkVisualsCollection.Find(
		context.Background(),
		bson.M{"script_id": scriptID},
		findOptions,
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	err = cursor.All(context.Background(), &chunkVisuals)
	return chunkVisuals, err
}

// Build video request from script and chunk visuals
func (yt *YtAutomation) buildVideoRequest(script *Script, chunkVisuals []ChunkVisual) (*VideoRequest, error) {
	// Calculate total duration from SRT if available
	duration := 30 // default duration
	if script.SRT != "" {
		calculatedDuration, err := yt.calculateDurationFromSRT(script.SRT)
		if err == nil && calculatedDuration > 0 {
			duration = calculatedDuration
		}
	}

	videoRequest := &VideoRequest{
		Title:      fmt.Sprintf("%s - %s", script.ChannelName, script.Topic),
		Duration:   duration,
		Width:      1920,
		Height:     1080,
		Background: "#1a1a1a",
		Images:     make([]ImageAsset, 0, len(chunkVisuals)),
		Audio: AudioConfig{
			VoiceOverURL: script.FullAudioFile,
			VoiceVolume:  1.0,
			Volume:       0.3, // Background music volume
		},
		Subtitles: SubtitleConfig{
			SRTData:    script.SRT,
			FontSize:   24,
			FontColor:  "white",
			Position:   "bottom",
			Background: "rgba(0,0,0,0.7)",
			Outline:    true,
		},
		Scenes: make([]Scene, 0),
	}

	// Process chunk visuals to create image assets
	currentTime := 0
	for i, chunk := range chunkVisuals {
		if chunk.ImagePath == "" {
			continue
		}

		// Calculate duration for this chunk (you may want to parse from SRT)
		chunkDuration := 10 // default 10 seconds per chunk
		if i < len(chunkVisuals)-1 {
			// You can implement better duration calculation here
			chunkDuration = yt.calculateChunkDuration(chunk, script.SRT)
		} else {
			// Last chunk takes remaining time
			chunkDuration = duration - currentTime
		}

		if chunkDuration <= 0 {
			chunkDuration = 5 // minimum duration
		}

		imageAsset := ImageAsset{
			ID:        fmt.Sprintf("chunk_%d_%s", i, chunk.ID.Hex()),
			URL:       chunk.ImagePath,
			StartTime: currentTime,
			Duration:  chunkDuration,
			X:         0,
			Y:         0,
			Width:     1920,
			Height:    1080,
			ZIndex:    1,
			Opacity:   1.0,
			Effect:    "fade",
			KenBurns: KenBurnsConfig{
				Enabled:    false,
				ZoomRate:   0.0002,
				ZoomStart:  1.0,
				ZoomEnd:    1.1,
				PanX:       "iw/2-(iw/zoom/2)",
				PanY:       "ih/2-(ih/zoom/2)",
				ScaleWidth: 8000,
				Direction:  "zoom_in",
			},
		}

		videoRequest.Images = append(videoRequest.Images, imageAsset)
		currentTime += chunkDuration
	}

	return videoRequest, nil
}

// Enhanced video generation with proper error handling and status updates
func (yt *YtAutomation) generateVideoAsync(statusID primitive.ObjectID, videoRequest *VideoRequest) error {
	// Update status to processing
	err := yt.updateVideoGenerationStatus(statusID, VideoGenerationStatus{
		Status:    "processing",
		Progress:  10,
		UpdatedAt: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Validate video request
	if err := yt.validateVideoRequest(videoRequest); err != nil {
		return fmt.Errorf("invalid video request: %w", err)
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(videoRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Update progress
	yt.updateVideoGenerationStatus(statusID, VideoGenerationStatus{
		Status:    "processing",
		Progress:  30,
		UpdatedAt: time.Now(),
	})

	// Make API call
	apiURL := os.Getenv("VIDEO_SERVER_API_URL")
	if apiURL == "" {
		return fmt.Errorf("VIDEO_SERVER_API_URL environment variable not set")
	}

	url := fmt.Sprintf("%s/generate", strings.TrimSuffix(apiURL, "/"))

	// Create request with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", getEnvWithDefault("USER_AGENT", "YT-Automation/1.0"))

	// Add API key if available
	if apiKey := os.Getenv("VIDEO_API_KEY"); apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	// Update progress
	yt.updateVideoGenerationStatus(statusID, VideoGenerationStatus{
		Status:    "processing",
		Progress:  50,
		UpdatedAt: time.Now(),
	})

	// Send request with retry logic
	var resp *http.Response
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		resp, err = yt.client.Do(req)
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to send request after %d retries: %w", maxRetries, err)
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Update progress
	yt.updateVideoGenerationStatus(statusID, VideoGenerationStatus{
		Status:    "processing",
		Progress:  80,
		UpdatedAt: time.Now(),
	})

	// Parse response
	var videoResponse VideoResponse
	if err := json.Unmarshal(responseBody, &videoResponse); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Handle API response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		errorMsg := fmt.Sprintf("API request failed with status %d", resp.StatusCode)
		if videoResponse.Error != "" {
			errorMsg += fmt.Sprintf(": %s", videoResponse.Error)
		} else if videoResponse.Message != "" {
			errorMsg += fmt.Sprintf(": %s", videoResponse.Message)
		} else {
			errorMsg += fmt.Sprintf(": %s", string(responseBody))
		}
		return fmt.Errorf(errorMsg)
	}

	// Update final status
	now := time.Now()
	finalStatus := VideoGenerationStatus{
		Status:      "completed",
		Progress:    100,
		VideoID:     videoResponse.VideoID,
		VideoURL:    videoResponse.VideoURL,
		ProcessID:   videoResponse.ProcessID,
		UpdatedAt:   now,
		CompletedAt: &now,
	}

	if !videoResponse.Success {
		finalStatus.Status = "failed"
		finalStatus.ErrorMsg = videoResponse.Message
		if videoResponse.Error != "" {
			finalStatus.ErrorMsg = videoResponse.Error
		}
	}

	return yt.updateVideoGenerationStatus(statusID, finalStatus)
}

// Video status endpoint
func (yt *YtAutomation) videoStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Extract status ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/video-status/")
	if path == "" {
		respondWithError(w, http.StatusBadRequest, "Status ID is required")
		return
	}

	statusID, err := primitive.ObjectIDFromHex(path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid status ID format")
		return
	}

	status, err := yt.getVideoGenerationStatusByID(statusID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Status not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	respondWithJSON(w, http.StatusOK, status)
}

// Database operations for video generation status
func (yt *YtAutomation) createVideoGenerationStatus(status *VideoGenerationStatus) (primitive.ObjectID, error) {
	result, err := videoStatusCollection.InsertOne(context.Background(), status)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return result.InsertedID.(primitive.ObjectID), nil
}

func (yt *YtAutomation) getVideoGenerationStatus(scriptID primitive.ObjectID) (*VideoGenerationStatus, error) {
	var status VideoGenerationStatus
	opts := options.FindOne().SetSort(bson.M{"created_at": -1})
	err := videoStatusCollection.FindOne(
		context.Background(),
		bson.M{"script_id": scriptID},
		opts,
	).Decode(&status)
	return &status, err
}

func (yt *YtAutomation) getVideoGenerationStatusByID(statusID primitive.ObjectID) (*VideoGenerationStatus, error) {
	var status VideoGenerationStatus
	err := videoStatusCollection.FindOne(
		context.Background(),
		bson.M{"_id": statusID},
	).Decode(&status)
	return &status, err
}

func (yt *YtAutomation) updateVideoGenerationStatus(statusID primitive.ObjectID, update VideoGenerationStatus) error {
	updateDoc := bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	if update.Status != "" {
		updateDoc["$set"].(bson.M)["status"] = update.Status
	}
	if update.Progress > 0 {
		updateDoc["$set"].(bson.M)["progress"] = update.Progress
	}
	if update.VideoID != "" {
		updateDoc["$set"].(bson.M)["video_id"] = update.VideoID
	}
	if update.VideoURL != "" {
		updateDoc["$set"].(bson.M)["video_url"] = update.VideoURL
	}
	if update.ProcessID != "" {
		updateDoc["$set"].(bson.M)["process_id"] = update.ProcessID
	}
	if update.ErrorMsg != "" {
		updateDoc["$set"].(bson.M)["error_msg"] = update.ErrorMsg
	}
	if update.CompletedAt != nil {
		updateDoc["$set"].(bson.M)["completed_at"] = update.CompletedAt
	}

	_, err := videoStatusCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": statusID},
		updateDoc,
	)
	return err
}

// Utility functions
func (yt *YtAutomation) validateVideoRequest(req *VideoRequest) error {
	if req.Title == "" {
		return fmt.Errorf("title is required")
	}
	if req.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return fmt.Errorf("width and height must be positive")
	}
	if len(req.Images) == 0 {
		return fmt.Errorf("at least one image is required")
	}
	return nil
}

func (yt *YtAutomation) calculateDurationFromSRT(srtContent string) (int, error) {
	// Simple SRT parsing to get the last timestamp
	lines := strings.Split(srtContent, "\n")
	var lastEndTime string

	for _, line := range lines {
		if strings.Contains(line, " --> ") {
			parts := strings.Split(line, " --> ")
			if len(parts) == 2 {
				lastEndTime = strings.TrimSpace(parts[1])
			}
		}
	}

	if lastEndTime == "" {
		return 0, fmt.Errorf("no timestamps found in SRT")
	}

	// Convert timestamp to seconds (format: HH:MM:SS,mmm)
	return yt.parseTimestamp(lastEndTime)
}

func (yt *YtAutomation) parseTimestamp(timestamp string) (int, error) {
	// Remove milliseconds
	if strings.Contains(timestamp, ",") {
		timestamp = strings.Split(timestamp, ",")[0]
	}

	parts := strings.Split(timestamp, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid timestamp format")
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])

	return hours*3600 + minutes*60 + seconds, nil
}

func (yt *YtAutomation) calculateChunkDuration(chunk ChunkVisual, srtContent string) int {
	// Parse start and end times from chunk or SRT
	if chunk.StartTime != "" && chunk.EndTime != "" {
		startSeconds, err1 := yt.parseTimestamp(chunk.StartTime)
		endSeconds, err2 := yt.parseTimestamp(chunk.EndTime)
		if err1 == nil && err2 == nil {
			return endSeconds - startSeconds
		}
	}
	return 10 // default duration
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
