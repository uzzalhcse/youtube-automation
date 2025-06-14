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
