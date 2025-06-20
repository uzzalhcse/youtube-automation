package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func ParsePromptsFromFile(filename string, sectionNumber int) ([]ImagePrompt, error) {
	// Read the file content
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts file: %w", err)
	}

	// Convert to string and clean up
	text := strings.TrimSpace(string(content))

	// Parse prompts using regex to match the pattern: Prompt X: "content"
	// (?s) flag enables dot to match newlines, .*? is non-greedy to stop at first closing quote
	re := regexp.MustCompile(`(?s)Prompt\s+\d+:\s*"(.*?)"`)
	matches := re.FindAllStringSubmatch(text, -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("no prompts found in file %s", filename)
	}

	// Extract the prompt content (capture group 1)
	var prompts []ImagePrompt
	for _, match := range matches {
		if len(match) > 1 {
			prompts = append(prompts, ImagePrompt{
				PromptText:    match[1],
				ImageType:     "visual_aid", // Default type, can be customized later
				SectionNumber: 0,            // Default section number, can be customized later
			})
		}
	}

	return prompts, nil
}
func splitTextByCharLimit(text string, charLimit int) []string {
	if len(text) <= charLimit {
		return []string{text}
	}

	// Define sentence-ending patterns (period, exclamation, question mark followed by space or end)
	sentenceRegex := regexp.MustCompile(`[.!?]+(?:\s+|$)`)

	var blocks []string
	currentBlock := ""

	// Find all sentence boundaries
	sentences := sentenceRegex.Split(text, -1)
	sentenceEnds := sentenceRegex.FindAllStringIndex(text, -1)

	// Reconstruct sentences with their punctuation
	var fullSentences []string
	for i, sentence := range sentences {
		if i < len(sentenceEnds) {
			// Get the punctuation from the original text
			endPos := sentenceEnds[i][1]
			startPos := sentenceEnds[i][0]
			punctuation := text[startPos:endPos]
			fullSentences = append(fullSentences, sentence+punctuation)
		} else if strings.TrimSpace(sentence) != "" {
			// Last sentence might not have punctuation
			fullSentences = append(fullSentences, sentence)
		}
	}

	for _, sentence := range fullSentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		// If adding this sentence would exceed the limit
		if len(currentBlock)+len(sentence) > charLimit {
			// If current block is not empty, save it
			if currentBlock != "" {
				blocks = append(blocks, strings.TrimSpace(currentBlock))
				currentBlock = ""
			}

			// If a single sentence is longer than the limit, we need to handle it
			if len(sentence) > charLimit {
				// Split the long sentence by words while staying under limit
				words := strings.Fields(sentence)
				tempSentence := ""

				for _, word := range words {
					if len(tempSentence)+len(word)+1 <= charLimit {
						if tempSentence == "" {
							tempSentence = word
						} else {
							tempSentence += " " + word
						}
					} else {
						if tempSentence != "" {
							blocks = append(blocks, tempSentence)
							tempSentence = word
						} else {
							// Single word is longer than limit, force add it
							blocks = append(blocks, word)
							tempSentence = ""
						}
					}
				}
				if tempSentence != "" {
					currentBlock = tempSentence
				}
			} else {
				currentBlock = sentence
			}
		} else {
			// Add sentence to current block
			if currentBlock == "" {
				currentBlock = sentence
			} else {
				currentBlock += " " + sentence
			}
		}
	}

	// Add the last block if it's not empty
	if currentBlock != "" {
		blocks = append(blocks, strings.TrimSpace(currentBlock))
	}

	return blocks
}
func saveAudioFile(data []byte, path string) error {
	return os.WriteFile(path, data, 0644)

}

