package main

// Add these validation functions to your code

import (
	"context"
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// SRTTimeRange represents a time range from SRT content
type SRTTimeRange struct {
	StartTime float64
	EndTime   float64
}

// parseTimeToFloat converts time string to float64 seconds
func parseTimeToFloat(timeStr string) (float64, error) {
	return strconv.ParseFloat(timeStr, 64)
}

// extractSRTTimeRanges extracts all time ranges from SRT content
func extractSRTTimeRanges(srtContent string) ([]SRTTimeRange, error) {
	var timeRanges []SRTTimeRange
	lines := strings.Split(srtContent, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, " --> ") {
			parts := strings.Split(line, " --> ")
			if len(parts) == 2 {
				startStr := strings.TrimSpace(parts[0])
				endStr := strings.TrimSpace(parts[1])

				// Convert SRT time format (00:00:01,930) to seconds
				startTime, err := srtTimeToSeconds(startStr)
				if err != nil {
					log.Printf("Warning: Failed to parse start time '%s': %v", startStr, err)
					continue
				}

				endTime, err := srtTimeToSeconds(endStr)
				if err != nil {
					log.Printf("Warning: Failed to parse end time '%s': %v", endStr, err)
					continue
				}

				timeRanges = append(timeRanges, SRTTimeRange{
					StartTime: startTime,
					EndTime:   endTime,
				})
			}
		}
	}

	return timeRanges, nil
}

// srtTimeToSeconds converts SRT time format to seconds
func srtTimeToSeconds(timeStr string) (float64, error) {
	// Format: 00:00:01,930 or 00:01:30,500
	timeStr = strings.ReplaceAll(timeStr, ",", ".")
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format: expected HH:MM:SS.mmm, got %s", timeStr)
	}

	hours, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hours: %v", err)
	}

	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid minutes: %v", err)
	}

	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds: %v", err)
	}

	return hours*3600 + minutes*60 + seconds, nil
}

