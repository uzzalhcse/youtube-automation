package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

func (t *TemplateService) getTemplateByType(templateType string) (*Template, error) {
	ctx := context.Background()

	var template Template
	err := templatesCollection.FindOne(ctx, bson.M{
		"type":      templateType,
		"is_active": true,
	}).Decode(&template)

	if err != nil {
		return nil, fmt.Errorf("template not found for type %s: %w", templateType, err)
	}

	return &template, nil
}

func (t *TemplateService) GetOutlineTemplate(topic string) (string, error) {
	template, err := t.getTemplateByType("outline")
	if err != nil {
		return "", err
	}
	return strings.Replace(template.Content, "[TOPIC]", topic, -1), nil
}

func (t *TemplateService) GetScriptTemplate() (string, error) {
	template, err := t.getTemplateByType("script")
	if err != nil {
		return "", err
	}
	return template.Content, nil
}

func (t *TemplateService) GetHookIntroTemplate() (string, error) {
	template, err := t.getTemplateByType("hook_intro")
	if err != nil {
		return "", err
	}
	return template.Content, nil
}

func (t *TemplateService) GetMetaTagTemplate() (string, error) {
	template, err := t.getTemplateByType("meta_tag")
	if err != nil {
		return "", err
	}
	return template.Content, nil
}

func (t *TemplateService) GetVisualGuidanceTemplate() (string, error) {
	template, err := t.getTemplateByType("visual_guidance")
	if err != nil {
		return "", err
	}
	return template.Content, nil
}
func (t *TemplateService) BuildOutlinePrompt(topic string, sectionCount int) (string, error) {
	template, err := t.GetOutlineTemplate(topic)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`%s

IMPORTANT REQUIREMENTS:
- Provide EXACTLY %d bullet points for the outline.
- Each bullet should be formatted as: Title: Description.
- No sub bullet points or nested lists in descriptions, only paragraphs.

Topic: %s

Please provide the outline following the template specifications exactly.`,
		template, sectionCount, topic), nil
}

func (t *TemplateService) BuildHookIntroPrompt(session *ScriptSession) (string, error) {
	template, err := t.GetHookIntroTemplate()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`%s

CONTEXT:
- Outline: %s
- Topic: %s

REQUIREMENTS:
**Hook & Introduction (%d words):**
 - Start with a relatable scenario, question, concern, or statement to hook the audience (e.g., 'Have you ever felt like your energy is fading faster than it used to?', "Tired of waking up with painful leg cramps?").  
 - Briefly introduce the topic and explain why it's important for seniors.  
 - Mention what the video will cover (e.g., 'In this video, we'll go over 5 simple habits to boost your energy and feel younger than ever!').  
 - End with a call to action, Ask them to like the video and subscribe to the channel for more helpful content.
 - Create smooth transition to main content.
 - Maintain consistency with the hook's tone.

Please write the section now, Without labeled like "Hook & Introduction:" Just continue.`,
		template, session.Outline, session.Config.Topic, session.Config.channel.Settings.WordLimitForHookIntro), nil
}

func (t *TemplateService) BuildMetaTagPrompt(session *ScriptSession) (string, error) {
	template, err := t.GetMetaTagTemplate()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`%s

CONTEXT:
- Outline: %s
- Topic: %s

Please write the section now, Without any labeled. Just continue.`,
		template, session.Outline, session.Config.Topic), nil
}

