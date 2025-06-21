package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"os"
	"strings"
	"time"
)

func (yt *YtAutomation) generateVisualPromptForChunks(scriptID primitive.ObjectID, chunks []ScriptSrt, styleID primitive.ObjectID, force bool) error {
	fmt.Printf("🎨 Starting visual prompt generation for %d chunks...\n", len(chunks))
	script, err := yt.getScriptByID(scriptID)
	if err != nil {
		return fmt.Errorf("loading script: %w", err)
	}

	for i, chunk := range chunks {
		fmt.Printf("Processing chunk %d/%d...\n", i+1, len(chunks))

		// Check if visuals already exist for this chunk
		shouldSkip, err := yt.checkExistingVisuals(scriptID, chunk, force)
		if err != nil {
			fmt.Printf("Warning: Failed to check existing visuals for chunk %d: %v\n", chunk.ChunkIndex, err)
			continue
		}

		if shouldSkip {
			fmt.Printf("Skipping chunk %d - visuals already exist\n", chunk.ChunkIndex)
			continue
		}

		fmt.Printf("Generating visual prompt for chunk %d/%d...\n", i+1, len(chunks))

		// Generate visual prompt using Gemini (expensive API call)
		visualPrompts, err := yt.generateVisualPrompts(chunk.Content, script, styleID)
		if err != nil {
			fmt.Printf("Warning: Failed to generate prompt visual for chunk %d: %v\n", chunk.ChunkIndex, err)
			continue
		}

		// Save visual prompts to database
		if err := yt.saveChunkVisuals(scriptID, chunk, visualPrompts, force); err != nil {
			fmt.Printf("Warning: Failed to save visuals prompt for chunk %d: %v\n", chunk.ChunkIndex, err)
			continue
		}

		// Update chunk to mark it has visual
		yt.updateChunkVisualStatus(chunk.ID, true)

		// Small delay between API calls
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("✓ Completed visual prompt generation for all chunks\n")
	return nil
}
func (yt *YtAutomation) generateVisualImagePromptForChunks(scriptID primitive.ObjectID, chunks []ChunkVisual) error {
	fmt.Printf("🎨 Starting visual generation for %d chunks...\n", len(chunks))
	globalOptions := map[string]interface{}{
		"imageModel":  os.Getenv("IMAGE_MODEL"),
		"aspectRatio": ASPECT_RATIO,
	}
	jobs := yt.CreateJobsFromPrompts(chunks, globalOptions)

	err := yt.MakeConcurrentRequests(jobs)
	if err != nil {
		log.Printf("Some requests encountered critical errors: %v", err)
		// Don't exit fatally - let the program complete and show summary
	}

	fmt.Printf("✓ Completed visual generation for all chunks\n")
	return nil
}
func (yt *YtAutomation) saveChunkVisuals(scriptID primitive.ObjectID, chunk ScriptSrt, visualPrompts []VisualPromptResponse, force bool) error {
	// Check if collection is initialized
	if chunkVisualsCollection == nil {
		return fmt.Errorf("chunk visuals collection is not initialized")
	}

	if force {
		// Delete existing visuals for this chunk before inserting new ones
		filter := bson.M{
			"script_id":   scriptID,
			"chunk_id":    chunk.ID,
			"chunk_index": chunk.ChunkIndex,
		}

		_, err := chunkVisualsCollection.DeleteMany(context.Background(), filter)
		if err != nil {
			return fmt.Errorf("failed to delete existing visuals: %w", err)
		}
	}

	var visualDocs []interface{}
	for i, prompt := range visualPrompts {
		visualDoc := ChunkVisual{
			ScriptID:    scriptID,
			ChunkID:     chunk.ID,
			ChunkIndex:  chunk.ChunkIndex,
			PromptIndex: i,
			StartTime:   prompt.StartTime,
			EndTime:     prompt.EndTime,
			Prompt:      prompt.Prompt,
			Status:      "pending",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		visualDocs = append(visualDocs, visualDoc)
	}

	if len(visualDocs) > 0 {
		_, err := chunkVisualsCollection.InsertMany(context.Background(), visualDocs)
		if err != nil {
			return fmt.Errorf("failed to save chunk visuals: %w", err)
		}
		fmt.Printf("Saved %d visual prompts for chunk %d\n", len(visualDocs), chunk.ChunkIndex)
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
