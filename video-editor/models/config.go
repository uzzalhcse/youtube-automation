package models

import (
	"encoding/json"
	"os"
)

// Keyframe represents a single animation keyframe
type Keyframe struct {
	Time    float64 `json:"time"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Opacity float64 `json:"opacity"`
	Scale   float64 `json:"scale,omitempty"`
}

// Overlay represents an overlay element with keyframe animations
type Overlay struct {
	Source    string     `json:"source"`
	Start     float64    `json:"start"`
	End       float64    `json:"end"`
	Keyframes []Keyframe `json:"keyframes"`
}

// Text represents a text element with keyframe animations
type Text struct {
	Text      string     `json:"text"`
	Start     float64    `json:"start"`
	End       float64    `json:"end"`
	FontSize  int        `json:"font_size,omitempty"`
	FontColor string     `json:"font_color,omitempty"`
	Keyframes []Keyframe `json:"keyframes"`
}

// VideoConfig represents the complete project configuration
type VideoConfig struct {
	Overlays []Overlay `json:"overlays"`
	Texts    []Text    `json:"texts"`
	Settings Settings  `json:"settings,omitempty"`
}

// Settings contains global video settings
// Settings contains global video settings
type Settings struct {
	Width          int     `json:"width,omitempty"`
	Height         int     `json:"height,omitempty"`
	FPS            int     `json:"fps,omitempty"`
	BGMVolume      float64 `json:"bgm_volume,omitempty"`
	VoiceVolume    float64 `json:"voice_volume,omitempty"`
	OverlayOpacity float64 `json:"overlay_opacity"` // Opacity for overlay videos (0.0 to 1.0)

	// Zoom Animation Settings
	ZoomSpeed               float64          `json:"zoom_speed,omitempty"`        // Speed of zoom (0.0005 to 0.005, default 0.0015)
	ZoomIntensity           float64          `json:"zoom_intensity,omitempty"`    // Maximum zoom level (1.1 to 2.0, default 1.3)
	PanSpeed                float64          `json:"pan_speed,omitempty"`         // Speed of panning (0.0001 to 0.001, default 0.0003)
	TransitionSmooth        float64          `json:"transition_smooth,omitempty"` // Smoothness factor (0.5 to 2.0, default 1.0)
	AnimationPreset         string           `json:"animation_preset,omitempty"`  // "gentle", "moderate", "dynamic", "custom"
	MaxConcurrentJobs       int              `json:"max_concurrent_jobs,omitempty"`
	UseGPU                  bool             `json:"use_gpu"`    // NEW: Enable GPU acceleration
	GPUDevice               string           `json:"gpu_device"` // NEW: GPU device selection
	UsePartialImageDuration bool             `json:"use_partial_image_duration"`
	ImagesDurationMinutes   float64          `json:"images_duration_minutes"`
	BlackScreenColor        string           `json:"black_screen_color"` // "black", "#000000", etc.
	UseAnimationEffects     bool             `json:"use_animation_effects"`
	ChromaKey               *ChromaKeyConfig `json:"chroma_key,omitempty"`
}
type ChromaKeyConfig struct {
	Enabled       bool    `json:"enabled"`
	Color         string  `json:"color"`
	Similarity    float64 `json:"similarity"`
	Blend         float64 `json:"blend"`
	YUVThreshold  float64 `json:"yuv_threshold"`
	EdgeFeather   float64 `json:"edge_feather"`
	AutoAdjust    bool    `json:"auto_adjust"`
	SpillSuppress bool    `json:"spill_suppress"`
}

// Example project.json configuration:
/*
{
  "settings": {
    "width": 1920,
    "height": 1080,
    "fps": 30,
    "voice_volume": 1.0,
    "bgm_volume": 0.3,
    "overlay_opacity": 0.5
  }
}
*/
// LoadConfig loads the project configuration from a JSON file
// LoadConfig loads the project configuration from a JSON file
func LoadConfig(configPath string) (*VideoConfig, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config VideoConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	// Set default values if not specified
	if config.Settings.Width <= 0 {
		config.Settings.Width = 1920
	}
	if config.Settings.Height <= 0 {
		config.Settings.Height = 1080
	}
	if config.Settings.FPS <= 0 {
		config.Settings.FPS = 30
	}
	if config.Settings.BGMVolume <= 0 {
		config.Settings.BGMVolume = 0.3
	}
	if config.Settings.VoiceVolume <= 0 {
		config.Settings.VoiceVolume = 1.0
	}

	// Set zoom animation defaults based on preset or individual settings
	if config.Settings.AnimationPreset == "" {
		config.Settings.AnimationPreset = "gentle"
	}

	config.applyAnimationPreset()

	// Set default font properties for texts
	for i := range config.Texts {
		if config.Texts[i].FontSize <= 0 {
			config.Texts[i].FontSize = 48
		}
		if config.Texts[i].FontColor == "" {
			config.Texts[i].FontColor = "white"
		}
	}

	return &config, nil
}

// applyAnimationPreset applies predefined animation settings
func (c *VideoConfig) applyAnimationPreset() {
	switch c.Settings.AnimationPreset {
	case "gentle":
		if c.Settings.ZoomSpeed <= 0 {
			c.Settings.ZoomSpeed = 0.0008
		}
		if c.Settings.ZoomIntensity <= 0 {
			c.Settings.ZoomIntensity = 1.15
		}
		if c.Settings.PanSpeed <= 0 {
			c.Settings.PanSpeed = 0.0002
		}
		if c.Settings.TransitionSmooth <= 0 {
			c.Settings.TransitionSmooth = 1.5
		}
	case "moderate":
		if c.Settings.ZoomSpeed <= 0 {
			c.Settings.ZoomSpeed = 0.0015
		}
		if c.Settings.ZoomIntensity <= 0 {
			c.Settings.ZoomIntensity = 1.25
		}
		if c.Settings.PanSpeed <= 0 {
			c.Settings.PanSpeed = 0.0003
		}
		if c.Settings.TransitionSmooth <= 0 {
			c.Settings.TransitionSmooth = 1.0
		}
	case "dynamic":
		if c.Settings.ZoomSpeed <= 0 {
			c.Settings.ZoomSpeed = 0.0025
		}
		if c.Settings.ZoomIntensity <= 0 {
			c.Settings.ZoomIntensity = 1.4
		}
		if c.Settings.PanSpeed <= 0 {
			c.Settings.PanSpeed = 0.0005
		}
		if c.Settings.TransitionSmooth <= 0 {
			c.Settings.TransitionSmooth = 0.8
		}
	default: // custom or fallback
		if c.Settings.ZoomSpeed <= 0 {
			c.Settings.ZoomSpeed = 0.0008
		}
		if c.Settings.ZoomIntensity <= 0 {
			c.Settings.ZoomIntensity = 1.15
		}
		if c.Settings.PanSpeed <= 0 {
			c.Settings.PanSpeed = 0.0002
		}
		if c.Settings.TransitionSmooth <= 0 {
			c.Settings.TransitionSmooth = 1.5
		}
	}
}

// GetImageDuration calculates the duration each image should be displayed
func (c *VideoConfig) GetImageDuration(totalDuration float64, imageCount int) float64 {
	if imageCount <= 0 {
		return 0
	}
	return totalDuration / float64(imageCount)
}
