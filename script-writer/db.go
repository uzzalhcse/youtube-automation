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
