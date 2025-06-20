// File: template_service.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"net/http"
	"strings"
	"time"
)

// TemplateService manages template loading and processing
type TemplateService struct {
}

// NewTemplateService creates a new template service
func NewTemplateService() *TemplateService {
	return &TemplateService{}
}

func (t *TemplateService) GetPromptTemplate(channelID primitive.ObjectID, templateType string) (*PromptTemplate, error) {
	ctx := context.Background()

	// Try channel-specific first
	var template PromptTemplate
	err := promptTemplatesCollection.FindOne(ctx, bson.M{
		"channel_id": channelID,
		"type":       templateType,
		"is_active":  true,
	}).Decode(&template)

	if err == nil {
		return &template, nil
	}

	// Fallback to global template
	err = promptTemplatesCollection.FindOne(ctx, bson.M{
		"is_global": true,
		"type":      templateType,
		"is_active": true,
	}).Decode(&template)

	if err != nil {
		return nil, fmt.Errorf("no prompt template found for type %s: %w", templateType, err)
	}

	return &template, nil
}

func (t *TemplateService) BuildDynamicPrompt(channelID primitive.ObjectID, templateType string, variables map[string]string) (string, string, error) {
	template, err := t.GetPromptTemplate(channelID, templateType)
	if err != nil {
		return "", "", err
	}

	systemPrompt := template.SystemPrompt
	userPrompt := template.UserPrompt

	// Replace variables
	for key, value := range variables {
		systemPrompt = strings.ReplaceAll(systemPrompt, key, value)
		userPrompt = strings.ReplaceAll(userPrompt, key, value)
	}

	return systemPrompt, userPrompt, nil
}

// Replace existing BuildOutlinePrompt
func (t *TemplateService) BuildOutlinePrompt(script *Script, sectionCount int) (string, string, error) {
	variables := map[string]string{
		"{TOPIC}":         script.Topic,
		"{SECTION_COUNT}": fmt.Sprintf("%d", sectionCount),
	}
	return t.BuildDynamicPrompt(script.ChannelID, "outline", variables)
}

// Replace existing BuildHookIntroPrompt
func (t *TemplateService) BuildHookIntroPrompt(script *Script, wordLimit int) (string, string, error) {
	variables := map[string]string{
		"{OUTLINE}":    script.Outline,
		"{TOPIC}":      script.Topic,
		"{WORD_LIMIT}": fmt.Sprintf("%d", wordLimit),
	}
	return t.BuildDynamicPrompt(script.ChannelID, "hook_intro", variables)
}

// Replace existing BuildMetaTagPrompt
func (t *TemplateService) BuildMetaTagPrompt(script *Script) (string, string, error) {
	variables := map[string]string{
		"{OUTLINE}": script.Outline,
		"{TOPIC}":   script.Topic,
	}
	return t.BuildDynamicPrompt(script.ChannelID, "meta_tag", variables)
}

func (t *TemplateService) BuildSectionPrompt(script *Script, sectionNumber int, outlinePoint string, wordLimit int) (string, string, error) {
	variables := map[string]string{
		"{SECTION_NUMBER}": fmt.Sprintf("%d", sectionNumber),
		"{OUTLINE_POINT}":  outlinePoint,
		"{WORD_LIMIT}":     fmt.Sprintf("%d", wordLimit),
		"{TOPIC}":          script.Topic,
	}
	return t.BuildDynamicPrompt(script.ChannelID, "section", variables)
}

// Replace existing BuildVisualGuidancePrompt method
func (t *TemplateService) BuildVisualGuidancePrompt(script *Script, sectionCount int, visualImageMultiplier int) (string, string, error) {
	variables := map[string]string{
		"{TOPIC}":                   script.Topic,
		"{SECTION_COUNT}":           fmt.Sprintf("%d", sectionCount),
		"{VISUAL_IMAGE_MULTIPLIER}": fmt.Sprintf("%d", visualImageMultiplier),
		"{TOTAL_VISUALS}":           fmt.Sprintf("%d", sectionCount*visualImageMultiplier),
	}

	return t.BuildDynamicPrompt(script.ChannelID, "visual_guidance", variables)
}

