package engine

import (
	"fmt"
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

	// Get all voice files
	voiceFiles, err := utils.GetVoiceFiles(audioDir)
	if err != nil {
		return 0, err
	}

	if len(voiceFiles) == 0 {
		return 0, fmt.Errorf("no voice files found in %s", audioDir)
	}

	// Sort voice files to ensure proper order
	sort.Strings(voiceFiles)

	// Create temporary concat file list
	concatFile := filepath.Join(ve.OutputDir, "voice_list.txt")
	if err := utils.CreateConcatFile(voiceFiles, concatFile); err != nil {
		return 0, err
	}
	defer os.Remove(concatFile)

	// Merge voice files using FFmpeg
	cmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", concatFile, "-c", "copy", outputPath)
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("failed to merge voice files: %v", err)
	}

	// Get duration of merged voice file
	duration, err := utils.GetAudioDuration(outputPath)
	if err != nil {
		return 0, err
	}

	return duration, nil
}

// ExtendBackgroundMusic loops the background music to match voice duration
func (ve *VideoEditor) ExtendBackgroundMusic(targetDuration float64) error {
	bgmPath := filepath.Join(ve.InputDir, "audio", "background.mp3")
	outputPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")

	// Check if background music exists
	if _, err := os.Stat(bgmPath); os.IsNotExist(err) {
		return fmt.Errorf("background music file not found: %s", bgmPath)
	}

	// Get original BGM duration
	originalDuration, err := utils.GetAudioDuration(bgmPath)
	if err != nil {
		return err
	}

	// Calculate how many loops we need
	loops := int(targetDuration/originalDuration) + 1

	// Create FFmpeg command to loop and trim background music
	cmd := exec.Command("ffmpeg", "-y",
		"-stream_loop", strconv.Itoa(loops),
		"-i", bgmPath,
		"-t", fmt.Sprintf("%.2f", targetDuration),
		"-af", fmt.Sprintf("volume=%.2f", ve.Config.Settings.BGMVolume),
		outputPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extend background music: %v", err)
	}

	return nil
}

// CreateSlideshow generates a slideshow from images
func (ve *VideoEditor) CreateSlideshow(totalDuration float64) error {
	imagesDir := filepath.Join(ve.InputDir, "images")
	outputPath := filepath.Join(ve.OutputDir, "slideshow.mp4")

	// Get all image files
	imageFiles, err := utils.GetImageFiles(imagesDir)
	if err != nil {
		return err
	}

	if len(imageFiles) == 0 {
		return fmt.Errorf("no image files found in %s", imagesDir)
	}

	// Sort image files
	sort.Strings(imageFiles)

	// Calculate duration per image
	imageDuration := ve.Config.GetImageDuration(totalDuration, len(imageFiles))

	// Build FFmpeg command for slideshow
	var args []string
	args = append(args, "-y")

	// Add input images
	for _, img := range imageFiles {
		args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.2f", imageDuration), "-i", img)
	}

	// Build filter complex for slideshow with transitions
	filterComplex := ve.buildSlideshowFilter(len(imageFiles), imageDuration)
	args = append(args, "-filter_complex", filterComplex)

	// Output settings
	args = append(args,
		"-map", "[slideshow]",
		"-c:v", "libx264",
		"-r", strconv.Itoa(ve.Config.Settings.FPS),
		"-s", fmt.Sprintf("%dx%d", ve.Config.Settings.Width, ve.Config.Settings.Height),
		"-pix_fmt", "yuv420p",
		outputPath)

	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create slideshow: %v", err)
	}

	return nil
}

