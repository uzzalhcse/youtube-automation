package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
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

	startTime := time.Now()
	fmt.Println("📅 Start time:", startTime.Format("2006-01-02 15:04:05"))
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

	err = editor.ProcessVideo()
	if err != nil {
		log.Fatalf("Failed to process video: %v", err)
	}

	fmt.Println("🎉 Video editing completed successfully!")
	fmt.Printf("📁 Output files saved to: %s\n", OutputDir)

	// Display final video info
	finalVideoPath := filepath.Join(OutputDir, "final_video.mp4")
	if info, err := os.Stat(finalVideoPath); err == nil {
		fmt.Printf("📊 Final video size: %.2f MB\n", float64(info.Size())/(1024*1024))
	}
	fmt.Println("📅 End time:", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("🕒 Total time taken:", time.Since(startTime))
}
