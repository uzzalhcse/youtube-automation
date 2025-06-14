package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ScriptService orchestrates the script generation process
type ScriptService struct {
	templateService *TemplateService
	geminiService   *GeminiService
	outlineParser   *OutlineParser
}

// NewScriptService creates a new script service
func NewScriptService(templateService *TemplateService, geminiService *GeminiService) *ScriptService {
	return &ScriptService{
		templateService: templateService,
		geminiService:   geminiService,
		outlineParser:   NewOutlineParser(),
	}
}

// GenerateCompleteScript generates a complete script from start to finish
func (s *ScriptService) GenerateCompleteScript(config *ScriptConfig) error {
	session := NewScriptSession(config)

	fmt.Printf("📁 Creating output folder: %s\n", session.OutputFolder)
	fmt.Printf("📄 Script will be saved to: %s\n", session.ScriptFilename)
	fmt.Printf("🏷️  Meta tags will be saved to: %s\n", session.MetaTagFilename)

	// Step 1: Generate outline
	if err := s.generateOutline(session); err != nil {
		return fmt.Errorf("generating outline: %w", err)
	}

	// Step 2: Parse outline points
	session.OutlinePoints = s.outlineParser.ParseOutlinePoints(session.Outline)
	fmt.Printf("✓ Parsed %d outline points\n", len(session.OutlinePoints))
	s.displayOutlinePoints(session.OutlinePoints)

	// Step 3: Generate hook and introduction
	if err := s.generateHookAndIntroduction(session); err != nil {
		return fmt.Errorf("generating hook and introduction: %w", err)
	}

	// Step 4: Generate all sections
	for i := 1; i <= config.SectionCount; i++ {
		session.CurrentStep = i
		fmt.Printf("\nAuto-generating Section %d...\n", i)

		if err := s.generateSection(session); err != nil {
			return fmt.Errorf("generating section %d: %w", i, err)
		}

		// Sleep between sections for rate limiting
		if i < config.SectionCount {
			fmt.Printf("Sleeping for %v before next section...\n", config.SleepBetweenSections)
			time.Sleep(config.SleepBetweenSections)
		}
	}

	// Step 5: Generate meta tags (description, tags, and thumbnail statement)
	if err := s.generateMetaTag(session); err != nil {
		fmt.Printf("Warning: Error generating meta tags: %v\n", err)
		// Continue even if meta tag generation fails
	}

	// Step 6: Generate visual guidance (if requested)
	if config.GenerateVisuals {
		if err := s.generateVisualGuidance(session); err != nil {
			fmt.Printf("Warning: Error generating visual guidance: %v\n", err)
			// Continue even if visual guidance fails
		}
	}

	// Final save for both files
	if err := s.saveScriptFile(session); err != nil {
		return fmt.Errorf("saving script file: %w", err)
	}

	if session.MetaTag != "" {
		if err := s.saveMetaTagFile(session); err != nil {
			return fmt.Errorf("saving meta tag file: %w", err)
		}
	}

	// Print completion summary
	fmt.Printf("\n🎉 SCRIPT GENERATION COMPLETED!\n")
	fmt.Printf("📁 Output folder: %s\n", session.OutputFolder)
	fmt.Printf("📄 Script file: %s\n", session.ScriptFilename)
	if session.MetaTag != "" {
		fmt.Printf("🏷️  Meta tag file: %s\n", session.MetaTagFilename)
	}

	return nil
}

func (s *ScriptService) generateOutline(session *ScriptSession) error {
	fmt.Println("Generating outline...")

	prompt := s.templateService.BuildOutlinePrompt(session.Config.Topic)
	response, err := s.geminiService.GenerateContent(prompt)
	if err != nil {
		return err
	}

	session.Outline = response
	session.UpdateContext(response, "outline")

	// Initialize file content
	s.initializeFileContent(session)

	// Display outline
	fmt.Println("OUTLINE GENERATED:")
	fmt.Println(response)

	return nil
}

func (s *ScriptService) generateHookAndIntroduction(session *ScriptSession) error {
	fmt.Println("Generating Hook and Introduction...")

	prompt := s.templateService.BuildHookIntroPrompt(session)
	response, err := s.geminiService.GenerateContextAwareContent(session, prompt)
	if err != nil {
		return err
	}

	session.Hook = response
	session.Introduction = response
	session.UpdateContext(response, "hook_intro")

	// Add to file content
	session.Content.WriteString(response)
	session.Content.WriteString("\n\n\n\n\n\n")

	// Display
	fmt.Println("HOOK & INTRODUCTION GENERATED:")
	fmt.Println(response)
	fmt.Println("\n" + strings.Repeat("-", 50))

	// Save progress
	s.saveScriptFile(session)
	return nil
}

