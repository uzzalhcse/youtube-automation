package main

import "os/exec"

// Helper function to create Ken Burns presets
func CreateKenBurnsPreset(preset string, duration int) KenBurnsConfig {
	switch preset {
	case "zoom_in_slow":
		return KenBurnsConfig{
			Enabled:    true,
			ZoomRate:   0.0002,
			PanX:       "iw/2-(iw/zoom/2)",
			PanY:       "ih/2-(ih/zoom/2)",
			ScaleWidth: 6000,
		}
	case "zoom_in_fast":
		return KenBurnsConfig{
			Enabled:    true,
			ZoomRate:   0.001,
			PanX:       "iw/2-(iw/zoom/2)",
			PanY:       "ih/2-(ih/zoom/2)",
			ScaleWidth: 8000,
		}
	case "pan_left":
		return KenBurnsConfig{
			Enabled:    true,
			ZoomRate:   0.0005,
			PanX:       "iw-iw/zoom",
			PanY:       "ih/2-(ih/zoom/2)",
			ScaleWidth: 8000,
		}
	case "pan_right":
		return KenBurnsConfig{
			Enabled:    true,
			ZoomRate:   0.0005,
			PanX:       "0",
			PanY:       "ih/2-(ih/zoom/2)",
			ScaleWidth: 8000,
		}
	case "pan_up":
		return KenBurnsConfig{
			Enabled:    true,
			ZoomRate:   0.0005,
			PanX:       "iw/2-(iw/zoom/2)",
			PanY:       "ih-ih/zoom",
			ScaleWidth: 8000,
		}
	case "pan_down":
		return KenBurnsConfig{
			Enabled:    true,
			ZoomRate:   0.0005,
			PanX:       "iw/2-(iw/zoom/2)",
			PanY:       "0",
			ScaleWidth: 8000,
		}
	default: // "standard"
		return KenBurnsConfig{
			Enabled:    true,
			ZoomRate:   0.0005,
			PanX:       "iw/2-(iw/zoom/2)",
			PanY:       "ih/2-(ih/zoom/2)",
			ScaleWidth: 8000,
		}
	}
}
func detectGPUAcceleration() (bool, []string) {
	// Try Intel QSV first (since you have Intel Iris Xe)
	cmd := exec.Command("ffmpeg", "-hide_banner", "-hwaccel", "qsv", "-hwaccel_output_format", "qsv", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1", "-c:v", "h264_qsv", "-f", "null", "-")
	if err := cmd.Run(); err == nil {
		return true, []string{"-hwaccel", "qsv"}
	}

	// Try VAAPI as fallback for Intel
	cmd = exec.Command("ffmpeg", "-hide_banner", "-hwaccel", "vaapi", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1", "-c:v", "h264_vaapi", "-f", "null", "-")
	if err := cmd.Run(); err == nil {
		return true, []string{"-hwaccel", "vaapi"}
	}

	return false, []string{} // CPU fallback
}