// 8. Add REST API handlers for template management
func (yt *YtAutomation) createTemplateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed. Use POST.")
		return
	}

	var req TemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	// Validate request
	if strings.TrimSpace(req.Name) == "" {
		respondWithError(w, http.StatusBadRequest, "Template name cannot be empty")
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		respondWithError(w, http.StatusBadRequest, "Template type cannot be empty")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		respondWithError(w, http.StatusBadRequest, "Template content cannot be empty")
		return
	}

	// Validate template type
	validTypes := []string{"outline", "script", "hook_intro", "meta_tag", "visual_guidance"}
	isValidType := false
	for _, validType := range validTypes {
		if req.Type == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		respondWithError(w, http.StatusBadRequest, "Invalid template type. Must be one of: outline, script, hook_intro, meta_tag, visual_guidance")
		return
	}

	template := Template{
		Name:        strings.TrimSpace(req.Name),
		Type:        strings.TrimSpace(req.Type),
		Content:     strings.TrimSpace(req.Content),
		Description: strings.TrimSpace(req.Description),
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
	}

	result, err := templatesCollection.InsertOne(context.Background(), template)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			respondWithError(w, http.StatusConflict, "Template with this name and type already exists")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create template: %v", err))
		return
	}

	template.ID = result.InsertedID.(primitive.ObjectID)

	response := TemplateResponse{
		Success: true,
		Message: "Template created successfully",
		Data:    &template,
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// BuildSectionPrompt creates a context-aware section prompt
func (t *TemplateService) BuildSectionPrompt(session *ScriptSession, sectionNumber int, outlinePoint string) string {
	basePrompt := fmt.Sprintf(`SECTION %d GENERATION REQUEST

CURRENT OUTLINE POINT: %s

REQUIREMENTS:
- Write approx %d words.
- Copy Paste Ready for voiceover script like ElevenLabs or other AI voice generators.
- Dont Start with any Section heading.
- Dont not include any visual guidance or image descriptions.
- Focus specifically on the outline point: "%s".
- Avoid using bullet points, numbered lists, or section titles. Instead, use natural transitions to guide the viewer through the content.  
- Expand precisely that content—no drifting to later bullets.  
- Maintain seamless flow from previous content.
- Absolutely **no** new section intros like “In this chapter…”—just continue.
- Use callbacks (“remember that crocodile‑dung sunscreen?”) for cohesion.  
- Do NOT re‑introduce the topic at each section break.  
- Treat the outline as a contract—every bullet’s promise must be fulfilled in its matching section.

Generate Section %d now, focusing on: %s`,
		sectionNumber, outlinePoint, session.Config.channel.Settings.WordLimitForHookIntro, outlinePoint, sectionNumber, outlinePoint)

	return basePrompt
}

func (t *TemplateService) BuildVisualGuidancePrompt(session *ScriptSession, sectionCount int) string {
	return fmt.Sprintf(`VISUAL GUIDANCE GENERATION

Based on the complete script content for topic: %s

REQUIREMENTS:
- EXACTLY %d visual descriptions for Hook and Introduction combined.
- EXACTLY %d visual descriptions for EACH of the %d sections (%d visuals total).
- Format: "Hook Visual 1:", "Introduction Visual 1:", "Section X Visual 1:", etc.
- Brief, clear descriptions suitable for AI image generation.
- Senior-friendly aesthetic: calm visuals, warm lighting, pastel colors.
- Simple, minimal, realistic-stylized approach.
- Focus on reassuring, gentle, and trustworthy imagery.
- Avoid complex or busy compositions.

VISUAL STYLE GUIDELINES:
- Warm, soft lighting.
- Pastel and earth tone color palette.
- Clean, uncluttered compositions.
- Senior-friendly imagery (diverse older adults when people are shown).
- Professional but approachable aesthetic.
- Clear, readable text elements when needed.

Please provide visual guidance following this exact format."`,
		session.Config.Topic, visualImageMultiplier, visualImageMultiplier, sectionCount, sectionCount*visualImageMultiplier)
}

func (yt *YtAutomation) generateVisualPrompts(srtContent string) ([]VisualPromptResponse, error) {
	masterPrompt := `You are a visual narration mapping assistant.
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
    "prompt": "A stick figure in a red scarf looks at the sky full of clocks and stars, dreaming. Minimalist cartoon style. Mood: hopeful."
  }
]

🎯 Target: Output ~1 visual per idea or emotional beat. Do not compress multiple beats into one. If in doubt, split it.

Now here is the .srt file:
` + srtContent

	response, err := yt.geminiService.GenerateContent(masterPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate visual prompts: %w", err)
	}

	// Clean response - remove markdown code blocks if present
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

	// Parse JSON response
	var visualPrompts []VisualPromptResponse
	if err := json.Unmarshal([]byte(cleanResponse), &visualPrompts); err != nil {
		return nil, fmt.Errorf("failed to parse visual prompts JSON: %w", err)
	}

	return visualPrompts, nil
}
func (yt *YtAutomation) getTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	templateType := r.URL.Query().Get("type")

	filter := bson.M{"is_active": true}
	if templateType != "" {
		filter["type"] = templateType
	}

	cursor, err := templatesCollection.Find(
		context.Background(),
		filter,
		options.Find().SetSort(bson.D{{"type", 1}, {"name", 1}}),
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}
	defer cursor.Close(context.Background())

	var templates []Template
	if err = cursor.All(context.Background(), &templates); err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding templates: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    templates,
		"count":   len(templates),
	})
}

func (yt *YtAutomation) updateTemplateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "PUT" {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed. Use PUT.")
		return
	}

	// Extract template ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/templates/")
	if path == "" {
		respondWithError(w, http.StatusBadRequest, "Template ID is required")
		return
	}

	objectID, err := primitive.ObjectIDFromHex(path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid template ID format")
		return
	}

	var req TemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}

	updateData := bson.M{
		"updated_at": time.Now(),
		"$inc":       bson.M{"version": 1},
	}

	if strings.TrimSpace(req.Name) != "" {
		updateData["name"] = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Content) != "" {
		updateData["content"] = strings.TrimSpace(req.Content)
	}
	if strings.TrimSpace(req.Description) != "" {
		updateData["description"] = strings.TrimSpace(req.Description)
	}

	result, err := templatesCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": updateData},
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update template: %v", err))
		return
	}

	if result.MatchedCount == 0 {
		respondWithError(w, http.StatusNotFound, "Template not found")
		return
	}

	// Fetch updated template
	var updatedTemplate Template
	err = templatesCollection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&updatedTemplate)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch updated template: %v", err))
		return
	}

	response := TemplateResponse{
		Success: true,
		Message: "Template updated successfully",
		Data:    &updatedTemplate,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (yt *YtAutomation) deleteTemplateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "DELETE" {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed. Use DELETE.")
		return
	}

	// Extract template ID from URL
	path := strings.TrimPrefix(r.URL.Path, "/templates/")
	if path == "" {
		respondWithError(w, http.StatusBadRequest, "Template ID is required")
		return
	}

	objectID, err := primitive.ObjectIDFromHex(path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid template ID format")
		return
	}

	// Soft delete by setting is_active to false
	result, err := templatesCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": bson.M{"is_active": false, "updated_at": time.Now()}},
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete template: %v", err))
		return
	}

	if result.MatchedCount == 0 {
		respondWithError(w, http.StatusNotFound, "Template not found")
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Template deleted successfully",
	})
}
