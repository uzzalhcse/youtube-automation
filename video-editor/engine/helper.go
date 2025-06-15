package engine

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"youtube_automation/video-editor/utils"
)

// MergeVoiceFiles concatenates all voice files and returns the total duration
func (ve *VideoEditor) MergeVoiceFiles() (float64, error) {
	audioDir := filepath.Join(ve.InputDir, "audio")
	outputPath := filepath.Join(ve.OutputDir, "merged_voice.mp3")

	log.Printf("Audio directory: %s", audioDir)
	log.Printf("Output path: %s", outputPath)

	// Ensure output directory exists
	if err := os.MkdirAll(ve.OutputDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create output directory: %v", err)
	}

	// Get all voice files
	voiceFiles, err := utils.GetVoiceFiles(audioDir)
	if err != nil {
		return 0, fmt.Errorf("failed to get voice files: %v", err)
	}

	if len(voiceFiles) == 0 {
		return 0, fmt.Errorf("no voice files found in %s", audioDir)
	}

	log.Printf("Found %d voice files", len(voiceFiles))

	// Sort voice files to ensure proper order
	sort.Strings(voiceFiles)

	// Validate all voice files exist and are readable
	for i, file := range voiceFiles {
		var fullPath string
		if filepath.IsAbs(file) {
			fullPath = file
		} else {
			if _, err := os.Stat(file); err == nil {
				fullPath = file
			} else {
				fullPath = filepath.Join(audioDir, file)
			}
		}

		voiceFiles[i] = fullPath

		if _, err := os.Stat(voiceFiles[i]); os.IsNotExist(err) {
			return 0, fmt.Errorf("voice file does not exist: %s", voiceFiles[i])
		}

		log.Printf("Voice file %d: %s", i+1, voiceFiles[i])
	}

	// Create temporary concat file list
	concatFile := filepath.Join(ve.OutputDir, "voice_list.txt")
	if err := ve.createConcatFile(voiceFiles, concatFile); err != nil {
		return 0, fmt.Errorf("failed to create concat file: %v", err)
	}
	defer func() {
		if err := os.Remove(concatFile); err != nil {
			log.Printf("Warning: failed to remove concat file: %v", err)
		}
	}()

	// Log the concat file contents for debugging
	if content, err := os.ReadFile(concatFile); err == nil {
		log.Printf("Concat file contents:\n%s", string(content))
	}

	// Remove existing output file if it exists
	if _, err := os.Stat(outputPath); err == nil {
		if err := os.Remove(outputPath); err != nil {
			log.Printf("Warning: failed to remove existing output file: %v", err)
		}
	}

	// Convert paths to forward slashes for FFmpeg (Windows compatibility)
	concatFileForFFmpeg := strings.ReplaceAll(concatFile, "\\", "/")
	outputPathForFFmpeg := strings.ReplaceAll(outputPath, "\\", "/")

	// Merge voice files using FFmpeg with enhanced error handling
	args := []string{
		"-y",           // Overwrite output file
		"-f", "concat", // Use concat demuxer
		"-safe", "0", // Allow unsafe file paths
		"-i", concatFileForFFmpeg, // Input concat file
		"-c", "copy", // Copy streams without re-encoding
		"-avoid_negative_ts", "make_zero", // Handle negative timestamps
		outputPathForFFmpeg, // Output file
	}

	log.Printf("FFmpeg command: ffmpeg %s", strings.Join(args, " "))
	log.Printf("Merging %d voice files...", len(voiceFiles))

	cmd := exec.Command("ffmpeg", args...)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg output: %s", string(output))
		return 0, fmt.Errorf("failed to merge voice files: %v", err)
	}

	log.Printf("✓ Voice files merged successfully")

	// Verify the output file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return 0, fmt.Errorf("output file was not created: %s", outputPath)
	}

	// Get duration of merged voice file
	duration, err := utils.GetAudioDuration(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get audio duration: %v", err)
	}

	log.Printf("Merged voice file duration: %.2f seconds", duration)
	return duration, nil
}

