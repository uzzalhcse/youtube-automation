package engine

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"youtube_automation/video-editor/models"
	"youtube_automation/video-editor/utils"
)

// ZoomEffect represents different zoom animation types
type ZoomEffect int

const (
	ZoomIn ZoomEffect = iota
	ZoomOut
	ZoomInOut
	PanZoom
)

// ZoomConfig holds configuration for zoom animations
type ZoomConfig struct {
	Effect     ZoomEffect
	StartScale float64 // Starting zoom scale (1.0 = normal)
	EndScale   float64 // Ending zoom scale
	StartX     float64 // Starting X position (0.5 = center)
	StartY     float64 // Starting Y position (0.5 = center)
	EndX       float64 // Ending X position
	EndY       float64 // Ending Y position
}

// Add these fields to VideoEditor struct
type VideoEditor struct {
	InputDir   string
	OutputDir  string
	Config     *models.VideoConfig
	MaxWorkers int           // Add this field for configurable concurrency
	WorkerPool chan struct{} // Add this field for worker pool
	UseGPU     bool          // NEW: GPU rendering flag
	GPUDevice  string        // NEW: GPU device (e.g., "0" for first GPU, "cuda", "opencl")
}

// Modified NewVideoEditor constructor for Colab
func NewVideoEditor(inputDir, outputDir string, config *models.VideoConfig) *VideoEditor {
	maxWorkers := runtime.NumCPU()
	if config.Settings.MaxConcurrentJobs > 0 {
		maxWorkers = config.Settings.MaxConcurrentJobs
	}

	// Initialize GPU settings
	useGPU := config.Settings.UseGPU
	gpuDevice := "0"
	if config.Settings.GPUDevice != "" {
		gpuDevice = config.Settings.GPUDevice
	}

	ve := &VideoEditor{
		InputDir:   inputDir,
		OutputDir:  outputDir,
		Config:     config,
		MaxWorkers: maxWorkers,
		WorkerPool: make(chan struct{}, maxWorkers),
		UseGPU:     useGPU,
		GPUDevice:  gpuDevice,
	}

	// NEW: Optimize for Colab environment
	ve.optimizeForColab()

	return ve
}

