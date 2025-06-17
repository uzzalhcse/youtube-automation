package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

var jobs = make(map[string]*JobStatus)

func main() {
	// Create directories
	directories := []string{"./output", "./temp", "./assets/images", "./assets/audio", "./assets/subtitles"}
	for _, dir := range directories {
		os.MkdirAll(dir, 0755)
	}

	r := mux.NewRouter()

	// API routes
	r.HandleFunc("/api/generate", generateVideoHandler).Methods("POST")
	r.HandleFunc("/api/status/{jobId}", getJobStatusHandler).Methods("GET")
	r.HandleFunc("/api/jobs", listJobsHandler).Methods("GET")
	r.HandleFunc("/api/cancel/{jobId}", cancelJobHandler).Methods("DELETE")

	// File upload endpoints
	r.HandleFunc("/api/upload/image", uploadImageHandler).Methods("POST")
	r.HandleFunc("/api/upload/audio", uploadAudioHandler).Methods("POST")
	r.HandleFunc("/api/upload/subtitle", uploadSubtitleHandler).Methods("POST")

	// Serve generated videos and assets
	r.PathPrefix("/videos/").Handler(http.StripPrefix("/videos/", http.FileServer(http.Dir("./output/"))))
	r.PathPrefix("/assets/").Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets/"))))

	// Health check
	r.HandleFunc("/health", healthCheckHandler).Methods("GET")

	fmt.Println("🎬 Enhanced JSON to Video API Server starting...")
	fmt.Println("📡 Server running on http://localhost:8088")
	fmt.Println("📚 API Endpoints:")
	fmt.Println("   POST /api/generate - Generate video from JSON")
	fmt.Println("   GET  /api/status/{jobId} - Check job status")
	fmt.Println("   GET  /api/jobs - List all jobs")
	fmt.Println("   DELETE /api/cancel/{jobId} - Cancel job")
	fmt.Println("   POST /api/upload/image - Upload image file")
	fmt.Println("   POST /api/upload/audio - Upload audio file")
	fmt.Println("   POST /api/upload/subtitle - Upload SRT file")
	fmt.Println("   GET  /videos/{filename} - Download generated videos")
	fmt.Println("   GET  /assets/{type}/{filename} - Download assets")
	fmt.Println("   GET  /health - Health check")

	log.Fatal(http.ListenAndServe(":8088", r))
}

