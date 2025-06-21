package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"os"
	"sync"
	"time"
)

type APIKeyManager struct {
	currentKey *APIKey
	mu         sync.RWMutex
}

func seedAPIKeys() error {
	ctx := context.Background()

	// Check if any API keys already exist
	count, err := apiKeysCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to count API keys: %w", err)
	}

	if count > 0 {
		fmt.Printf("API keys already exist (%d found), skipping seed\n", count)
		return nil
	}

	// Get API keys from environment variables
	whiskKey := os.Getenv("API_AUTH_TOKEN")
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")

	var keysToInsert []interface{}

	// Add Whisk API key if available
	if whiskKey != "" {
		whiskAPIKey := APIKey{
			ID:         primitive.NewObjectID(),
			KeyValue:   whiskKey,
			Provider:   "whisk",
			IsActive:   true,
			LastUsed:   time.Now(),
			ErrorCount: 0,
			CreatedAt:  time.Now(),
		}
		keysToInsert = append(keysToInsert, whiskAPIKey)
		fmt.Println("✓ Added Whisk API key")
	}

	// Add ElevenLabs API key if available
	if elevenLabsKey != "" {
		elevenLabsAPIKey := APIKey{
			ID:         primitive.NewObjectID(),
			KeyValue:   elevenLabsKey,
			Provider:   "elevenlabs",
			IsActive:   true,
			LastUsed:   time.Now(),
			ErrorCount: 0,
			CreatedAt:  time.Now(),
		}
		keysToInsert = append(keysToInsert, elevenLabsAPIKey)
		fmt.Println("✓ Added ElevenLabs API key")
	}

	if len(keysToInsert) == 0 {
		return fmt.Errorf("no API keys found in environment variables")
	}

	// Insert all keys
	_, err = apiKeysCollection.InsertMany(ctx, keysToInsert)
	if err != nil {
		return fmt.Errorf("failed to insert API keys: %w", err)
	}

	fmt.Printf("✓ Successfully seeded %d API keys\n", len(keysToInsert))
	return nil
}

// 2. Add this function to manually add API keys
func addAPIKey(provider, keyValue string) error {
	ctx := context.Background()

	apiKey := APIKey{
		ID:         primitive.NewObjectID(),
		KeyValue:   keyValue,
		Provider:   provider,
		IsActive:   true,
		LastUsed:   time.Now(),
		ErrorCount: 0,
		CreatedAt:  time.Now(),
	}

	_, err := apiKeysCollection.InsertOne(ctx, apiKey)
	if err != nil {
		return fmt.Errorf("failed to add API key: %w", err)
	}

	fmt.Printf("✓ Added API key for provider: %s\n", provider)
	return nil
}
func (yt *YtAutomation) getAPIKeyStats() error {
	ctx := context.Background()

	cursor, err := apiKeysCollection.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to find API keys: %w", err)
	}
	defer cursor.Close(ctx)

	var keys []APIKey
	if err = cursor.All(ctx, &keys); err != nil {
		return fmt.Errorf("failed to decode API keys: %w", err)
	}

	fmt.Printf("\n=== API Key Usage Statistics ===\n")
	for _, key := range keys {
		status := "INACTIVE"
		if key.IsActive {
			status = "ACTIVE"
		}

		timeSinceLastUsed := time.Since(key.LastUsed)
		fmt.Printf("ID: %s | Provider: %s | Status: %s | Errors: %d | Last Used: %s ago\n",
			key.ID.Hex()[:8]+"...", key.Provider, status, key.ErrorCount,
			timeSinceLastUsed.Round(time.Minute))
	}
	fmt.Printf("=================================\n\n")

	return nil
}

// 3. Improved APIKeyManager with better error handling
func (akm *APIKeyManager) GetActiveKey(provider string) (*APIKey, error) {
	akm.mu.RLock()
	if akm.currentKey != nil && akm.currentKey.IsActive && akm.currentKey.Provider == provider {
		akm.mu.RUnlock()
		return akm.currentKey, nil
	}
	akm.mu.RUnlock()

	akm.mu.Lock()
	defer akm.mu.Unlock()

	// Find active key for provider
	var apiKey APIKey
	err := apiKeysCollection.FindOne(
		context.Background(),
		bson.M{"is_active": true, "provider": provider},
		options.FindOne().SetSort(bson.M{"last_used": 1}),
	).Decode(&apiKey)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("no active API keys found for provider '%s'", provider)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Update last used timestamp
	_, updateErr := apiKeysCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": apiKey.ID},
		bson.M{"$set": bson.M{"last_used": time.Now()}},
	)
	if updateErr != nil {
		log.Printf("Warning: failed to update last_used timestamp: %v", updateErr)
	}

	akm.currentKey = &apiKey
	return &apiKey, nil
}

// 4. Function to list all API keys (for debugging)
func listAPIKeys() error {
	ctx := context.Background()

	cursor, err := apiKeysCollection.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to find API keys: %w", err)
	}
	defer cursor.Close(ctx)

	var keys []APIKey
	if err = cursor.All(ctx, &keys); err != nil {
		return fmt.Errorf("failed to decode API keys: %w", err)
	}

	fmt.Printf("\n=== API Keys in Database ===\n")
	for _, key := range keys {
		status := "INACTIVE"
		if key.IsActive {
			status = "ACTIVE"
		}
		fmt.Printf("Provider: %s | Status: %s | Errors: %d | Last Used: %s\n",
			key.Provider, status, key.ErrorCount, key.LastUsed.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("===========================\n\n")

	return nil
}

func (akm *APIKeyManager) FlagKeyAsProblematic(keyID primitive.ObjectID, errorMsg string) error {
	akm.mu.Lock()
	defer akm.mu.Unlock()

	_, err := apiKeysCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": keyID},
		bson.M{
			"$set": bson.M{"is_active": false},
			"$inc": bson.M{"error_count": 1},
		},
	)

	if akm.currentKey != nil && akm.currentKey.ID == keyID {
		akm.currentKey = nil // Force getting new key
	}

	log.Printf("API Key flagged as problematic: %s", errorMsg)
	return err
}
