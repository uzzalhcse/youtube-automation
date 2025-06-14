package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GetVoiceFiles returns all voice files from the audio directory
func GetVoiceFiles(audioDir string) ([]string, error) {
	var voiceFiles []string

	err := filepath.Walk(audioDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			filename := strings.ToLower(info.Name())
			if strings.HasPrefix(filename, "voice") &&
				(strings.HasSuffix(filename, ".mp3") ||
					strings.HasSuffix(filename, ".wav") ||
					strings.HasSuffix(filename, ".m4a")) {
				voiceFiles = append(voiceFiles, path)
			}
		}
		return nil
	})

	return voiceFiles, err
}

// GetImageFiles returns all image files from the images directory
func GetImageFiles(imagesDir string) ([]string, error) {
	var imageFiles []string

	err := filepath.Walk(imagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			filename := strings.ToLower(info.Name())
			if strings.HasSuffix(filename, ".jpg") ||
				strings.HasSuffix(filename, ".jpeg") ||
				strings.HasSuffix(filename, ".png") ||
				strings.HasSuffix(filename, ".bmp") ||
				strings.HasSuffix(filename, ".tiff") {
				imageFiles = append(imageFiles, path)
			}
		}
		return nil
	})

	return imageFiles, err
}

// CreateConcatFile creates a temporary file for FFmpeg concat demuxer
func CreateConcatFile(files []string, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, f := range files {
		// Escape single quotes in file paths
		escapedPath := strings.ReplaceAll(f, "'", "\\'")
		fmt.Fprintf(file, "file '%s'\n", escapedPath)
	}

	return nil
}

// GetAudioDuration returns the duration of an audio file in seconds using ffprobe
func GetAudioDuration(filePath string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_entries",
		"format=duration", "-of", "csv=p=0", filePath)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get audio duration: %v", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %v", err)
	}

	return duration, nil
}

// GetVideoDuration returns the duration of a video file in seconds using ffprobe
func GetVideoDuration(filePath string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_entries",
		"format=duration", "-of", "csv=p=0", filePath)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %v", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %v", err)
	}

	return duration, nil
}

// ValidateFFmpegInstalled checks if FFmpeg and FFprobe are installed
func ValidateFFmpegInstalled() error {
	// Check FFmpeg
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH. Please install FFmpeg")
	}

	// Check FFprobe
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf("ffprobe not found in PATH. Please install FFmpeg")
	}

	return nil
}

// SanitizeFilename removes or replaces invalid characters in filenames
func SanitizeFilename(filename string) string {
	// Replace invalid characters with underscores
	reg := regexp.MustCompile(`[<>:"/\\|?*]`)
	sanitized := reg.ReplaceAllString(filename, "_")

	// Remove leading/trailing spaces and dots
	sanitized = strings.Trim(sanitized, " .")

	// Ensure filename is not empty
	if sanitized == "" {
		sanitized = "untitled"
	}

	return sanitized
}

// EnsureDirectoryExists creates a directory if it doesn't exist
func EnsureDirectoryExists(dirPath string) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return os.MkdirAll(dirPath, 0755)
	}
	return nil
}

// CleanupTempFiles removes temporary files created during processing
func CleanupTempFiles(files []string) {
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			fmt.Printf("Warning: failed to remove temp file %s: %v\n", file, err)
		}
	}
}

// GetFileSize returns the size of a file in bytes
func GetFileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// FileExists checks if a file exists
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// GetVideoFiles returns all video files in the specified directory
func GetVideoFiles(dir string) ([]string, error) {
	var videoFiles []string

	// Supported video extensions
	videoExtensions := map[string]bool{
		".mp4":  true,
		".avi":  true,
		".mov":  true,
		".mkv":  true,
		".wmv":  true,
		".flv":  true,
		".webm": true,
		".m4v":  true,
		".3gp":  true,
		".ogv":  true,
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if videoExtensions[ext] {
				// Get relative path from the directory
				relPath, err := filepath.Rel(dir, path)
				if err != nil {
					return err
				}
				videoFiles = append(videoFiles, relPath)
			}
		}

		return nil
	})

	return videoFiles, err
}
