package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TranscriptionRequest struct {
	Language  string `json:"language,omitempty"`
	Model     string `json:"model,omitempty"`
	OutputSRT bool   `json:"output_srt,omitempty"`
	MaxLength int    `json:"max_length,omitempty"` // Max characters per subtitle line
	MaxLines  int    `json:"max_lines,omitempty"`  // Max lines per subtitle segment
}

type TranscriptionResponse struct {
	Text      string  `json:"text"`
	SRT       string  `json:"srt,omitempty"`
	Language  string  `json:"language,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Timestamp string  `json:"timestamp"`
	Filename  string  `json:"filename"`
	Format    string  `json:"format"` // "text" or "srt"
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

type SRTSegment struct {
	Index     int
	StartTime string
	EndTime   string
	Text      string
}

const (
	UPLOAD_DIR         = "/tmp/uploads"
	MODELS_DIR         = "/models"
	DEFAULT_MODEL      = "ggml-base.bin"
	MAX_FILE_SIZE      = 100 << 20 // 100MB
	DEFAULT_MAX_LENGTH = 42        // Default max characters per subtitle line
	DEFAULT_MAX_LINES  = 2         // Default max lines per subtitle segment
)

func main() {
	// Create upload directory
	os.MkdirAll(UPLOAD_DIR, 0755)

	// Initialize Gin router
	r := gin.Default()

	// Middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	// Routes
	r.GET("/health", healthCheck)
	r.POST("/transcribe", transcribeAudio)
	r.GET("/models", listModels)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}

	log.Printf("🎙️  Whisper API Server starting on port %s", port)
	log.Fatal(r.Run(":" + port))
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().Format(time.RFC3339),
		Version:   "1.0.0",
	})
}

func listModels(c *gin.Context) {
	files, err := os.ReadDir(MODELS_DIR)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed_to_list_models",
			Message: "Could not read models directory",
		})
		return
	}

	var models []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".bin") {
			models = append(models, file.Name())
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"models":  models,
		"default": DEFAULT_MODEL,
	})
}

func transcribeAudio(c *gin.Context) {
	// Parse multipart form
	err := c.Request.ParseMultipartForm(MAX_FILE_SIZE)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "Failed to parse multipart form or file too large",
		})
		return
	}

	// Get uploaded file
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_audio_file",
			Message: "No audio file provided in 'audio' field",
		})
		return
	}
	defer file.Close()

	// Validate file type
	if !isValidAudioFile(header.Filename) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_file_type",
			Message: "Supported formats: mp3, wav, flac, m4a, ogg, webm, mp4",
		})
		return
	}

	// Parse request parameters
	var req TranscriptionRequest
	language := c.PostForm("language")
	model := c.PostForm("model")
	outputSRT := c.PostForm("output_srt")
	maxLength := c.PostForm("max_length")
	maxLines := c.PostForm("max_lines")

	if language != "" {
		req.Language = language
	}
	if model != "" {
		req.Model = model
	} else {
		req.Model = DEFAULT_MODEL
	}

	req.OutputSRT = outputSRT == "true" || outputSRT == "1"

	if maxLength != "" {
		if ml, err := strconv.Atoi(maxLength); err == nil && ml > 0 {
			req.MaxLength = ml
		} else {
			req.MaxLength = DEFAULT_MAX_LENGTH
		}
	} else {
		req.MaxLength = DEFAULT_MAX_LENGTH
	}

	if maxLines != "" {
		if ml, err := strconv.Atoi(maxLines); err == nil && ml > 0 {
			req.MaxLines = ml
		} else {
			req.MaxLines = DEFAULT_MAX_LINES
		}
	} else {
		req.MaxLines = DEFAULT_MAX_LINES
	}

	// Generate unique filename
	fileID := uuid.New().String()
	ext := filepath.Ext(header.Filename)
	tempFilePath := filepath.Join(UPLOAD_DIR, fileID+ext)

	// Save uploaded file
	out, err := os.Create(tempFilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "file_save_error",
			Message: "Failed to save uploaded file",
		})
		return
	}
	defer out.Close()
	defer os.Remove(tempFilePath) // Clean up

	_, err = io.Copy(out, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "file_save_error",
			Message: "Failed to save uploaded file",
		})
		return
	}

	// Perform transcription
	startTime := time.Now()
	transcription, srtContent, err := performTranscription(tempFilePath, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "transcription_failed",
			Message: err.Error(),
		})
		return
	}
	duration := time.Since(startTime).Seconds()

	// Prepare response
	response := TranscriptionResponse{
		Text:      transcription,
		Language:  req.Language,
		Duration:  duration,
		Timestamp: time.Now().Format(time.RFC3339),
		Filename:  header.Filename,
	}

	if req.OutputSRT {
		response.SRT = srtContent
		response.Format = "srt"
	} else {
		response.Format = "text"
	}

	c.JSON(http.StatusOK, response)
}

func isValidAudioFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	validExts := []string{".mp3", ".wav", ".flac", ".m4a", ".ogg", ".webm", ".mp4"}

	for _, validExt := range validExts {
		if ext == validExt {
			return true
		}
	}
	return false
}

func performTranscription(audioPath string, req TranscriptionRequest) (string, string, error) {
	modelPath := filepath.Join(MODELS_DIR, req.Model)

	// Check if model exists
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("model not found: %s", req.Model)
	}

	// Build whisper command
	args := []string{
		"-m", modelPath,
		"-f", audioPath,
	}

	// Add language if specified
	if req.Language != "" {
		args = append(args, "-l", req.Language)
	}

	var transcription, srtContent string
	var err error

	if req.OutputSRT {
		// Generate SRT with timestamps
		args = append(args, "--output-srt")
		transcription, srtContent, err = executeWhisperForSRT(args, req)
	} else {
		// Generate plain text
		args = append(args, "--output-txt", "--no-timestamps")
		transcription, err = executeWhisperForText(args)
	}

	return transcription, srtContent, err
}

func executeWhisperForText(args []string) (string, error) {
	cmd := exec.Command("whisper-cli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper execution failed: %s - %s", err.Error(), string(output))
	}

	// Parse output - whisper-cli outputs text directly
	transcription := strings.TrimSpace(string(output))

	// Remove any whisper metadata lines
	lines := strings.Split(transcription, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "whisper_") && !strings.Contains(line, "processing") {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, " "), nil
}

func executeWhisperForSRT(args []string, req TranscriptionRequest) (string, string, error) {
	cmd := exec.Command("whisper-cli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("whisper execution failed: %s - %s", err.Error(), string(output))
	}

	// Parse timestamps from whisper output
	segments, err := parseWhisperTimestamps(string(output))
	if err != nil {
		return "", "", err
	}

	// Generate SRT content
	srtContent := generateSRT(segments, req.MaxLength, req.MaxLines)

	// Generate plain text from segments
	var textParts []string
	for _, segment := range segments {
		textParts = append(textParts, segment.Text)
	}
	transcription := strings.Join(textParts, " ")

	return transcription, srtContent, nil
}

func parseWhisperTimestamps(output string) ([]SRTSegment, error) {
	var segments []SRTSegment
	lines := strings.Split(output, "\n")

	// Regex to match timestamp format: [00:00:00.000 --> 00:00:05.000]  text
	timestampRegex := regexp.MustCompile(`\[(\d{2}:\d{2}:\d{2}\.\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}\.\d{3})\]\s*(.+)`)

	index := 1
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := timestampRegex.FindStringSubmatch(line)
		if len(matches) == 4 {
			startTime := convertToSRTTime(matches[1])
			endTime := convertToSRTTime(matches[2])
			text := strings.TrimSpace(matches[3])

			if text != "" {
				segments = append(segments, SRTSegment{
					Index:     index,
					StartTime: startTime,
					EndTime:   endTime,
					Text:      text,
				})
				index++
			}
		}
	}

	return segments, nil
}

func convertToSRTTime(whisperTime string) string {
	// Convert from HH:MM:SS.mmm to HH:MM:SS,mmm (SRT format)
	return strings.Replace(whisperTime, ".", ",", 1)
}

func generateSRT(segments []SRTSegment, maxLength, maxLines int) string {
	var result strings.Builder

	for _, segment := range segments {
		// Split long text into multiple lines if needed
		lines := splitTextForSRT(segment.Text, maxLength, maxLines)

		result.WriteString(fmt.Sprintf("%d\n", segment.Index))
		result.WriteString(fmt.Sprintf("%s --> %s\n", segment.StartTime, segment.EndTime))

		for _, line := range lines {
			result.WriteString(line + "\n")
		}

		result.WriteString("\n") // Empty line between segments
	}

	return strings.TrimSpace(result.String())
}

func splitTextForSRT(text string, maxLength, maxLines int) []string {
	words := strings.Fields(text)
	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		// Check if adding this word would exceed the max length
		if currentLine.Len() > 0 && currentLine.Len()+len(word)+1 > maxLength {
			lines = append(lines, currentLine.String())
			currentLine.Reset()

			// If we've reached max lines, stop splitting
			if len(lines) >= maxLines {
				// Add remaining words to the last line
				remainingWords := []string{word}
				for i := len(words) - len(remainingWords); i < len(words)-1; i++ {
					remainingWords = append(remainingWords, words[i+1])
				}
				if len(lines) > 0 {
					lines[len(lines)-1] += " " + strings.Join(remainingWords, " ")
				}
				break
			}
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	// Add the last line if it has content
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	// Ensure we don't exceed max lines
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	return lines
}
