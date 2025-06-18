package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
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
