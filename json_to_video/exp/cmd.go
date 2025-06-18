package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// VideoRequest represents the input parameters for video generation
type VideoRequest struct {
	Background string `json:"background"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Duration   int    `json:"duration"`
	Images     []struct {
		Data     string `json:"data"`
		URL      string `json:"url"`
		Duration int    `json:"duration"`
	} `json:"images"`
	Audio struct {
		BackgroundMusic string `json:"background_music"`
		BackgroundURL   string `json:"background_url"`
		VoiceOver       string `json:"voice_over"`
		VoiceOverURL    string `json:"voice_over_url"`
	} `json:"audio"`
	Subtitles struct {
		SRTData string `json:"srt_data"`
		SRTURL  string `json:"srt_url"`
		Style   struct {
			FontSize     int    `json:"font_size"`
			PrimaryColor string `json:"primary_color"`
			OutlineColor string `json:"outline_color"`
			Outline      int    `json:"outline"`
			Alignment    int    `json:"alignment"`
		} `json:"style"`
	} `json:"subtitles"`
	Text struct {
		Content   string `json:"content"`
		FontSize  int    `json:"font_size"`
		FontColor string `json:"font_color"`
		StartTime int    `json:"start_time"`
		EndTime   int    `json:"end_time"`
	} `json:"text"`
	KenBurns struct {
		Enabled   bool    `json:"enabled"`
		ZoomSpeed float64 `json:"zoom_speed"`
		ScaleSize int     `json:"scale_size"`
	} `json:"ken_burns"`
	HardwareAccel string `json:"hardware_accel"` // "qsv", "nvenc", "none"
}

func buildFFmpegCommand(jobID string, req *VideoRequest) ([]string, error) {
	tempDir := filepath.Join("temp", jobID)
	args := []string{}

	// Hardware acceleration
	if req.HardwareAccel == "qsv" {
		args = append(args, "-hwaccel", "qsv")
	} else if req.HardwareAccel == "nvenc" {
		args = append(args, "-hwaccel", "cuda")
	}

	videoInputCount := 0
	audioInputCount := 0

	// Background color/image
	if req.Background != "" && req.Background[0] == '#' {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("color=%s:size=%dx%d:duration=%d:rate=25",
				req.Background, req.Width, req.Height, req.Duration))
		videoInputCount++
	}

	// Add image inputs with proper durations
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

	// Build complex filter with Ken Burns effects
	filterComplex := buildFilterComplexWithKenBurns(req, videoInputCount, audioInputs)

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

	// Output settings with hardware encoding if available
	if req.HardwareAccel == "qsv" {
		args = append(args, "-c:v", "h264_qsv", "-preset", "fast", "-b:v", "8M")
	} else if req.HardwareAccel == "nvenc" {
		args = append(args, "-c:v", "h264_nvenc", "-preset", "fast", "-b:v", "8M")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "fast", "-b:v", "8M")
	}

	args = append(args, "-pix_fmt", "yuv420p")

	if len(audioInputs) > 0 {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	}

	args = append(args, "-t", strconv.Itoa(req.Duration), "-y")

	return args, nil
}

func buildFilterComplexWithKenBurns(req *VideoRequest, videoInputCount int, audioInputs []string) string {
	var filters []string

	// Default Ken Burns settings
	zoomSpeed := 0.0005
	scaleSize := 8000
	if req.KenBurns.Enabled {
		if req.KenBurns.ZoomSpeed > 0 {
			zoomSpeed = req.KenBurns.ZoomSpeed
		}
		if req.KenBurns.ScaleSize > 0 {
			scaleSize = req.KenBurns.ScaleSize
		}
	}

	// Generate Ken Burns effects for each image
	currentTime := 0
	for i, img := range req.Images {
		if img.Data != "" || img.URL != "" {
			inputIndex := i + 1 // +1 because background is index 0

			// Calculate duration in frames (25 fps)
			durationFrames := img.Duration * 25

			if req.KenBurns.Enabled {
				// Ken Burns effect with zoompan
				kenBurnsFilter := fmt.Sprintf(
					"[%d:v]scale=%d:-1,zoompan=z='zoom+%.6f':x=iw/2-(iw/zoom/2):y=0:d=%d:s=%dx%d:fps=25[kb%d]",
					inputIndex, scaleSize, zoomSpeed, durationFrames, req.Width, req.Height, i+1)
				filters = append(filters, kenBurnsFilter)
			} else {
				// Simple scale without Ken Burns
				scaleFilter := fmt.Sprintf(
					"[%d:v]scale=%d:%d[kb%d]",
					inputIndex, req.Width, req.Height, i+1)
				filters = append(filters, scaleFilter)
			}
		}
	}

	// Create overlay chain
	overlayChain := "[0:v]" // Start with background
	currentTime = 0

	for i, img := range req.Images {
		if img.Data != "" || img.URL != "" {
			endTime := currentTime + img.Duration

			if i == 0 {
				// First overlay
				overlayFilter := fmt.Sprintf(
					"%s[kb%d]overlay=0:0:enable='between(t,%d,%d)'[v%d]",
					overlayChain, i+1, currentTime, endTime, i+1)
				filters = append(filters, overlayFilter)
				overlayChain = fmt.Sprintf("[v%d]", i+1)
			} else {
				// Subsequent overlays
				overlayFilter := fmt.Sprintf(
					"%s[kb%d]overlay=0:0:enable='between(t,%d,%d)'[v%d]",
					overlayChain, i+1, currentTime, endTime, i+1)
				filters = append(filters, overlayFilter)
				overlayChain = fmt.Sprintf("[v%d]", i+1)
			}

			currentTime = endTime
		}
	}

	// Add text overlay if specified
	if req.Text.Content != "" {
		fontSize := 64
		fontColor := "white"
		if req.Text.FontSize > 0 {
			fontSize = req.Text.FontSize
		}
		if req.Text.FontColor != "" {
			fontColor = req.Text.FontColor
		}

		textFilter := fmt.Sprintf(
			"%sdrawtext=text='%s':fontsize=%d:fontcolor=%s:x=(w-text_w)/2:y=(h-text_h)/2:enable='between(t,%d,%d)'[v_text]",
			overlayChain, req.Text.Content, fontSize, fontColor, req.Text.StartTime, req.Text.EndTime)
		filters = append(filters, textFilter)
		overlayChain = "[v_text]"
	}

	// Prepare for subtitles (will be added later if needed)
	preSubtitleFilter := fmt.Sprintf("%sdrawtext=text='':fontsize=1[vt0]", overlayChain)
	filters = append(filters, preSubtitleFilter)

	// Audio mixing
	if len(audioInputs) > 0 {
		if len(audioInputs) == 1 {
			audioFilter := fmt.Sprintf("%samix=inputs=1[a]", audioInputs[0])
			filters = append(filters, audioFilter)
		} else {
			audioMixInputs := strings.Join(audioInputs, "")
			audioFilter := fmt.Sprintf("%samix=inputs=%d[a]", audioMixInputs, len(audioInputs))
			filters = append(filters, audioFilter)
		}
	}

	return strings.Join(filters, ";")
}

func addSubtitlesToFilterComplex(filterComplex, srtPath string, subtitles struct {
	SRTData string `json:"srt_data"`
	SRTURL  string `json:"srt_url"`
	Style   struct {
		FontSize     int    `json:"font_size"`
		PrimaryColor string `json:"primary_color"`
		OutlineColor string `json:"outline_color"`
		Outline      int    `json:"outline"`
		Alignment    int    `json:"alignment"`
	} `json:"style"`
}) string {
	// Default subtitle styling
	fontSize := 24
	primaryColor := "&Hffffff"
	outlineColor := "&H000000"
	outline := 2
	alignment := 2

	if subtitles.Style.FontSize > 0 {
		fontSize = subtitles.Style.FontSize
	}
	if subtitles.Style.PrimaryColor != "" {
		primaryColor = subtitles.Style.PrimaryColor
	}
	if subtitles.Style.OutlineColor != "" {
		outlineColor = subtitles.Style.OutlineColor
	}
	if subtitles.Style.Outline > 0 {
		outline = subtitles.Style.Outline
	}
	if subtitles.Style.Alignment > 0 {
		alignment = subtitles.Style.Alignment
	}

	// Replace [vt0] with subtitle filter
	subtitleStyle := fmt.Sprintf("FontSize=%d,PrimaryColour=%s,OutlineColour=%s,Outline=%d,Alignment=%d",
		fontSize, primaryColor, outlineColor, outline, alignment)

	subtitleFilter := fmt.Sprintf("[vt0]subtitles=%s:force_style='%s'[v]", srtPath, subtitleStyle)

	// Replace the dummy drawtext filter with subtitle filter
	updatedFilter := strings.Replace(filterComplex, "drawtext=text='':fontsize=1[vt0]", subtitleFilter, 1)

	return updatedFilter
}

// Example usage function
func main() {
	// Example VideoRequest
	req := &VideoRequest{
		Background: "#1a1a1a",
		Width:      1920,
		Height:     1080,
		Duration:   60,
		Images: []struct {
			Data     string `json:"data"`
			URL      string `json:"url"`
			Duration int    `json:"duration"`
		}{
			{URL: "assets/images/97d5e87c_prompt_1_panel_0_image_0_seed_738224.jpg", Duration: 10},
			{URL: "assets/images/f3889327_prompt_2_panel_0_image_0_seed_738224.jpg", Duration: 20},
			{URL: "assets/images/597bf893_prompt_3_panel_0_image_0_seed_738224.jpg", Duration: 80},
			{URL: "assets/images/sample2.jpg", Duration: 120},
		},
		Audio: struct {
			BackgroundMusic string `json:"background_music"`
			BackgroundURL   string `json:"background_url"`
			VoiceOver       string `json:"voice_over"`
			VoiceOverURL    string `json:"voice_over_url"`
		}{
			VoiceOverURL: "assets/audio/voice01.mp3",
		},
		Subtitles: struct {
			SRTData string `json:"srt_data"`
			SRTURL  string `json:"srt_url"`
			Style   struct {
				FontSize     int    `json:"font_size"`
				PrimaryColor string `json:"primary_color"`
				OutlineColor string `json:"outline_color"`
				Outline      int    `json:"outline"`
				Alignment    int    `json:"alignment"`
			} `json:"style"`
		}{
			SRTURL: "assets/subtitles/d6e6add0_voice.srt",
			Style: struct {
				FontSize     int    `json:"font_size"`
				PrimaryColor string `json:"primary_color"`
				OutlineColor string `json:"outline_color"`
				Outline      int    `json:"outline"`
				Alignment    int    `json:"alignment"`
			}{
				FontSize:     24,
				PrimaryColor: "&Hffffff",
				OutlineColor: "&H000000",
				Outline:      2,
				Alignment:    2,
			},
		},
		Text: struct {
			Content   string `json:"content"`
			FontSize  int    `json:"font_size"`
			FontColor string `json:"font_color"`
			StartTime int    `json:"start_time"`
			EndTime   int    `json:"end_time"`
		}{
			Content:   "Your Brand Here",
			FontSize:  64,
			FontColor: "white",
			StartTime: 2,
			EndTime:   10,
		},
		KenBurns: struct {
			Enabled   bool    `json:"enabled"`
			ZoomSpeed float64 `json:"zoom_speed"`
			ScaleSize int     `json:"scale_size"`
		}{
			Enabled:   true,
			ZoomSpeed: 0.0005,
			ScaleSize: 8000,
		},
		HardwareAccel: "qsv",
	}

	args, err := buildFFmpegCommand("test_job", req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("FFmpeg command:")
	for i, arg := range args {
		if i > 0 {
			fmt.Printf(" ")
		}
		fmt.Printf("%s", arg)
	}
	fmt.Println()
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing command: %v\nOutput: %s\n", err, output)
		return
	}
}
