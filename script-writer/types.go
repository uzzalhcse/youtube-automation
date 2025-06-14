package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScriptConfig holds configuration for script generation
type ScriptConfig struct {
	Topic                string
	ChannelName          string // New field for channel name
	GenerateVisuals      bool
	OutputFilename       string // This will be the script filename
	MetaTagFilename      string // New field for meta tag filename
	OutputFolder         string // New field for output folder path (video title)
	SectionCount         int
	SleepBetweenSections time.Duration
}

// ScriptSession represents the current state of script generation
type ScriptSession struct {
	Config          *ScriptConfig
	ScriptFilename  string // Path to script file
	MetaTagFilename string // Path to meta tag file
	OutputFolder    string // Full folder path
	CurrentStep     int
	Outline         string
	OutlinePoints   []string
	Hook            string
	MetaTag         string
	Introduction    string
	Content         strings.Builder
	Context         *ScriptContext // Enhanced context tracking
}

// ScriptContext maintains context across API calls to improve coherence
type ScriptContext struct {
	PreviousContent  string
	KeyThemes        []string
	ToneAndStyle     string
	TargetAudience   string
	MainObjectives   []string
	ContentSummary   string
	TransitionPhrase string
}

const (
	defaultSectionCount         = 3
	visualImageMultiplier       = 5
	defaultSleepBetweenSections = 1 * time.Second
	maxRetries                  = 5
	channelName                 = "Wisderly" // Default channel name
)

// NewScriptConfig creates a new script configuration with proper file paths
func NewScriptConfig(topic, channelName, videoTitle string) *ScriptConfig {
	// Sanitize video title for folder name
	sanitizedVideoTitle := sanitizeFilename(videoTitle)

	config := &ScriptConfig{
		Topic:                topic,
		ChannelName:          channelName,
		GenerateVisuals:      true,
		OutputFolder:         sanitizedVideoTitle, // Just the video title, not combined
		OutputFilename:       "script.txt",
		MetaTagFilename:      "metatag.txt",
		SectionCount:         defaultSectionCount,
		SleepBetweenSections: defaultSleepBetweenSections,
	}

	return config
}

// NewScriptSession creates a new script session
func NewScriptSession(config *ScriptConfig) *ScriptSession {
	// Create the full folder path: ChannelName/VideoTitle
	fullFolderPath := filepath.Join("output-scripts", config.ChannelName, config.OutputFolder)

	// Create output folder if it doesn't exist
	if err := os.MkdirAll(fullFolderPath, 0755); err != nil {
		fmt.Printf("Warning: Could not create output folder: %v\n", err)
	}

	return &ScriptSession{
		Config:          config,
		ScriptFilename:  filepath.Join(fullFolderPath, config.OutputFilename),
		MetaTagFilename: filepath.Join(fullFolderPath, config.MetaTagFilename),
		OutputFolder:    fullFolderPath,
		Context: &ScriptContext{
			ToneAndStyle:   "calm, trustworthy, senior-friendly",
			TargetAudience: "seniors aged 60+",
			KeyThemes:      []string{},
			MainObjectives: []string{},
		},
	}
}

// UpdateContext updates the session context with new content
func (s *ScriptSession) UpdateContext(newContent, contentType string) {
	s.Context.PreviousContent = newContent

	// Extract key information based on content type
	switch contentType {
	case "outline":
		s.extractOutlineThemes(newContent)
	case "hook_intro":
		s.extractHookIntroElements(newContent)
	case "section":
		s.updateSectionContext(newContent)
	case "meta_tag":
		s.extractMetaTagElements(newContent)
	}

	// Update content summary
	s.updateContentSummary()
}

func (s *ScriptSession) extractOutlineThemes(outline string) {
	// Simple theme extraction from outline
	lines := strings.Split(outline, "\n")
	for _, line := range lines {
		if strings.Contains(line, "**") {
			// Extract bold text as key themes
			start := strings.Index(line, "**")
			end := strings.LastIndex(line, "**")
			if start != -1 && end != -1 && end > start+2 {
				theme := strings.TrimSpace(line[start+2 : end])
				if theme != "" {
					s.Context.KeyThemes = append(s.Context.KeyThemes, theme)
				}
			}
		}
	}
}

