// File: template_service.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"net/http"
	"sort"
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

	// Handle visual style injection for visual_prompts type
	if templateType == "visual_prompts" && len(template.StyleIDs) > 0 {
		// Use the first style by default, or you can add logic to select specific style
		styleID := template.StyleIDs[0]

		var style VisualStyle
		err := visualStylesCollection.FindOne(context.Background(), bson.M{"_id": styleID, "is_active": true}).Decode(&style)
		if err != nil {
			return "", "", fmt.Errorf("visual style not found: %w", err)
		}

		// Build style rules text
		styleRulesText := "🎨 STRICT VISUAL STYLE (MANDATORY):\n"
		for _, rule := range style.StyleRules {
			styleRulesText += "- " + rule + "\n"
		}

		// Build prompt template text
		promptTemplateText := "🖼️ PROMPT TEMPLATE (REQUIRED FORMAT):\n\"" + style.PromptTemplate + "\""

		// Replace style placeholders
		userPrompt = strings.ReplaceAll(userPrompt, "{VISUAL_STYLE_RULES}", styleRulesText)
		userPrompt = strings.ReplaceAll(userPrompt, "{PROMPT_TEMPLATE}", promptTemplateText)
	}

	// Replace other variables
	for key, value := range variables {
		systemPrompt = strings.ReplaceAll(systemPrompt, key, value)
		userPrompt = strings.ReplaceAll(userPrompt, key, value)
	}

	return systemPrompt, userPrompt, nil
}
func (t *TemplateService) BuildDynamicPromptWithStyle(channelID primitive.ObjectID, templateType string, styleID primitive.ObjectID, variables map[string]string) (string, string, error) {
	template, err := t.GetPromptTemplate(channelID, templateType)
	if err != nil {
		return "", "", err
	}

	systemPrompt := template.SystemPrompt
	userPrompt := template.UserPrompt

	// Handle visual style injection
	if templateType == "visual_prompts" {
		var style VisualStyle
		err := visualStylesCollection.FindOne(context.Background(), bson.M{"_id": styleID, "is_active": true}).Decode(&style)
		if err != nil {
			return "", "", fmt.Errorf("visual style not found: %w", err)
		}

		// Build style rules text
		styleRulesText := "🎨 STRICT VISUAL STYLE (MANDATORY):\n"
		for _, rule := range style.StyleRules {
			styleRulesText += "- " + rule + "\n"
		}

		// Build prompt template text
		promptTemplateText := "🖼️ PROMPT TEMPLATE (REQUIRED FORMAT):\n\"" + style.PromptTemplate + "\""

		// Replace style placeholders
		userPrompt = strings.ReplaceAll(userPrompt, "{VISUAL_STYLE_RULES}", styleRulesText)
		userPrompt = strings.ReplaceAll(userPrompt, "{PROMPT_TEMPLATE}", promptTemplateText)

		systemPrompt = strings.ReplaceAll(systemPrompt, "{VISUAL_STYLE_NAME}", style.Name)
	}

	// Replace other variables
	for key, value := range variables {
		systemPrompt = strings.ReplaceAll(systemPrompt, key, value)
		userPrompt = strings.ReplaceAll(userPrompt, key, value)
	}

	return systemPrompt, userPrompt, nil
}

func (t *TemplateService) BuildOutlinePrompt(script *Script, sectionCount int) (string, string, error) {
	variables := map[string]string{
		"{TOPIC}":         script.Topic,
		"{SECTION_COUNT}": fmt.Sprintf("%d", sectionCount),
	}
	return t.BuildDynamicPrompt(script.ChannelID, "outline", variables)
}

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
		"{OUTLINE}":        script.Outline,
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
func (yt *YtAutomation) generateVisualPrompts(srtContent string, script *Script, styleID primitive.ObjectID) ([]VisualPromptResponse, error) {
	return yt.generateVisualPromptsWithStyle(srtContent, script, styleID)
}

