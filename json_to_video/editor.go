package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func buildFilterComplexWithCounts(req *VideoRequest, videoInputCount int, audioInputs []string) string {
	var filters []string
	currentVideo := "[0:v]"
	hasProcessedVideo := false
	videoInputIndex := 1

	for _, img := range req.Images {
		if (img.Data != "" || img.URL != "") && img.Duration > 0 && videoInputIndex < videoInputCount {
			hasProcessedVideo = true

			var scaleFilter string
			var overlayFilter string

			if img.KenBurns.Enabled {
				// Calculate duration in frames with float precision (25fps)
				durationFrames := int(img.Duration * 25)

				kenBurnsFilter := fmt.Sprintf("[%d:v]scale=%d:-1,zoompan=z='zoom+%f':x=%s:y=%s:d=%d:s=%dx%d:fps=25[kb%d]",
					videoInputIndex, img.KenBurns.ScaleWidth, img.KenBurns.ZoomRate,
					img.KenBurns.PanX, img.KenBurns.PanY, durationFrames, req.Width, req.Height, videoInputIndex)

				// Use float precision in timing
				overlayFilter = fmt.Sprintf("%s[kb%d]overlay=0:0:enable='between(t,%.3f,%.3f)'[v%d]",
					currentVideo, videoInputIndex, img.StartTime, img.StartTime+img.Duration, videoInputIndex)

				filters = append(filters, kenBurnsFilter)
				filters = append(filters, overlayFilter)
			} else {
				// Regular image processing with float timing
				isFullscreen := (img.Width == req.Width && img.Height == req.Height)

				if isFullscreen || (img.X == 0 && img.Y == 0 && img.Width >= req.Width && img.Height >= req.Height) {
					scaleFilter = fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d[scaled%d]",
						videoInputIndex, req.Width, req.Height, req.Width, req.Height, videoInputIndex)

					overlayFilter = fmt.Sprintf("%s[scaled%d]overlay=0:0:enable='between(t,%.3f,%.3f)'[v%d]",
						currentVideo, videoInputIndex, img.StartTime, img.StartTime+img.Duration, videoInputIndex)
				} else {
					scaleFilter = fmt.Sprintf("[%d:v]scale=%d:%d[scaled%d]",
						videoInputIndex, img.Width, img.Height, videoInputIndex)

					overlayFilter = fmt.Sprintf("%s[scaled%d]overlay=%d:%d:enable='between(t,%.3f,%.3f)'[v%d]",
						currentVideo, videoInputIndex, img.X, img.Y, img.StartTime, img.StartTime+img.Duration, videoInputIndex)
				}

				filters = append(filters, scaleFilter)
				filters = append(filters, overlayFilter)
			}

			currentVideo = fmt.Sprintf("[v%d]", videoInputIndex)
			videoInputIndex++
		}
	}
	// Process text overlays with float timing
	sceneIndex := 0
	hasProcessedText := false
	for _, scene := range req.Scenes {
		if scene.Text == "" {
			continue
		}

		hasProcessedText = true
		fontSize := scene.FontSize
		if fontSize <= 0 {
			fontSize = 24
		}

		fontColor := scene.FontColor
		if fontColor == "" {
			fontColor = "white"
		}

		x, y := getTextPosition(scene.Position, req.Width, req.Height, scene.X, scene.Y)

		// Use float precision for scene timing
		textFilter := fmt.Sprintf("%sdrawtext=text='%s':fontsize=%d:fontcolor=%s:x=%s:y=%s:enable='between(t,%.3f,%.3f)'[vt%d]",
			currentVideo,
			strings.ReplaceAll(scene.Text, "'", "\\'"),
			fontSize, fontColor, x, y,
			scene.StartTime, scene.StartTime+scene.Duration, sceneIndex)

		filters = append(filters, textFilter)
		currentVideo = fmt.Sprintf("[vt%d]", sceneIndex)
		sceneIndex++
	}

	// Ensure we always have a final video output labeled [v]
	if hasProcessedVideo || hasProcessedText {
		// If we processed video or text, rename the current output to [v]
		if len(filters) > 0 {
			lastFilter := filters[len(filters)-1]
			// Replace the last output label with [v]
			if strings.Contains(lastFilter, "[v") {
				// Find the last bracket pair and replace it
				lastBracketStart := strings.LastIndex(lastFilter, "[")
				lastBracketEnd := strings.LastIndex(lastFilter, "]")
				if lastBracketStart > 0 && lastBracketEnd > lastBracketStart {
					filters[len(filters)-1] = lastFilter[:lastBracketStart] + "[v]"
				}
			}
		}
	} else {
		// If we didn't process any images or text, just copy the background
		filters = append(filters, "[0:v]copy[v]")
	}

	// Handle audio mixing (existing logic remains the same)
	if len(audioInputs) > 0 {
		if len(audioInputs) == 1 {
			audioFilter := fmt.Sprintf("%samix=inputs=1[a]", audioInputs[0])
			filters = append(filters, audioFilter)
		} else {
			audioFilter := fmt.Sprintf("%samix=inputs=%d[a]", strings.Join(audioInputs, ""), len(audioInputs))
			filters = append(filters, audioFilter)
		}
	}

	return strings.Join(filters, ";")
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
		fmt.Println("❌ Job failed:", job.Error)
		return
	}
	job.Progress = 90

	// Cleanup temp files
	cleanupTempFiles(jobID)

	job.Status = "completed"
	job.Progress = 100
	job.VideoPath = outputPath
}

func buildFFmpegCommand(jobID string, req *VideoRequest) ([]string, error) {
	tempDir := filepath.Join("temp", jobID)
	args := []string{}
	videoInputCount := 0
	audioInputCount := 0

	// Detect GPU first
	hasGPU, encoderArgs := detectGPUAcceleration()

	// Add hardware acceleration
	if hasGPU && len(encoderArgs) >= 2 && encoderArgs[0] == "-hwaccel" {
		args = append(args, "-hwaccel", encoderArgs[1])
		if encoderArgs[1] == "qsv" {
			args = append(args, "-hwaccel_output_format", "qsv")
		}
	}

	// Background with float duration
	if req.Background != "" && req.Background[0] == '#' {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("color=%s:size=%dx%d:duration=%.3f:rate=25",
				req.Background, req.Width, req.Height, req.Duration)) // Use %.3f for float
		videoInputCount++
	}

	// Add image inputs with float duration
	imageCount := 0
	for i, img := range req.Images {
		if img.Data != "" {
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.3f", img.Duration), // Use %.3f for float
				"-i", filepath.Join(tempDir, fmt.Sprintf("image_%d.png", i)))
			videoInputCount++
			imageCount++
		} else if img.URL != "" {
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.3f", img.Duration), // Use %.3f for float
				"-i", img.URL)
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

	// Output settings with GPU/CPU detection
	if hasGPU {
		if len(encoderArgs) >= 2 && encoderArgs[1] == "h264_qsv" {
			args = append(args, "-c:v", "h264_qsv", "-preset", "medium", "-global_quality", "23")
		} else if len(encoderArgs) >= 2 && encoderArgs[1] == "h264_vaapi" {
			args = append(args, "-c:v", "h264_vaapi", "-qp", "23")
		}
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "23")
	}

	args = append(args, "-pix_fmt", "yuv420p")

	if len(audioInputs) > 0 {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	}
	args = append(args, "-t", fmt.Sprintf("%.3f", req.Duration), "-y") // Use %.3f for float

	return args, nil
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
