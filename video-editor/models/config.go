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
type Settings struct {
	Width          int     `json:"width,omitempty"`
	Height         int     `json:"height,omitempty"`
	FPS            int     `json:"fps,omitempty"`
	BGMVolume      float64 `json:"bgm_volume,omitempty"`
	VoiceVolume    float64 `json:"voice_volume,omitempty"`
	OverlayOpacity float64 `json:"overlay_opacity"` // NEW: Opacity for overlay videos (0.0 to 1.0)
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

// GetImageDuration calculates the duration each image should be displayed
func (c *VideoConfig) GetImageDuration(totalDuration float64, imageCount int) float64 {
	if imageCount <= 0 {
		return 0
	}
	return totalDuration / float64(imageCount)
}
