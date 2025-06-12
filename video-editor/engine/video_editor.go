package engine

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"youtube_automation/video-editor/models"
	"youtube_automation/video-editor/utils"
)

// VideoEditor handles all video editing operations
type VideoEditor struct {
	InputDir  string
	OutputDir string
	Config    *models.VideoConfig
}

// NewVideoEditor creates a new video editor instance
func NewVideoEditor(inputDir, outputDir string, config *models.VideoConfig) *VideoEditor {
	return &VideoEditor{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Config:    config,
	}
}

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

// CreateSlideshow generates a slideshow from images with proper duration calculation
func (ve *VideoEditor) CreateSlideshow(targetDuration float64) error {
	imagesDir := filepath.Join(ve.InputDir, "images")
	outputPath := filepath.Join(ve.OutputDir, "slideshow.mp4")

	log.Printf("Images directory: %s", imagesDir)
	log.Printf("Slideshow output: %s", outputPath)
	log.Printf("Target slideshow duration: %.2f seconds", targetDuration)

	// Get all image files
	imageFiles, err := utils.GetImageFiles(imagesDir)
	if err != nil {
		return fmt.Errorf("failed to get image files: %v", err)
	}

	if len(imageFiles) == 0 {
		return fmt.Errorf("no image files found in %s", imagesDir)
	}

	log.Printf("Found %d image files", len(imageFiles))

	// Sort image files
	sort.Strings(imageFiles)

	// Validate all image files exist
	for i, file := range imageFiles {
		var fullPath string
		if filepath.IsAbs(file) {
			fullPath = file
		} else {
			if _, err := os.Stat(file); err == nil {
				fullPath = file
			} else {
				fullPath = filepath.Join(imagesDir, file)
			}
		}

		imageFiles[i] = fullPath

		if _, err := os.Stat(imageFiles[i]); os.IsNotExist(err) {
			return fmt.Errorf("image file does not exist: %s", imageFiles[i])
		}

		log.Printf("Image file %d: %s", i+1, imageFiles[i])
	}

	// Calculate duration per image based on target duration
	imageDuration := targetDuration / float64(len(imageFiles))
	log.Printf("Duration per image: %.2f seconds (total: %.2f seconds)", imageDuration, targetDuration)

	// Convert paths for FFmpeg compatibility
	outputPathForFFmpeg := strings.ReplaceAll(outputPath, "\\", "/")

	// Build FFmpeg command for slideshow - simplified approach
	var args []string
	args = append(args, "-y")

	// Add input images with proper duration
	for _, img := range imageFiles {
		imgPath := strings.ReplaceAll(img, "\\", "/")
		args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.2f", imageDuration), "-i", imgPath)
	}

	// Simple concatenation without complex transitions
	var filterInputs []string
	for i := 0; i < len(imageFiles); i++ {
		filterInputs = append(filterInputs, fmt.Sprintf("[%d:v]scale=%d:%d,setsar=1[v%d]",
			i, ve.Config.Settings.Width, ve.Config.Settings.Height, i))
	}

	// Concatenate all scaled inputs
	var concatInputs []string
	for i := 0; i < len(imageFiles); i++ {
		concatInputs = append(concatInputs, fmt.Sprintf("[v%d]", i))
	}

	filterComplex := strings.Join(filterInputs, ";") + ";" +
		strings.Join(concatInputs, "") + fmt.Sprintf("concat=n=%d:v=1:a=0[slideshow]", len(imageFiles))

	args = append(args, "-filter_complex", filterComplex)

	// Output settings
	args = append(args,
		"-map", "[slideshow]",
		"-c:v", "libx264",
		"-r", strconv.Itoa(ve.Config.Settings.FPS),
		"-pix_fmt", "yuv420p",
		"-preset", "fast", // Use faster preset
		outputPathForFFmpeg)

	log.Printf("Creating slideshow with %d images (%.2f seconds each)...", len(imageFiles), imageDuration)

	cmd := exec.Command("ffmpeg", args...)

	// Use simpler output capture
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg slideshow output: %s", string(output))
		return fmt.Errorf("failed to create slideshow: %v", err)
	}

	log.Printf("✓ Slideshow created successfully")

	// Verify output file exists and get its duration
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return fmt.Errorf("slideshow output file was not created: %s", outputPath)
	}

	// Check actual duration
	if actualDuration, err := ve.getVideoDuration(outputPath); err == nil {
		log.Printf("Actual slideshow duration: %.2f seconds", actualDuration)
	}

	return nil
}