func (yt *YtAutomation) generateVisualPromptsWithStyle(srtContent string, script *Script, styleID primitive.ObjectID) ([]VisualPromptResponse, error) {
	templateService := NewTemplateService()
	variables := map[string]string{
		"{SRT_CONTENT}": srtContent,
	}

	var systemPrompt, userPrompt string
	var err error

	// Use specific style if provided, otherwise use default from template
	if styleID != primitive.NilObjectID {
		systemPrompt, userPrompt, err = templateService.BuildDynamicPromptWithStyle(script.ChannelID, "visual_prompts", styleID, variables)
	} else {
		systemPrompt, userPrompt, err = templateService.BuildDynamicPrompt(script.ChannelID, "visual_prompts", variables)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to build visual prompt template: %w", err)
	}

	// Enhanced system prompt to ensure clean JSON response
	enhancedSystemPrompt := systemPrompt + `

CRITICAL JSON RESPONSE REQUIREMENTS:
- Return ONLY a valid JSON array
- NO explanatory text before or after JSON
- NO markdown code blocks
- NO "Here is..." or similar prefixes
- Start response directly with [ and end with ]
- Ensure all JSON is properly formatted and valid`

	// Use the enhanced system prompt
	response, err := yt.aiService.GenerateContentWithSystem(enhancedSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate visual prompts: %w", err)
	}

	// Clean the response using the enhanced function
	cleanResponse := cleanJSONResponse(response)

	// Log for debugging
	log.Printf("Original response length: %d", len(response))
	log.Printf("Cleaned response length: %d", len(cleanResponse))
	log.Printf("First 100 chars of cleaned response: %s", truncateString(cleanResponse, 100))

	// Additional JSON formatting fixes
	cleanResponse = fixJSONFormatting(cleanResponse)

	var visualPrompts []VisualPromptResponse
	if err := json.Unmarshal([]byte(cleanResponse), &visualPrompts); err != nil {
		log.Printf("JSON parsing error: %v", err)
		log.Printf("Problematic response (first 500 chars): %s", truncateString(cleanResponse, 500))

		// Try one more cleaning attempt - look for JSON array in the middle of text
		if jsonStart := strings.Index(cleanResponse, "[{"); jsonStart != -1 {
			if jsonEnd := strings.LastIndex(cleanResponse, "}]"); jsonEnd != -1 && jsonEnd > jsonStart {
				fallbackJSON := cleanResponse[jsonStart : jsonEnd+2]
				log.Printf("Attempting fallback JSON extraction: %s", truncateString(fallbackJSON, 200))

				if err := json.Unmarshal([]byte(fallbackJSON), &visualPrompts); err == nil {
					log.Printf("Fallback JSON parsing successful!")
					return visualPrompts, nil
				}
			}
		}

		return nil, fmt.Errorf("failed to parse visual prompts JSON: %w", err)
	}

	return visualPrompts, nil
}

