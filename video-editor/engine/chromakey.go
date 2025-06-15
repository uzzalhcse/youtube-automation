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
		Color:         "green", // Supported: "green", "blue", "red", "white", "black", or hex values
		Similarity:    0.51,    // Tolerance: 0.0-1.0 (0=exact match, 1=very loose)
		Blend:         0.9,     // Offset/Blend: 0.0-1.0 (0=hard edge, 1=soft blend)
		YUVThreshold:  -2.00,   // Edge Thickness: -10.0 to +10.0 (negative=thinner, positive=thicker)
		AutoAdjust:    true,    // Auto color adjustment: true/false
		SpillSuppress: true,    // Spill suppression: true/false
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
			EdgeFeather:   ve.Config.Settings.ChromaKey.EdgeFeather,
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
		// Use yuv420p instead of yuva420p for better codec compatibility
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,format=yuv420p,colorchannelmixer=aa=%.2f",
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

	// Build chroma key filter - the chromakey filter doesn't have a 'threshold' option
	// Available options are: color, similarity, blend, yuv
	if config.AutoAdjust {
		// Use chromakey filter with YUV mode enabled for better color matching
		chromaFilter = fmt.Sprintf("chromakey=color=%s:similarity=%.3f:blend=%.3f:yuv=1",
			colorValue, config.Similarity, config.Blend)
	} else {
		// Use basic colorkey filter
		chromaFilter = fmt.Sprintf("colorkey=color=%s:similarity=%.3f:blend=%.3f",
			colorValue, config.Similarity, config.Blend)
	}

	// Add spill suppression if enabled
	var spillFilter string
	if config.SpillSuppress {
		spillMix := 0.7    // default
		spillExpand := 0.1 // default

		// Adjust spill parameters based on EdgeFeather value
		if config.EdgeFeather > 0 {
			spillMix = config.EdgeFeather / 10.0 // Scale EdgeFeather to reasonable range
			spillExpand = config.EdgeFeather / 20.0
		}

		if strings.ToLower(config.Color) == "green" {
			spillFilter = fmt.Sprintf(",despill=type=green:mix=%.2f:expand=%.2f", spillMix, spillExpand)
		} else if strings.ToLower(config.Color) == "blue" {
			spillFilter = fmt.Sprintf(",despill=type=blue:mix=%.2f:expand=%.2f", spillMix, spillExpand)
		}
	}

	// Add edge smoothing filter if EdgeFeather is specified
	var edgeFilter string
	if config.EdgeFeather > 0 {
		// Use a gentler edge smoothing approach
		// Apply a slight blur to soften edges
		blurRadius := config.EdgeFeather / 4.0
		if blurRadius > 0.1 {
			edgeFilter = fmt.Sprintf(",gblur=sigma=%.2f", blurRadius)
		}
	}

	// Determine the appropriate pixel format based on whether we need alpha channel
	pixelFormat := "yuv420p"
	if config.Enabled {
		// For chroma key, we need alpha channel support, but use a more compatible format
		// Try yuva420p first, but have fallback logic in your encoding
		pixelFormat = "yuva420p"
	}

	// Combine all filters with proper transparency handling
	fullFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,%s%s%s,format=%s",
		width, height, width, height, chromaFilter, spillFilter, edgeFilter, pixelFormat)

	// Add opacity adjustment if needed
	if overlayOpacity < 1.0 {
		fullFilter += fmt.Sprintf(",colorchannelmixer=aa=%.2f", overlayOpacity)
	}

	return fullFilter
}

// Alternative method to create chroma key filter with better codec compatibility
func (ve *VideoEditor) createCompatibleChromaKeyFilter(config ChromaKeyConfig, width, height int, overlayOpacity float64) string {
	if !config.Enabled {
		// Return basic scaling filter without chroma key
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black",
			width, height, width, height)
	}

	// Determine color value
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
	}

	// Use colorkey filter which is more widely supported
	chromaFilter := fmt.Sprintf("colorkey=color=%s:similarity=%.3f:blend=%.3f",
		colorValue, config.Similarity, config.Blend)

	// Add spill suppression
	var spillFilter string
	if config.SpillSuppress {
		spillMix := 0.7
		if config.EdgeFeather > 0 {
			spillMix = config.EdgeFeather / 10.0
		}

		if strings.ToLower(config.Color) == "green" {
			spillFilter = fmt.Sprintf(",despill=type=green:mix=%.2f", spillMix)
		} else if strings.ToLower(config.Color) == "blue" {
			spillFilter = fmt.Sprintf(",despill=type=blue:mix=%.2f", spillMix)
		}
	}

	// Combine filters with compatible pixel format
	fullFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,%s%s,format=yuv420p",
		width, height, width, height, chromaFilter, spillFilter)

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
