package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// OutlineParser handles parsing of outline responses
type OutlineParser struct {
	bulletRegex   *regexp.Regexp
	numberedRegex *regexp.Regexp
	simpleRegex   *regexp.Regexp
}

// NewOutlineParser creates a new outline parser
func NewOutlineParser() *OutlineParser {
	return &OutlineParser{
		bulletRegex:   regexp.MustCompile(`^\s*[\*\-•]\s+\*\*(.+?)\*\*:?[\s–-]*?(.*)$`),
		numberedRegex: regexp.MustCompile(`^\s*\d+\.\s+\*\*(.+):\*\*\s+(.+)$`),
		simpleRegex:   regexp.MustCompile(`^\s*\*\*(.+):\*\*\s+(.+)$`),
	}
}

func (p *OutlineParser) ParseOutlineJSON(outlineResponse string, sectionCount int) ([]OutlineSection, error) {
	var response OutlineResponse

	// Clean response - remove any markdown code blocks
	cleanResponse := strings.TrimSpace(outlineResponse)
	cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
	cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	cleanResponse = strings.TrimSpace(cleanResponse)

	if err := json.Unmarshal([]byte(cleanResponse), &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON outline: %w", err)
	}

	if len(response.Sections) != sectionCount {
		return nil, fmt.Errorf("expected %d sections, got %d", sectionCount, len(response.Sections))
	}

	return response.Sections, nil
}

// combineMatches combines title and description from regex matches
func (p *OutlineParser) combineMatches(title, description string) string {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	if title == "" {
		return ""
	}

	if description == "" {
		return title
	}

	return title + ": " + description
}

// isMetaContent checks if the point is metadata rather than content
func (p *OutlineParser) isMetaContent(point string) bool {
	metaKeywords := []string{
		"bullet point", "outline", "section", "step", "topic", "video", "script",
	}

	pointLower := strings.ToLower(point)
	for _, keyword := range metaKeywords {
		if strings.Contains(pointLower, keyword) {
			return true
		}
	}
	return false
}

// extractMainPoints attempts to extract main points from unformatted text
func (p *OutlineParser) extractMainPoints(lines []string) []string {
	var points []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 10 || len(line) > 200 { // Skip very short or very long lines
			continue
		}

		// Look for lines that start with a capital letter and contain a colon
		if p.looksLikeMainPoint(line) {
			points = append(points, line)
		}
	}

	return points
}

// looksLikeMainPoint heuristically determines if a line looks like a main point
func (p *OutlineParser) looksLikeMainPoint(line string) bool {
	// Must start with capital letter
	if len(line) == 0 || !p.isCapitalLetter(rune(line[0])) {
		return false
	}

	// Should contain either a colon or be a complete thought
	hasColon := strings.Contains(line, ":")
	isComplete := strings.HasSuffix(line, ".") || strings.HasSuffix(line, "?") || strings.HasSuffix(line, "!")

	return hasColon || isComplete
}

func (p *OutlineParser) isCapitalLetter(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func (p *OutlineParser) validateAndCleanPoints(points []string, sectionCount int) []string {
	var cleaned []string

	for _, point := range points {
		// Clean the point
		cleaned_point := p.cleanPoint(point, sectionCount)

		// Validate the point
		//if p.isValidPoint(cleaned_point) {
		//	cleaned = append(cleaned, cleaned_point)
		//}
		cleaned = append(cleaned, cleaned_point)
	}

	// Limit to maximum expected points
	if len(cleaned) > 5 {
		cleaned = cleaned[:5]
	}

	return cleaned
}

func (p *OutlineParser) cleanPoint(point string, sectionCount int) string {
	// Remove excessive whitespace
	point = strings.TrimSpace(point)

	// Remove common prefixes
	prefixes := []string{"**", "*", "-", "•"}
	for i := 1; i <= sectionCount; i++ {
		prefixes = append(prefixes, strconv.Itoa(i)+".")
	}
	for _, prefix := range prefixes {
		point = strings.TrimPrefix(point, prefix)
		point = strings.TrimSpace(point)
	}

	// Remove trailing **
	point = strings.TrimSuffix(point, "**")
	point = strings.TrimSpace(point)

	return point
}

// isValidPoint checks if a point meets quality criteria
func (p *OutlineParser) isValidPoint(point string) bool {
	if len(point) < 5 || len(point) > 300 { // Changed from defaultSectionCount to 5
		return false
	}

	// Check if it's not just metadata
	if p.isMetaContent(point) {
		return false
	}

	// Should contain some meaningful content
	words := strings.Fields(point)
	return len(words) >= 2
}
func (p *OutlineParser) ParseHookIntroJSON(response string) (HookIntroContent, error) {
	var hookResponse HookIntroResponse

	cleanResponse := p.cleanJSONResponse(response)

	if err := json.Unmarshal([]byte(cleanResponse), &hookResponse); err != nil {
		return HookIntroContent{}, fmt.Errorf("failed to parse hook intro JSON: %w", err)
	}

	return hookResponse.HookIntro, nil
}

func (p *OutlineParser) ParseSectionJSON(response string) (SectionContent, error) {
	var sectionResponse SectionResponse

	cleanResponse := p.cleanJSONResponse(response)

	if err := json.Unmarshal([]byte(cleanResponse), &sectionResponse); err != nil {
		return SectionContent{}, fmt.Errorf("failed to parse section JSON: %w", err)
	}

	return sectionResponse.Section, nil
}

func (p *OutlineParser) ParseMetaJSON(response string) (MetaContent, error) {
	var metaResponse MetaResponse

	cleanResponse := p.cleanJSONResponse(response)

	if err := json.Unmarshal([]byte(cleanResponse), &metaResponse); err != nil {
		return MetaContent{}, fmt.Errorf("failed to parse meta JSON: %w", err)
	}

	return metaResponse.Meta, nil
}

func (p *OutlineParser) cleanJSONResponse(response string) string {
	cleanResponse := strings.TrimSpace(response)
	cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
	cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	return strings.TrimSpace(cleanResponse)
}
