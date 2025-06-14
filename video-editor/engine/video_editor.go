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

// ChromaKeyConfig holds configuration for chroma key removal
type ChromaKeyConfig struct {
	Enabled       bool    // Whether to apply chroma key removal
	Color         string  // Color to remove (e.g., "green", "blue", "#00FF00")
	Similarity    float64 // Color similarity threshold (0.0-1.0, default: 0.3)
	Blend         float64 // Edge blending amount (0.0-1.0, default: 0.1)
	YUVThreshold  float64 // YUV threshold for better keying (0.0-1.0, default: 0.0)
	AutoAdjust    bool    // Auto-adjust thresholds for better results
	SpillSuppress bool    // Enable spill suppression for better edge quality
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

// getDefaultChromaKeyConfig returns default chroma key settings
func (ve *VideoEditor) getDefaultChromaKeyConfig() ChromaKeyConfig {
	return ChromaKeyConfig{
		Enabled:       true,
		Color:         "green",
		Similarity:    0.3,
		Blend:         0.1,
		YUVThreshold:  0.0,
		AutoAdjust:    true,
		SpillSuppress: true,
	}
}

// getChromaKeyConfig gets chroma key config from video config or returns default
func (ve *VideoEditor) getChromaKeyConfig() ChromaKeyConfig {
	// Check if config has chroma key settings
	if ve.Config.Settings.ChromaKey != nil {
		return ChromaKeyConfig{
			Enabled:       ve.Config.Settings.ChromaKey.Enabled,
			Color:         ve.Config.Settings.ChromaKey.Color,
			Similarity:    ve.Config.Settings.ChromaKey.Similarity,
			Blend:         ve.Config.Settings.ChromaKey.Blend,
			YUVThreshold:  ve.Config.Settings.ChromaKey.YUVThreshold,
			AutoAdjust:    ve.Config.Settings.ChromaKey.AutoAdjust,
			SpillSuppress: ve.Config.Settings.ChromaKey.SpillSuppress,
		}
	}

	// Return default config if not specified
	return ve.getDefaultChromaKeyConfig()
}

func (ve *VideoEditor) createChromaKeyFilter(config ChromaKeyConfig, width, height int, overlayOpacity float64) string {
	if !config.Enabled {
		// Return basic scaling filter without chroma key but with alpha support
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black@0.0,format=yuva420p,colorchannelmixer=aa=%.2f",
			width, height, width, height, overlayOpacity)
	}

	var chromaFilter string

	// Determine color value based on config
	colorValue := config.Color
	switch strings.ToLower(config.Color) {
	case "green":
		colorValue = "0x00FF00"
	case "blue":
		colorValue = "0x0000FF"
	case "red":
		colorValue = "0xFF0000"
	case "white":
		colorValue = "0xFFFFFF"
	case "black":
		colorValue = "0x000000"
	default:
		// Use the color as-is if it's a hex value or color name
		if !strings.HasPrefix(config.Color, "0x") && !strings.HasPrefix(config.Color, "#") {
			colorValue = config.Color
		}
	}

	// Build chroma key filter with advanced settings
	if config.AutoAdjust {
		// Use chromakey filter with auto-adjustment
		chromaFilter = fmt.Sprintf("chromakey=color=%s:similarity=%.3f:blend=%.3f:yuv=1",
			colorValue, config.Similarity, config.Blend)
	} else {
		// Use basic colorkey filter
		chromaFilter = fmt.Sprintf("colorkey=color=%s:similarity=%.3f:blend=%.3f",
			colorValue, config.Similarity, config.Blend)
	}

	// Add spill suppression if enabled
	var spillFilter string
	if config.SpillSuppress && strings.ToLower(config.Color) == "green" {
		spillFilter = ",despill=type=green:mix=0.7:expand=0.1"
	} else if config.SpillSuppress && strings.ToLower(config.Color) == "blue" {
		spillFilter = ",despill=type=blue:mix=0.7:expand=0.1"
	}

	// Combine all filters with proper transparency handling
	fullFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black@0.0,%s%s,format=yuva420p,colorchannelmixer=aa=%.2f",
		width, height, width, height, chromaFilter, spillFilter, overlayOpacity)

	return fullFilter
}