func (s *ScriptSession) extractHookIntroElements(content string) {
	// Extract main objectives from hook and introduction
	if strings.Contains(strings.ToLower(content), "important") {
		s.Context.MainObjectives = append(s.Context.MainObjectives, "establish importance")
	}
	if strings.Contains(strings.ToLower(content), "help") {
		s.Context.MainObjectives = append(s.Context.MainObjectives, "provide help")
	}
}

func (s *ScriptSession) extractMetaTagElements(content string) {
	// Extract key SEO themes from meta tag content
	contentLower := strings.ToLower(content)

	// Look for SEO-related keywords that might be useful for maintaining consistency
	if strings.Contains(contentLower, "senior") || strings.Contains(contentLower, "elderly") {
		s.Context.KeyThemes = append(s.Context.KeyThemes, "senior-focused")
	}
	if strings.Contains(contentLower, "health") {
		s.Context.KeyThemes = append(s.Context.KeyThemes, "health-related")
	}
	if strings.Contains(contentLower, "longevity") || strings.Contains(contentLower, "life") {
		s.Context.KeyThemes = append(s.Context.KeyThemes, "longevity-focused")
	}
}

func (s *ScriptSession) updateSectionContext(content string) {
	// Generate transition phrase for next section
	lastSentences := s.getLastSentences(content, 2)
	if len(lastSentences) > 0 {
		s.Context.TransitionPhrase = "Building on this understanding"
	}
}

func (s *ScriptSession) updateContentSummary() {
	summary := fmt.Sprintf("Topic: %s. ", s.Config.Topic)
	if len(s.Context.KeyThemes) > 0 {
		// Remove duplicates from key themes
		uniqueThemes := removeDuplicates(s.Context.KeyThemes)
		summary += fmt.Sprintf("Key themes: %s. ", strings.Join(uniqueThemes, ", "))
	}
	if len(s.Context.MainObjectives) > 0 {
		summary += fmt.Sprintf("Objectives: %s. ", strings.Join(s.Context.MainObjectives, ", "))
	}
	s.Context.ContentSummary = summary
}

func (s *ScriptSession) getLastSentences(content string, count int) []string {
	sentences := strings.Split(content, ".")
	if len(sentences) < count {
		return sentences
	}
	return sentences[len(sentences)-count:]
}

// GetContextPrompt returns a context-aware prompt prefix
func (s *ScriptSession) GetContextPrompt() string {
	contextPrompt := fmt.Sprintf(`CONTEXT FOR COHERENT CONTINUATION:
- Topic: %s
- Target Audience: %s
- Tone & Style: %s
- Content Summary: %s`,
		s.Config.Topic,
		s.Context.TargetAudience,
		s.Context.ToneAndStyle,
		s.Context.ContentSummary)

	if len(s.Context.KeyThemes) > 0 {
		uniqueThemes := removeDuplicates(s.Context.KeyThemes)
		contextPrompt += fmt.Sprintf("\n- Key Themes to Maintain: %s", strings.Join(uniqueThemes, ", "))
	}

	if s.Context.TransitionPhrase != "" {
		contextPrompt += fmt.Sprintf("\n- Suggested Transition: %s", s.Context.TransitionPhrase)
	}

	if s.Context.PreviousContent != "" {
		// Include last part of previous content for continuity
		lastPart := s.getContentTail(s.Context.PreviousContent, 200)
		contextPrompt += fmt.Sprintf("\n- Previous Content Ending: ...%s", lastPart)
	}

	return contextPrompt + "\n\n"
}

func (s *ScriptSession) getContentTail(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	return content[len(content)-maxLength:]
}

// Helper function to remove duplicates from string slice
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}

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
