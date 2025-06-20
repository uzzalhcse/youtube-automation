package main

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type Channel struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ChannelName  string             `bson:"channel_name" json:"channel_name"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
	TotalScripts int                `bson:"total_scripts" json:"total_scripts"`
	Settings     ChannelSettings    `bson:"settings" json:"settings"`
}

type ChannelSettings struct {
	DefaultSectionCount     int  `bson:"default_section_count" json:"default_section_count"`
	PreferredVisualGuidance bool `bson:"preferred_visual_guidance" json:"preferred_visual_guidance"`
	WordLimitForHookIntro   int  `bson:"word_limit_for_hook_intro" json:"word_limit_for_hook_intro"`
	VisualImageMultiplier   int  `bson:"visual_image_multiplier" json:"visual_image_multiplier"`
	WordLimitPerSection     int  `bson:"word_limit_per_section" json:"word_limit_per_section"`
}

type OutlinePoint struct {
	SectionNumber int    `bson:"section_number" json:"section_number"`
	Title         string `bson:"title" json:"title"`
	Summary       string `bson:"summary" json:"summary"`
}

type ImagePrompt struct {
	SectionNumber int    `bson:"section_number" json:"section_number"`
	PromptText    string `bson:"prompt_text" json:"prompt_text"`
	ImageType     string `bson:"image_type" json:"image_type"` // "thumbnail", "section", "visual_aid"
}

type Script struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ChannelID       primitive.ObjectID `bson:"channel_id" json:"channel_id"` // Reference to Channel
	ChannelName     string             `bson:"channel_name" json:"channel_name"`
	Topic           string             `bson:"topic" json:"topic"`
	Status          string             `bson:"status" json:"status"`
	GenerateVisuals bool               `bson:"generate_visuals" json:"generate_visuals"`

	// Content stored in DB instead of files
	Outline       string         `bson:"outline" json:"outline"`
	OutlinePoints []OutlinePoint `bson:"outline_points" json:"outline_points"`
	FullScript    string         `bson:"full_script" json:"full_script"`
	Meta          string         `bson:"meta" json:"meta"`
	SRT           string         `bson:"srt" json:"srt"` // SRT content for subtitles
	FullAudioFile string         `bson:"full_audio_file,omitempty" json:"full_audio_file,omitempty"`

	CreatedAt         time.Time  `bson:"created_at" json:"created_at"`
	StartedAt         *time.Time `bson:"started_at,omitempty" json:"started_at,omitempty"`
	CompletedAt       *time.Time `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
	ErrorMessage      string     `bson:"error_message,omitempty" json:"error_message,omitempty"`
	ProcessingTime    float64    `bson:"processing_time_seconds,omitempty" json:"processing_time_seconds,omitempty"`
	SectionsGenerated int        `bson:"sections_generated,omitempty" json:"sections_generated,omitempty"`
	CurrentSection    int        `bson:"current_section,omitempty" json:"current_section,omitempty"`
}

// API Request/Response structures
type ScriptRequest struct {
	Topic           string `json:"topic"`
	GenerateVisuals bool   `json:"generate_visuals"`
	ChannelName     string `json:"channel_name"`
}

type ScriptResponse struct {
	Success         bool   `json:"success"`
	ScriptID        string `json:"script_id,omitempty"`
	Message         string `json:"message,omitempty"`
	Status          string `json:"status,omitempty"`
	Topic           string `json:"topic,omitempty"`
	ChannelName     string `json:"channel_name,omitempty"`
	OutputFolder    string `json:"output_folder,omitempty"`
	OutputFilename  string `json:"output_filename,omitempty"`
	MetaTagFilename string `json:"metatag_filename,omitempty"`
	GeneratedAt     string `json:"generated_at,omitempty"`
	Error           string `json:"error,omitempty"`
}

