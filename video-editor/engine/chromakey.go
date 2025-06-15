package engine

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
)

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
