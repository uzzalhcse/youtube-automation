package main

import (
	"time"
)

// ScriptConfig holds configuration for script generation
type ScriptConfig struct {
	Topic                string
	GenerateVisuals      bool
	OutputFilename       string // This will be the script filename
	MetaTagFilename      string // New field for meta tag filename
	OutputFolder         string // New field for output folder path (video title)
	SleepBetweenSections time.Duration
	channel              Channel
}

// SRTEntry represents a single subtitle entry
type SRTEntry struct {
	Index     int
	StartTime time.Duration
	EndTime   time.Duration
	Text      string
}

const (
	debugMode                   = true
	defaultSectionCount         = 3
	visualImageMultiplier       = 5
	defaultSleepBetweenSections = 1 * time.Second
	maxRetries                  = 5
	splitVoiceByCharLimit       = 4990 // Maximum character limit for splitting text into manageable chunks for voice generation
	splitSrtByCharLimit         = 280
	splitByCharLimit            = 1000 // Maximum character limit for splitting text into manageable chunks for visual generation
)

// Gemini API types
type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}
