package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	// Get API key - in production, use environment variable
	apiKey := "AIzaSyDykpGH35C4BRC_V3OK-GoHAIJ97RfwvMc"

	fmt.Println("=== Wisderly YouTube Script Generator ===")
	fmt.Println("This tool will automatically generate a complete YouTube script.")
	fmt.Println()

	// Initialize services
	templateService := NewTemplateService()
	geminiService := NewGeminiService(apiKey)
	scriptService := NewScriptService(templateService, geminiService)

	// Load templates
	if err := templateService.LoadAllTemplates(); err != nil {
		fmt.Printf("Error loading templates: %v\n", err)
		return
	}
	fmt.Println("✓ All templates loaded successfully\n")

	// Get user input
	config, err := getUserInput()
	if err != nil {
		fmt.Printf("Error getting user input: %v\n", err)
		return
	}

	// Generate script
	if err := scriptService.GenerateCompleteScript(config); err != nil {
		fmt.Printf("Error generating script: %v\n", err)
		return
	}

	fmt.Printf("\n🎉 Complete script saved to: %s\n", config.OutputFilename)
	fmt.Println(">>> End of script. Stay Healthy.")
}

func getUserInput() (*ScriptConfig, error) {
	reader := bufio.NewReader(os.Stdin)

	// Get topic from user
	fmt.Print("Enter your video topic: ")
	topic, _ := reader.ReadString('\n')
	topic = strings.TrimSpace(topic)

	if topic == "" {
		return nil, fmt.Errorf("topic cannot be empty")
	}

	// Ask for visual guidance preference
	fmt.Print("Generate Visual Guidance? (y/n): ")
	visualInput, _ := reader.ReadString('\n')
	generateVisuals := strings.TrimSpace(strings.ToLower(visualInput)) == "y"

	config := &ScriptConfig{
		Topic:                topic,
		GenerateVisuals:      generateVisuals,
		OutputFilename:       fmt.Sprintf("wisderly_script_%s_%d.txt", sanitizeFilename(topic), time.Now().Unix()),
		SectionCount:         defaultSectionCount,
		SleepBetweenSections: defaultSleepBetweenSections,
	}

	fmt.Printf("\nGenerating script for topic: %s\n", topic)
	fmt.Printf("Visual Guidance: %t\n", generateVisuals)
	fmt.Printf("Output file: %s\n", config.OutputFilename)
	fmt.Println("\n" + strings.Repeat("=", 50))

	return config, nil
}

func sanitizeFilename(topic string) string {
	replacements := []string{" ", "_", "/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_"}

	sanitized := topic
	for i := 0; i < len(replacements); i += 2 {
		sanitized = strings.ReplaceAll(sanitized, replacements[i], replacements[i+1])
	}

	// Limit length
	if len(sanitized) > 30 {
		sanitized = sanitized[:30]
	}

	return sanitized
}
