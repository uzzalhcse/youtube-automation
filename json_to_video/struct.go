package main

import "time"

type VideoRequest struct {
	Title      string         `json:"title" validate:"required,min=1,max=200"`
	Duration   float64        `json:"duration" validate:"required,min=1,max=3600"` // Changed from int
	Width      int            `json:"width" validate:"required,min=480,max=4096"`
	Height     int            `json:"height" validate:"required,min=360,max=4096"`
	Background string         `json:"background" validate:"required"`
	Images     []ImageAsset   `json:"images" validate:"required,min=1"`
	Audio      AudioConfig    `json:"audio"`
	Subtitles  SubtitleConfig `json:"subtitles"`
	Scenes     []Scene        `json:"scenes"`
}

type ImageAsset struct {
	ID        string         `json:"id"`
	Data      string         `json:"data,omitempty"`
	URL       string         `json:"url,omitempty"`
	StartTime float64        `json:"starttime"` // Changed from int
	Duration  float64        `json:"duration"`  // Changed from int
	X         int            `json:"x"`
	Y         int            `json:"y"`
	Width     int            `json:"width"`
	Height    int            `json:"height"`
	ZIndex    int            `json:"zindex"`
	Opacity   float64        `json:"opacity"`
	Effect    string         `json:"effect,omitempty"`
	KenBurns  KenBurnsConfig `json:"kenburns,omitempty"`
}

type Scene struct {
	ID        string  `json:"id"`
	Text      string  `json:"text"`
	StartTime float64 `json:"starttime"` // Changed from int
	Duration  float64 `json:"duration"`  // Changed from int
	X         int     `json:"x"`
	Y         int     `json:"y"`
	FontSize  int     `json:"fontsize"`
	FontColor string  `json:"fontcolor"`
	Position  string  `json:"position"`
}
type AudioConfig struct {
	BackgroundMusic string  `json:"background_music"` // base64 encoded audio
	BackgroundURL   string  `json:"background_url"`   // or audio URL
	Volume          float64 `json:"volume"`
	FadeIn          int     `json:"fade_in"`        // fade in duration (seconds)
	FadeOut         int     `json:"fade_out"`       // fade out duration (seconds)
	VoiceOver       string  `json:"voice_over"`     // base64 encoded voice
	VoiceOverURL    string  `json:"voice_over_url"` // or voice URL
	VoiceVolume     float64 `json:"voice_volume"`
}

type SubtitleConfig struct {
	SRTData    string `json:"srt_data"` // SRT content as string
	SRTURL     string `json:"srt_url"`  // or SRT file URL
	FontSize   int    `json:"font_size"`
	FontColor  string `json:"font_color"`
	Position   string `json:"position"`   // "bottom", "top", "center"
	Background string `json:"background"` // subtitle background color
	Outline    bool   `json:"outline"`    // text outline
}

// VideoResponse represents the API response
type VideoResponse struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	VideoURL string `json:"video_url,omitempty"`
	Progress int    `json:"progress,omitempty"` // 0-100
}

// JobStatus tracks video generation jobs
type JobStatus struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"` // "pending", "processing", "completed", "failed"
	Progress  int           `json:"progress"`
	CreatedAt time.Time     `json:"created_at"`
	VideoPath string        `json:"video_path"`
	Error     string        `json:"error,omitempty"`
	Request   *VideoRequest `json:"request,omitempty"`
}

// New struct for Ken Burns effect configuration
type KenBurnsConfig struct {
	Enabled    bool    `json:"enabled"`
	ZoomRate   float64 `json:"zoom_rate"`   // zoom increment per frame (e.g., 0.0005)
	ZoomStart  float64 `json:"zoom_start"`  // initial zoom level (default: 1.0)
	ZoomEnd    float64 `json:"zoom_end"`    // final zoom level (calculated from rate and duration)
	PanX       string  `json:"pan_x"`       // pan X expression (e.g., "iw/2-(iw/zoom/2)")
	PanY       string  `json:"pan_y"`       // pan Y expression (e.g., "ih/2-(ih/zoom/2)")
	ScaleWidth int     `json:"scale_width"` // scale to this width before zooming (e.g., 8000)
	Direction  string  `json:"direction"`   // "zoom_in", "zoom_out", "pan_left", "pan_right"
}