// Replace existing generateVisualPrompts method
func (yt *YtAutomation) generateVisualPrompts(srtContent string, script *Script) ([]VisualPromptResponse, error) {
	// Get dynamic prompt template
	templateService := NewTemplateService()
	variables := map[string]string{
		"{SRT_CONTENT}": srtContent,
	}

	systemPrompt, userPrompt, err := templateService.BuildDynamicPrompt(script.ChannelID, "visual_prompts", variables)
	if err != nil {
		// Fallback to hardcoded prompt if template not found
		systemPrompt = "You are a visual narration mapping assistant."
		userPrompt = `You are a visual narration mapping assistant.
I will give you .srt subtitle data. Your job is to output a dense series of visual prompts that match each key beat of the spoken narration.

🧠 Visual Chunking Rules:
New sentence? → Start a new visual unit
Short line (e.g. <3s or <6 words)? → Merge only if it feels like a continuous idea
Powerful/emotional words (e.g. "Boom." "Yeah?" "Lazy.") → Give their own visual moment
Conceptual/emotional shifts? → Start new visual
If uncertain: Prefer more visuals, not fewer
⚠️ Avoid merging more than 10 seconds of narration into a single prompt.

🎨 Visual Prompt Template:
For each chunk, generate a unique visual using this template:
A hand-drawn cartoon scene with a stick figure in a red scarf. Scene: {scene_concept}. Style: sketchy black-and-white with minimal red accent. Background in soft beige. Emotion: {emotion_or_mood}.

🧠 scene_concept should reflect what's happening or being said (literal, metaphorical, or symbolic).
💬 emotion_or_mood should capture the tone: hopeful, anxious, dreamy, frustrated, etc.

✅ Output Format:
IMPORTANT: Return ONLY the JSON array, no markdown formatting, no backticks, no code blocks.
A JSON array like this:
[
  {
    "start": "00:00:00,000",
    "end": "00:00:04,940",
    "prompt": "A stick figure in a red scarf looks at the sky full of clouds and stars, dreaming. Minimalist cartoon style. Mood: hopeful."
  }
]

🎯 Target: Output ~1 visual per idea or emotional beat. Do not compress multiple beats into one. If in doubt, split it.

Now here is the .srt file:
` + srtContent
	}

	// Rest of the method remains the same...
	finalPrompt := systemPrompt + "\n\n" + userPrompt

	response, err := yt.geminiService.GenerateContent(finalPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate visual prompts: %w", err)
	}

	// Rest of the existing parsing logic remains the same
	cleanResponse := strings.TrimSpace(response)
	if strings.HasPrefix(cleanResponse, "```json") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
	}
	if strings.HasPrefix(cleanResponse, "```") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "```")
	}
	if strings.HasSuffix(cleanResponse, "```") {
		cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	}
	cleanResponse = strings.TrimSpace(cleanResponse)

	var visualPrompts []VisualPromptResponse
	if err := json.Unmarshal([]byte(cleanResponse), &visualPrompts); err != nil {
		return nil, fmt.Errorf("failed to parse visual prompts JSON: %w", err)
	}

	return visualPrompts, nil
}
func (yt *YtAutomation) createPromptTemplateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req PromptTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	template := PromptTemplate{
		Name:         req.Name,
		Type:         req.Type,
		SystemPrompt: req.SystemPrompt,
		UserPrompt:   req.UserPrompt,
		Variables:    req.Variables,
		IsActive:     true,
		IsGlobal:     req.IsGlobal,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Version:      1,
	}

	if req.ChannelID != "" && !req.IsGlobal {
		channelID, err := primitive.ObjectIDFromHex(req.ChannelID)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid channel ID")
			return
		}
		template.ChannelID = channelID
	}

	result, err := promptTemplatesCollection.InsertOne(context.Background(), template)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create prompt template")
		return
	}

	template.ID = result.InsertedID.(primitive.ObjectID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    template,
	})
}
func (yt *YtAutomation) getPromptTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	channelID := r.URL.Query().Get("channel_id")
	templateType := r.URL.Query().Get("type")

	filter := bson.M{"is_active": true}

	if channelID != "" {
		objID, err := primitive.ObjectIDFromHex(channelID)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid channel ID")
			return
		}
		filter = bson.M{
			"is_active":  true,
			"channel_id": objID,
			"$or": []bson.M{
				{"is_global": false},
			},
		}
	}

	if templateType != "" {
		filter["type"] = templateType
	}

	cursor, err := promptTemplatesCollection.Find(context.Background(), filter)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer cursor.Close(context.Background())

	var templates []PromptTemplate
	cursor.All(context.Background(), &templates)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    templates,
	})
}