// GenerateFinalVideo combines slideshow with audio and applies overlays/effects
func (ve *VideoEditor) GenerateFinalVideo() error {
	slideshowPath := filepath.Join(ve.OutputDir, "slideshow.mp4")
	voicePath := filepath.Join(ve.OutputDir, "merged_voice.mp3")
	bgmPath := filepath.Join(ve.OutputDir, "extended_bgm.mp3")
	finalPath := filepath.Join(ve.OutputDir, "final_video.mp4")

	// Build complex FFmpeg command
	var args []string
	args = append(args, "-y")

	// Input files
	args = append(args, "-i", slideshowPath) // [0] - slideshow video
	args = append(args, "-i", voicePath)     // [1] - voice audio
	args = append(args, "-i", bgmPath)       // [2] - background music

	inputIndex := 3

	// Add overlay images
	overlayMap := make(map[string]int)
	for _, overlay := range ve.Config.Overlays {
		overlayPath := filepath.Join(ve.InputDir, "overlays", overlay.Source)
		if _, exists := overlayMap[overlay.Source]; !exists {
			args = append(args, "-i", overlayPath)
			overlayMap[overlay.Source] = inputIndex
			inputIndex++
		}
	}

	// Build filter complex
	filterComplex := ve.buildFinalVideoFilter(overlayMap)
	args = append(args, "-filter_complex", filterComplex)

	// Output settings
	args = append(args,
		"-map", "[final_video]",
		"-map", "[final_audio]",
		"-c:v", "libx264",
		"-c:a", "aac",
		"-preset", "medium",
		"-crf", "23",
		finalPath)

	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate final video: %v", err)
	}

	return nil
}

// buildSlideshowFilter creates the filter for slideshow with fade transitions
func (ve *VideoEditor) buildSlideshowFilter(imageCount int, imageDuration float64) string {
	var filters []string

	// Scale all inputs to the same size
	for i := 0; i < imageCount; i++ {
		filters = append(filters, fmt.Sprintf("[%d:v]scale=%d:%d,setsar=1[img%d]",
			i, ve.Config.Settings.Width, ve.Config.Settings.Height, i))
	}

	// Create transitions between images
	transitionDuration := 0.5 // 0.5 second fade transition
	if imageCount == 1 {
		filters = append(filters, "[img0]null[slideshow]")
	} else {
		// First transition
		offset := imageDuration - transitionDuration
		filters = append(filters, fmt.Sprintf("[img0][img1]xfade=transition=fade:duration=%.2f:offset=%.2f[fade1]",
			transitionDuration, offset))

		// Subsequent transitions
		for i := 2; i < imageCount; i++ {
			offset = float64(i-1)*imageDuration + (imageDuration - transitionDuration)
			filters = append(filters, fmt.Sprintf("[fade%d][img%d]xfade=transition=fade:duration=%.2f:offset=%.2f[fade%d]",
				i-1, i, transitionDuration, offset, i))
		}

		filters = append(filters, fmt.Sprintf("[fade%d]null[slideshow]", imageCount-1))
	}

	return strings.Join(filters, ";")
}

// buildFinalVideoFilter creates the complete filter for final video with overlays and text
func (ve *VideoEditor) buildFinalVideoFilter(overlayMap map[string]int) string {
	var filters []string

	// Start with the slideshow video
	currentVideo := "[0:v]"

	// Apply overlays
	overlayCount := 0
	for _, overlay := range ve.Config.Overlays {
		overlayCount++
		inputIndex := overlayMap[overlay.Source]

		// Generate overlay filter with keyframe animations
		overlayFilter := ve.generateOverlayFilter(inputIndex, overlay, currentVideo, overlayCount)
		filters = append(filters, overlayFilter)
		currentVideo = fmt.Sprintf("[overlay%d]", overlayCount)
	}

	// Apply text overlays
	textCount := 0
	for _, text := range ve.Config.Texts {
		textCount++
		textFilter := ve.generateTextFilter(text, currentVideo, textCount)
		filters = append(filters, textFilter)
		currentVideo = fmt.Sprintf("[text%d]", textCount)
	}

	// Final video output
	filters = append(filters, fmt.Sprintf("%s[final_video]", currentVideo))

	// Mix audio tracks
	voiceVolume := ve.Config.Settings.VoiceVolume
	filters = append(filters, fmt.Sprintf("[1:a]volume=%.2f[voice];[2:a][voice]amix=inputs=2:duration=first[final_audio]", voiceVolume))

	return strings.Join(filters, ";")
}

