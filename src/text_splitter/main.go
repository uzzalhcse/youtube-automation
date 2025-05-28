package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// splitTextByCharLimit splits text into blocks with character limit while preserving complete sentences
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

func main() {
	inputFile := "story_scripts.txt"
	outputDir := "output"
	charLimit := 10000

	// Parse character limit from command line if provided
	if len(os.Args) >= 2 {
		var err error
		if _, err = fmt.Sscanf(os.Args[1], "%d", &charLimit); err != nil {
			log.Fatalf("Invalid character limit: %s", os.Args[1])
		}
	}

	// Read input file
	content, err := ioutil.ReadFile(inputFile)
	if err != nil {
		log.Fatalf("Error reading file %s: %v", inputFile, err)
	}

	text := strings.TrimSpace(string(content))
	if text == "" {
		fmt.Println("input.txt is empty or contains only whitespace")
		return
	}

	fmt.Printf("Reading from: %s\n", inputFile)
	fmt.Printf("Character limit: %d\n", charLimit)
	fmt.Printf("Input total (%d chars):\n%s\n\n", len(text), text)

	// Split text into blocks
	blocks := splitTextByCharLimit(text, charLimit)

	fmt.Printf("Generated %d blocks:\n", len(blocks))
	for i, block := range blocks {
		fmt.Printf("Block %d (%d chars): %s\n", i+1, len(block), block)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}

	// Save each block to separate files
	fmt.Printf("\nSaving blocks to %s folder:\n", outputDir)
	for i, block := range blocks {
		filename := filepath.Join(outputDir, fmt.Sprintf("part_%d.txt", i+1))
		err := ioutil.WriteFile(filename, []byte(block), 0644)
		if err != nil {
			log.Printf("Error saving %s: %v", filename, err)
		} else {
			fmt.Printf("✓ Saved: %s (%d chars)\n", filename, len(block))
		}
	}

	fmt.Printf("\nDone! %d files saved in %s folder\n", len(blocks), outputDir)
}

// Remove the saveToFile function as we're now saving individual files
