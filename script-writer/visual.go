package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"os"
	"sort"
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

func (yt *YtAutomation) generateVisualPromptForChunksWithRecovery(scriptID primitive.ObjectID, scriptSrtChunks []ScriptSrt, styleID primitive.ObjectID, force bool) error {
	fmt.Printf("🎨 Starting visual prompt generation for %d chunks...\n", len(scriptSrtChunks))
	script, err := yt.getScriptByID(scriptID)
	if err != nil {
		return fmt.Errorf("loading script: %w", err)
	}

	for i, chunk := range scriptSrtChunks {
		fmt.Printf("Processing chunk %d/%d...\n", i+1, len(scriptSrtChunks))

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

		fmt.Printf("Generating visual prompt for chunk %d/%d...\n", i+1, len(scriptSrtChunks))

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

	// After generating all visual prompts for chunks, perform gap analysis and recovery
	for _, chunk := range scriptSrtChunks {
		// Get existing visual prompts for this chunk
		ctx := context.Background()
		visualCursor, err := chunkVisualsCollection.Find(ctx, bson.M{
			"script_id":   scriptID,
			"chunk_index": chunk.ChunkIndex,
		})
		if err != nil {
			continue
		}

		var existingPrompts []VisualPromptResponse
		if err = visualCursor.All(ctx, &existingPrompts); err != nil {
			visualCursor.Close(ctx)
			continue
		}
		visualCursor.Close(ctx)

		// Get script for context
		var script Script
		if err := scriptsCollection.FindOne(ctx, bson.M{"_id": scriptID}).Decode(&script); err != nil {
			continue
		}

		// Validate and recover gaps
		recoveredPrompts, err := yt.validateAndRecoverVisualPrompts(chunk.Content, &script, styleID, existingPrompts)
		if err != nil {
			log.Printf("Gap recovery failed for chunk %d: %v", chunk.ChunkIndex, err)
			continue
		}

		// If recovery prompts were added, save them
		if len(recoveredPrompts) > len(existingPrompts) {
			newPrompts := recoveredPrompts[len(existingPrompts):]

			// Get the highest existing prompt_index to avoid duplicates
			maxPromptIndex := -1
			existingVisuals, err := chunkVisualsCollection.Find(ctx, bson.M{
				"script_id":   scriptID,
				"chunk_index": chunk.ChunkIndex,
			})
			if err == nil {
				var existingChunkVisuals []ChunkVisual
				if err = existingVisuals.All(ctx, &existingChunkVisuals); err == nil {
					for _, visual := range existingChunkVisuals {
						if visual.PromptIndex > maxPromptIndex {
							maxPromptIndex = visual.PromptIndex
						}
					}
				}
				existingVisuals.Close(ctx)
			}

			// Save recovery prompts with proper prompt_index sequencing
			for i, prompt := range newPrompts {
				chunkVisual := ChunkVisual{
					ID:          primitive.NewObjectID(),
					ScriptID:    scriptID,
					ChunkID:     chunk.ID,
					ChunkIndex:  chunk.ChunkIndex,
					PromptIndex: maxPromptIndex + 1 + i, // Ensure unique prompt_index
					StartTime:   prompt.StartTime,
					EndTime:     prompt.EndTime,
					Prompt:      prompt.Prompt,
					Status:      "pending",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					IsRecovered: true, // Flag to identify recovery prompts
				}

				if _, err := chunkVisualsCollection.InsertOne(ctx, chunkVisual); err != nil {
					log.Printf("Failed to save recovery prompt: %v", err)
				}
			}
			log.Printf("Saved %d recovery prompts for chunk %d", len(newPrompts), chunk.ChunkIndex)
		}
	}

	return nil
}

// Helper function to validate and recover gaps (enhanced version)
func (yt *YtAutomation) validateAndRecoverVisualPrompts(srtContent string, script *Script, styleID primitive.ObjectID, existingPrompts []VisualPromptResponse) ([]VisualPromptResponse, error) {
	// Extract SRT time ranges
	srtRanges, err := extractSRTTimeRanges(srtContent)
	if err != nil {
		return existingPrompts, fmt.Errorf("failed to extract SRT time ranges: %w", err)
	}

	if len(srtRanges) == 0 {
		log.Printf("No SRT ranges found for validation")
		return existingPrompts, nil
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
	for i, gap := range gaps {
		log.Printf("Gap %d: %.2f-%.2f seconds (%.2f second duration)",
			i+1, gap.StartTime, gap.EndTime, gap.EndTime-gap.StartTime)
	}

	// Generate recovery prompts
	recoveryPrompts, err := yt.generateGapRecoveryPrompts(gaps, script, styleID)
	if err != nil {
		log.Printf("Gap recovery failed: %v", err)
		return existingPrompts, nil // Return original prompts if recovery fails
	}

	if len(recoveryPrompts) == 0 {
		log.Printf("No recovery prompts generated")
		return existingPrompts, nil
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

	// Validate the recovery was successful
	finalGaps, err := detectGapsInSequence(srtRanges, allPrompts)
	if err == nil {
		if len(finalGaps) < len(gaps) {
			log.Printf("Gap recovery successful: reduced gaps from %d to %d", len(gaps), len(finalGaps))
		} else if len(finalGaps) == len(gaps) {
			log.Printf("Gap recovery had no effect: %d gaps remain", len(finalGaps))
		} else {
			log.Printf("Gap recovery may have introduced new issues: gaps increased from %d to %d", len(gaps), len(finalGaps))
		}
	}

	return allPrompts, nil
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
