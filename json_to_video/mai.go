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
	videoInputCount := 0
	audioInputCount := 0

	// Background color/image
	if req.Background != "" && req.Background[0] == '#' {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("color=%s:size=%dx%d:duration=%d:rate=25",
				req.Background, req.Width, req.Height, req.Duration))
		videoInputCount++
	}

	// Add image inputs - track only actual image files
	imageCount := 0
	for i, img := range req.Images {
		if img.Data != "" {
			args = append(args, "-loop", "1", "-t", strconv.Itoa(img.Duration),
				"-i", filepath.Join(tempDir, fmt.Sprintf("image_%d.png", i)))
			videoInputCount++
			imageCount++
		} else if img.URL != "" {
			args = append(args, "-loop", "1", "-t", strconv.Itoa(img.Duration), "-i", img.URL)
			videoInputCount++
			imageCount++
		}
	}

	// Track where audio inputs start
	audioStartIndex := videoInputCount

	// Add audio inputs
	audioInputs := []string{}
	if req.Audio.BackgroundMusic != "" {
		args = append(args, "-i", filepath.Join(tempDir, "background.mp3"))
		audioInputs = append(audioInputs, fmt.Sprintf("[%d:a]", audioStartIndex+audioInputCount))
		audioInputCount++
	}
	if req.Audio.BackgroundURL != "" {
		args = append(args, "-i", req.Audio.BackgroundURL)
		audioInputs = append(audioInputs, fmt.Sprintf("[%d:a]", audioStartIndex+audioInputCount))
		audioInputCount++
	}
	if req.Audio.VoiceOver != "" {
		args = append(args, "-i", filepath.Join(tempDir, "voiceover.mp3"))
		audioInputs = append(audioInputs, fmt.Sprintf("[%d:a]", audioStartIndex+audioInputCount))
		audioInputCount++
	}
	if req.Audio.VoiceOverURL != "" {
		args = append(args, "-i", req.Audio.VoiceOverURL)
		audioInputs = append(audioInputs, fmt.Sprintf("[%d:a]", audioStartIndex+audioInputCount))
		audioInputCount++
	}

	// Build complex filter with correct input counts
	filterComplex := buildFilterComplexWithCounts(req, videoInputCount, audioInputs)

	// Add subtitles to the filter complex if needed
	if req.Subtitles.SRTData != "" || req.Subtitles.SRTURL != "" {
		srtPath := ""
		if req.Subtitles.SRTData != "" {
			srtPath = filepath.Join(tempDir, "subtitles.srt")
		} else if req.Subtitles.SRTURL != "" {
			srtPath = req.Subtitles.SRTURL
		}

		if srtPath != "" {
			filterComplex = addSubtitlesToFilterComplex(filterComplex, srtPath, req.Subtitles)
		}
	}

	if filterComplex != "" {
		args = append(args, "-filter_complex", filterComplex)
		args = append(args, "-map", "[v]")
		if len(audioInputs) > 0 {
			args = append(args, "-map", "[a]")
		}
	}

	// Output settings
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p")
	if len(audioInputs) > 0 {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	}
	args = append(args, "-t", strconv.Itoa(req.Duration), "-y")

	return args, nil
}

func buildFilterComplexWithCounts(req *VideoRequest, videoInputCount int, audioInputs []string) string {
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

func addSubtitlesToFilterComplex(filterComplex, srtPath string, subtitles SubtitleConfig) string {
	// Ensure we have a proper filter complex base
	if filterComplex == "" {
		filterComplex = "[0:v]copy[v_pre]"
	} else {
		// Replace the last video output tag to prepare for subtitles
		filterComplex = strings.ReplaceAll(filterComplex, "[v]", "[v_pre]")
		filterComplex = strings.ReplaceAll(filterComplex, "[vt", "[v_pre];[v_pre]drawtext=text='':fontsize=1[vt")

		// Find the last video filter and rename its output
		parts := strings.Split(filterComplex, ";")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			// If it's an audio filter, don't modify it
			if !strings.Contains(lastPart, "[a]") {
				// Find the last bracket and replace it
				lastBracketIndex := strings.LastIndex(lastPart, "]")
				if lastBracketIndex > 0 {
					parts[len(parts)-1] = lastPart[:lastBracketIndex] + "_pre]"
				}
			}
		}
		filterComplex = strings.Join(parts, ";")
	}

	// Build subtitle filter
	fontSize := subtitles.FontSize
	if fontSize <= 0 {
		fontSize = 24
	}

	fontColor := subtitles.FontColor
	if fontColor == "" {
		fontColor = "white"
	}

	// Convert Windows path separators for FFmpeg
	srtPath = strings.ReplaceAll(srtPath, "\\", "/")

	// Escape the path for FFmpeg
	srtPath = strings.ReplaceAll(srtPath, ":", "\\:")

	// Build subtitle style
	style := fmt.Sprintf("FontSize=%d,PrimaryColour=&H%s", fontSize, getFFmpegColorHex(fontColor))

	if subtitles.Outline {
		style += ",OutlineColour=&H000000,Outline=2"
	}

	if subtitles.Background != "" && subtitles.Background != "transparent" {
		style += fmt.Sprintf(",BackColour=&H%s", getFFmpegColorHex(subtitles.Background))
	}

	// Position alignment
	alignment := "2" // bottom center
	switch strings.ToLower(subtitles.Position) {
	case "top":
		alignment = "8" // top center
	case "center":
		alignment = "5" // middle center
	case "bottom":
		alignment = "2" // bottom center
	default:
		alignment = "2" // default to bottom
	}
	style += fmt.Sprintf(",Alignment=%s", alignment)

	// Find the last video stream name
	lastVideoStream := "[v_pre]"
	if strings.Contains(filterComplex, "[vt") {
		// Find the highest numbered vt stream
		maxVt := -1
		parts := strings.Split(filterComplex, "[vt")
		for _, part := range parts[1:] {
			endIndex := strings.Index(part, "]")
			if endIndex > 0 {
				numStr := part[:endIndex]
				if num, err := strconv.Atoi(numStr); err == nil && num > maxVt {
					maxVt = num
				}
			}
		}
		if maxVt >= 0 {
			lastVideoStream = fmt.Sprintf("[vt%d]", maxVt)
		}
	}

	// Add subtitle filter
	subtitleFilter := fmt.Sprintf("%ssubtitles=%s:force_style='%s'[v]", lastVideoStream, srtPath, style)

	if filterComplex != "" {
		filterComplex += ";" + subtitleFilter
	} else {
		filterComplex = subtitleFilter
	}

	return filterComplex
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

func getFFmpegColor(color string) string {
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
