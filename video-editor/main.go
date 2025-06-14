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
	fmt.Println("🎬 Starting Automated Video Editor with Overlay Support...")

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

	// Check for overlay videos
	overlayVideos, err := editor.GetOverlayVideos()
	if err != nil {
		log.Printf("Warning: Failed to check for overlay videos: %v", err)
	} else if len(overlayVideos) > 0 {
		fmt.Printf("📹 Found %d overlay videos:\n", len(overlayVideos))
		for i, overlay := range overlayVideos {
			fmt.Printf("   %d. %s\n", i+1, filepath.Base(overlay))
		}

		// Display overlay opacity setting
		overlayOpacity := 0.7 // Default
		if config.Settings.OverlayOpacity > 0 {
			overlayOpacity = config.Settings.OverlayOpacity
		}
		fmt.Printf("🔍 Overlay opacity: %.0f%%\n", overlayOpacity*100)
	} else {
		fmt.Println("📹 No overlay videos found in ./video-input/overlays/")
	}

	editor.ProcessVideo()
	// Step 1: Merge voice files
	//fmt.Println("🎙️ Step 1: Merging voice files...")
	//voiceDuration, err := editor.MergeVoiceFiles()
	//if err != nil {
	//	log.Fatalf("Failed to merge voice files: %v", err)
	//}
	//fmt.Printf("✅ Voice files merged. Total duration: %.2f seconds\n", voiceDuration)
	//
	//// Step 2: Extend background music
	//fmt.Println("🔊 Step 2: Extending background music...")
	//if err := editor.ExtendBackgroundMusic(voiceDuration); err != nil {
	//	log.Fatalf("Failed to extend background music: %v", err)
	//}
	//fmt.Println("✅ Background music extended")
	//
	//// Step 3: Create slideshow
	//fmt.Println("🖼️ Step 3: Creating slideshow...")
	//if err := editor.CreateSlideshow(voiceDuration); err != nil {
	//	log.Fatalf("Failed to create slideshow: %v", err)
	//}
	//fmt.Println("✅ Slideshow created")
	//
	//// Step 4: Generate final video with overlays and effects
	//if len(overlayVideos) > 0 {
	//	fmt.Println("🎛️ Step 4: Applying overlays and generating final video...")
	//	if err := editor.GenerateFinalVideoWithOverlays(); err != nil {
	//		log.Fatalf("Failed to generate final video with overlays: %v", err)
	//	}
	//	fmt.Printf("✅ Final video generated with %d overlays\n", len(overlayVideos))
	//} else {
	//	fmt.Println("🎛️ Step 4: Generating final video (no overlays)...")
	//	if err := editor.GenerateFinalVideoSimplified(); err != nil {
	//		log.Fatalf("Failed to generate final video: %v", err)
	//	}
	//	fmt.Println("✅ Final video generated")
	//}

	fmt.Println("🎉 Video editing completed successfully!")
	fmt.Printf("📁 Output files saved to: %s\n", OutputDir)

	// Display final video info
	finalVideoPath := filepath.Join(OutputDir, "final_video.mp4")
	if info, err := os.Stat(finalVideoPath); err == nil {
		fmt.Printf("📊 Final video size: %.2f MB\n", float64(info.Size())/(1024*1024))
	}
}