// createConcatFile creates a properly formatted concat file for FFmpeg
func (ve *VideoEditor) createConcatFile(files []string, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, audioFile := range files {
		// Convert to absolute path and normalize for FFmpeg
		absPath, err := filepath.Abs(audioFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %s: %v", audioFile, err)
		}

		// Convert to forward slashes for FFmpeg compatibility
		ffmpegPath := strings.ReplaceAll(absPath, "\\", "/")

		// Escape the path for FFmpeg concat format
		escapedPath := strings.ReplaceAll(ffmpegPath, "'", "'\\''")

		// Write in the format expected by FFmpeg concat demuxer
		if _, err := fmt.Fprintf(file, "file '%s'\n", escapedPath); err != nil {
			return err
		}
	}

	return nil
}

// ExtendBackgroundMusic loops the background music to match voice duration
func (ve *VideoEditor) ExtendBackgroundMusic(targetDuration float64) error {
	bgmPath := filepath.Join(ve.InputDir, "audio", "background.mp3")
	outputPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")

	log.Printf("Background music path: %s", bgmPath)
	log.Printf("Target duration: %.2f seconds", targetDuration)

	// Check if background music exists
	if _, err := os.Stat(bgmPath); os.IsNotExist(err) {
		return fmt.Errorf("background music file not found: %s", bgmPath)
	}

	// Get original BGM duration
	originalDuration, err := utils.GetAudioDuration(bgmPath)
	if err != nil {
		return fmt.Errorf("failed to get BGM duration: %v", err)
	}

	log.Printf("Original BGM duration: %.2f seconds", originalDuration)

	// Calculate how many loops we need
	loops := int(targetDuration/originalDuration) + 1
	log.Printf("Calculated loops needed: %d", loops)

	// Convert paths for FFmpeg compatibility
	bgmPathForFFmpeg := strings.ReplaceAll(bgmPath, "\\", "/")
	outputPathForFFmpeg := strings.ReplaceAll(outputPath, "\\", "/")

	// Create FFmpeg command to loop and trim background music
	args := []string{
		"-y",
		"-stream_loop", strconv.Itoa(loops),
		"-i", bgmPathForFFmpeg,
		"-t", fmt.Sprintf("%.2f", targetDuration),
		"-af", fmt.Sprintf("volume=%.2f", ve.Config.Settings.BGMVolume),
		outputPathForFFmpeg,
	}

	log.Printf("FFmpeg BGM command: ffmpeg %s", strings.Join(args, " "))
	log.Printf("Extending background music to %.2f seconds...", targetDuration)

	cmd := exec.Command("ffmpeg", args...)

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg BGM output: %s", string(output))
		return fmt.Errorf("failed to extend background music: %v", err)
	}

	log.Printf("✓ Background music extended successfully")
	return nil
}

// getVideoDuration gets the duration of a video file
func (ve *VideoEditor) getVideoDuration(videoPath string) (float64, error) {
	// Convert path for FFmpeg compatibility
	pathForFFmpeg := strings.ReplaceAll(videoPath, "\\", "/")

	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_entries", "format=duration", "-of", "csv=p=0", pathForFFmpeg)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, err
	}

	return duration, nil
}

// NEW: Check if running in Google Colab
func (ve *VideoEditor) isGoogleColab() bool {
	// Check for Colab-specific environment variables
	if os.Getenv("COLAB_GPU") != "" {
		return true
	}

	// Check for Colab-specific paths
	if _, err := os.Stat("/content"); err == nil {
		return true
	}

	// Check for Colab runtime
	if _, err := os.Stat("/usr/local/lib/python*/dist-packages/google/colab"); err == nil {
		return true
	}

	return false
}

