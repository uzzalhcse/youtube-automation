package main

import "time"

// VideoRequest represents the JSON structure for video generation
type VideoRequest struct {
	Title      string         `json:"title"`
	Duration   int            `json:"duration"` // in seconds
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	Background string         `json:"background"` // color or base64 image
	Images     []ImageAsset   `json:"images"`
	Audio      AudioConfig    `json:"audio"`
	Subtitles  SubtitleConfig `json:"subtitles"`
	Scenes     []Scene        `json:"scenes"`
}

type ImageAsset struct {
	ID        string  `json:"id"`
	Data      string  `json:"data"`       // base64 encoded image
	URL       string  `json:"url"`        // or image URL
	StartTime int     `json:"start_time"` // when to show (seconds)
	Duration  int     `json:"duration"`   // how long to show (seconds)
	X         int     `json:"x"`          // position x
	Y         int     `json:"y"`          // position y
	Width     int     `json:"width"`      // scaled width
	Height    int     `json:"height"`     // scaled height
	ZIndex    int     `json:"z_index"`    // layer order
	Opacity   float64 `json:"opacity"`    // 0.0 to 1.0
	Effect    string  `json:"effect"`     // "fade", "slide", "zoom", "none"

	// Ken Burns effect parameters
	KenBurns KenBurnsConfig `json:"ken_burns"`
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

type Scene struct {
	StartTime  int    `json:"start_time"` // in seconds
	Duration   int    `json:"duration"`   // in seconds
	Text       string `json:"text"`
	FontSize   int    `json:"font_size"`
	FontColor  string `json:"font_color"`
	Position   string `json:"position"`   // "center", "top", "bottom"
	X          int    `json:"x"`          // custom x position
	Y          int    `json:"y"`          // custom y position
	Effect     string `json:"effect"`     // "fade", "slide", "typewriter", "none"
	Background string `json:"background"` // text background
	Outline    bool   `json:"outline"`    // text outline
	Animation  string `json:"animation"`  // "bounce", "shake", "pulse"
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