// detectChromaKeyInVideo analyzes video to detect if it contains chroma key background
func (ve *VideoEditor) detectChromaKeyInVideo(videoPath string) (bool, string, error) {
	// Use ffprobe to analyze the video and detect dominant colors
	args := []string{
		"-v", "quiet",
		"-select_streams", "v:0",
		"-show_entries", "frame=pkt_pts_time",
		"-of", "csv=p=0",
		"-read_intervals", "%+#1", // Analyze first frame only
		videoPath,
	}

	cmd := exec.Command("ffprobe", args...)
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Warning: Could not analyze video for chroma key detection: %v", err)
		return false, "", nil
	}

	// For now, we'll use a simple heuristic
	// In a production environment, you might want to use more sophisticated analysis
	videoPathLower := strings.ToLower(filepath.Base(videoPath))

	// Check filename for common indicators
	if strings.Contains(videoPathLower, "green") || strings.Contains(videoPathLower, "greenscreen") || strings.Contains(videoPathLower, "chroma") {
		return true, "green", nil
	}
	if strings.Contains(videoPathLower, "blue") || strings.Contains(videoPathLower, "bluescreen") {
		return true, "blue", nil
	}

	// TODO: Implement actual color analysis using ffmpeg histogram analysis
	// This would involve extracting a frame and analyzing its color distribution

	return false, "", nil
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

	// Calculate image duration based on configuration
	var imageDuration float64
	var actualImagesDuration float64

	if ve.Config.Settings.UsePartialImageDuration && ve.Config.Settings.ImagesDurationMinutes > 0 {
		// Mode 2: Use specified duration for all images (convert minutes to seconds)
		actualImagesDuration = ve.Config.Settings.ImagesDurationMinutes * 60.0
		imageDuration = actualImagesDuration / float64(len(imageFiles))
		log.Printf("Using partial image duration: %.2f minutes (%.2f seconds) for all images",
			ve.Config.Settings.ImagesDurationMinutes, actualImagesDuration)
	} else {
		// Mode 1: Spread images across entire duration (current behavior)
		actualImagesDuration = targetDuration
		imageDuration = targetDuration / float64(len(imageFiles))
		log.Printf("Using full video duration for images: %.2f seconds", actualImagesDuration)
	}

	log.Printf("Duration per image: %.2f seconds", imageDuration)

	// Check animation settings
	useAnimations := ve.Config.Settings.UseAnimationEffects
	log.Printf("Animation effects: %s", map[bool]string{true: "ENABLED", false: "DISABLED"}[useAnimations])

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

	// Ensure output directory exists
	if err := os.MkdirAll(ve.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// CONCURRENT PROCESSING: Create individual video clips
	var videoClips []string
	videoClips = make([]string, len(imageFiles))

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

			var videoFilter string
			var effectName string

			if useAnimations {
				// Generate zoom configuration and filter
				zoomConfig := ve.generateZoomConfig(index)
				videoFilter = ve.createZoomFilter(zoomConfig, imageDuration, ve.Config.Settings.Width, ve.Config.Settings.Height)
				effectName = ve.getEffectName(zoomConfig.Effect)
			} else {
				// Simple static filter without animation
				videoFilter = ve.createStaticFilter(ve.Config.Settings.Width, ve.Config.Settings.Height)
				effectName = "static"
			}

			log.Printf("Creating clip %d with %s effect (%s)...", index+1, effectName,
				map[bool]string{true: "GPU", false: "CPU"}[ve.UseGPU])

			// Build args with GPU/CPU encoder
			args := []string{
				"-y",
				"-loop", "1",
				"-i", imgPathForFFmpeg,
				"-vf", videoFilter,
				"-t", fmt.Sprintf("%.2f", imageDuration),
				"-c:v", encoder,
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
				args = append(args, "-threads", "1")
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

	// Remove any empty entries
	var validClips []string
	for _, clip := range videoClips {
		if clip != "" {
			validClips = append(validClips, clip)
		}
	}
	videoClips = validClips

	// Concatenate all clips
	if err := ve.concatenateClips(videoClips, outputPath); err != nil {
		return fmt.Errorf("failed to concatenate clips: %v", err)
	}

	// If using partial duration, add black screen for remaining time
	if ve.Config.Settings.UsePartialImageDuration && actualImagesDuration < targetDuration {
		blackScreenDuration := targetDuration - actualImagesDuration
		log.Printf("Adding black screen for remaining %.2f seconds (%.2f minutes)",
			blackScreenDuration, blackScreenDuration/60.0)

		if err := ve.addBlackScreenToSlideshow(blackScreenDuration); err != nil {
			return fmt.Errorf("failed to add black screen: %v", err)
		}
	}

	log.Printf("✓ Slideshow created successfully with %s effects",
		map[bool]string{true: "animated", false: "static"}[useAnimations])

	// Clean up temporary files
	for _, clip := range videoClips {
		if err := os.Remove(clip); err != nil {
			log.Printf("Warning: failed to clean up clip %s: %v", clip, err)
		}
	}

	// Verify output file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return fmt.Errorf("slideshow output file was not created: %s", outputPath)
	}

	// Check actual duration
	if actualDuration, err := ve.getVideoDuration(outputPath); err == nil {
		log.Printf("Actual slideshow duration: %.2f seconds (%.2f minutes)",
			actualDuration, actualDuration/60.0)
	}

	return nil
}

