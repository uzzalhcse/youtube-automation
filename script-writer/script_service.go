package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ScriptService orchestrates the script generation process
//type ScriptService struct {
//	templateService  *TemplateService
//	geminiService    *GeminiService
//	outlineParser    *OutlineParser
//	elevenLabsClient *elevenlabs.ElevenLabsClient
//	client           *http.Client
//}

//func NewScriptService(templateService *TemplateService, geminiService *GeminiService) *ScriptService {
//	return &ScriptService{
//		templateService: templateService,
//		geminiService:   geminiService,
//		outlineParser:   NewOutlineParser(),
//		elevenLabsClient: elevenlabs.NewElevenLabsClient(os.Getenv("ELEVENLABS_API_KEY"), &elevenlabs.Proxy{
//			Server:   os.Getenv("PROXY_SERVER"),
//			Username: os.Getenv("PROXY_USERNAME"),
//			Password: os.Getenv("PROXY_PASSWORD"),
//		}),
//		client: &http.Client{Timeout: timeout},
//	}
//}

func (yt *YtAutomation) GenerateCompleteScript(config *ScriptConfig, scriptID primitive.ObjectID) error {
	session := NewScriptSession(config, scriptID)

	// Step 1: Generate outline
	if err := yt.generateOutline(session, config.channel.Settings.DefaultSectionCount); err != nil {
		return fmt.Errorf("generating outline: %w", err)
	}

	// Step 2: Parse outline points
	session.OutlinePoints = yt.outlineParser.ParseOutlinePoints(session.Outline, config.channel.Settings.DefaultSectionCount)
	fmt.Printf("✓ Parsed %d outline points\n", len(session.OutlinePoints))
	yt.displayOutlinePoints(session.OutlinePoints)

	// Step 3: Generate hook and introduction
	if err := yt.generateHookAndIntroduction(session); err != nil {
		return fmt.Errorf("generating hook and introduction: %w", err)
	}
	// Step 4: Generate all sections
	for i := 1; i <= config.channel.Settings.DefaultSectionCount; i++ {
		session.CurrentStep = i
		fmt.Printf("\nAuto-generating Section %d...\n", i)

		if err := yt.generateSection(session); err != nil {
			return fmt.Errorf("generating section %d: %w", i, err)
		}
		// Sleep between sections for rate limiting
		if i < config.channel.Settings.DefaultSectionCount {
			fmt.Printf("Sleeping for %v before next section...\n", config.SleepBetweenSections)
			time.Sleep(config.SleepBetweenSections)
		}
	}

	// Step 5: Generate meta tags (description, tags, and thumbnail statement)
	if err := yt.generateMetaTag(session); err != nil {
		fmt.Printf("Warning: Error generating meta tags: %v\n", err)
		// Continue even if meta tag generation fails
	}

	// Final save for both files
	if err := yt.saveScriptFile(session); err != nil {
		return fmt.Errorf("saving script file: %w", err)
	}

	if session.MetaTag != "" {
		if err := yt.saveMetaTagFile(session); err != nil {
			return fmt.Errorf("saving meta tag file: %w", err)
		}
	}

	// Update in database
	updateData := bson.M{
		"full_script": session.Content.String(),
		"meta_tag":    session.MetaTag,
	}

	// You'll need to pass scriptID to this method
	if err := yt.updateScriptInDB(session.ScriptID, updateData); err != nil {
		fmt.Printf("Warning: Failed to update outline in DB: %v\n", err)
	}
	// Print completion summary
	fmt.Printf("\n🎉 SCRIPT GENERATION COMPLETED!\n")
	fmt.Printf("📁 Output folder: %s\n", session.OutputFolder)
	fmt.Printf("📄 Script file: %s\n", session.ScriptFilename)
	if session.MetaTag != "" {
		fmt.Printf("🏷️  Meta tag file: %s\n", session.MetaTagFilename)
	}

	// If full_script is being updated, also save chunks
	//if fullScript, exists := updateData["full_script"]; exists {
	//	if scriptStr, ok := fullScript.(string); ok && scriptStr != "" {
	//		if chunkErr := yt.saveScriptChunks(scriptID, scriptStr); chunkErr != nil {
	//			fmt.Printf("Warning: Failed to save script chunks: %v\n", chunkErr)
	//		}
	//	}
	//}

	return nil
}