func (s *ScriptService) generateMetaTag(session *ScriptSession) error {
	fmt.Println("Generating Meta Tags (Description, Tags, and Thumbnail Statement)...")

	prompt := s.templateService.BuildMetaTagPrompt(session)
	response, err := s.geminiService.GenerateContextAwareContent(session, prompt)
	if err != nil {
		return err
	}

	session.MetaTag = response
	session.UpdateContext(response, "meta_tag")

	// Display
	fmt.Println("META TAGS GENERATED:")
	fmt.Println(response)
	fmt.Println("\n" + strings.Repeat("-", 50))

	// Save meta tag file immediately
	return s.saveMetaTagFile(session)
}

func (s *ScriptService) generateSection(session *ScriptSession) error {
	sectionNumber := session.CurrentStep

	// Get the outline point for this section
	var outlinePoint string
	if sectionNumber <= len(session.OutlinePoints) {
		outlinePoint = session.OutlinePoints[sectionNumber-1]
	}

	fmt.Printf("Generating Section %d", sectionNumber)
	if outlinePoint != "" {
		fmt.Printf(" (Focus: %s)", outlinePoint)
	}
	fmt.Println("...")

	prompt := s.templateService.BuildSectionPrompt(session, sectionNumber, outlinePoint)
	response, err := s.geminiService.GenerateContextAwareContent(session, prompt)
	if err != nil {
		return err
	}

	session.UpdateContext(response, "section")

	// Add to content
	session.Content.WriteString(response)
	session.Content.WriteString("\n\n\n\n\n\n")

	// Display
	fmt.Printf("SECTION %d GENERATED:\n", sectionNumber)
	if outlinePoint != "" {
		fmt.Printf("Focus: %s\n", outlinePoint)
	}
	fmt.Println(response)
	fmt.Println("\n" + strings.Repeat("-", 50))

	// Save progress
	return s.saveScriptFile(session)
}

func (s *ScriptService) generateVisualGuidance(session *ScriptSession) error {
	fmt.Println("Generating Visual Guidance...")

	prompt := s.templateService.BuildVisualGuidancePrompt(session)
	response, err := s.geminiService.GenerateContextAwareContent(session, prompt)
	if err != nil {
		return err
	}

	// Add visual guidance to script content
	session.Content.WriteString("\n\n" + strings.Repeat("=", 60) + "\n")
	session.Content.WriteString("VISUAL GUIDANCE\n")
	session.Content.WriteString(strings.Repeat("=", 60) + "\n\n")
	session.Content.WriteString(response)
	session.Content.WriteString("\n\n\n")

	fmt.Println("VISUAL GUIDANCE GENERATED:")
	fmt.Println(response)

	return s.saveScriptFile(session)
}

func (s *ScriptService) initializeFileContent(session *ScriptSession) {
	session.Content.WriteString("\n")
}

func (s *ScriptService) displayOutlinePoints(points []string) {
	if len(points) > 0 {
		fmt.Println("\nParsed Outline Points:")
		for i, point := range points {
			fmt.Printf("  %d. %s\n", i+1, point)
		}
		fmt.Println()
	}
}

func (s *ScriptService) saveScriptFile(session *ScriptSession) error {
	file, err := os.Create(session.ScriptFilename)
	if err != nil {
		return fmt.Errorf("creating script file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(session.Content.String())
	if err != nil {
		return fmt.Errorf("writing to script file: %w", err)
	}

	fmt.Printf("📄 Script saved to: %s\n", session.ScriptFilename)
	return nil
}

func (s *ScriptService) saveMetaTagFile(session *ScriptSession) error {
	if session.MetaTag == "" {
		return nil // Nothing to save
	}

	file, err := os.Create(session.MetaTagFilename)
	if err != nil {
		return fmt.Errorf("creating meta tag file: %w", err)
	}
	defer file.Close()

	// Create a formatted meta tag file
	content := fmt.Sprintf("=== WISDERLY YOUTUBE META INFORMATION ===\n")
	content += fmt.Sprintf("Channel: %s\n", session.Config.ChannelName)
	content += fmt.Sprintf("Topic: %s\n", session.Config.Topic)
	content += fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	content += fmt.Sprintf("Outline: \n%s\n\n", session.Outline)
	content += strings.Repeat("=", 60) + "\n\n"
	content += session.MetaTag

	_, err = file.WriteString(content)
	if err != nil {
		return fmt.Errorf("writing to meta tag file: %w", err)
	}

	fmt.Printf("🏷️  Meta tags saved to: %s\n", session.MetaTagFilename)
	return nil
}