// GenerateFinalVideoSimplified creates final video with proper path handling
func (ve *VideoEditor) GenerateFinalVideoSimplified() error {
	slideshowPath := filepath.Join(ve.OutputDir, "slideshow.mp4")
	voicePath := filepath.Join(ve.OutputDir, "merged_voice.mp3")
	bgmPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")
	finalPath := filepath.Join(ve.OutputDir, "final_video.mp4")

	log.Printf("Generating final video (simplified)...")
	log.Printf("Slideshow: %s", slideshowPath)
	log.Printf("Voice: %s", voicePath)
	log.Printf("BGM: %s", bgmPath)
	log.Printf("Final output: %s", finalPath)

	// Verify all input files exist
	requiredFiles := map[string]string{
		"slideshow": slideshowPath,
		"voice":     voicePath,
		"bgm":       bgmPath,
	}

	for name, path := range requiredFiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("required %s file does not exist: %s", name, path)
		}
	}

	// Convert all paths for FFmpeg compatibility
	slideshowForFFmpeg := strings.ReplaceAll(slideshowPath, "\\", "/")
	voiceForFFmpeg := strings.ReplaceAll(voicePath, "\\", "/")
	bgmForFFmpeg := strings.ReplaceAll(bgmPath, "\\", "/")
	finalForFFmpeg := strings.ReplaceAll(finalPath, "\\", "/")

	// Remove existing output file if it exists
	if _, err := os.Stat(finalPath); err == nil {
		if err := os.Remove(finalPath); err != nil {
			log.Printf("Warning: failed to remove existing final video: %v", err)
		}
	}

	// Simplified FFmpeg command without complex overlays
	args := []string{
		"-y",
		"-i", slideshowForFFmpeg, // [0] - slideshow video
		"-i", voiceForFFmpeg, // [1] - voice audio
		"-i", bgmForFFmpeg, // [2] - background music
		"-filter_complex",
		fmt.Sprintf("[1:a]volume=%.2f[voice];[2:a]volume=%.2f[bgm];[voice][bgm]amix=inputs=2:duration=first:dropout_transition=0[final_audio]",
			ve.Config.Settings.VoiceVolume, ve.Config.Settings.BGMVolume),
		"-map", "0:v", // Use video from slideshow
		"-map", "[final_audio]", // Use mixed audio
		"-c:v", "copy", // Copy video stream (faster)
		"-c:a", "aac",
		"-b:a", "128k", // Set audio bitrate
		"-shortest", // End when shortest stream ends
		finalForFFmpeg,
	}

	log.Printf("FFmpeg final command: ffmpeg %s", strings.Join(args, " "))

	cmd := exec.Command("ffmpeg", args...)

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg final video output: %s", string(output))
		return fmt.Errorf("failed to generate final video: %v", err)
	}

	log.Printf("✓ Final video generated successfully")

	// Verify final output
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		return fmt.Errorf("final video file was not created: %s", finalPath)
	}

	// Log final video info
	if finalDuration, err := ve.getVideoDuration(finalPath); err == nil {
		log.Printf("Final video duration: %.2f seconds", finalDuration)
	}

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

// ProcessVideo is the main method to process the entire video
func (ve *VideoEditor) ProcessVideo() error {
	log.Printf("🎬 Starting video processing...")
	log.Printf("Input directory: %s", ve.InputDir)
	log.Printf("Output directory: %s", ve.OutputDir)

	// Step 1: Merge voice files and get total duration
	log.Printf("🎤 Step 1: Merging voice files...")
	voiceDuration, err := ve.MergeVoiceFiles()
	if err != nil {
		return fmt.Errorf("failed to merge voice files: %v", err)
	}
	log.Printf("✅ Voice files merged. Total duration: %.2f seconds", voiceDuration)

	// Step 2: Extend background music to match voice duration
	log.Printf("🎵 Step 2: Extending background music...")
	if err := ve.ExtendBackgroundMusic(voiceDuration); err != nil {
		return fmt.Errorf("failed to extend background music: %v", err)
	}
	log.Printf("✅ Background music extended")

	// Step 3: Create slideshow with the same duration as voice
	log.Printf("🖼️ Step 3: Creating slideshow...")
	if err := ve.CreateSlideshow(voiceDuration); err != nil {
		return fmt.Errorf("failed to create slideshow: %v", err)
	}
	log.Printf("✅ Slideshow created")

	// Step 4: Generate final video
	log.Printf("🎬 Step 4: Generating final video...")
	if err := ve.GenerateFinalVideoSimplified(); err != nil {
		return fmt.Errorf("failed to generate final video: %v", err)
	}
	log.Printf("✅ Final video generated successfully!")

	return nil
}