// generateOverlayFilter creates FFmpeg filter for animated overlay
func (ve *VideoEditor) generateOverlayFilter(inputIndex int, overlay models.Overlay, inputVideo string, overlayNum int) string {
	// Generate position expressions based on keyframes
	xExpr := ve.generateKeyframeExpression("x", overlay.Keyframes, overlay.Start, overlay.End)
	yExpr := ve.generateKeyframeExpression("y", overlay.Keyframes, overlay.Start, overlay.End)
	alphaExpr := ve.generateKeyframeExpression("opacity", overlay.Keyframes, overlay.Start, overlay.End)

	// Build overlay filter
	return fmt.Sprintf("[%d:v]format=rgba,colorchannelmixer=aa=%s[overlay_alpha%d];%s[overlay_alpha%d]overlay=x=%s:y=%s:enable='between(t,%.2f,%.2f)'[overlay%d]",
		inputIndex, alphaExpr, overlayNum, inputVideo, overlayNum, xExpr, yExpr, overlay.Start, overlay.End, overlayNum)
}

// generateTextFilter creates FFmpeg filter for animated text
func (ve *VideoEditor) generateTextFilter(text models.Text, inputVideo string, textNum int) string {
	// Generate position expressions based on keyframes
	xExpr := ve.generateKeyframeExpression("x", text.Keyframes, text.Start, text.End)
	yExpr := ve.generateKeyframeExpression("y", text.Keyframes, text.Start, text.End)
	alphaExpr := ve.generateKeyframeExpression("opacity", text.Keyframes, text.Start, text.End)

	// Build drawtext filter
	return fmt.Sprintf("%sdrawtext=text='%s':fontsize=%d:fontcolor=%s@%s:x=%s:y=%s:enable='between(t,%.2f,%.2f)'[text%d]",
		inputVideo, text.Text, text.FontSize, text.FontColor, alphaExpr, xExpr, yExpr, text.Start, text.End, textNum)
}

// generateKeyframeExpression creates FFmpeg expression for keyframe interpolation
func (ve *VideoEditor) generateKeyframeExpression(property string, keyframes []models.Keyframe, start, end float64) string {
	if len(keyframes) <= 1 {
		// If only one keyframe or none, use constant value
		if len(keyframes) == 1 {
			switch property {
			case "x":
				return fmt.Sprintf("%.2f", keyframes[0].X)
			case "y":
				return fmt.Sprintf("%.2f", keyframes[0].Y)
			case "opacity":
				return fmt.Sprintf("%.2f", keyframes[0].Opacity)
			case "scale":
				return fmt.Sprintf("%.2f", keyframes[0].Scale)
			}
		}
		return "0"
	}

	// Sort keyframes by time
	sortedKeyframes := make([]models.Keyframe, len(keyframes))
	copy(sortedKeyframes, keyframes)
	sort.Slice(sortedKeyframes, func(i, j int) bool {
		return sortedKeyframes[i].Time < sortedKeyframes[j].Time
	})

	// Build piecewise linear interpolation expression
	var conditions []string
	for i := 0; i < len(sortedKeyframes)-1; i++ {
		current := sortedKeyframes[i]
		next := sortedKeyframes[i+1]

		var currentVal, nextVal float64
		switch property {
		case "x":
			currentVal, nextVal = current.X, next.X
		case "y":
			currentVal, nextVal = current.Y, next.Y
		case "opacity":
			currentVal, nextVal = current.Opacity, next.Opacity
		case "scale":
			currentVal, nextVal = current.Scale, next.Scale
		}

		// Linear interpolation between keyframes
		if current.Time == next.Time {
			conditions = append(conditions, fmt.Sprintf("%.2f", nextVal))
		} else {
			slope := (nextVal - currentVal) / (next.Time - current.Time)
			intercept := currentVal - slope*current.Time
			conditions = append(conditions, fmt.Sprintf("if(between(t,%.2f,%.2f),%.2f*t+%.2f",
				current.Time, next.Time, slope, intercept))
		}
	}

	// Close all if conditions and add fallback
	expression := strings.Join(conditions, ",") + strings.Repeat(")", len(conditions)-1)

	// Add fallback value (last keyframe value)
	lastKeyframe := sortedKeyframes[len(sortedKeyframes)-1]
	var fallbackVal float64
	switch property {
	case "x":
		fallbackVal = lastKeyframe.X
	case "y":
		fallbackVal = lastKeyframe.Y
	case "opacity":
		fallbackVal = lastKeyframe.Opacity
	case "scale":
		fallbackVal = lastKeyframe.Scale
	}

	if len(conditions) > 1 {
		expression += fmt.Sprintf(",%.2f)", fallbackVal)
	} else {
		expression = fmt.Sprintf("%.2f", fallbackVal)
	}

	return expression
}
