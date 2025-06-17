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

// VideoRequest represents the JSON structure for video generation
type VideoRequest struct {
	Title      string         `json:"title"`
	Duration   int            `json:"duration"` // in seconds
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	Background string         `json:"background"` // color or base64 image
	Images     []ImageAsset   `json:"images"`
	Audio      AudioConfig    `json:"audio"`
	Subtitles  SubtitleConfig `json:"subtitles"`
	Scenes     []Scene        `json:"scenes"`
}

type ImageAsset struct {
	ID        string  `json:"id"`
	Data      string  `json:"data"`       // base64 encoded image
	URL       string  `json:"url"`        // or image URL
	StartTime int     `json:"start_time"` // when to show (seconds)
	Duration  int     `json:"duration"`   // how long to show (seconds)
	X         int     `json:"x"`          // position x
	Y         int     `json:"y"`          // position y
	Width     int     `json:"width"`      // scaled width
	Height    int     `json:"height"`     // scaled height
	ZIndex    int     `json:"z_index"`    // layer order
	Opacity   float64 `json:"opacity"`    // 0.0 to 1.0
	Effect    string  `json:"effect"`     // "fade", "slide", "zoom", "none"
}

type AudioConfig struct {
	BackgroundMusic string  `json:"background_music"` // base64 encoded audio
	BackgroundURL   string  `json:"background_url"`   // or audio URL
	Volume          float64 `json:"volume"`
	FadeIn          int     `json:"fade_in"`        // fade in duration (seconds)
	FadeOut         int     `json:"fade_out"`       // fade out duration (seconds)
	VoiceOver       string  `json:"voice_over"`     // base64 encoded voice
	VoiceOverURL    string  `json:"voice_over_url"` // or voice URL
	VoiceVolume     float64 `json:"voice_volume"`
}

type SubtitleConfig struct {
	SRTData    string `json:"srt_data"` // SRT content as string
	SRTURL     string `json:"srt_url"`  // or SRT file URL
	FontSize   int    `json:"font_size"`
	FontColor  string `json:"font_color"`
	Position   string `json:"position"`   // "bottom", "top", "center"
	Background string `json:"background"` // subtitle background color
	Outline    bool   `json:"outline"`    // text outline
}

type Scene struct {
	StartTime  int    `json:"start_time"` // in seconds
	Duration   int    `json:"duration"`   // in seconds
	Text       string `json:"text"`
	FontSize   int    `json:"font_size"`
	FontColor  string `json:"font_color"`
	Position   string `json:"position"`   // "center", "top", "bottom"
	X          int    `json:"x"`          // custom x position
	Y          int    `json:"y"`          // custom y position
	Effect     string `json:"effect"`     // "fade", "slide", "typewriter", "none"
	Background string `json:"background"` // text background
	Outline    bool   `json:"outline"`    // text outline
	Animation  string `json:"animation"`  // "bounce", "shake", "pulse"
}

// VideoResponse represents the API response
type VideoResponse struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	VideoURL string `json:"video_url,omitempty"`
	Progress int    `json:"progress,omitempty"` // 0-100
}

// JobStatus tracks video generation jobs
type JobStatus struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"` // "pending", "processing", "completed", "failed"
	Progress  int           `json:"progress"`
	CreatedAt time.Time     `json:"created_at"`
	VideoPath string        `json:"video_path"`
	Error     string        `json:"error,omitempty"`
	Request   *VideoRequest `json:"request,omitempty"`
}

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

func generateVideo(jobID string, req *VideoRequest) {
	job := jobs[jobID]
	job.Status = "processing"
	job.Progress = 10

	defer func() {
		if r := recover(); r != nil {
			job.Status = "failed"
			job.Error = fmt.Sprintf("Panic: %v", r)
		}
	}()

	// Process assets
	if err := processAssets(jobID, req); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		return
	}
	job.Progress = 30

	// Generate FFmpeg command
	ffmpegArgs, err := buildFFmpegCommand(jobID, req)
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		return
	}
	job.Progress = 50

	// Execute FFmpeg
	outputPath := filepath.Join("output", fmt.Sprintf("%s_%s.mp4",
		sanitizeFilename(req.Title), jobID[:8]))

	if err := executeFFmpegCommand(ffmpegArgs, outputPath); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		return
	}
	job.Progress = 90

	// Cleanup temp files
	cleanupTempFiles(jobID)

	job.Status = "completed"
	job.Progress = 100
	job.VideoPath = outputPath
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

func buildFFmpegCommand(jobID string, req *VideoRequest) ([]string, error) {
	tempDir := filepath.Join("temp", jobID)

	args := []string{}

	// Input sources
	inputCount := 0

	// Background color/image
	if req.Background != "" && req.Background[0] == '#' {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("color=%s:size=%dx%d:duration=%d:rate=25",
				req.Background, req.Width, req.Height, req.Duration))
		inputCount++
	}

	// Add image inputs
	for i, img := range req.Images {
		if img.Data != "" {
			args = append(args, "-i", filepath.Join(tempDir, fmt.Sprintf("image_%d.png", i)))
			inputCount++
		} else if img.URL != "" {
			args = append(args, "-i", img.URL)
			inputCount++
		}
	}

	// Add audio inputs
	if req.Audio.BackgroundMusic != "" {
		args = append(args, "-i", filepath.Join(tempDir, "background.mp3"))
		inputCount++
	}
	if req.Audio.VoiceOver != "" {
		args = append(args, "-i", filepath.Join(tempDir, "voiceover.mp3"))
		inputCount++
	}

	// Build filter complex
	filterComplex := buildFilterComplex(req, inputCount)
	if filterComplex != "" {
		args = append(args, "-filter_complex", filterComplex)
	}

	// Add subtitle filter if needed
	if req.Subtitles.SRTData != "" {
		srtPath := filepath.Join(tempDir, "subtitles.srt")
		args = append(args, "-vf", fmt.Sprintf("subtitles=%s", srtPath))
	}

	// Output settings
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p")
	args = append(args, "-c:a", "aac", "-b:a", "128k")
	args = append(args, "-shortest", "-y")

	return args, nil
}

func buildFilterComplex(req *VideoRequest, inputCount int) string {
	var filters []string

	// Add text overlays for scenes
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

		textFilter := fmt.Sprintf("drawtext=text='%s':fontsize=%d:fontcolor=%s:x=%s:y=%s:enable='between(t,%d,%d)'",
			strings.ReplaceAll(scene.Text, "'", "\\'"),
			fontSize, fontColor, x, y,
			scene.StartTime, scene.StartTime+scene.Duration)

		filters = append(filters, textFilter)
	}

	// Add image overlays
	for _, img := range req.Images {
		if img.StartTime >= 0 && img.Duration > 0 {
			overlay := fmt.Sprintf("overlay=%d:%d:enable='between(t,%d,%d)'",
				img.X, img.Y, img.StartTime, img.StartTime+img.Duration)
			filters = append(filters, overlay)
		}
	}

	return strings.Join(filters, ",")
}

func executeFFmpegCommand(args []string, outputPath string) error {
	args = append(args, outputPath)

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