// NEW: Check if T4 GPU is available
func (ve *VideoEditor) isT4Available() bool {
	// Method 1: Check nvidia-smi output
	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	output, err := cmd.Output()
	if err == nil {
		gpuName := strings.ToLower(string(output))
		if strings.Contains(gpuName, "t4") {
			return true
		}
	}

	// Method 2: Check /proc/driver/nvidia/gpus
	if entries, err := os.ReadDir("/proc/driver/nvidia/gpus"); err == nil {
		for _, entry := range entries {
			infoPath := filepath.Join("/proc/driver/nvidia/gpus", entry.Name(), "information")
			if data, err := os.ReadFile(infoPath); err == nil {
				if strings.Contains(strings.ToLower(string(data)), "t4") {
					return true
				}
			}
		}
	}

	return false
}

// generateZoomConfig creates a configurable zoom configuration
func (ve *VideoEditor) generateZoomConfig(index int) ZoomConfig {
	rand.Seed(time.Now().UnixNano() + int64(index))

	effects := []ZoomEffect{ZoomIn, ZoomOut, ZoomInOut, PanZoom}
	effect := effects[rand.Intn(len(effects))]

	config := ZoomConfig{
		Effect: effect,
		StartX: 0.5,
		StartY: 0.5,
		EndX:   0.5,
		EndY:   0.5,
	}

	// Use configurable zoom intensity
	baseZoom := ve.Config.Settings.ZoomIntensity
	zoomVariation := 0.1 * ve.Config.Settings.TransitionSmooth

	switch effect {
	case ZoomIn:
		config.StartScale = 1.0
		config.EndScale = baseZoom + rand.Float64()*zoomVariation

	case ZoomOut:
		config.StartScale = baseZoom + rand.Float64()*zoomVariation
		config.EndScale = 1.0

	case ZoomInOut:
		config.StartScale = 1.0
		config.EndScale = 1.0
		// Peak zoom will be handled in the filter

	case PanZoom:
		config.StartScale = baseZoom * 0.9
		config.EndScale = baseZoom * 1.1

		// More controlled panning with configurable speed
		panRange := 0.2 * ve.Config.Settings.TransitionSmooth
		centerOffset := (1.0 - panRange) / 2

		config.StartX = centerOffset + rand.Float64()*panRange
		config.StartY = centerOffset + rand.Float64()*panRange
		config.EndX = centerOffset + rand.Float64()*panRange
		config.EndY = centerOffset + rand.Float64()*panRange
	}

	return config
}

// Remove the getHardwareAccelFilter method or simplify it
func (ve *VideoEditor) getHardwareAccelFilter() string {
	// Don't rely on hardware acceleration filters
	// GPU encoding will happen at the encoder level, not filter level
	return ""
}

// Simplified getOptimalEncoderSettings that focuses on encoding, not filtering
func (ve *VideoEditor) getOptimalEncoderSettings() (string, []string) {
	if ve.UseGPU {
		// Check for T4 GPU in Google Colab with enhanced detection
		if ve.isGoogleColab() && ve.isT4Available() {
			log.Printf("🚀 Using T4 GPU encoding for Google Colab")
			return "h264_nvenc", []string{
				"-preset", "fast",
				"-rc", "vbr",
				"-cq", "21",
				"-b:v", "6M",
				"-maxrate", "8M",
				"-bufsize", "12M",
				"-profile:v", "main",
				"-level", "4.0",
				"-bf", "2",
				"-g", "120",
			}
		}

		// Fallback NVIDIA settings
		if ve.isEncoderAvailable("h264_nvenc") {
			log.Printf("🚀 Using standard NVIDIA GPU encoding")
			return "h264_nvenc", []string{
				"-preset", "medium",
				"-rc", "vbr",
				"-cq", "23",
				"-b:v", "4M",
				"-maxrate", "6M",
				"-bufsize", "8M",
				"-profile:v", "main",
			}
		}
	}

	// CPU fallback with Colab optimization
	log.Printf("🖥️ Using CPU encoding (fallback)")
	if ve.isGoogleColab() {
		return "libx264", []string{
			"-preset", "medium",
			"-crf", "23",
			"-threads", "2",
			"-tune", "film",
		}
	}

	return "libx264", []string{
		"-preset", "fast",
		"-crf", "21",
	}
}

