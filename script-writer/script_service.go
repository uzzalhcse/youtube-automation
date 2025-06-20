// File: script_service.go
package main

import (
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strings"
	"time"
)

// Replace entire GenerateCompleteScript method
func (yt *YtAutomation) GenerateCompleteScript(scriptID primitive.ObjectID) error {
	// Load script from DB
	script, err := yt.getScriptByID(scriptID)
	if err != nil {
		return fmt.Errorf("loading script: %w", err)
	}

	// Get channel settings
	channel, err := yt.getChannelByID(script.ChannelID)
	if err != nil {
		return fmt.Errorf("loading channel: %w", err)
	}

	// Update status
	yt.updateScriptStatus(scriptID, "generating_outline")

	// Step 1: Generate outline
	if err := yt.generateOutline(script, channel.Settings.DefaultSectionCount); err != nil {
		yt.updateScriptError(scriptID, err.Error())
		return fmt.Errorf("generating outline: %w", err)
	}
	// reload updated script from db
	script, err = yt.getScriptByID(scriptID)
	if err != nil {
		return fmt.Errorf("loading script: %w", err)
	}

	// Step 2: Generate hook and introduction
	yt.updateScriptStatus(scriptID, "generating_hook")
	if err := yt.generateHookAndIntroduction(script, channel.Settings.WordLimitForHookIntro); err != nil {
		yt.updateScriptError(scriptID, err.Error())
		return fmt.Errorf("generating hook: %w", err)
	}

	// Step 3: Generate sections
	yt.updateScriptStatus(scriptID, "generating_sections")
	for i := 1; i <= channel.Settings.DefaultSectionCount; i++ {
		yt.updateScriptCurrentSection(scriptID, i)
		if err := yt.generateSection(script, i, channel.Settings.WordLimitPerSection); err != nil {
			yt.updateScriptError(scriptID, err.Error())
			return fmt.Errorf("generating section %d: %w", i, err)
		}
		time.Sleep(time.Second * 2) // Rate limiting
	}

	// Step 4: Generate meta tags
	yt.updateScriptStatus(scriptID, "generating_meta")
	if err := yt.generateMetaTag(script); err != nil {
		fmt.Printf("Warning: Meta tag generation failed: %v\n", err)
	}

	// Mark as completed
	yt.updateScriptStatus(scriptID, "completed")
	yt.updateScriptCompletedAt(scriptID)

	fmt.Printf("🎉 Script generation completed for: %s\n", script.Topic)
	return nil
}

func (yt *YtAutomation) generateOutline(script *Script, sectionCount int) error {
	fmt.Println("Generating outline...")

	systemPrompt, userPrompt, err := yt.templateService.BuildOutlinePrompt(script, sectionCount)
	if err != nil {
		return fmt.Errorf("building outline prompt: %w", err)
	}

	response, err := yt.aiService.GenerateContentWithSystem(systemPrompt, userPrompt)
	if err != nil {
		return err
	}

	// Parse JSON outline
	sections, err := yt.outlineParser.ParseOutlineJSON(response, sectionCount)
	if err != nil {
		return fmt.Errorf("parsing outline JSON: %w", err)
	}

	// Convert sections to string format
	var outlineString strings.Builder
	for i, section := range sections {
		outlineString.WriteString(fmt.Sprintf("%d. %s\n", i+1, section.Title))
		if section.Summary != "" {
			outlineString.WriteString(fmt.Sprintf("   Summary: %s\n", section.Summary))
		}
		outlineString.WriteString("\n")
	}

	var dbOutlinePoints []OutlinePoint
	for i, section := range sections {
		dbOutlinePoints = append(dbOutlinePoints, OutlinePoint{
			SectionNumber: i + 1,
			Title:         section.Title,
			Summary:       section.Summary,
		})
	}

	updateData := bson.M{
		"outline":        strings.TrimSpace(outlineString.String()),
		"outline_points": dbOutlinePoints,
	}

	return yt.updateScriptInDB(script.ID, updateData)
}

func (yt *YtAutomation) generateHookAndIntroduction(script *Script, wordLimit int) error {
	fmt.Println("Generating Hook and Introduction...")

	systemPrompt, userPrompt, err := yt.templateService.BuildHookIntroPrompt(script, wordLimit)
	if err != nil {
		return fmt.Errorf("building hook prompt: %w", err)
	}

	response, err := yt.aiService.GenerateContentWithSystem(systemPrompt, userPrompt)
	if err != nil {
		return err
	}

	// Parse JSON hook intro
	hookContent, err := yt.outlineParser.ParseHookIntroJSON(response)
	if err != nil {
		return fmt.Errorf("parsing hook intro JSON: %w", err)
	}

	updateData := bson.M{
		"full_script": hookContent.Content + "\n\n\n\n\n\n",
		"hook_mode":   hookContent.ModeUsed,
	}

	return yt.updateScriptInDB(script.ID, updateData)
}

func (yt *YtAutomation) generateSection(script *Script, sectionNumber int, wordLimit int) error {
	// Reload script to get latest content
	updatedScript, err := yt.getScriptByID(script.ID)
	if err != nil {
		return err
	}

	var outlinePoint string
	if sectionNumber <= len(updatedScript.OutlinePoints) {
		outlinePoint = updatedScript.OutlinePoints[sectionNumber-1].Title
	}

	fmt.Printf("Generating Section %d: %s\n", sectionNumber, outlinePoint)

	systemPrompt, userPrompt, err := yt.templateService.BuildSectionPrompt(updatedScript, sectionNumber, outlinePoint, wordLimit)
	if err != nil {
		return fmt.Errorf("building section prompt: %w", err)
	}

	response, err := yt.aiService.GenerateContentWithSystem(systemPrompt, userPrompt)
	if err != nil {
		return err
	}

	// Parse JSON section
	sectionContent, err := yt.outlineParser.ParseSectionJSON(response)
	if err != nil {
		return fmt.Errorf("parsing section JSON: %w", err)
	}

	// Append to existing script content
	newContent := updatedScript.FullScript + sectionContent.Content + "\n\n\n\n\n\n"
	updateData := bson.M{
		"full_script":        newContent,
		"sections_generated": sectionNumber,
	}

	return yt.updateScriptInDB(script.ID, updateData)
}

func (yt *YtAutomation) generateMetaTag(script *Script) error {
	fmt.Println("Generating Meta Tags...")

	updatedScript, err := yt.getScriptByID(script.ID)
	if err != nil {
		return err
	}

	systemPrompt, userPrompt, err := yt.templateService.BuildMetaTagPrompt(updatedScript)
	if err != nil {
		return fmt.Errorf("building meta tag prompt: %w", err)
	}

	response, err := yt.aiService.GenerateContentWithSystem(systemPrompt, userPrompt)
	if err != nil {
		return err
	}

	// Parse JSON meta
	metaContent, err := yt.outlineParser.ParseMetaJSON(response)
	if err != nil {
		return fmt.Errorf("parsing meta JSON: %w", err)
	}

	return yt.updateScriptInDB(script.ID, bson.M{
		"meta": metaContent,
	})
}
func (yt *YtAutomation) displayOutlinePoints(points []string) {
	if len(points) > 0 {
		fmt.Println("\nParsed Outline Points:")
		for i, point := range points {
			fmt.Printf("  %d. %s\n", i+1, point)
		}
		fmt.Println()
	}
}
