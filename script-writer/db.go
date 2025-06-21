package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

func (yt *YtAutomation) getScriptByID(scriptID primitive.ObjectID) (*Script, error) {
	var script Script
	err := scriptsCollection.FindOne(context.Background(), bson.M{"_id": scriptID}).Decode(&script)
	if err != nil {
		return nil, err
	}
	return &script, nil
}

func (yt *YtAutomation) getChannelByID(channelID primitive.ObjectID) (*Channel, error) {
	var channel Channel
	err := channelsCollection.FindOne(context.Background(), bson.M{"_id": channelID}).Decode(&channel)
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

func (yt *YtAutomation) getScriptAudioByID(scriptAudioID primitive.ObjectID) (*ScriptAudio, error) {
	var scriptAudio ScriptAudio
	err := scriptAudiosCollection.FindOne(context.Background(), bson.M{"_id": scriptAudioID}).Decode(&scriptAudio)
	if err != nil {
		return nil, err
	}
	return &scriptAudio, nil
}

func (yt *YtAutomation) updateScriptStatus(scriptID primitive.ObjectID, status string) {
	yt.updateScriptInDB(scriptID, bson.M{"status": status})
}

func (yt *YtAutomation) updateScriptCurrentSection(scriptID primitive.ObjectID, section int) {
	yt.updateScriptInDB(scriptID, bson.M{"current_section": section})
}

func (yt *YtAutomation) updateScriptError(scriptID primitive.ObjectID, errorMsg string) {
	yt.updateScriptInDB(scriptID, bson.M{
		"status":        "error",
		"error_message": errorMsg,
	})
}

func (yt *YtAutomation) updateScriptCompletedAt(scriptID primitive.ObjectID) {
	now := time.Now()
	yt.updateScriptInDB(scriptID, bson.M{"completed_at": &now})
}

func (yt *YtAutomation) updateScriptInDB(scriptID primitive.ObjectID, updateData bson.M) error {
	_, err := scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": scriptID},
		bson.M{"$set": updateData},
	)

	return err
}

func (yt *YtAutomation) SaveScriptAudioFile(chunkID primitive.ObjectID, filepath string) {
	_, err := scriptAudiosCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		bson.M{"$set": bson.M{"audio_file": filepath}},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to save chunk audio file path: %v\n", err)
	}
}

func (yt *YtAutomation) UpdateScriptCollection(scriptID primitive.ObjectID, filepath string) {
	_, err := scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": scriptID},
		bson.M{"$set": bson.M{"full_audio_file": filepath}},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to save merged audio file path: %v\n", err)
	}
}
func (yt *YtAutomation) updateVisualChunkStatus(chunkID primitive.ObjectID, status string) {
	updateDoc := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	// Add specific tracking based on status
	switch status {
	case "processing":
		updateDoc["$set"].(bson.M)["processing_started_at"] = time.Now()
	case "completed":
		updateDoc["$set"].(bson.M)["completed_at"] = time.Now()
	case "failed":
		updateDoc["$inc"] = bson.M{"retry_count": 1}
		updateDoc["$set"].(bson.M)["failed_at"] = time.Now()
	}

	_, err := chunkVisualsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		updateDoc,
	)
	if err != nil {
		fmt.Printf("Warning: Failed to update chunk status to %s: %v\n", status, err)
	}
}
func (yt *YtAutomation) updateVisualChunkWithAPIKeyError(chunkID primitive.ObjectID, errorMsg string) {
	_, err := chunkVisualsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		bson.M{
			"$set": bson.M{
				"status":        "failed",
				"error_message": errorMsg,
				"failed_at":     time.Now(),
				"updated_at":    time.Now(),
			},
			"$inc": bson.M{"retry_count": 1},
		},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to update chunk with API key error: %v\n", err)
	}
}

// 7. Add method to update chunk visual with API key used
func (yt *YtAutomation) updateVisualChunkWithAPIKey(chunkID primitive.ObjectID, apiKeyID primitive.ObjectID, provider string) {
	_, err := chunkVisualsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		bson.M{
			"$set": bson.M{
				"api_key_id":   apiKeyID,
				"api_provider": provider,
				"updated_at":   time.Now(),
			},
		},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to update chunk with API key info: %v\n", err)
	}
}

// 8. Add method to update chunk visual with processing details
func (yt *YtAutomation) updateVisualChunkWithProcessingInfo(chunkID primitive.ObjectID, seed int, attempts int) {
	_, err := chunkVisualsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		bson.M{
			"$set": bson.M{
				"seed_used":           seed,
				"processing_attempts": attempts,
				"updated_at":          time.Now(),
			},
		},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to update chunk with processing info: %v\n", err)
	}
}
func (yt *YtAutomation) checkExistingVisuals(scriptID primitive.ObjectID, chunk ScriptSrt, force bool) (bool, error) {
	if force {
		// If force is true, don't skip - we'll update existing entries
		return false, nil
	}

	// Check if any visuals exist for this chunk
	filter := bson.M{
		"script_id":   scriptID,
		"chunk_id":    chunk.ID,
		"chunk_index": chunk.ChunkIndex,
	}

	count, err := chunkVisualsCollection.CountDocuments(context.Background(), filter)
	if err != nil {
		return false, fmt.Errorf("failed to check existing visuals: %w", err)
	}

	// If any visuals exist, skip this chunk
	return count > 0, nil
}