// parseSRT parses an SRT string into a slice of SRTEntry
func parseSRT(srtContent string) ([]SRTEntry, error) {
	var entries []SRTEntry

	// Split by double newlines to separate entries
	blocks := regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(srtContent), -1)

	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 3 {
			continue // Skip malformed entries
		}

		// Parse index
		index, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			continue // Skip if index is not a number
		}

		// Parse time range
		timeRegex := regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2}),(\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2}),(\d{3})`)
		timeMatch := timeRegex.FindStringSubmatch(lines[1])
		if len(timeMatch) != 9 {
			continue // Skip if time format is invalid
		}

		startTime, err := parseTime(timeMatch[1], timeMatch[2], timeMatch[3], timeMatch[4])
		if err != nil {
			continue
		}

		endTime, err := parseTime(timeMatch[5], timeMatch[6], timeMatch[7], timeMatch[8])
		if err != nil {
			continue
		}

		// Combine text lines (everything after the timestamp)
		text := strings.Join(lines[2:], " ")
		text = strings.TrimSpace(text)

		entries = append(entries, SRTEntry{
			Index:     index,
			StartTime: startTime,
			EndTime:   endTime,
			Text:      text,
		})
	}

	return entries, nil
}

// parseTime converts time components to time.Duration
func parseTime(hours, minutes, seconds, milliseconds string) (time.Duration, error) {
	h, err := strconv.Atoi(hours)
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(minutes)
	if err != nil {
		return 0, err
	}
	s, err := strconv.Atoi(seconds)
	if err != nil {
		return 0, err
	}
	ms, err := strconv.Atoi(milliseconds)
	if err != nil {
		return 0, err
	}

	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
		time.Duration(s)*time.Second + time.Duration(ms)*time.Millisecond, nil
}

// formatTime converts time.Duration back to SRT time format
func formatTime(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	milliseconds := int(d.Nanoseconds()/1000000) % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, milliseconds)
}

// splitSRTByDuration splits SRT content into chunks of approximately the specified duration
func splitSRTByDuration(srtContent string, targetDuration time.Duration) ([]string, error) {
	entries, err := parseSRT(srtContent)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return []string{}, nil
	}

	var chunks []string
	var currentChunk []SRTEntry
	chunkStartTime := entries[0].StartTime

	for _, entry := range entries {
		// Calculate current chunk duration
		currentDuration := entry.EndTime - chunkStartTime

		// If adding this entry would exceed target duration and we have entries in current chunk
		if currentDuration > targetDuration && len(currentChunk) > 0 {
			// Finalize current chunk
			chunkSRT := buildSRTChunk(currentChunk)
			chunks = append(chunks, chunkSRT)

			// Start new chunk
			currentChunk = []SRTEntry{entry}
			chunkStartTime = entry.StartTime
		} else {
			// Add entry to current chunk
			currentChunk = append(currentChunk, entry)
		}
	}

	// Add the last chunk if it has entries
	if len(currentChunk) > 0 {
		chunkSRT := buildSRTChunk(currentChunk)
		chunks = append(chunks, chunkSRT)
	}

	return chunks, nil
}

// buildSRTChunk converts a slice of SRTEntry back to SRT format with renumbered indices
func buildSRTChunk(entries []SRTEntry) string {
	var result strings.Builder

	for i, entry := range entries {
		// Renumber the index starting from 1
		result.WriteString(fmt.Sprintf("%d\n", i+1))
		result.WriteString(fmt.Sprintf("%s --> %s\n",
			formatTime(entry.StartTime),
			formatTime(entry.EndTime)))
		result.WriteString(entry.Text)
		result.WriteString("\n\n")
	}

	return strings.TrimSpace(result.String())
}

// splitSRTByCharLimit adapts your existing function to work with SRT format
func splitSRTByCharLimit(srtContent string, charLimit int) ([]string, error) {
	entries, err := parseSRT(srtContent)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return []string{}, nil
	}

	// Extract all text for character-based splitting
	var allText strings.Builder
	var textToEntryMap []int // Maps text positions back to entry indices

	for i, entry := range entries {
		if allText.Len() > 0 {
			allText.WriteString(" ")
		}
		startPos := allText.Len()
		allText.WriteString(entry.Text)
		endPos := allText.Len()

		// Mark which characters belong to which entry
		for j := startPos; j < endPos; j++ {
			textToEntryMap = append(textToEntryMap, i)
		}
		if allText.Len() > startPos {
			textToEntryMap = append(textToEntryMap, i) // for the space
		}
	}

	// Use your existing text splitting logic
	textChunks := splitTextByCharLimit(allText.String(), charLimit)

	// Convert text chunks back to SRT chunks
	var srtChunks []string
	currentPos := 0

	for _, textChunk := range textChunks {
		var chunkEntries []SRTEntry
		usedEntries := make(map[int]bool)

		// Find which entries are needed for this text chunk
		chunkEnd := currentPos + len(textChunk)
		for pos := currentPos; pos < chunkEnd && pos < len(textToEntryMap); pos++ {
			entryIndex := textToEntryMap[pos]
			if !usedEntries[entryIndex] {
				chunkEntries = append(chunkEntries, entries[entryIndex])
				usedEntries[entryIndex] = true
			}
		}

		if len(chunkEntries) > 0 {
			srtChunk := buildSRTChunk(chunkEntries)
			srtChunks = append(srtChunks, srtChunk)
		}

		currentPos = chunkEnd + 1 // +1 for space between chunks
	}

	return srtChunks, nil
}

func (yt *YtAutomation) generateVoiceOver(script Script, chunks []ScriptAudio) error {
	// Check ElevenLabs credits first
	subInfo, err := yt.elevenLabsClient.GetSubscriptionInfo()
	if err != nil {
		fmt.Printf("Warning: Could not fetch subscription info: %v\n", err)
	} else {
		remaining := subInfo.CharacterLimit - subInfo.CharacterCount
		fmt.Printf("📊 ElevenLabs Credits - Used: %d/%d, Remaining: %d characters\n",
			subInfo.CharacterCount, subInfo.CharacterLimit, remaining)

		if remaining < 1000 {
			fmt.Printf("⚠️  Warning: Low credits remaining (%d characters)\n", remaining)
		}
	}

	var audioFiles []string
	pendingChunks := yt.getPendingChunks(chunks)

	fmt.Printf("📋 Total chunks: %d, Pending generation: %d\n", len(chunks), len(pendingChunks))

	// Generate only pending chunks
	for i, chunk := range pendingChunks {
		fmt.Printf("🎵 Generating voice for chunk %d/%d (Chunk Index: %d)...\n",
			i+1, len(pendingChunks), chunk.ChunkIndex)

		// Update status to generating
		yt.updateChunkStatus(chunk.ID, "generating", "")

		// Generate speech
		audioData, err := yt.elevenLabsClient.TextToSpeech(chunk.Content, os.Getenv("VOICE_ID"))
		if err != nil {
			fmt.Printf("❌ Error generating speech for chunk %d: %v\n", chunk.ChunkIndex, err)
			yt.updateChunkStatus(chunk.ID, "failed", "")
			continue // Continue with next chunk instead of failing completely
		}

		timestamp := time.Now().Format("20060102_15_04_05")
		filename := fmt.Sprintf("%s_voiceover_%d_%s.mp3", script.ChannelName, chunk.ChunkIndex, timestamp)
		path := filepath.Join("assets", "audio", filename)

		if err = saveAudioFile(audioData, path); err != nil {
			fmt.Printf("❌ Error saving audio file for chunk %d: %v\n", chunk.ChunkIndex, err)
			yt.updateChunkStatus(chunk.ID, "failed", "")
			continue
		}

		// Update status to completed with file path
		yt.updateChunkStatus(chunk.ID, "completed", path)
		fmt.Printf("✅ Voice generation complete for chunk %d\n", chunk.ChunkIndex)
	}

	// Collect all completed audio files (including previously generated ones)
	audioFiles = yt.getCompletedAudioFiles(chunks)

	if len(audioFiles) == 0 {
		return fmt.Errorf("no audio files generated successfully")
	}

	// Merge all available audio files
	fmt.Printf("🔄 Merging %d audio files...\n", len(audioFiles))
	mergedFilename := fmt.Sprintf("%s_complete_voiceover_%s.mp3",
		script.ChannelName,
		time.Now().Format("20060102_15_04_05"))
	mergedPath := filepath.Join("assets", "audio", mergedFilename)

	if err := yt.mergeAudioFiles(audioFiles, mergedPath); err != nil {
		return fmt.Errorf("error merging audio files: %v", err)
	}

	// Update script collection with merged audio file
	yt.UpdateScriptCollection(script.ID, mergedPath)

	completedCount := len(yt.getCompletedChunks(chunks))
	fmt.Printf("✅ Voiceover generation finished! Completed: %d/%d chunks\n", completedCount, len(chunks))

	if completedCount < len(chunks) {
		fmt.Printf("ℹ️  %d chunks still pending. Run again to resume.\n", len(chunks)-completedCount)
	}

	return nil
}
func (yt *YtAutomation) getPendingChunks(chunks []ScriptAudio) []ScriptAudio {
	var pending []ScriptAudio
	for _, chunk := range chunks {
		if chunk.GenerationStatus != "completed" || !yt.audioFileExists(chunk.AudioFilePath) {
			pending = append(pending, chunk)
		}
	}
	return pending
}

func (yt *YtAutomation) getCompletedChunks(chunks []ScriptAudio) []ScriptAudio {
	var completed []ScriptAudio
	for _, chunk := range chunks {
		updatedScriptAudio, err := yt.getScriptAudioByID(chunk.ID)
		if err != nil {
			fmt.Printf("Warning: Failed to load chunk %s: %v\n", chunk.ID.Hex(), err)
			continue // Skip this chunk if it can't be loaded
		}
		if updatedScriptAudio.GenerationStatus == "completed" && yt.audioFileExists(chunk.AudioFilePath) {
			completed = append(completed, chunk)
		}
	}
	return completed
}

func (yt *YtAutomation) getCompletedAudioFiles(chunks []ScriptAudio) []string {
	var audioFiles []string
	completed := yt.getCompletedChunks(chunks)

	// Sort by chunk index to maintain order
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].ChunkIndex < completed[j].ChunkIndex
	})

	for _, chunk := range completed {
		audioFiles = append(audioFiles, chunk.AudioFilePath)
	}
	return audioFiles
}

func (yt *YtAutomation) audioFileExists(filepath string) bool {
	if filepath == "" {
		return false
	}
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		return false
	}
	return true
}

func (yt *YtAutomation) updateChunkStatus(chunkID primitive.ObjectID, status, audioPath string) {
	update := bson.M{
		"$set": bson.M{
			"generation_status": status,
			"updated_at":        time.Now(),
		},
	}

	if audioPath != "" {
		update["$set"].(bson.M)["audio_file_path"] = audioPath
	}

	_, err := scriptAudiosCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		update,
	)
	if err != nil {
		fmt.Printf("Warning: Failed to update chunk status: %v\n", err)
	}
}

// mergeAudioFiles combines multiple audio files into one using FFmpeg
func (yt *YtAutomation) mergeAudioFiles(inputFiles []string, outputFile string) error {
	if len(inputFiles) == 0 {
		return fmt.Errorf("no input files to merge")
	}

	if len(inputFiles) == 1 {
		// If only one file, just copy it
		return yt.copyFile(inputFiles[0], outputFile)
	}

	// Verify all input files exist
	for _, file := range inputFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return fmt.Errorf("input file does not exist: %s", file)
		}
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create a temporary file list for FFmpeg
	listFile := filepath.Join(outputDir, "temp_filelist.txt")
	defer os.Remove(listFile)

	file, err := os.Create(listFile)
	if err != nil {
		return fmt.Errorf("failed to create file list: %v", err)
	}
	defer file.Close()

	// Write file paths to the list file (use forward slashes for FFmpeg compatibility)
	for _, audioFile := range inputFiles {
		absPath, err := filepath.Abs(audioFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %s: %v", audioFile, err)
		}
		// Convert Windows backslashes to forward slashes for FFmpeg
		absPath = filepath.ToSlash(absPath)
		_, err = file.WriteString(fmt.Sprintf("file '%s'\n", absPath))
		if err != nil {
			return fmt.Errorf("failed to write to file list: %v", err)
		}
	}
	file.Close()

	// Use FFmpeg to concatenate the files
	cmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", outputFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %v, output: %s", err, string(output))
	}

	fmt.Printf("✅ Successfully merged %d audio files into: %s\n", len(inputFiles), outputFile)
	return nil
}

// copyFile copies a single file (fallback when only one audio file exists)
func (yt *YtAutomation) copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// cleanupTempFiles removes temporary individual audio files
func (yt *YtAutomation) cleanupTempFiles(files []string) error {
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			fmt.Printf("Warning: Failed to remove temp file %s: %v\n", file, err)
		}
	}
	return nil
}

// Helper function to fix common JSON formatting issues
func fixJSONFormatting(jsonStr string) string {
	// Fix decimal numbers with spaces (e.g., "1. 5" -> "1.5")
	re := regexp.MustCompile(`(\d+)\.\s+(\d+)`)
	jsonStr = re.ReplaceAllString(jsonStr, "$1.$2")

	// Fix numbers with trailing spaces before commas/brackets
	re = regexp.MustCompile(`(\d+)\s+([,\]\}])`)
	jsonStr = re.ReplaceAllString(jsonStr, "$1$2")

	// Fix numbers with leading spaces after colons
	re = regexp.MustCompile(`:\s+(\d+\.\s+\d+)`)
	jsonStr = re.ReplaceAllStringFunc(jsonStr, func(match string) string {
		parts := strings.Split(match, ":")
		if len(parts) == 2 {
			number := strings.TrimSpace(parts[1])
			number = strings.ReplaceAll(number, " ", "")
			return ":" + number
		}
		return match
	})

	// NEW: Convert numeric timestamps to strings
	re = regexp.MustCompile(`"(start_time|end_time)":\s*(\d+\.?\d*)`)
	jsonStr = re.ReplaceAllString(jsonStr, `"$1": "$2"`)

	// Remove any extra whitespace that might cause issues
	lines := strings.Split(jsonStr, "\n")
	var cleanLines []string
	for _, line := range lines {
		cleanLine := strings.TrimSpace(line)
		if cleanLine != "" {
			cleanLines = append(cleanLines, cleanLine)
		}
	}

	return strings.Join(cleanLines, "\n")
}
