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

// PrepareOverlayVideo processes a single overlay video to fit the screen and loop for the required duration
func (ve *VideoEditor) PrepareOverlayVideo(overlayPath string, duration float64, index int) (string, error) {
	outputPath := filepath.Join(ve.OutputDir, fmt.Sprintf("prepared_overlay_%d.mp4", index))

	// Convert paths for FFmpeg compatibility
	overlayForFFmpeg := strings.ReplaceAll(overlayPath, "\\", "/")
	outputForFFmpeg := strings.ReplaceAll(outputPath, "\\", "/")

	// Get the original overlay duration
	originalDuration, err := ve.getVideoDuration(overlayPath)
	if err != nil {
		return "", fmt.Errorf("failed to get overlay duration: %v", err)
	}

	log.Printf("Preparing overlay video %d: %s (original: %.2fs, target: %.2fs)",
		index, filepath.Base(overlayPath), originalDuration, duration)

	// Get overlay opacity from config (default 0.7 if not specified)
	overlayOpacity := 0.7 // Default opacity
	if ve.Config.Settings.OverlayOpacity > 0 {
		overlayOpacity = ve.Config.Settings.OverlayOpacity
	}

	var videoFilter string
	if originalDuration >= duration {
		// If overlay is longer than or equal to target duration, just trim it
		videoFilter = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black",
			ve.Config.Settings.Width, ve.Config.Settings.Height,
			ve.Config.Settings.Width, ve.Config.Settings.Height)

		args := []string{
			"-y",
			"-i", overlayForFFmpeg,
			"-vf", videoFilter,
			"-t", fmt.Sprintf("%.2f", duration),
			"-c:v", "libx264",
			"-r", strconv.Itoa(ve.Config.Settings.FPS),
			"-pix_fmt", "yuv420p", // Use alpha channel for transparency
			"-an", // Remove audio from overlay
			outputForFFmpeg,
		}

		cmd := exec.Command("ffmpeg", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("FFmpeg overlay preparation output: %s", string(output))
			return "", fmt.Errorf("failed to prepare overlay video: %v", err)
		}
	} else {
		// If overlay is shorter than target duration, loop it
		// Calculate how many loops we need
		loopCount := int(duration/originalDuration) + 1

		videoFilter = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,format=yuva420p,colorchannelmixer=aa=%.2f",
			ve.Config.Settings.Width, ve.Config.Settings.Height,
			ve.Config.Settings.Width, ve.Config.Settings.Height,
			overlayOpacity)

		args := []string{
			"-y",
			"-stream_loop", strconv.Itoa(loopCount), // Loop the input
			"-i", overlayForFFmpeg,
			"-vf", videoFilter,
			"-t", fmt.Sprintf("%.2f", duration), // Trim to exact duration
			"-c:v", "libx264",
			"-r", strconv.Itoa(ve.Config.Settings.FPS),
			"-pix_fmt", "yuva420p", // Use alpha channel for transparency
			"-an", // Remove audio from overlay
			outputForFFmpeg,
		}

		log.Printf("Looping overlay %d times to reach target duration", loopCount)

		cmd := exec.Command("ffmpeg", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("FFmpeg overlay preparation output: %s", string(output))
			return "", fmt.Errorf("failed to prepare overlay video: %v", err)
		}
	}

	log.Printf("✓ Overlay video %d prepared successfully with %.0f%% opacity", index, overlayOpacity*100)
	return outputPath, nil
}