func generateVideoHandler(w http.ResponseWriter, r *http.Request) {
	var req VideoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON format: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if err := validateVideoRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create job
	jobID := uuid.New().String()
	job := &JobStatus{
		ID:        jobID,
		Status:    "pending",
		Progress:  0,
		CreatedAt: time.Now(),
		Request:   &req,
	}
	jobs[jobID] = job

	// Start video generation in background
	go generateVideo(jobID, &req)

	response := VideoResponse{
		JobID:    jobID,
		Status:   "pending",
		Message:  "Video generation started",
		Progress: 0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getJobStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	job, exists := jobs[jobID]
	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	response := VideoResponse{
		JobID:    job.ID,
		Status:   job.Status,
		Message:  getStatusMessage(job.Status),
		Progress: job.Progress,
	}

	if job.Status == "completed" && job.VideoPath != "" {
		response.VideoURL = fmt.Sprintf("/videos/%s", filepath.Base(job.VideoPath))
	}

	if job.Status == "failed" {
		response.Message = job.Error
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func listJobsHandler(w http.ResponseWriter, r *http.Request) {
	jobList := make([]*JobStatus, 0, len(jobs))
	for _, job := range jobs {
		jobList = append(jobList, job)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobList)
}

func cancelJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobId"]

	job, exists := jobs[jobID]
	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if job.Status == "processing" {
		job.Status = "cancelled"
		job.Error = "Job cancelled by user"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Job cancelled"})
}

func uploadImageHandler(w http.ResponseWriter, r *http.Request) {
	handleFileUpload(w, r, "images", []string{".jpg", ".jpeg", ".png", ".gif", ".bmp"})
}

func uploadAudioHandler(w http.ResponseWriter, r *http.Request) {
	handleFileUpload(w, r, "audio", []string{".mp3", ".wav", ".aac", ".m4a", ".ogg"})
}

func uploadSubtitleHandler(w http.ResponseWriter, r *http.Request) {
	handleFileUpload(w, r, "subtitles", []string{".srt", ".vtt", ".ass"})
}

func handleFileUpload(w http.ResponseWriter, r *http.Request, assetType string, allowedExt []string) {
	r.ParseMultipartForm(10 << 20) // 10 MB limit

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(handler.Filename))
	valid := false
	for _, allowedExt := range allowedExt {
		if ext == allowedExt {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "Invalid file type", http.StatusBadRequest)
		return
	}

	// Generate unique filename
	filename := fmt.Sprintf("%s_%s%s", uuid.New().String()[:8],
		strings.TrimSuffix(handler.Filename, ext), ext)

	filepath := filepath.Join("assets", assetType, filename)

	// Create file
	dst, err := os.Create(filepath)
	if err != nil {
		http.Error(w, "Error creating file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy file content
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"filename": filename,
		"url":      fmt.Sprintf("/assets/%s/%s", assetType, filename),
		"type":     assetType,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	// Check FFmpeg availability
	ffmpegAvailable := true
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		ffmpegAvailable = false
	}

	response := map[string]interface{}{
		"status":           "healthy",
		"timestamp":        time.Now().Format(time.RFC3339),
		"version":          "2.0.0",
		"ffmpeg_available": ffmpegAvailable,
		"active_jobs":      len(jobs),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func validateVideoRequest(req *VideoRequest) error {
	if req.Title == "" {
		return fmt.Errorf("title is required")
	}
	if req.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return fmt.Errorf("width and height must be positive")
	}
	return nil
}

func processAssets(jobID string, req *VideoRequest) error {
	tempDir := filepath.Join("temp", jobID)
	os.MkdirAll(tempDir, 0755)

	// Process images
	for i, img := range req.Images {
		if img.Data != "" {
			// Handle base64 encoded images
			if err := saveBase64Asset(img.Data, filepath.Join(tempDir, fmt.Sprintf("image_%d.png", i))); err != nil {
				return fmt.Errorf("failed to process image %d: %v", i, err)
			}
		}
	}

	// Process audio
	if req.Audio.BackgroundMusic != "" {
		if err := saveBase64Asset(req.Audio.BackgroundMusic, filepath.Join(tempDir, "background.mp3")); err != nil {
			return fmt.Errorf("failed to process background music: %v", err)
		}
	}
	if req.Audio.VoiceOver != "" {
		if err := saveBase64Asset(req.Audio.VoiceOver, filepath.Join(tempDir, "voiceover.mp3")); err != nil {
			return fmt.Errorf("failed to process voice over: %v", err)
		}
	}

	// Process subtitles
	if req.Subtitles.SRTData != "" {
		srtPath := filepath.Join(tempDir, "subtitles.srt")
		if err := os.WriteFile(srtPath, []byte(req.Subtitles.SRTData), 0644); err != nil {
			return fmt.Errorf("failed to process subtitles: %v", err)
		}
	}

	return nil
}

func saveBase64Asset(base64Data, filepath string) error {
	// This is a simplified version - you'd want to properly decode base64
	// and handle different formats in a real implementation
	return os.WriteFile(filepath, []byte(base64Data), 0644)
}

func buildFilterComplexWithCounts1(req *VideoRequest, videoInputCount int, audioInputs []string) string {
	var filters []string

	// Start with background
	currentVideo := "[0:v]"

	// Process images with proper scaling and timing
	videoInputIndex := 1 // Start from 1 (0 is background)

	for _, img := range req.Images {
		if (img.Data != "" || img.URL != "") && img.Duration > 0 && videoInputIndex < videoInputCount {

			// Check if this should be fullscreen (image dimensions match video dimensions)
			isFullscreen := (img.Width == req.Width && img.Height == req.Height)

			var scaleFilter string
			if isFullscreen || (img.X == 0 && img.Y == 0 && img.Width >= req.Width && img.Height >= req.Height) {
				// For fullscreen images, scale to fill entire screen (may crop)
				scaleFilter = fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d[scaled%d]",
					videoInputIndex, req.Width, req.Height, req.Width, req.Height, videoInputIndex)

				// For fullscreen, overlay at 0,0
				overlayFilter := fmt.Sprintf("%s[scaled%d]overlay=0:0:enable='between(t,%d,%d)'[v%d]",
					currentVideo, videoInputIndex, img.StartTime, img.StartTime+img.Duration, videoInputIndex)
				filters = append(filters, scaleFilter)
				filters = append(filters, overlayFilter)
			} else {
				// For non-fullscreen images, scale to specified dimensions
				scaleFilter = fmt.Sprintf("[%d:v]scale=%d:%d[scaled%d]",
					videoInputIndex, img.Width, img.Height, videoInputIndex)

				// Create overlay with specified positioning
				overlayFilter := fmt.Sprintf("%s[scaled%d]overlay=%d:%d:enable='between(t,%d,%d)'[v%d]",
					currentVideo, videoInputIndex, img.X, img.Y, img.StartTime, img.StartTime+img.Duration, videoInputIndex)
				filters = append(filters, scaleFilter)
				filters = append(filters, overlayFilter)
			}

			currentVideo = fmt.Sprintf("[v%d]", videoInputIndex)
			videoInputIndex++
		}
	}

	// Process text overlays for scenes
	sceneIndex := 0
	for _, scene := range req.Scenes {
		if scene.Text == "" {
			continue
		}

		fontSize := scene.FontSize
		if fontSize <= 0 {
			fontSize = 24
		}

		fontColor := scene.FontColor
		if fontColor == "" {
			fontColor = "white"
		}

		x, y := getTextPosition(scene.Position, req.Width, req.Height, scene.X, scene.Y)

		textFilter := fmt.Sprintf("%sdrawtext=text='%s':fontsize=%d:fontcolor=%s:x=%s:y=%s:enable='between(t,%d,%d)'[vt%d]",
			currentVideo,
			strings.ReplaceAll(scene.Text, "'", "\\'"),
			fontSize, fontColor, x, y,
			scene.StartTime, scene.StartTime+scene.Duration, sceneIndex)

		filters = append(filters, textFilter)
		currentVideo = fmt.Sprintf("[vt%d]", sceneIndex)
		sceneIndex++
	}

	// Handle audio mixing
	if len(audioInputs) > 0 {
		if len(audioInputs) == 1 {
			// Single audio input
			audioFilter := fmt.Sprintf("%samix=inputs=1[a]", audioInputs[0])
			filters = append(filters, audioFilter)
		} else {
			// Multiple audio inputs - mix them
			audioFilter := fmt.Sprintf("%samix=inputs=%d[a]", strings.Join(audioInputs, ""), len(audioInputs))
			filters = append(filters, audioFilter)
		}
	}

	// The video output will be handled by addSubtitlesToFilterComplex if subtitles are present
	// Otherwise, we need to set the final video output here
	return strings.Join(filters, ";")
}

func executeFFmpegCommand(args []string, outputPath string) error {
	args = append(args, outputPath)

	fmt.Printf("Executing FFmpeg command: ffmpeg %s\n", strings.Join(args, " "))

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("FFmpeg error: %v, output: %s", err, string(output))
	}

	return nil
}

func getTextPosition(position string, width, height, customX, customY int) (string, string) {
	if customX > 0 && customY > 0 {
		return strconv.Itoa(customX), strconv.Itoa(customY)
	}

	switch strings.ToLower(position) {
	case "top":
		return "(w-text_w)/2", "50"
	case "bottom":
		return "(w-text_w)/2", "h-text_h-50"
	case "center":
		fallthrough
	default:
		return "(w-text_w)/2", "(h-text_h)/2"
	}
}

func getFFmpegColorHex(color string) string {
	// Convert color names to FFmpeg hex format
	switch strings.ToLower(color) {
	case "white":
		return "ffffff"
	case "black":
		return "000000"
	case "red":
		return "ff0000"
	case "green":
		return "00ff00"
	case "blue":
		return "0000ff"
	case "yellow":
		return "ffff00"
	default:
		if strings.HasPrefix(color, "#") && len(color) == 7 {
			return color[1:] // Remove # prefix
		}
		return "ffffff" // Default to white
	}
}

func getStatusMessage(status string) string {
	switch status {
	case "pending":
		return "Video generation is queued"
	case "processing":
		return "Video is being generated"
	case "completed":
		return "Video generation completed successfully"
	case "failed":
		return "Video generation failed"
	case "cancelled":
		return "Video generation was cancelled"
	default:
		return "Unknown status"
	}
}

func sanitizeFilename(filename string) string {
	// Remove invalid characters for filenames
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalid {
		filename = strings.ReplaceAll(filename, char, "_")
	}
	return filename
}

func cleanupTempFiles(jobID string) {
	tempDir := filepath.Join("temp", jobID)
	os.RemoveAll(tempDir)
}
