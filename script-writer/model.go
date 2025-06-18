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
}

type OutlinePoint struct {
	SectionNumber int    `bson:"section_number" json:"section_number"`
	Title         string `bson:"title" json:"title"`
	Description   string `bson:"description" json:"description"`
}

type ImagePrompt struct {
	SectionNumber int    `bson:"section_number" json:"section_number"`
	PromptText    string `bson:"prompt_text" json:"prompt_text"`
	ImageType     string `bson:"image_type" json:"image_type"` // "thumbnail", "section", "visual_aid"
}

type ScriptGeneration struct {
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
	MetaTag       string         `bson:"meta_tag" json:"meta_tag"`
	ImagePrompts  []ImagePrompt  `bson:"image_prompts" json:"image_prompts"`

	// Keep file references for backward compatibility (optional)
	OutputFolder    string `bson:"output_folder,omitempty" json:"output_folder,omitempty"`
	OutputFilename  string `bson:"output_filename,omitempty" json:"output_filename,omitempty"`
	MetaTagFilename string `bson:"metatag_filename,omitempty" json:"metatag_filename,omitempty"`

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

type ScriptChunk struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ScriptID   primitive.ObjectID `bson:"script_id" json:"script_id"`
	ChunkIndex int                `bson:"chunk_index" json:"chunk_index"`
	Content    string             `bson:"content" json:"content"`
	CharCount  int                `bson:"char_count" json:"char_count"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}