func (yt *YtAutomation) generateOutline(session *ScriptSession, sectionCount int) error {
	fmt.Println("Generating outline...")

	prompt := yt.templateService.BuildOutlinePrompt(session.Config.Topic, sectionCount)
	response, err := yt.geminiService.GenerateContent(prompt)
	if err != nil {
		return err
	}

	session.Outline = response
	session.UpdateContext(response, "outline")

	// Parse outline points
	session.OutlinePoints = yt.outlineParser.ParseOutlinePoints(response, sectionCount)

	// Convert to MongoDB format
	var outlinePoints []OutlinePoint
	for i, point := range session.OutlinePoints {
		outlinePoints = append(outlinePoints, OutlinePoint{
			SectionNumber: i + 1,
			Title:         point,
			Description:   "", // Can be enhanced later
		})
	}

	// Update in database
	updateData := bson.M{
		"outline":        response,
		"outline_points": outlinePoints,
	}

	// You'll need to pass scriptID to this method
	if err := yt.updateScriptInDB(session.ScriptID, updateData); err != nil {
		fmt.Printf("Warning: Failed to update outline in DB: %v\n", err)
	}

	fmt.Println("OUTLINE GENERATED:")
	fmt.Println(response)
	return nil
}

func (yt *YtAutomation) generateHookAndIntroduction(session *ScriptSession) error {
	fmt.Println("Generating Hook and Introduction...")

	prompt := yt.templateService.BuildHookIntroPrompt(session)
	response, err := yt.geminiService.GenerateContextAwareContent(session, prompt)
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
	yt.saveScriptFile(session)
	return nil
}

func (yt *YtAutomation) generateMetaTag(session *ScriptSession) error {
	fmt.Println("Generating Meta Tags (Description, Tags, and Thumbnail Statement)...")

	prompt := yt.templateService.BuildMetaTagPrompt(session)
	response, err := yt.geminiService.GenerateContextAwareContent(session, prompt)
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
	return yt.saveMetaTagFile(session)
}

func (yt *YtAutomation) generateSection(session *ScriptSession) error {
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

	prompt := yt.templateService.BuildSectionPrompt(session, sectionNumber, outlinePoint)
	response, err := yt.geminiService.GenerateContextAwareContent(session, prompt)
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
	return yt.saveScriptFile(session)
}

