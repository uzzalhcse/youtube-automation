package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strings"
	"time"
)

func (yt *YtAutomation) generateVisualsForChunks(scriptID primitive.ObjectID, chunks []ScriptAudio) error {
	fmt.Printf("🎨 Starting visual generation for %d chunks...\n", len(chunks))

	for i, chunk := range chunks {
		fmt.Printf("Generating visual for chunk %d/%d...\n", i+1, len(chunks))

		// Create SRT-like format for this chunk
		srtContent := yt.createSRTFromChunk(chunk, i)

		// Generate visual prompt using Gemini
		visualPrompts, err := yt.generateVisualPrompts(srtContent)
		if err != nil {
			fmt.Printf("Warning: Failed to generate visual for chunk %d: %v\n", chunk.ChunkIndex, err)
			continue
		}

		// Save visual prompts to database
		if err := yt.saveChunkVisuals(scriptID, chunk, visualPrompts); err != nil {
			fmt.Printf("Warning: Failed to save visuals for chunk %d: %v\n", chunk.ChunkIndex, err)
			continue
		}

		// Update chunk to mark it has visual
		yt.updateChunkVisualStatus(chunk.ID, true)

		// Small delay between API calls
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("✓ Completed visual generation for all chunks\n")
	return nil
}

func (yt *YtAutomation) createSRTFromChunk(chunk ScriptAudio, index int) string {
	// Create a simple SRT format for the chunk
	// Estimate timing based on chunk length (average reading speed)
	wordsCount := len(strings.Fields(chunk.Content))
	durationSeconds := max(3, wordsCount/3) // ~3 words per second reading speed

	startTime := fmt.Sprintf("00:00:%02d,000", index*durationSeconds)
	endTime := fmt.Sprintf("00:00:%02d,000", (index+1)*durationSeconds)

	return fmt.Sprintf("%d\n%s --> %s\n%s\n\n",
		index+1, startTime, endTime, chunk.Content)
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
func (yt *YtAutomation) saveChunkVisuals(scriptID primitive.ObjectID, chunk ScriptAudio, visualPrompts []VisualPromptResponse) error {
	// Check if collection is initialized
	if chunkVisualsCollection == nil {
		return fmt.Errorf("chunk visuals collection is not initialized")
	}

	var visualDocs []interface{}
	for _, prompt := range visualPrompts {
		visualDoc := ChunkVisual{
			ScriptID:   scriptID,
			ChunkID:    chunk.ID,
			ChunkIndex: chunk.ChunkIndex,
			StartTime:  prompt.Start,
			EndTime:    prompt.End,
			Prompt:     prompt.Prompt,
			CreatedAt:  time.Now(),
		}
		visualDocs = append(visualDocs, visualDoc)
	}

	if len(visualDocs) > 0 {
		_, err := chunkVisualsCollection.InsertMany(context.Background(), visualDocs)
		if err != nil {
			return fmt.Errorf("failed to save chunk visuals: %w", err)
		}
	}

	return nil
}
func (yt *YtAutomation) updateChunkVisualStatus(chunkID primitive.ObjectID, hasVisual bool) {
	_, err := scriptAudiosCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		bson.M{"$set": bson.M{"has_visual": hasVisual}},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to update chunk visual status: %v\n", err)
	}
}

func (yt *YtAutomation) extractEmotion(prompt string) string {
	// Simple extraction - look for "Mood: " pattern
	if idx := strings.Index(prompt, "Mood: "); idx != -1 {
		emotion := prompt[idx+6:]
		if endIdx := strings.Index(emotion, "."); endIdx != -1 {
			emotion = emotion[:endIdx]
		}
		return strings.TrimSpace(emotion)
	}
	return "neutral"
}

func (yt *YtAutomation) extractSceneConcept(prompt string) string {
	// Simple extraction - look for "Scene: " pattern
	if idx := strings.Index(prompt, "Scene: "); idx != -1 {
		scene := prompt[idx+7:]
		if endIdx := strings.Index(scene, "."); endIdx != -1 {
			scene = scene[:endIdx]
		}
		return strings.TrimSpace(scene)
	}
	return "general scene"
}
