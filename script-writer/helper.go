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