// PrepareOverlayVideo processes a single overlay video to fit the screen and loop for the required duration
// Now includes chroma key removal functionality
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

	// Get chroma key configuration
	chromaConfig := ve.getChromaKeyConfig()

	// Auto-detect chroma key if enabled
	if chromaConfig.Enabled && chromaConfig.Color == "auto" {
		detected, detectedColor, err := ve.detectChromaKeyInVideo(overlayPath)
		if err != nil {
			log.Printf("Warning: Failed to auto-detect chroma key for %s: %v", overlayPath, err)
		} else if detected {
			chromaConfig.Color = detectedColor
			log.Printf("Auto-detected chroma key color '%s' in overlay %d", detectedColor, index)
		} else {
			log.Printf("No chroma key detected in overlay %d, disabling chroma key removal", index)
			chromaConfig.Enabled = false
		}
	}

	// Log chroma key status
	if chromaConfig.Enabled {
		log.Printf("Applying chroma key removal to overlay %d (color: %s, similarity: %.2f)",
			index, chromaConfig.Color, chromaConfig.Similarity)
	}

	// Get encoder settings
	encoder, encoderArgs := ve.getEncoderSettings()

	// Create video filter with chroma key support
	videoFilter := ve.createChromaKeyFilter(chromaConfig, ve.Config.Settings.Width, ve.Config.Settings.Height, overlayOpacity)

	if originalDuration >= duration {
		// Overlay is longer than needed, just trim it
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
			"-pix_fmt", "yuva420p", // Use yuva420p for alpha channel support
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
			"-pix_fmt", "yuva420p", // Use yuva420p for alpha channel support
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

	// Log success with chroma key status
	chromaStatus := ""
	if chromaConfig.Enabled {
		chromaStatus = fmt.Sprintf(" with %s chroma key removal", chromaConfig.Color)
	}
	log.Printf("✓ Overlay video %d prepared successfully with %.0f%% opacity%s",
		index, overlayOpacity*100, chromaStatus)

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

	log.Printf("Preparing %d overlay videos concurrently with chroma key support", len(overlayVideos))

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

func (ve *VideoEditor) GenerateFinalVideoWithOverlays() error {
	slideshowPath := filepath.Join(ve.OutputDir, "slideshow.mp4")
	voicePath := filepath.Join(ve.OutputDir, "merged_voice.mp3")
	bgmPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")
	finalPath := filepath.Join(ve.OutputDir, "final_video.mp4")

	log.Printf("Generating final video with overlays and chroma key support...")
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

	// CONCURRENT OVERLAY PREPARATION WITH CHROMA KEY
	preparedOverlays, err := ve.PrepareOverlayVideosConcurrently(overlayVideos, slideshowDuration)
	if err != nil {
		return fmt.Errorf("failed to prepare overlay videos with chroma key: %v", err)
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

	// Add final encoding parameters
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

	log.Printf("Executing final video generation with %d chroma key overlays...", len(preparedOverlays))

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("FFmpeg final video generation output: %s", string(output))
		return fmt.Errorf("failed to generate final video with overlays: %v", err)
	}

	// Verify final video was created
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		return fmt.Errorf("final video file was not created: %s", finalPath)
	}

	// Check final video duration
	if finalDuration, err := ve.getVideoDuration(finalPath); err == nil {
		log.Printf("Final video duration: %.2f seconds (%.2f minutes)", finalDuration, finalDuration/60.0)
	}

	log.Printf("✓ Final video with chroma key overlays generated successfully: %s", finalPath)

	// Clean up prepared overlay files
	for i, overlay := range preparedOverlays {
		if err := os.Remove(overlay); err != nil {
			log.Printf("Warning: failed to clean up prepared overlay %d: %v", i, err)
		}
	}

	return nil
}

// getPresetForEncoder returns appropriate preset based on encoder type
func (ve *VideoEditor) getPresetForEncoder(encoder string) string {
	switch encoder {
	case "h264_nvenc", "hevc_nvenc":
		return "fast" // NVIDIA presets: ultrafast, superfast, veryfast, faster, fast, medium, slow, slower, veryslow
	case "h264_qsv", "hevc_qsv":
		return "fast" // Intel QSV presets
	case "h264_videotoolbox", "hevc_videotoolbox":
		return "fast" // Apple VideoToolbox presets
	case "libx264", "libx265":
		if ve.isGoogleColab() {
			return "ultrafast" // Fastest preset for Colab
		}
		return "fast" // CPU presets
	default:
		return "fast"
	}
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