// validateVisualPromptSequence validates the visual prompt response against SRT content
func validateVisualPromptSequence(srtContent string, visualPrompts []VisualPromptResponse) ([]VisualPromptResponse, error) {
	// Extract SRT time ranges
	srtRanges, err := extractSRTTimeRanges(srtContent)
	if err != nil {
		return nil, fmt.Errorf("failed to extract SRT time ranges: %w", err)
	}

	if len(srtRanges) == 0 {
		log.Printf("Warning: No SRT time ranges found to validate against")
		return visualPrompts, nil
	}

	log.Printf("Found %d SRT time ranges for validation", len(srtRanges))

	// Sort SRT ranges by start time
	sort.Slice(srtRanges, func(i, j int) bool {
		return srtRanges[i].StartTime < srtRanges[j].StartTime
	})

	// Convert visual prompts to comparable format and validate
	var validPrompts []VisualPromptResponse
	var invalidPrompts []string

	for i, vp := range visualPrompts {
		startTime, err := parseTimeToFloat(vp.StartTime)
		if err != nil {
			invalidPrompts = append(invalidPrompts, fmt.Sprintf("prompt %d: invalid start time '%s'", i+1, vp.StartTime))
			continue
		}

		endTime, err := parseTimeToFloat(vp.EndTime)
		if err != nil {
			invalidPrompts = append(invalidPrompts, fmt.Sprintf("prompt %d: invalid end time '%s'", i+1, vp.EndTime))
			continue
		}

		if startTime >= endTime {
			invalidPrompts = append(invalidPrompts, fmt.Sprintf("prompt %d: start time %.2f >= end time %.2f", i+1, startTime, endTime))
			continue
		}

		validPrompts = append(validPrompts, vp)
	}

	// Log invalid prompts
	if len(invalidPrompts) > 0 {
		log.Printf("Found %d invalid visual prompts:", len(invalidPrompts))
		for _, invalid := range invalidPrompts {
			log.Printf("  - %s", invalid)
		}
	}

	// Sort valid prompts by start time
	sort.Slice(validPrompts, func(i, j int) bool {
		startI, _ := parseTimeToFloat(validPrompts[i].StartTime)
		startJ, _ := parseTimeToFloat(validPrompts[j].StartTime)
		return startI < startJ
	})

	// Check coverage of SRT ranges
	coveredRanges := 0
	totalSRTDuration := 0.0
	coveredDuration := 0.0

	for _, srtRange := range srtRanges {
		totalSRTDuration += srtRange.EndTime - srtRange.StartTime
		covered := false

		for _, vp := range validPrompts {
			vpStart, _ := parseTimeToFloat(vp.StartTime)
			vpEnd, _ := parseTimeToFloat(vp.EndTime)

			// Check if visual prompt overlaps with SRT range
			if vpStart <= srtRange.EndTime && vpEnd >= srtRange.StartTime {
				covered = true
				// Calculate overlap duration
				overlapStart := math.Max(vpStart, srtRange.StartTime)
				overlapEnd := math.Min(vpEnd, srtRange.EndTime)
				if overlapEnd > overlapStart {
					coveredDuration += overlapEnd - overlapStart
				}
				break
			}
		}

		if covered {
			coveredRanges++
		} else {
			log.Printf("Warning: SRT range %.2f-%.2f seconds not covered by any visual prompt",
				srtRange.StartTime, srtRange.EndTime)
		}
	}

	// Calculate coverage statistics
	coveragePercent := 0.0
	if totalSRTDuration > 0 {
		coveragePercent = (coveredDuration / totalSRTDuration) * 100
	}

	log.Printf("Visual prompt coverage: %d/%d ranges covered (%.1f%% time coverage)",
		coveredRanges, len(srtRanges), coveragePercent)

	// Check for sequence gaps
	gapCount := 0
	for i := 1; i < len(validPrompts); i++ {
		prevEnd, _ := parseTimeToFloat(validPrompts[i-1].EndTime)
		currStart, _ := parseTimeToFloat(validPrompts[i].StartTime)

		if currStart > prevEnd+0.5 { // Gap threshold: 0.5 seconds
			gapCount++
			log.Printf("Warning: Gap detected between prompts: %.2f to %.2f seconds (%.2f second gap)",
				prevEnd, currStart, currStart-prevEnd)
		}
	}

	if gapCount > 0 {
		log.Printf("Found %d gaps in visual prompt sequence", gapCount)
	}

	return validPrompts, nil
}

// Add this test function to verify your SRT parsing works correctly
func (yt *YtAutomation) testSRTValidation(scriptSrt ScriptSrt) ([]VisualPromptResponse, error) {
	ctx := context.Background()
	// Get all visual prompts for the script
	visualCursor, err := chunkVisualsCollection.Find(ctx, bson.M{"script_id": scriptSrt.ScriptID})
	if err != nil {
		fmt.Printf("failed to fetch visual prompts: %w", err)
	}
	defer visualCursor.Close(ctx)

	var visualPrompts []VisualPromptResponse
	if err = visualCursor.All(ctx, &visualPrompts); err != nil {
		fmt.Printf("failed to decode visual prompts: %w", err)
	}
	ranges, err := extractSRTTimeRanges(scriptSrt.Content)
	if err != nil {
		log.Printf("Error parsing SRT: %v", err)
		return nil, fmt.Errorf("failed to extract SRT time ranges: %w", err)
	}

	log.Printf("Parsed %d SRT ranges:", len(ranges))
	for i, r := range ranges {
		log.Printf("Range %d: %.3f - %.3f seconds", i+1, r.StartTime, r.EndTime)
	}

	validated, err := validateVisualPromptSequence(scriptSrt.Content, visualPrompts)
	if err != nil {
		log.Printf("Validation error: %v", err)
		return nil, fmt.Errorf("failed to validate visual prompts: %w", err)
	}

	log.Printf("Validation complete. Valid prompts: %d", len(validated))
	return validated, nil
}