// GenerateFinalVideoWithOverlays creates final video with overlay support
func (ve *VideoEditor) GenerateFinalVideoWithOverlays() error {
	slideshowPath := filepath.Join(ve.OutputDir, "slideshow.mp4")
	voicePath := filepath.Join(ve.OutputDir, "merged_voice.mp3")
	bgmPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")
	finalPath := filepath.Join(ve.OutputDir, "final_video.mp4")

	log.Printf("Generating final video with overlays...")
	log.Printf("Slideshow: %s", slideshowPath)
	log.Printf("Voice: %s", voicePath)
	log.Printf("BGM: %s", bgmPath)
	log.Printf("Final output: %s", finalPath)

	// Get overlay opacity from config (default 0.7 if not specified)
	overlayOpacity := 0.3 // Default opacity
	if ve.Config.Settings.OverlayOpacity > 0 {
		overlayOpacity = ve.Config.Settings.OverlayOpacity
	}
	// Verify required files exist
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

	// Get slideshow duration
	slideshowDuration, err := ve.getVideoDuration(slideshowPath)
	if err != nil {
		return fmt.Errorf("failed to get slideshow duration: %v", err)
	}

	// Get overlay videos
	overlayVideos, err := ve.GetOverlayVideos()
	if err != nil {
		return fmt.Errorf("failed to get overlay videos: %v", err)
	}

	// Convert paths for FFmpeg compatibility
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

	var args []string
	var filterComplex string

	if len(overlayVideos) == 0 {
		// No overlays - use simplified version
		log.Printf("No overlay videos found, using simplified generation")
		return ve.GenerateFinalVideoSimplified()
	}

	// Prepare overlay videos
	var preparedOverlays []string
	for i, overlayPath := range overlayVideos {
		// Use full slideshow duration for each overlay (they will be timed separately in the overlay filter)
		preparedOverlay, err := ve.PrepareOverlayVideo(overlayPath, slideshowDuration, i)
		if err != nil {
			log.Printf("Warning: failed to prepare overlay %d: %v", i, err)
			continue
		}
		preparedOverlays = append(preparedOverlays, preparedOverlay)
	}

	if len(preparedOverlays) == 0 {
		log.Printf("No overlay videos could be prepared, using simplified generation")
		return ve.GenerateFinalVideoSimplified()
	}

	// Build FFmpeg command with overlays
	args = append(args, "-y")
	args = append(args, "-i", slideshowForFFmpeg) // [0] - slideshow

	// Add prepared overlay inputs
	for _, overlay := range preparedOverlays {
		overlayForFFmpeg := strings.ReplaceAll(overlay, "\\", "/")
		args = append(args, "-i", overlayForFFmpeg)
	}

	// Add audio inputs
	args = append(args, "-i", voiceForFFmpeg) // voice
	args = append(args, "-i", bgmForFFmpeg)   // bgm

	// Build filter complex for overlays
	// Build filter complex for overlays
	baseVideo := "[0:v]"

	if len(preparedOverlays) == 1 {
		// Single overlay with proper alpha blending
		filterComplex = fmt.Sprintf("%s[1:v]blend=all_mode=overlay:all_opacity=%.2f,format=yuv420p[final_video]",
			baseVideo, overlayOpacity)
	} else {
		// Multiple overlays - apply them sequentially with proper blending
		currentInput := baseVideo
		for i := 0; i < len(preparedOverlays); i++ {
			overlayIndex := i + 1
			outputTag := fmt.Sprintf("[overlay%d]", i)

			if i == len(preparedOverlays)-1 {
				outputTag = "[final_video]"
			}

			// Calculate timing for each overlay (distribute evenly)
			startTime := float64(i) * (slideshowDuration / float64(len(preparedOverlays)))
			endTime := startTime + (slideshowDuration / float64(len(preparedOverlays)))

			var blendFilter string
			if i == len(preparedOverlays)-1 {
				blendFilter = fmt.Sprintf("%s[%d:v]blend=all_mode=overlay:all_opacity=%.2f:enable='between(t,%.2f,%.2f)',format=yuv420p%s",
					currentInput, overlayIndex, overlayOpacity, startTime, endTime, outputTag)
			} else {
				blendFilter = fmt.Sprintf("%s[%d:v]blend=all_mode=overlay:all_opacity=%.2f:enable='between(t,%.2f,%.2f)'%s",
					currentInput, overlayIndex, overlayOpacity, startTime, endTime, outputTag)
			}

			if filterComplex != "" {
				filterComplex += ";"
			}
			filterComplex += blendFilter
			currentInput = outputTag
		}
	}

	// Add audio mixing
	voiceIndex := len(preparedOverlays) + 1
	bgmIndex := len(preparedOverlays) + 2

	audioMix := fmt.Sprintf("[%d:a]volume=%.2f[voice];[%d:a]volume=%.2f[bgm];[voice][bgm]amix=inputs=2:duration=first:dropout_transition=0[final_audio]",
		voiceIndex, ve.Config.Settings.VoiceVolume, bgmIndex, ve.Config.Settings.BGMVolume)

	if filterComplex != "" {
		filterComplex += ";"
	}
	filterComplex += audioMix

	args = append(args, "-filter_complex", filterComplex)
	args = append(args, "-map", "[final_video]")
	args = append(args, "-map", "[final_audio]")
	args = append(args, "-c:v", "libx264")
	args = append(args, "-c:a", "aac")
	args = append(args, "-b:a", "128k")
	args = append(args, "-r", strconv.Itoa(ve.Config.Settings.FPS))
	args = append(args, "-shortest")
	args = append(args, finalForFFmpeg)

	log.Printf("FFmpeg final command with overlays: ffmpeg %s", strings.Join(args, " "))

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg final video with overlays output: %s", string(output))
		return fmt.Errorf("failed to generate final video with overlays: %v", err)
	}

	log.Printf("✓ Final video with overlays generated successfully")

	// Clean up prepared overlay files
	for _, overlay := range preparedOverlays {
		if err := os.Remove(overlay); err != nil {
			log.Printf("Warning: failed to clean up prepared overlay %s: %v", overlay, err)
		}
	}

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

// GenerateFinalVideoSimplified creates final video with proper path handling (original method)
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

	// Step 4: Generate final video with overlays
	log.Printf("🎬 Step 4: Generating final video with overlays...")
	if err := ve.GenerateFinalVideoWithOverlays(); err != nil {
		return fmt.Errorf("failed to generate final video: %v", err)
	}
	log.Printf("✅ Final video generated successfully!")

	return nil
}