// getEffectName returns human-readable effect name
func (ve *VideoEditor) getEffectName(effect ZoomEffect) string {
	switch effect {
	case ZoomIn:
		return "zoom in"
	case ZoomOut:
		return "zoom out"
	case ZoomInOut:
		return "zoom in-out"
	case PanZoom:
		return "pan & zoom"
	default:
		return "zoom"
	}
}

// GetOverlayVideos finds and returns all overlay video files
func (ve *VideoEditor) GetOverlayVideos() ([]string, error) {
	overlayDir := filepath.Join(ve.InputDir, "overlays")

	// Check if overlay directory exists
	if _, err := os.Stat(overlayDir); os.IsNotExist(err) {
		log.Printf("No overlay directory found at %s", overlayDir)
		return []string{}, nil
	}

	// Get all video files in overlay directory
	overlayFiles, err := utils.GetVideoFiles(overlayDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get overlay video files: %v", err)
	}

	// Convert to full paths
	var fullPaths []string
	for _, file := range overlayFiles {
		fullPath := filepath.Join(overlayDir, file)
		if _, err := os.Stat(fullPath); err == nil {
			fullPaths = append(fullPaths, fullPath)
		}
	}

	sort.Strings(fullPaths)
	log.Printf("Found %d overlay videos", len(fullPaths))

	return fullPaths, nil
}

// createStaticFilter creates a simple static filter without animation
func (ve *VideoEditor) createStaticFilter(width, height int) string {
	// Simple scale and pad filter for static images
	return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black",
		width, height, width, height)
}

// concatenateClips concatenates all video clips into a single video
func (ve *VideoEditor) concatenateClips(videoClips []string, outputPath string) error {
	// Create a list file for concatenation
	listFile := filepath.Join(ve.OutputDir, "clips_list.txt")
	listContent := ""
	for _, clip := range videoClips {
		absPath, err := filepath.Abs(clip)
		if err != nil {
			absPath = clip
		}
		clipForFFmpeg := strings.ReplaceAll(absPath, "\\", "/")
		listContent += fmt.Sprintf("file '%s'\n", clipForFFmpeg)
	}

	log.Printf("Creating clips list file: %s", listFile)
	if err := os.WriteFile(listFile, []byte(listContent), 0644); err != nil {
		return fmt.Errorf("failed to create clips list file: %v", err)
	}

	// Concatenate all clips
	listFileForFFmpeg := strings.ReplaceAll(listFile, "\\", "/")
	outputPathForFFmpeg := strings.ReplaceAll(outputPath, "\\", "/")

	log.Printf("Concatenating %d clips...", len(videoClips))

	concatArgs := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFileForFFmpeg,
		"-c", "copy",
		outputPathForFFmpeg,
	}

	concatCmd := exec.Command("ffmpeg", concatArgs...)
	concatOutput, err := concatCmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg concatenation output: %s", string(concatOutput))
		return fmt.Errorf("failed to concatenate clips: %v", err)
	}

	// Clean up list file
	if err := os.Remove(listFile); err != nil {
		log.Printf("Warning: failed to clean up list file: %v", err)
	}

	return nil
}

