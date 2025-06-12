package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"youtube_automation/video-editor/engine"
	"youtube_automation/video-editor/models"
)

const (
	InputDir  = "./video-input"
	OutputDir = "./output"
	ConfigDir = "./video-input/config"
)

func main() {
	fmt.Println("🎬 Starting Automated Video Editor...")

	// Ensure output directory exists
	if err := os.MkdirAll(OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Load project configuration
	configPath := filepath.Join(ConfigDir, "project.json")
	config, err := models.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize video editor engine
	editor := engine.NewVideoEditor(InputDir, OutputDir, config)

	// Step 1: Merge voice files
	fmt.Println("🎙️ Step 1: Merging voice files...")
	voiceDuration, err := editor.MergeVoiceFiles()
	if err != nil {
		log.Fatalf("Failed to merge voice files: %v", err)
	}
	fmt.Printf("✅ Voice files merged. Total duration: %.2f seconds\n", voiceDuration)

	// Step 2: Extend background music
	fmt.Println("🔊 Step 2: Extending background music...")
	if err := editor.ExtendBackgroundMusic(voiceDuration); err != nil {
		log.Fatalf("Failed to extend background music: %v", err)
	}
	fmt.Println("✅ Background music extended")

	// Step 3: Create slideshow
	fmt.Println("🖼️ Step 3: Creating slideshow...")
	if err := editor.CreateSlideshow(voiceDuration); err != nil {
		log.Fatalf("Failed to create slideshow: %v", err)
	}
	fmt.Println("✅ Slideshow created")

	// Step 4: Generate final video with overlays and effects
	fmt.Println("🎛️ Step 4: Applying overlays and effects...")
	if err := editor.GenerateFinalVideo(); err != nil {
		log.Fatalf("Failed to generate final video: %v", err)
	}
	fmt.Println("✅ Final video generated")

	fmt.Println("🎉 Video editing completed successfully!")
	fmt.Printf("📁 Output files saved to: %s\n", OutputDir)
}
