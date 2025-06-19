package main

import (
	"fmt"
	"os"
	"regexp"
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