// Simplified createZoomFilter that doesn't rely on CUDA filters
func (ve *VideoEditor) createZoomFilter(config ZoomConfig, duration float64, width, height int) string {
	// Calculate frames for smooth animation
	totalFrames := int(duration * float64(ve.Config.Settings.FPS))

	// Adjust scale based on GPU availability and Colab constraints
	inputScale := 2.0
	if ve.isGoogleColab() {
		// Reduce scale in Colab to manage memory
		inputScale = 1.5
	}

	scaledWidth := int(float64(width) * inputScale)
	scaledHeight := int(float64(height) * inputScale)

	var baseFilter string

	// Use standard filters regardless of GPU - encoding will use GPU
	switch config.Effect {
	case ZoomIn:
		zoomIncrement := ve.Config.Settings.ZoomSpeed * ve.Config.Settings.TransitionSmooth
		baseFilter = fmt.Sprintf("scale=%d:%d,zoompan=z='min(1+%.6f*frame,%.3f)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d",
			scaledWidth, scaledHeight,
			zoomIncrement, config.EndScale, totalFrames, width, height)

	case ZoomOut:
		zoomDecrement := ve.Config.Settings.ZoomSpeed * ve.Config.Settings.TransitionSmooth
		baseFilter = fmt.Sprintf("scale=%d:%d,zoompan=z='max(%.3f-%.6f*frame,1.0)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d",
			scaledWidth, scaledHeight,
			config.StartScale, zoomDecrement, totalFrames, width, height)

	case ZoomInOut:
		// Zoom in-out effect: zoom in first half, zoom out second half
		halfFrames := totalFrames / 2
		zoomPeak := config.StartScale + 0.3*ve.Config.Settings.ZoomIntensity

		baseFilter = fmt.Sprintf("scale=%d:%d,zoompan=z='if(lt(frame,%d),1+%.6f*frame,%.3f-%.6f*(frame-%d))':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d",
			scaledWidth, scaledHeight,
			halfFrames, zoomPeak/float64(halfFrames), zoomPeak, zoomPeak/float64(halfFrames), halfFrames,
			totalFrames, width, height)

	case PanZoom:
		// Pan and zoom effect with smooth movement
		baseFilter = fmt.Sprintf("scale=%d:%d,zoompan=z='%.3f+%.6f*frame':d=%d:x='iw*%.3f+(iw*%.3f-iw*%.3f)*frame/%d':y='ih*%.3f+(ih*%.3f-ih*%.3f)*frame/%d':s=%dx%d",
			scaledWidth, scaledHeight,
			config.StartScale, (config.EndScale-config.StartScale)/float64(totalFrames), totalFrames,
			config.StartX, config.EndX, config.StartX, totalFrames,
			config.StartY, config.EndY, config.StartY, totalFrames,
			width, height)

	default:
		// Default zoom in effect
		baseFilter = fmt.Sprintf("scale=%d:%d,zoompan=z='min(1+%.6f*frame,%.3f)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d",
			scaledWidth, scaledHeight,
			ve.Config.Settings.ZoomSpeed, ve.Config.Settings.ZoomIntensity,
			totalFrames, width, height)
	}

	return baseFilter
}

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
	sort.Strings(imageFiles)
	encoder, encoderArgs := ve.getEncoderSettings()

	// Log GPU/CPU usage
	if ve.UseGPU {
		log.Printf("🚀 Using GPU acceleration with encoder: %s", encoder)
	} else {
		log.Printf("🖥️ Using CPU encoding with encoder: %s", encoder)
	}
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
	}

	// Calculate duration per image based on target duration
	imageDuration := targetDuration / float64(len(imageFiles))
	log.Printf("Duration per image: %.2f seconds (total: %.2f seconds)", imageDuration, targetDuration)

	// Ensure output directory exists
	if err := os.MkdirAll(ve.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// CONCURRENT PROCESSING: Create individual video clips with zoom animations
	var videoClips []string
	videoClips = make([]string, len(imageFiles)) // Pre-allocate slice

	var wg sync.WaitGroup
	var mu sync.Mutex
	errorChan := make(chan error, len(imageFiles))

	log.Printf("Using %d concurrent workers for clip generation", ve.MaxWorkers)

	// Process clips concurrently
	for i, img := range imageFiles {
		wg.Add(1)
		go func(index int, imgPath string) {
			defer wg.Done()

			// Limit concurrent workers
			ve.WorkerPool <- struct{}{}
			defer func() { <-ve.WorkerPool }()

			clipPath := filepath.Join(ve.OutputDir, fmt.Sprintf("clip_%d.mp4", index))
			imgPathForFFmpeg := strings.ReplaceAll(imgPath, "\\", "/")
			clipPathForFFmpeg := strings.ReplaceAll(clipPath, "\\", "/")

			// Generate zoom configuration and filter
			zoomConfig := ve.generateZoomConfig(index)
			zoomFilter := ve.createZoomFilter(zoomConfig, imageDuration, ve.Config.Settings.Width, ve.Config.Settings.Height)

			log.Printf("Creating clip %d with %s animation (%s)...", index+1, ve.getEffectName(zoomConfig.Effect),
				map[bool]string{true: "GPU", false: "CPU"}[ve.UseGPU])

			// NEW: Build args with GPU/CPU encoder
			args := []string{
				"-y",
				"-loop", "1",
				"-i", imgPathForFFmpeg,
				"-vf", zoomFilter,
				"-t", fmt.Sprintf("%.2f", imageDuration),
				"-c:v", encoder, // Use selected encoder
			}

			// Add encoder-specific arguments
			args = append(args, encoderArgs...)

			// Add common arguments
			args = append(args,
				"-r", strconv.Itoa(ve.Config.Settings.FPS),
				"-pix_fmt", "yuv420p",
				"-avoid_negative_ts", "make_zero",
			)

			// Adjust thread count based on GPU/CPU
			if !ve.UseGPU {
				args = append(args, "-threads", "1") // Limit threads for CPU
			}

			args = append(args, clipPathForFFmpeg)

			cmd := exec.Command("ffmpeg", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("FFmpeg clip creation output: %s", string(output))
				errorChan <- fmt.Errorf("failed to create clip %d: %v", index, err)
				return
			}

			// Verify clip was created
			if _, err := os.Stat(clipPath); os.IsNotExist(err) {
				errorChan <- fmt.Errorf("clip file was not created: %s", clipPath)
				return
			}

			// Thread-safe assignment to slice
			mu.Lock()
			videoClips[index] = clipPath
			mu.Unlock()
		}(i, img)
	}

	// Wait for all clips to be created
	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		if err != nil {
			return err
		}
	}

	// Remove any empty entries (shouldn't happen, but safety check)
	var validClips []string
	for _, clip := range videoClips {
		if clip != "" {
			validClips = append(validClips, clip)
		}
	}
	videoClips = validClips

	// Create a list file for concatenation (rest remains the same)
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

	log.Printf("Concatenating %d animated clips...", len(videoClips))

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

	log.Printf("✓ Animated slideshow created successfully using concurrent processing")

	// Clean up temporary files
	for _, clip := range videoClips {
		if err := os.Remove(clip); err != nil {
			log.Printf("Warning: failed to clean up clip %s: %v", clip, err)
		}
	}
	if err := os.Remove(listFile); err != nil {
		log.Printf("Warning: failed to clean up list file: %v", err)
	}

	// Verify output file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return fmt.Errorf("slideshow output file was not created: %s", outputPath)
	}

	// Check actual duration
	if actualDuration, err := ve.getVideoDuration(outputPath); err == nil {
		log.Printf("Actual slideshow duration: %.2f seconds", actualDuration)
	}

	return nil
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
	// Get encoder settings
	encoder, encoderArgs := ve.getEncoderSettings()
	var videoFilter string
	if originalDuration >= duration {
		args := []string{
			"-y",
			"-i", overlayForFFmpeg,
			"-vf", videoFilter,
			"-t", fmt.Sprintf("%.2f", duration),
			"-c:v", encoder, // Use selected encoder
		}

		// Add encoder-specific arguments
		args = append(args, encoderArgs...)
		args = append(args,
			"-r", strconv.Itoa(ve.Config.Settings.FPS),
			"-pix_fmt", "yuv420p",
			"-an",
			outputForFFmpeg,
		)

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
			"-stream_loop", strconv.Itoa(loopCount),
			"-i", overlayForFFmpeg,
			"-vf", videoFilter,
			"-t", fmt.Sprintf("%.2f", duration),
			"-c:v", encoder, // Use selected encoder
		}

		// Add encoder-specific arguments
		args = append(args, encoderArgs...)
		args = append(args,
			"-r", strconv.Itoa(ve.Config.Settings.FPS),
			"-pix_fmt", "yuva420p",
			"-an",
			outputForFFmpeg,
		)

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

