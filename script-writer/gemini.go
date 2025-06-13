package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}

type ScriptSession struct {
	Topic         string
	Filename      string
	CurrentStep   int
	Outline       string
	OutlinePoints []string // New field to store parsed outline points
	Hook          string
	Introduction  string
	Content       strings.Builder
}

const (
	baseURL               = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
	timeout               = 60 * time.Second
	outlineTemplateFile   = "outline_template.txt"
	templateFile          = "script_template.txt"
	hookIntroTemplateFile = "hook_intro_template.txt"
)

func loadScriptTemplate(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("reading template file: %w", err)
	}
	return string(data), nil
}

// New function to parse outline points from the response
// Fixed function to parse outline points from the response
func parseOutlinePoints(outlineResponse string) []string {
	var points []string

	// Split by lines and look for bullet points
	lines := strings.Split(outlineResponse, "\n")

	// Fixed regex patterns - escape the bullet character properly or use the actual character
	bulletRegex := regexp.MustCompile(`^\s*[\*\-•]\s*\*\*(.+?)\*\*:?\s*(.*)$`)
	numberedRegex := regexp.MustCompile(`^\s*\d+\.\s*\*\*(.+?)\*\*:?\s*(.*)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to match bullet points with bold formatting
		if matches := bulletRegex.FindStringSubmatch(line); len(matches) >= 3 {
			// Combine title and description
			point := strings.TrimSpace(matches[1] + ": " + matches[2])
			if point != "" {
				points = append(points, point)
			}
		} else if matches := numberedRegex.FindStringSubmatch(line); len(matches) >= 3 {
			// Handle numbered lists
			point := strings.TrimSpace(matches[1] + ": " + matches[2])
			if point != "" {
				points = append(points, point)
			}
		}
	}

	// If no formatted points found, try simpler approach
	if len(points) == 0 {
		simpleRegex := regexp.MustCompile(`^\s*[\*\-•]\s*(.+)$`)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if matches := simpleRegex.FindStringSubmatch(line); len(matches) >= 2 {
				point := strings.TrimSpace(matches[1])
				if point != "" && !strings.Contains(strings.ToLower(point), "bullet point") {
					points = append(points, point)
				}
			}
		}
	}

	return points
}

// New function to get current section outline point
func getCurrentSectionOutline(session *ScriptSession) string {
	if session.CurrentStep > 0 && session.CurrentStep <= len(session.OutlinePoints) {
		return session.OutlinePoints[session.CurrentStep-1]
	}
	return ""
}

func main() {
	// Get API key from environment variable
	apiKey := "AIzaSyDykpGH35C4BRC_V3OK-GoHAIJ97RfwvMc"
	if apiKey == "" {
		fmt.Println("Error: GEMINI_API_KEY environment variable not set")
		fmt.Println("Please set it with: export GEMINI_API_KEY=\"your-api-key-here\"")
		return
	}

	fmt.Println("=== Wisderly YouTube Script Generator ===")
	fmt.Println("This tool will automatically generate a complete YouTube script.")
	fmt.Println()

	// Load outline template from file
	outlineTemplate, err := loadScriptTemplate(outlineTemplateFile)
	if err != nil {
		fmt.Printf("Error loading outline template from '%s': %v\n", outlineTemplateFile, err)
		fmt.Println("Please make sure the template file exists in the same directory.")
		return
	}
	fmt.Printf("✓ Outline template loaded from: %s\n", outlineTemplateFile)

	// Load script template from file
	scriptTemplate, err := loadScriptTemplate(templateFile)
	if err != nil {
		fmt.Printf("Error loading script template from '%s': %v\n", templateFile, err)
		fmt.Println("Please make sure the template file exists in the same directory.")
		return
	}
	fmt.Printf("✓ Script template loaded from: %s\n", templateFile)

	// Load hook & introduction template from file
	hookIntroTemplate, err := loadScriptTemplate(hookIntroTemplateFile)
	if err != nil {
		fmt.Printf("Error loading hook & intro template from '%s': %v\n", hookIntroTemplateFile, err)
		fmt.Println("Please make sure the template file exists in the same directory.")
		return
	}
	fmt.Printf("✓ Hook & Introduction template loaded from: %s\n\n", hookIntroTemplateFile)

	reader := bufio.NewReader(os.Stdin)

	// Get topic from user
	fmt.Print("Enter your video topic: ")
	topic, _ := reader.ReadString('\n')
	topic = strings.TrimSpace(topic)

	if topic == "" {
		fmt.Println("Topic cannot be empty.")
		return
	}

	// Ask for visual guidance preference
	fmt.Print("Generate Visual Guidance? (y/n): ")
	visualInput, _ := reader.ReadString('\n')
	generateVisuals := strings.TrimSpace(strings.ToLower(visualInput)) == "y"

	// Initialize session
	session := &ScriptSession{
		Topic:       topic,
		Filename:    fmt.Sprintf("wisderly_script_%s_%d.txt", sanitizeFilename(topic), time.Now().Unix()),
		CurrentStep: 0,
	}

	fmt.Printf("\nGenerating script for topic: %s\n", topic)
	fmt.Printf("Visual Guidance: %t\n", generateVisuals)
	fmt.Printf("Output file: %s\n", session.Filename)
	fmt.Println("\n" + strings.Repeat("=", 50))

	// Step 1: Generate outline
	if err := generateOutline(apiKey, session, outlineTemplate); err != nil {
		fmt.Printf("Error generating outline: %v\n", err)
		return
	}

	// Parse outline points after generation
	session.OutlinePoints = parseOutlinePoints(session.Outline)
	fmt.Printf("✓ Parsed %d outline points\n", len(session.OutlinePoints))

	// Display parsed points for verification
	if len(session.OutlinePoints) > 0 {
		fmt.Println("\nParsed Outline Points:")
		for i, point := range session.OutlinePoints {
			fmt.Printf("  %d. %s\n", i+1, point)
		}
		fmt.Println()
	}

	// Step 2: Generate hook and introduction
	if err := generateHookAndIntroduction(apiKey, session, hookIntroTemplate); err != nil {
		fmt.Printf("Error generating hook and introduction: %v\n", err)
		return
	}

	// Auto-generate all 5 sections
	for session.CurrentStep < 5 {
		session.CurrentStep++
		fmt.Printf("\nAuto-generating Section %d...\n", session.CurrentStep)

		if err := generateSection(apiKey, session, scriptTemplate); err != nil {
			fmt.Printf("Error generating section %d: %v\n", session.CurrentStep, err)
			return
		}

		// Small delay between sections for better readability
		fmt.Println("Sleeping for 15 seconds before next section...")
		time.Sleep(15 * time.Second)
	}

	// Generate visual guidance (optional)
	if generateVisuals {
		if err := generateVisualGuidance(apiKey, session, scriptTemplate); err != nil {
			fmt.Printf("Error generating visual guidance: %v\n", err)
		}
	}

	// Final save
	saveSession(session)
	fmt.Printf("\n🎉 Complete script saved to: %s\n", session.Filename)
	fmt.Println(">>> End of script. Stay Healthy.")
}

func generateOutline(apiKey string, session *ScriptSession, scriptTemplate string) error {
	prompt := fmt.Sprintf("%s\n\nTopic: %s\n\nPlease provide EXACTLY 5 bullets for the outline as specified in Step 1.",
		strings.Replace(scriptTemplate, "[TOPIC]", session.Topic, -1), session.Topic)

	response, err := callGeminiAPI(apiKey, prompt)
	if err != nil {
		return err
	}

	session.Outline = response

	// Initialize file content with header only (no outline)
	session.Content.WriteString("=== WISDERLY YOUTUBE SCRIPT ===\n")
	session.Content.WriteString(fmt.Sprintf("Topic: %s\n", session.Topic))
	session.Content.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	session.Content.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Display outline in terminal only
	fmt.Println("OUTLINE GENERATED:")
	fmt.Println(response)
	fmt.Println("\n" + strings.Repeat("-", 50))

	return nil
}

func generateHookAndIntroduction(apiKey string, session *ScriptSession, hookIntroTemplate string) error {
	prompt := fmt.Sprintf(`%s

Based on this outline:
%s

Topic: %s

Please generate both the Hook and Introduction following the template specifications exactly:

1️⃣ **Hook (first 30 seconds):**
- Open with a question, surprising fact, or curiosity to grab attention
- Calm pacing, simple words
- Include trustworthy humble CTA

2️⃣ **Introduction (30 seconds):**
- Start with a relatable scenario, question, or statement to hook the audience
- Briefly introduce the topic and explain why it's important for seniors

Please write both sections now, clearly labeled as "HOOK:" and "INTRODUCTION:"`,
		hookIntroTemplate, session.Outline, session.Topic)

	fmt.Println("Generating Hook and Introduction...")

	response, err := callGeminiAPI(apiKey, prompt)
	if err != nil {
		return err
	}

	// Store hook and introduction
	session.Hook = response
	session.Introduction = response

	// Add to file content
	session.Content.WriteString("HOOK & INTRODUCTION:\n")
	session.Content.WriteString(response)
	session.Content.WriteString("\n\n")
	session.Content.WriteString(strings.Repeat("-", 60) + "\n\n")

	// Display in terminal
	fmt.Println("HOOK & INTRODUCTION GENERATED:")
	fmt.Println(response)
	fmt.Println("\n" + strings.Repeat("-", 50))

	// Save progress
	saveSession(session)

	return nil
}

func generateSection(apiKey string, session *ScriptSession, scriptTemplate string) error {
	// Get the current section's outline point
	currentOutlinePoint := getCurrentSectionOutline(session)

	var prompt string
	if currentOutlinePoint != "" {
		prompt = fmt.Sprintf(`Based on this complete outline:
%s

Current Section Focus: %s

Topic: %s

Please write Section %d focusing specifically on the outline point: "%s"

Remember to follow the exact specifications:
- 1000-1100 words
- Start with "Section %d" heading
- Seamless narration continuing from previous sections
- Focus specifically on the current outline point while maintaining flow

Generate Section %d now:`, session.Outline, currentOutlinePoint, session.Topic, session.CurrentStep, currentOutlinePoint, session.CurrentStep, session.CurrentStep)
	} else {
		// Fallback to original approach if parsing failed
		prompt = fmt.Sprintf(`Based on this outline:
%s

Hook and Introduction:
%s

And the topic: %s

Please write Section %d following the exact specifications. Remember:
- 1000-1100 words
- Start with "Section %d" heading
- Seamless narration continuing from previous sections and the hook/introduction
- End with word count and ">>> Awaiting CONTINUE"

Generate Section %d now:`, session.Outline, session.Hook, session.Topic, session.CurrentStep, session.CurrentStep, session.CurrentStep)
	}

	fmt.Printf("Generating Section %d", session.CurrentStep)
	if currentOutlinePoint != "" {
		fmt.Printf(" (Focus: %s)", currentOutlinePoint)
	}
	fmt.Println("...")

	response, err := callGeminiAPI(apiKey, prompt)
	if err != nil {
		return err
	}

	session.Content.WriteString(response)
	session.Content.WriteString("\n\n")

	fmt.Printf("SECTION %d GENERATED:\n", session.CurrentStep)
	if currentOutlinePoint != "" {
		fmt.Printf("Focus: %s\n", currentOutlinePoint)
	}
	fmt.Println(response)
	fmt.Println("\n" + strings.Repeat("-", 50))

	// Save progress after each section
	saveSession(session)

	return nil
}

func generateVisualGuidance(apiKey string, session *ScriptSession, scriptTemplate string) error {
	prompt := fmt.Sprintf(`Based on this complete script outline:
%s

Hook and Introduction:
%s

Topic: %s

Please provide Visual Guidance notes for the Hook, Introduction, and each of the 5 sections as specified:
- EXACTLY 2 visual descriptions for Hook and Introduction
- EXACTLY 3 visual descriptions for EACH of the 5 sections (15 visuals total)
- Format: "Hook Visual 1:", "Introduction Visual 1:", "Section X Visual 1:", etc.
- Brief notes for visuals for AI image generation
- Calm visuals, warm lighting, pastel colors, simple, minimal, realistic-stylized
- Focus on senior-friendly, gentle, and reassuring imagery
- End with ">>> End of script. Stay Healthy."

Example format:
Hook Visual 1: [description]
Introduction Visual 1: [description]
Section 1 Visual 1: [description]
Section 1 Visual 2: [description]  
Section 1 Visual 3: [description]
[continue for all 5 sections]`, session.Outline, session.Hook, session.Topic)

	fmt.Println("Generating Visual Guidance...")

	response, err := callGeminiAPI(apiKey, prompt)
	if err != nil {
		return err
	}

	session.Content.WriteString("VISUAL GUIDANCE:\n")
	session.Content.WriteString(response)
	session.Content.WriteString("\n")

	fmt.Println("VISUAL GUIDANCE GENERATED:")
	fmt.Println(response)

	return nil
}

func callGeminiAPI(apiKey, prompt string) (string, error) {
	requestBody := GeminiRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	fmt.Println("Prompt payload size in KB:", len(prompt)/1024)
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshalling JSON: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", baseURL, apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "PostmanRuntime/7.37.3")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}

func saveSession(session *ScriptSession) {
	file, err := os.Create(session.Filename)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	_, err = file.WriteString(session.Content.String())
	if err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
		return
	}

	fmt.Printf("Progress saved to: %s\n", session.Filename)
}

func sanitizeFilename(topic string) string {
	// Replace spaces and special characters for filename
	sanitized := strings.ReplaceAll(topic, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, "\\", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	sanitized = strings.ReplaceAll(sanitized, "*", "_")
	sanitized = strings.ReplaceAll(sanitized, "?", "_")
	sanitized = strings.ReplaceAll(sanitized, "\"", "_")
	sanitized = strings.ReplaceAll(sanitized, "<", "_")
	sanitized = strings.ReplaceAll(sanitized, ">", "_")
	sanitized = strings.ReplaceAll(sanitized, "|", "_")

	// Limit length
	if len(sanitized) > 30 {
		sanitized = sanitized[:30]
	}

	return sanitized
}