// buildGapRecoveryPrompt creates a specialized prompt for filling gaps
func (t *TemplateService) buildGapRecoveryPrompt(channelID primitive.ObjectID, styleID primitive.ObjectID, gapReq GapRecoveryRequest) (string, string, error) {
	// Master Gap Recovery System Prompt with enhanced JSON requirements
	systemPrompt := `You are a specialized Visual Continuity Recovery AI. Your CRITICAL mission is to fill missing visual prompt gaps to ensure seamless video flow.

CONTEXT: The visual prompt sequence has gaps that break continuity. You must generate visual prompts SPECIFICALLY for the missing time ranges.

STRICT REQUIREMENTS:
1. Generate prompts ONLY for the exact time range specified
2. Ensure smooth visual transition from previous to next existing prompts  
3. Maintain narrative consistency and visual flow
4. NO overlapping with existing prompts
5. Cover the ENTIRE gap duration with logical scene progression

CRITICAL JSON RESPONSE REQUIREMENTS:
- Return ONLY a valid JSON array
- NO explanatory text, comments, or markdown
- NO "Here is..." or similar prefixes
- Start response directly with [ and end with ]
- Each object must have exactly: start_time, end_time, prompt
- All values must be properly quoted strings
- Ensure valid JSON syntax throughout`

	// Get visual style if specified
	var styleRulesText, promptTemplateText string
	if styleID != primitive.NilObjectID {
		var style VisualStyle
		err := visualStylesCollection.FindOne(context.Background(), bson.M{"_id": styleID, "is_active": true}).Decode(&style)
		if err == nil {
			styleRulesText = "🎨 MANDATORY VISUAL STYLE:\n"
			for _, rule := range style.StyleRules {
				styleRulesText += "- " + rule + "\n"
			}
			promptTemplateText = "🖼️ REQUIRED PROMPT FORMAT:\n\"" + style.PromptTemplate + "\""
		}
	}

	userPrompt := fmt.Sprintf(`🚨 CRITICAL GAP RECOVERY MISSION 🚨

GAP TO FILL:
- Start Time: %.2f seconds
- End Time: %.2f seconds  
- Duration: %.2f seconds

MISSING SRT CONTENT:
%s

VISUAL CONTEXT FOR CONTINUITY:
%s

%s

%s

RESPONSE FORMAT - RETURN ONLY THIS JSON STRUCTURE:
[
  {
    "start_time": "%.2f",
    "end_time": "[calculated_end_time]",  
    "prompt": "[visual_prompt_description]"
  }
]

CRITICAL: Return ONLY the JSON array above. No explanations, no markdown, no additional text.`,
		gapReq.StartTime, gapReq.EndTime, gapReq.EndTime-gapReq.StartTime,
		gapReq.SRTContent,
		gapReq.Context,
		styleRulesText,
		promptTemplateText,
		gapReq.StartTime)

	return systemPrompt, userPrompt, nil
}
func (yt *YtAutomation) generateGapRecoveryPrompts(gaps []GapRecoveryRequest, script *Script, styleID primitive.ObjectID) ([]VisualPromptResponse, error) {
	var recoveryPrompts []VisualPromptResponse
	templateService := NewTemplateService()

	for _, gap := range gaps {
		log.Printf("Generating recovery prompts for gap: %.2f-%.2f seconds", gap.StartTime, gap.EndTime)

		systemPrompt, userPrompt, err := templateService.buildGapRecoveryPrompt(script.ChannelID, styleID, gap)
		if err != nil {
			log.Printf("Failed to build gap recovery prompt: %v", err)
			continue
		}

		response, err := yt.aiService.GenerateContentWithSystem(systemPrompt, userPrompt)
		if err != nil {
			log.Printf("Failed to generate gap recovery prompts: %v", err)
			continue
		}

		// Clean and parse JSON response
		cleanResponse := strings.TrimSpace(response)
		if strings.HasPrefix(cleanResponse, "```json") {
			cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
		} else if strings.HasPrefix(cleanResponse, "```") {
			cleanResponse = strings.TrimPrefix(cleanResponse, "```")
		}
		if strings.HasSuffix(cleanResponse, "```") {
			cleanResponse = strings.TrimSuffix(cleanResponse, "```")
		}
		cleanResponse = strings.TrimSpace(cleanResponse)
		cleanResponse = fixJSONFormatting(cleanResponse)

		var gapPrompts []VisualPromptResponse
		if err := json.Unmarshal([]byte(cleanResponse), &gapPrompts); err != nil {
			log.Printf("Failed to parse gap recovery JSON: %v\nResponse: %s", err, cleanResponse)
			continue
		}

		recoveryPrompts = append(recoveryPrompts, gapPrompts...)
		log.Printf("Generated %d recovery prompts for gap", len(gapPrompts))
	}

	return recoveryPrompts, nil
}

// Enhanced validation function that also performs gap recovery
func (yt *YtAutomation) validateAndRecoverVisualPrompts1(srtContent string, script *Script, styleID primitive.ObjectID, existingPrompts []VisualPromptResponse) ([]VisualPromptResponse, error) {
	// Extract SRT time ranges
	srtRanges, err := extractSRTTimeRanges(srtContent)
	if err != nil {
		return existingPrompts, fmt.Errorf("failed to extract SRT time ranges: %w", err)
	}

	// Detect gaps
	gaps, err := detectGapsInSequence(srtRanges, existingPrompts)
	if err != nil {
		return existingPrompts, fmt.Errorf("failed to detect gaps: %w", err)
	}

	if len(gaps) == 0 {
		log.Printf("No gaps detected in visual prompt sequence")
		return existingPrompts, nil
	}

	log.Printf("Detected %d gaps, initiating recovery process", len(gaps))

	// Generate recovery prompts
	recoveryPrompts, err := yt.generateGapRecoveryPrompts(gaps, script, styleID)
	if err != nil {
		log.Printf("Gap recovery failed: %v", err)
		return existingPrompts, nil // Return original prompts if recovery fails
	}

	// Merge existing and recovery prompts
	allPrompts := append(existingPrompts, recoveryPrompts...)

	// Sort by start time
	sort.Slice(allPrompts, func(i, j int) bool {
		startI, _ := parseTimeToFloat(allPrompts[i].StartTime)
		startJ, _ := parseTimeToFloat(allPrompts[j].StartTime)
		return startI < startJ
	})

	log.Printf("Gap recovery complete. Total prompts: %d (original: %d, recovered: %d)",
		len(allPrompts), len(existingPrompts), len(recoveryPrompts))

	return allPrompts, nil
}