// Add concurrent overlay preparation method
func (ve *VideoEditor) PrepareOverlayVideosConcurrently(overlayVideos []string, duration float64) ([]string, error) {
	if len(overlayVideos) == 0 {
		return []string{}, nil
	}

	var preparedOverlays []string
	preparedOverlays = make([]string, len(overlayVideos))

	var wg sync.WaitGroup
	var mu sync.Mutex
	errorChan := make(chan error, len(overlayVideos))

	log.Printf("Preparing %d overlay videos concurrently", len(overlayVideos))

	for i, overlayPath := range overlayVideos {
		wg.Add(1)
		go func(index int, path string) {
			defer wg.Done()

			// Limit concurrent workers
			ve.WorkerPool <- struct{}{}
			defer func() { <-ve.WorkerPool }()

			preparedOverlay, err := ve.PrepareOverlayVideo(path, duration, index)
			if err != nil {
				errorChan <- fmt.Errorf("failed to prepare overlay %d: %v", index, err)
				return
			}

			mu.Lock()
			preparedOverlays[index] = preparedOverlay
			mu.Unlock()
		}(i, overlayPath)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		if err != nil {
			return nil, err
		}
	}

	// Remove empty entries
	var validOverlays []string
	for _, overlay := range preparedOverlays {
		if overlay != "" {
			validOverlays = append(validOverlays, overlay)
		}
	}

	return validOverlays, nil
}