// addBlackScreenToSlideshow adds black screen to the end of slideshow
func (ve *VideoEditor) addBlackScreenToSlideshow(blackDuration float64) error {
	slideshowPath := filepath.Join(ve.OutputDir, "slideshow.mp4")
	tempSlideshowPath := filepath.Join(ve.OutputDir, "slideshow_temp.mp4")
	blackScreenPath := filepath.Join(ve.OutputDir, "black_screen.mp4")

	// Create black screen video
	if err := ve.createBlackScreen(blackScreenPath, blackDuration); err != nil {
		return fmt.Errorf("failed to create black screen: %v", err)
	}

	// Concatenate slideshow + black screen
	if err := ve.concatenateWithBlackScreen(slideshowPath, blackScreenPath, tempSlideshowPath); err != nil {
		return fmt.Errorf("failed to concatenate with black screen: %v", err)
	}

	// Replace original slideshow with the new one
	if err := os.Rename(tempSlideshowPath, slideshowPath); err != nil {
		return fmt.Errorf("failed to replace slideshow: %v", err)
	}

	// Clean up
	os.Remove(blackScreenPath)

	return nil
}

func (ve *VideoEditor) createBlackScreen(outputPath string, duration float64) error {
	color := "black"
	if ve.Config.Settings.BlackScreenColor != "" {
		color = ve.Config.Settings.BlackScreenColor
	}

	encoder, encoderArgs := ve.getEncoderSettings()

	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=%s:size=%dx%d:duration=%.2f:rate=%d",
			color, ve.Config.Settings.Width, ve.Config.Settings.Height, duration, ve.Config.Settings.FPS),
		"-c:v", encoder,
	}

	args = append(args, encoderArgs...)
	args = append(args, "-pix_fmt", "yuv420p", outputPath)

	log.Printf("Creating black screen: %.2f seconds (%.2f minutes)", duration, duration/60.0)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg black screen output: %s", string(output))
		return fmt.Errorf("failed to create black screen: %v", err)
	}

	return nil
}

func (ve *VideoEditor) concatenateWithBlackScreen(slideshowPath, blackScreenPath, outputPath string) error {
	// Ensure all paths are absolute to avoid directory issues
	absSlideshow, err := filepath.Abs(slideshowPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for slideshow: %v", err)
	}

	absBlackScreen, err := filepath.Abs(blackScreenPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for black screen: %v", err)
	}

	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for output: %v", err)
	}

	// Verify input files exist
	if _, err := os.Stat(absSlideshow); os.IsNotExist(err) {
		return fmt.Errorf("slideshow file does not exist: %s", absSlideshow)
	}

	if _, err := os.Stat(absBlackScreen); os.IsNotExist(err) {
		return fmt.Errorf("black screen file does not exist: %s", absBlackScreen)
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(absOutput)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create concat list file in a temp location to avoid path issues
	tempDir := os.TempDir()
	listFile := filepath.Join(tempDir, fmt.Sprintf("concat_list_%d.txt", time.Now().UnixNano()))

	// Use forward slashes for FFmpeg compatibility (works on Windows too)
	listContent := fmt.Sprintf("file '%s'\nfile '%s'\n",
		strings.ReplaceAll(absSlideshow, "\\", "/"),
		strings.ReplaceAll(absBlackScreen, "\\", "/"))

	if err := os.WriteFile(listFile, []byte(listContent), 0644); err != nil {
		return fmt.Errorf("failed to create concat list: %v", err)
	}

	// Cleanup temp file
	defer os.Remove(listFile)

	// Log the concat list content for debugging
	log.Printf("Concat list content:\n%s", listContent)
	log.Printf("Output path: %s", absOutput)

	args := []string{
		"-y",           // Overwrite output file
		"-f", "concat", // Use concat format
		"-safe", "0", // Allow unsafe file paths
		"-i", strings.ReplaceAll(listFile, "\\", "/"), // Input concat list
		"-c", "copy", // Copy streams without re-encoding
		strings.ReplaceAll(absOutput, "\\", "/"), // Output file
	}

	cmd := exec.Command("ffmpeg", args...)

	// Set working directory to avoid relative path issues
	cmd.Dir = filepath.Dir(absOutput)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg command: ffmpeg %s", strings.Join(args, " "))
		log.Printf("FFmpeg concatenation output: %s", string(output))
		return fmt.Errorf("failed to concatenate: %v", err)
	}

	log.Printf("Successfully concatenated video to: %s", absOutput)
	return nil
}