func (yt *YtAutomation) generateVisualPromptsWithStyleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req struct {
		ScriptID string `json:"script_id"`
		StyleID  string `json:"style_id"`
		Force    bool   `json:"force,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	scriptID, err := primitive.ObjectIDFromHex(req.ScriptID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid script ID")
		return
	}

	styleID, err := primitive.ObjectIDFromHex(req.StyleID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid style ID")
		return
	}
	var script Script
	err = scriptsCollection.FindOne(context.Background(), bson.M{"_id": scriptID}).Decode(&script)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	// Check if chunks already exist for this script
	existingCount, err := scriptAudiosCollection.CountDocuments(
		context.Background(),
		bson.M{"script_id": scriptID},
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error checking existing chunks: %v", err))
		return
	}

	var scriptSrtChunks []ScriptSrt

	if existingCount > 0 {

		findOptions := options.Find().SetSort(bson.M{"chunk_index": 1})
		cursor, err := scriptSrtCollection.Find(
			context.Background(),
			bson.M{"script_id": scriptID},
			findOptions,
		)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error fetching existing chunks: %v", err))
			return
		}
		defer cursor.Close(context.Background())

		if err = cursor.All(context.Background(), &scriptSrtChunks); err != nil {
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error decoding existing chunks: %v", err))
			return
		}
	}
	go func() {
		if err := yt.generateVisualPromptForChunksWithRecovery(scriptID, scriptSrtChunks, styleID, req.Force); err != nil {
			fmt.Printf("Warning: Failed to generate visuals for chunks: %v\n", err)
		}
	}()

	data := map[string]interface{}{
		"script_id": scriptID,
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Visual Prompt generation InProgress",
		"data":    data,
	})
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

	// Convert string style IDs to ObjectIDs
	var styleIDs []primitive.ObjectID
	for _, styleID := range req.StyleIDs {
		if styleID != "" {
			objID, err := primitive.ObjectIDFromHex(styleID)
			if err != nil {
				respondWithError(w, http.StatusBadRequest, "Invalid style ID: "+styleID)
				return
			}
			styleIDs = append(styleIDs, objID)
		}
	}

	template := PromptTemplate{
		Name:         req.Name,
		Type:         req.Type,
		SystemPrompt: req.SystemPrompt,
		UserPrompt:   req.UserPrompt,
		Variables:    req.Variables,
		StyleIDs:     styleIDs, // NEW: Store style references
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
func (yt *YtAutomation) createVisualStyleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req VisualStyleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	style := VisualStyle{
		Name:           req.Name,
		Category:       req.Category,
		Description:    req.Description,
		StyleRules:     req.StyleRules,
		PromptTemplate: req.PromptTemplate,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	result, err := visualStylesCollection.InsertOne(context.Background(), style)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create visual style")
		return
	}

	style.ID = result.InsertedID.(primitive.ObjectID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    style,
	})
}

func (yt *YtAutomation) getVisualStylesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	category := r.URL.Query().Get("category")
	filter := bson.M{"is_active": true}
	if category != "" {
		filter["category"] = category
	}

	cursor, err := visualStylesCollection.Find(context.Background(), filter)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch visual styles")
		return
	}
	defer cursor.Close(context.Background())

	var styles []VisualStyle
	if err = cursor.All(context.Background(), &styles); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to decode visual styles")
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    styles,
	})
}

// NEW UTILITY FUNCTION - For runtime style injection
func (yt *YtAutomation) buildPromptWithStyle(template PromptTemplate, styleID primitive.ObjectID, variables map[string]string) (string, error) {
	// Get the visual style
	var style VisualStyle
	err := visualStylesCollection.FindOne(context.Background(), bson.M{"_id": styleID, "is_active": true}).Decode(&style)
	if err != nil {
		return "", fmt.Errorf("visual style not found")
	}

	// Build style rules text
	styleRulesText := "🎨 STRICT VISUAL STYLE (MANDATORY):\n"
	for _, rule := range style.StyleRules {
		styleRulesText += "- " + rule + "\n"
	}

	// Build prompt template
	promptTemplateText := "🖼️ PROMPT TEMPLATE (REQUIRED FORMAT):\n\"" + style.PromptTemplate + "\""

	// Replace placeholders in user prompt
	userPrompt := template.UserPrompt
	userPrompt = strings.ReplaceAll(userPrompt, "{VISUAL_STYLE_RULES}", styleRulesText)
	userPrompt = strings.ReplaceAll(userPrompt, "{PROMPT_TEMPLATE}", promptTemplateText)

	// Replace other variables
	for key, value := range variables {
		userPrompt = strings.ReplaceAll(userPrompt, key, value)
	}

	return userPrompt, nil
}