// Fix the GenerateFinalVideoWithOverlays method to use consistent encoder
func (ve *VideoEditor) GenerateFinalVideoWithOverlays() error {
	slideshowPath := filepath.Join(ve.OutputDir, "slideshow.mp4")
	voicePath := filepath.Join(ve.OutputDir, "merged_voice.mp3")
	bgmPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")
	finalPath := filepath.Join(ve.OutputDir, "final_video.mp4")

	log.Printf("Generating final video with overlays...")

	// Get overlay opacity from config
	overlayOpacity := 0.3
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

	if len(overlayVideos) == 0 {
		log.Printf("No overlay videos found, using simplified generation")
		return ve.GenerateFinalVideoSimplified()
	}

	// CONCURRENT OVERLAY PREPARATION
	preparedOverlays, err := ve.PrepareOverlayVideosConcurrently(overlayVideos, slideshowDuration)
	if err != nil {
		return fmt.Errorf("failed to prepare overlays concurrently: %v", err)
	}

	if len(preparedOverlays) == 0 {
		log.Printf("No overlay videos could be prepared, using simplified generation")
		return ve.GenerateFinalVideoSimplified()
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

	// Get encoder settings for final video - CONSISTENT ENCODER
	encoder, encoderArgs := ve.getOptimalEncoderSettings()
	log.Printf("Using encoder: %s for final video generation", encoder)

	// Build FFmpeg command with overlays
	var args []string
	var filterComplex string

	args = append(args, "-y")
	args = append(args, "-i", slideshowForFFmpeg)

	for _, overlay := range preparedOverlays {
		overlayForFFmpeg := strings.ReplaceAll(overlay, "\\", "/")
		args = append(args, "-i", overlayForFFmpeg)
	}

	args = append(args, "-i", voiceForFFmpeg)
	args = append(args, "-i", bgmForFFmpeg)

	// Build filter complex for overlays
	baseVideo := "[0:v]"

	if len(preparedOverlays) == 1 {
		filterComplex = fmt.Sprintf("%s[1:v]blend=all_mode=overlay:all_opacity=%.2f,format=yuv420p[final_video]",
			baseVideo, overlayOpacity)
	} else {
		currentInput := baseVideo
		for i := 0; i < len(preparedOverlays); i++ {
			overlayIndex := i + 1
			outputTag := fmt.Sprintf("[overlay%d]", i)

			if i == len(preparedOverlays)-1 {
				outputTag = "[final_video]"
			}

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

	// Build the final FFmpeg command with consistent encoder
	args = append(args, "-filter_complex", filterComplex)
	args = append(args, "-map", "[final_video]")
	args = append(args, "-map", "[final_audio]")
	args = append(args, "-c:v", encoder) // Use selected encoder consistently

	// Add encoder-specific arguments
	args = append(args, encoderArgs...)

	// Add common arguments
	args = append(args,
		"-c:a", "aac",
		"-b:a", "128k",
		"-r", strconv.Itoa(ve.Config.Settings.FPS),
		"-shortest",
		finalForFFmpeg,
	)

	log.Printf("FFmpeg final command with overlays: ffmpeg %s", strings.Join(args, " "))

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg final video with overlays output: %s", string(output))
		return fmt.Errorf("failed to generate final video with overlays: %v", err)
	}

	log.Printf("✓ Final video with overlays generated successfully using concurrent processing")

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

// Also update the GenerateFinalVideoSimplified method to use consistent encoder
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

	// Get encoder settings for consistency
	encoder, encoderArgs := ve.getOptimalEncoderSettings()
	log.Printf("Using encoder: %s for simplified final video", encoder)

	// Build FFmpeg command args
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
	}

	// Add encoder and its settings
	if encoder == "h264_nvenc" && ve.UseGPU {
		// Use GPU encoding - re-encode video for consistency
		args = append(args, "-c:v", encoder)
		args = append(args, encoderArgs...)
	} else {
		// Use CPU encoding or copy stream if already compatible
		args = append(args, "-c:v", "copy") // Copy video stream if possible
	}

	// Add audio encoding settings
	args = append(args,
		"-c:a", "aac",
		"-b:a", "128k",
		"-shortest", // End when shortest stream ends
		finalForFFmpeg,
	)

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
	log.Printf("🎬 Starting video processing with zoom animations...")
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

	// Step 3: Create animated slideshow with zoom effects
	log.Printf("🖼️ Step 3: Creating animated slideshow with zoom effects...")
	if err := ve.CreateSlideshow(voiceDuration); err != nil {
		return fmt.Errorf("failed to create slideshow: %v", err)
	}
	log.Printf("✅ Animated slideshow created")

	// Step 4: Generate final video with overlays
	log.Printf("🎬 Step 4: Generating final video with overlays...")
	if err := ve.GenerateFinalVideoWithOverlays(); err != nil {
		return fmt.Errorf("failed to generate final video: %v", err)
	}
	log.Printf("✅ Final video with zoom animations generated successfully!")

	return nil
}