// GenerateFinalVideoSimplified generates final video without overlays (fallback method)
func (ve *VideoEditor) GenerateFinalVideoSimplified() error {
	slideshowPath := filepath.Join(ve.OutputDir, "slideshow.mp4")
	voicePath := filepath.Join(ve.OutputDir, "merged_voice.mp3")
	bgmPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")
	finalPath := filepath.Join(ve.OutputDir, "final_video.mp4")

	log.Printf("Generating simplified final video without overlays...")

	// Get encoder settings
	encoder, encoderArgs := ve.getEncoderSettings()

	// Convert paths for FFmpeg
	slideshowForFFmpeg := strings.ReplaceAll(slideshowPath, "\\", "/")
	voiceForFFmpeg := strings.ReplaceAll(voicePath, "\\", "/")
	bgmForFFmpeg := strings.ReplaceAll(bgmPath, "\\", "/")
	finalForFFmpeg := strings.ReplaceAll(finalPath, "\\", "/")

	args := []string{
		"-y",
		"-i", slideshowForFFmpeg,
		"-i", voiceForFFmpeg,
		"-i", bgmForFFmpeg,
		"-filter_complex", "[1:a][2:a]amix=inputs=2:duration=first:dropout_transition=0[a]",
		"-map", "0:v",
		"-map", "[a]",
		"-c:v", encoder,
	}

	args = append(args, encoderArgs...)
	args = append(args,
		"-c:a", "aac",
		"-b:a", "128k",
		"-ar", "44100",
		"-ac", "2",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		"-preset", ve.getPresetForEncoder(encoder),
		finalForFFmpeg,
	)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg simplified video generation output: %s", string(output))
		return fmt.Errorf("failed to generate simplified final video: %v", err)
	}

	log.Printf("✓ Simplified final video generated successfully: %s", finalPath)
	return nil
}

// NEW: Initialize multi-GPU setup
func (ve *VideoEditor) initializeMultiGPU() error {
	log.Printf("🔧 Initializing multi-GPU setup...")

	var availableGPUs []GPUDevice

	// Check NVIDIA GPU (MX450)
	if ve.isNVIDIAGPUAvailable() {
		encoder, args := ve.getNVIDIAEncoderSettings()
		nvGPU := GPUDevice{
			Type:     "nvidia",
			Device:   "0",
			Encoder:  encoder,
			Args:     args,
			Priority: 2, // Higher priority for discrete GPU
		}
		availableGPUs = append(availableGPUs, nvGPU)
		log.Printf("✅ NVIDIA GeForce MX450 added to GPU pool")
	}

	// Check Intel GPU (Iris Xe)
	if ve.isIntelGPUAvailable() {
		encoder, args := ve.getIntelEncoderSettings()
		intelGPU := GPUDevice{
			Type:     "intel",
			Device:   "0",
			Encoder:  encoder,
			Args:     args,
			Priority: 1, // Lower priority for integrated GPU
		}
		availableGPUs = append(availableGPUs, intelGPU)
		log.Printf("✅ Intel Iris Xe added to GPU pool")
	}

	if len(availableGPUs) >= 2 {
		ve.UseMultiGPU = true
		ve.GPUDevices = availableGPUs
		ve.GPUPool = make(chan GPUDevice, len(availableGPUs))

		// Fill GPU pool
		for _, gpu := range availableGPUs {
			ve.GPUPool <- gpu
		}

		log.Printf("🚀 Multi-GPU setup complete: %d GPUs available", len(availableGPUs))
		return nil
	}

	log.Printf("⚠️ Only %d GPU(s) available, falling back to single GPU", len(availableGPUs))
	ve.UseMultiGPU = false
	return nil
}
