package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Configuration constants - modify these values as needed
const (
	DIRECTORY_PATH = "/home/uzzal/Videos/Youtube/Boring Naptime/videos/Boring History For Sleep Your Life as Boudica and more/Audio"
	START_NUMBER   = 1
	END_NUMBER     = 13
)

func main() {
	fmt.Println("🎵 Audio Merger Tool")
	fmt.Println("==================")
	fmt.Printf("Directory: %s\n", DIRECTORY_PATH)
	fmt.Printf("Range: %d - %d\n\n", START_NUMBER, END_NUMBER)

	// Validate range
	if START_NUMBER > END_NUMBER {
		log.Fatal("❌ Start number must be less than or equal to end number")
	}

	// Check if directory exists
	if _, err := os.Stat(DIRECTORY_PATH); os.IsNotExist(err) {
		log.Fatalf("❌ Directory does not exist: %s", DIRECTORY_PATH)
	}

	// Check if ffmpeg is installed
	if !isFFmpegInstalled() {
		log.Fatal("❌ FFmpeg is not installed or not found in PATH. Please install FFmpeg first.")
	}

	// Find audio files in the range
	fmt.Println("🔍 Searching for audio files...")
	inputFiles, err := findAudioFiles(DIRECTORY_PATH, START_NUMBER, END_NUMBER)
	if err != nil {
		log.Fatalf("❌ Error finding audio files: %v", err)
	}

	if len(inputFiles) == 0 {
		log.Fatal("❌ No audio files found in the specified range")
	}

	// Generate output filename
	outputFile := filepath.Join(DIRECTORY_PATH, fmt.Sprintf("merged_%d_%d.mp3", START_NUMBER, END_NUMBER))

	fmt.Printf("\n✅ Found %d audio files in range %d-%d\n", len(inputFiles), START_NUMBER, END_NUMBER)
	fmt.Printf("📁 Output file: %s\n\n", filepath.Base(outputFile))

	// List files to be merged
	fmt.Println("📋 Files to merge:")
	for i, file := range inputFiles {
		fmt.Printf("   %d. %s\n", i+1, filepath.Base(file))
	}

	fmt.Println("\n🔄 Starting merge process...")

	// Merge the audio files with progress
	err = concatenateAudioWithProgress(inputFiles, outputFile)
	if err != nil {
		log.Fatalf("❌ Error merging audio files: %v", err)
	}

	fmt.Println("\n🎉 Audio files merged successfully!")
	fmt.Printf("📁 Output file: %s\n", outputFile)

	// Verify the file exists and show its details
	if info, err := os.Stat(outputFile); err == nil {
		fmt.Printf("✅ File verified - Size: %s, Created: %s\n",
			getFileSize(outputFile),
			info.ModTime().Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("⚠️  Warning: Could not verify output file: %v\n", err)
	}
}

// findAudioFiles searches for audio files in the specified directory with sequential numbering
func findAudioFiles(dirPath string, startNum, endNum int) ([]string, error) {
	var inputFiles []string
	supportedExts := []string{".mp3", ".wav", ".mp4", ".aac", ".flac", ".ogg", ".m4a", ".wma"}

	fmt.Println()
	for i := startNum; i <= endNum; i++ {
		var foundFile string

		// Try different extensions for each number
		for _, ext := range supportedExts {
			filename := fmt.Sprintf("%d%s", i, ext)
			fullPath := filepath.Join(dirPath, filename)

			if _, err := os.Stat(fullPath); err == nil {
				foundFile = fullPath
				break
			}
		}

		if foundFile != "" {
			inputFiles = append(inputFiles, foundFile)
			fmt.Printf("   ✅ Found: %s\n", filepath.Base(foundFile))
		} else {
			fmt.Printf("   ⚠️  Missing: %d.* (no file found with supported extensions)\n", i)
		}
	}

	return inputFiles, nil
}

// concatenateAudioWithProgress merges audio files with a progress bar
func concatenateAudioWithProgress(inputFiles []string, outputFile string) error {
	// Create a temporary file list for ffmpeg concat
	tempListFile := filepath.Join(filepath.Dir(outputFile), "temp_file_list.txt")
	defer os.Remove(tempListFile)

	// Phase 1: Create file list
	fmt.Print("\n📝 Creating file list... ")

	file, err := os.Create(tempListFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file list: %v", err)
	}
	defer file.Close()

	for _, inputFile := range inputFiles {
		// Convert to absolute path and escape for ffmpeg
		absPath, err := filepath.Abs(inputFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %s: %v", inputFile, err)
		}
		// Escape single quotes and backslashes for ffmpeg
		escapedPath := strings.ReplaceAll(absPath, "'", "'\"'\"'")
		_, err = file.WriteString(fmt.Sprintf("file '%s'\n", escapedPath))
		if err != nil {
			return fmt.Errorf("failed to write to temp file list: %v", err)
		}
	}
	file.Close()

	fmt.Print("✅\n")

	// Phase 2: Run ffmpeg with progress
	fmt.Print("🔄 Merging audio files... ")

	// Run ffmpeg command to concatenate
	cmd := exec.Command("ffmpeg", "-f", "concat", "-safe", "0", "-i", tempListFile, "-c", "copy", outputFile, "-y")

	// Start the command and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Print("❌\n")
		fmt.Printf("\n🚨 FFmpeg Error Details:\n")
		fmt.Printf("Command: %s\n", cmd.String())
		fmt.Printf("Error: %v\n", err)
		fmt.Printf("Output: %s\n", string(output))
		return fmt.Errorf("ffmpeg command failed: %v", err)
	}

	fmt.Print("✅\n")

	// Verify output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		return fmt.Errorf("output file was not created: %s", outputFile)
	}

	// Show file size
	if _, err := os.Stat(outputFile); err == nil {
		fmt.Printf("📊 Output file size: %s\n", getFileSize(outputFile))
	}

	return nil
}