// HTTP Handler for the missing SRT range check
func (yt *YtAutomation) checkMissingSRTRangesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	scriptIDStr := r.URL.Query().Get("script_id")
	if scriptIDStr == "" {
		respondWithError(w, http.StatusBadRequest, "script_id parameter is required")
		return
	}

	scriptID, err := primitive.ObjectIDFromHex(scriptIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid script ID format")
		return
	}

	// Check if script exists
	var scriptSrt ScriptSrt
	err = scriptSrtCollection.FindOne(context.Background(), bson.M{"script_id": scriptID}).Decode(&scriptSrt)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Script not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	// Perform coverage analysis
	report, err := yt.testSRTValidation(scriptSrt)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to analyze coverage: %v", err))
		return
	}

	// Return detailed report
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    report,
	})
}
func detectGapsInSequence(srtRanges []SRTTimeRange, visualPrompts []VisualPromptResponse) ([]GapRecoveryRequest, error) {
	var gaps []GapRecoveryRequest

	// Sort visual prompts by start time
	sort.Slice(visualPrompts, func(i, j int) bool {
		startI, _ := parseTimeToFloat(visualPrompts[i].StartTime)
		startJ, _ := parseTimeToFloat(visualPrompts[j].StartTime)
		return startI < startJ
	})

	// Check for gaps between consecutive visual prompts
	for i := 1; i < len(visualPrompts); i++ {
		prevEnd, _ := parseTimeToFloat(visualPrompts[i-1].EndTime)
		currStart, _ := parseTimeToFloat(visualPrompts[i].StartTime)

		if currStart > prevEnd+0.5 { // Gap threshold: 0.5 seconds
			// Find SRT content for this gap
			gapSRTContent := extractSRTContentForTimeRange(srtRanges, prevEnd, currStart)

			if gapSRTContent != "" {
				// Build context from surrounding prompts
				context := fmt.Sprintf("Previous visual: %s\nNext visual: %s",
					visualPrompts[i-1].Prompt, visualPrompts[i].Prompt)

				gaps = append(gaps, GapRecoveryRequest{
					StartTime:  prevEnd,
					EndTime:    currStart,
					SRTContent: gapSRTContent,
					Context:    context,
				})
			}
		}
	}

	// Check for gaps at the beginning and end
	if len(visualPrompts) > 0 && len(srtRanges) > 0 {
		firstPromptStart, _ := parseTimeToFloat(visualPrompts[0].StartTime)
		if srtRanges[0].StartTime < firstPromptStart-0.5 {
			gapSRTContent := extractSRTContentForTimeRange(srtRanges, srtRanges[0].StartTime, firstPromptStart)
			if gapSRTContent != "" {
				gaps = append([]GapRecoveryRequest{{
					StartTime:  srtRanges[0].StartTime,
					EndTime:    firstPromptStart,
					SRTContent: gapSRTContent,
					Context:    fmt.Sprintf("Opening section before: %s", visualPrompts[0].Prompt),
				}}, gaps...)
			}
		}

		lastPromptEnd, _ := parseTimeToFloat(visualPrompts[len(visualPrompts)-1].EndTime)
		lastSRTEnd := srtRanges[len(srtRanges)-1].EndTime
		if lastSRTEnd > lastPromptEnd+0.5 {
			gapSRTContent := extractSRTContentForTimeRange(srtRanges, lastPromptEnd, lastSRTEnd)
			if gapSRTContent != "" {
				gaps = append(gaps, GapRecoveryRequest{
					StartTime:  lastPromptEnd,
					EndTime:    lastSRTEnd,
					SRTContent: gapSRTContent,
					Context:    fmt.Sprintf("Closing section after: %s", visualPrompts[len(visualPrompts)-1].Prompt),
				})
			}
		}
	}

	return gaps, nil
}

// extractSRTContentForTimeRange extracts SRT content within a specific time range
func extractSRTContentForTimeRange(srtRanges []SRTTimeRange, startTime, endTime float64) string {
	var relevantContent []string

	for _, srtRange := range srtRanges {
		// Check if SRT range overlaps with the gap
		if srtRange.StartTime < endTime && srtRange.EndTime > startTime {
			// This would need the actual SRT content parsing - you'd need to modify
			// your SRT parsing to also extract the text content, not just time ranges
			relevantContent = append(relevantContent, fmt.Sprintf("[%.2f-%.2f]", srtRange.StartTime, srtRange.EndTime))
		}
	}

	return strings.Join(relevantContent, " ")
}