type ScriptAudio struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ScriptID   primitive.ObjectID `bson:"script_id" json:"script_id"`
	ChunkIndex int                `bson:"chunk_index" json:"chunk_index"`
	Content    string             `bson:"content" json:"content"`
	CharCount  int                `bson:"char_count" json:"char_count"`
	HasVisual  bool               `bson:"has_visual" json:"has_visual"`                     // need to remove or move main script collection
	AudioFile  string             `bson:"audio_file,omitempty" json:"audio_file,omitempty"` // Optional audio file reference
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}
type ScriptSrt struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ScriptID   primitive.ObjectID `bson:"script_id" json:"script_id"`
	ChunkIndex int                `bson:"chunk_index" json:"chunk_index"`
	Content    string             `bson:"content" json:"content"`
	CharCount  int                `bson:"char_count" json:"char_count"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}
type ChunkVisual struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ScriptID     primitive.ObjectID `bson:"script_id" json:"script_id"`
	ChunkID      primitive.ObjectID `bson:"chunk_id" json:"chunk_id"`
	ChunkIndex   int                `bson:"chunk_index" json:"chunk_index"`
	Prompt       string             `bson:"prompt" json:"prompt"`
	PromptIndex  int                `bson:"prompt_index" json:"prompt_index"` // Index of the prompt in the chunk
	StartTime    string             `bson:"start_time" json:"start_time"`
	EndTime      string             `bson:"end_time" json:"end_time"`
	ImagePath    string             `bson:"image_path,omitempty" json:"image_path,omitempty"` // Optional image path
	Emotion      string             `bson:"emotion" json:"emotion"`
	SceneConcept string             `bson:"scene_concept" json:"scene_concept"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
}
type VisualPromptResponse struct {
	StartTime string `bson:"start_time" json:"start_time"`
	EndTime   string `bson:"end_time" json:"end_time"`
	Prompt    string `json:"prompt"`
}

/*

 */

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
type Template struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        string             `bson:"name" json:"name"`
	Type        string             `bson:"type" json:"type"` // outline, script, hook_intro, visual_guidance, meta_tag
	Content     string             `bson:"content" json:"content"`
	Description string             `bson:"description,omitempty" json:"description,omitempty"`
	IsActive    bool               `bson:"is_active" json:"is_active"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
	Version     int                `bson:"version" json:"version"`
}

// NEW REQUEST STRUCTURES
type VisualStyleRequest struct {
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	StyleRules     []string `json:"style_rules"`
	PromptTemplate string   `json:"prompt_template"`
}

type PromptTemplateRequest struct {
	ChannelID    string   `json:"channel_id,omitempty"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	SystemPrompt string   `json:"system_prompt"`
	UserPrompt   string   `json:"user_prompt"`
	Variables    []string `json:"variables,omitempty"`
	StyleIDs     []string `json:"style_ids,omitempty"` // NEW: Style IDs as strings
	IsGlobal     bool     `json:"is_global"`
}

type PromptTemplate struct {
	ID           primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	ChannelID    primitive.ObjectID   `bson:"channel_id,omitempty" json:"channel_id"` // nil for global templates
	Name         string               `bson:"name" json:"name"`
	Type         string               `bson:"type" json:"type"` // outline, script, hook_intro, etc.
	SystemPrompt string               `bson:"system_prompt" json:"system_prompt"`
	UserPrompt   string               `bson:"user_prompt" json:"user_prompt"`
	Variables    []string             `bson:"variables" json:"variables"`           // ["{TOPIC}", "{SECTION_COUNT}", etc.]
	StyleIDs     []primitive.ObjectID `bson:"style_ids,omitempty" json:"style_ids"` // NEW: Reference to visual styles
	IsActive     bool                 `bson:"is_active" json:"is_active"`
	IsGlobal     bool                 `bson:"is_global" json:"is_global"` // true for default templates
	CreatedAt    time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time            `bson:"updated_at" json:"updated_at"`
	Version      int                  `bson:"version" json:"version"`
}

type VisualStyle struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name           string             `bson:"name" json:"name"`
	Category       string             `bson:"category" json:"category"` // "minimalist", "artistic", "abstract"
	Description    string             `bson:"description" json:"description"`
	StyleRules     []string           `bson:"style_rules" json:"style_rules"`
	PromptTemplate string             `bson:"prompt_template" json:"prompt_template"` // Template with [CONTENT] placeholder
	IsActive       bool               `bson:"is_active" json:"is_active"`
	CreatedAt      time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at" json:"updated_at"`
}
type OutlineResponse struct {
	Sections []OutlineSection `json:"sections"`
}

type OutlineSection struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}
type HookIntroResponse struct {
	HookIntro HookIntroContent `json:"hook_intro"`
}

type HookIntroContent struct {
	Content   string `json:"content"`
	WordCount int    `json:"word_count"`
	ModeUsed  string `json:"mode_used"`
}

type SectionResponse struct {
	Section SectionContent `json:"section"`
}

type SectionContent struct {
	Content         string `json:"content"`
	WordCount       int    `json:"word_count"`
	NarrativeFormat string `json:"narrative_format"`
}

type MetaResponse struct {
	Meta MetaContent `json:"meta"`
}

type MetaContent struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	ThumbnailText string   `json:"thumbnail_text"`
}