// showProgress displays a progress bar
func showProgress(current, total int) {
	percent := float64(current) / float64(total) * 100
	barLength := 30
	filledLength := int(float64(barLength) * float64(current) / float64(total))

	bar := strings.Repeat("█", filledLength) + strings.Repeat("░", barLength-filledLength)
	fmt.Printf("\r[%s] %.1f%%", bar, percent)
}

// isFFmpegInstalled checks if ffmpeg is available in the system PATH
func isFFmpegInstalled() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// getFileSize returns the size of a file in a human-readable format
func getFileSize(filePath string) string {
	info, err := os.Stat(filePath)
	if err != nil {
		return "unknown"
	}

	size := info.Size()
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

/*
Configuration:
=============
Modify the constants at the top of the file:

const (
	DIRECTORY_PATH = "/your/audio/files/directory"
	START_NUMBER   = 1
	END_NUMBER     = 16
)

Features:
=========
✅ Uses const variables instead of command line arguments
✅ Shows progress bar during merge process
✅ Colorful emoji-based output for better user experience
✅ Automatically finds sequentially numbered audio files
✅ Supports multiple audio formats: MP3, WAV, MP4, AAC, FLAC, OGG, M4A, WMA
✅ Creates output file in the same directory as input files
✅ Shows detailed file discovery process
✅ Handles missing files gracefully with warnings
✅ Progress indication during FFmpeg execution

Output Format:
=============
- Output filename: merged_<start>_<end>.mp3
- Saved in the same directory as input files

Prerequisites:
=============
- Install FFmpeg on your system
- Make sure ffmpeg is in your system PATH

Installation:
- Windows: Download from https://ffmpeg.org/download.html
- macOS: brew install ffmpeg
- Linux: sudo apt install ffmpeg (Ubuntu/Debian)

Usage:
======
1. Modify the constants at the top of the file
2. Run: go run audio_merger.go
3. Watch the progress and enjoy your merged audio file!
*/