func (yt *YtAutomation) initializeFileContent(session *ScriptSession) {
	session.Content.WriteString("\n")
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

func (yt *YtAutomation) saveScriptFile(session *ScriptSession) error {
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

func (yt *YtAutomation) saveMetaTagFile(session *ScriptSession) error {
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
	content += fmt.Sprintf("Channel: %s\n", session.Config.channel.ChannelName)
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

func (yt *YtAutomation) updateScriptInDB(scriptID primitive.ObjectID, updateData bson.M) error {
	_, err := scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": scriptID},
		bson.M{"$set": updateData},
	)

	return err
}
func (yt *YtAutomation) saveScriptChunks(scriptID primitive.ObjectID, fullScript string) error {
	// Split the script into chunks
	chunks := splitTextByCharLimit(fullScript, splitByCharLimit)

	// Prepare chunk documents for batch insert
	var chunkDocs []interface{}
	var savedChunks []ScriptAudio

	for i, chunk := range chunks {
		chunkDoc := ScriptAudio{
			ScriptID:   scriptID,
			ChunkIndex: i + 1,
			Content:    chunk,
			CharCount:  len(chunk),
			HasVisual:  false,
			CreatedAt:  time.Now(),
		}
		chunkDocs = append(chunkDocs, chunkDoc)
	}

	// Insert all chunks in batch
	if len(chunkDocs) > 0 {
		result, err := scriptAudiosCollection.InsertMany(context.Background(), chunkDocs)
		if err != nil {
			return fmt.Errorf("failed to save script chunks: %w", err)
		}

		// Prepare saved chunks with IDs for visual generation
		for i, insertedID := range result.InsertedIDs {
			chunk := chunkDocs[i].(ScriptAudio)
			chunk.ID = insertedID.(primitive.ObjectID)
			savedChunks = append(savedChunks, chunk)
		}

		fmt.Printf("✓ Saved %d script chunks to database\n", len(chunkDocs))

		// Generate audio for all chunks
		//go func() {
		//	if err := yt.generateVoiceOver(savedChunks); err != nil {
		//		fmt.Printf("Warning: Failed to generate audio for chunks: %v\n", err)
		//	}
		//}()
		// Generate visuals for all chunks
		go func() {
			if err := yt.generateVisualsForChunks(scriptID, savedChunks); err != nil {
				fmt.Printf("Warning: Failed to generate visuals for chunks: %v\n", err)
			}
		}()
	}

	return nil
}

func (yt *YtAutomation) generateVoiceOver(script Script, chunks []ScriptAudio) error {
	var audioFiles []string

	// Generate individual audio files
	for i, chunk := range chunks {
		fmt.Printf("Generating Voice for chunk %d/%d...\n", i+1, len(chunks))

		// Generate speech
		audioData, err := yt.elevenLabsClient.TextToSpeech(chunk.Content, os.Getenv("VOICE_ID"))
		if err != nil {
			return fmt.Errorf("Error generating speech: %v\n", err)
		}

		timestamp := time.Now().Format("20060102_15_04_05")
		filename := fmt.Sprintf("%s_voiceover_%d_%s.mp3", script.ChannelName, i, timestamp)
		path := filepath.Join("assets", "audio", filename)

		if err = saveAudioFile(audioData, path); err != nil {
			return fmt.Errorf("Error saving audio file: %v\n", err)
		}

		// Save individual chunk audio file path
		yt.SaveScriptAudioFile(chunk.ID, path)
		audioFiles = append(audioFiles, path)

		fmt.Printf("🎨 Voice generation complete for %d/%d...\n", i+1, len(chunks))
	}

	// Merge all audio files
	fmt.Println("Merging audio files...")
	mergedFilename := fmt.Sprintf("%s_complete_voiceover_%s.mp3",
		script.ChannelName,
		time.Now().Format("20060102_15_04_05"))
	mergedPath := filepath.Join("assets", "audio", mergedFilename)

	if err := yt.mergeAudioFiles(audioFiles, mergedPath); err != nil {
		return fmt.Errorf("Error merging audio files: %v", err)
	}

	// Update script collection with merged audio file
	yt.UpdateScriptCollection(script.ID, mergedPath)

	// Optionally clean up individual files
	if err := yt.cleanupTempFiles(audioFiles); err != nil {
		fmt.Printf("Warning: Failed to cleanup temp files: %v\n", err)
	}

	fmt.Println("✅ Complete voiceover generation and merging finished!")
	return nil
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

func (yt *YtAutomation) SaveScriptAudioFile(chunkID primitive.ObjectID, filepath string) {
	_, err := scriptAudiosCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": chunkID},
		bson.M{"$set": bson.M{"audio_file": filepath}},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to save chunk audio file path: %v\n", err)
	}
}

func (yt *YtAutomation) UpdateScriptCollection(scriptID primitive.ObjectID, filepath string) {
	_, err := scriptsCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": scriptID},
		bson.M{"$set": bson.M{"full_audio_file": filepath}},
	)
	if err != nil {
		fmt.Printf("Warning: Failed to save merged audio file path: %v\n", err)
	}
}
