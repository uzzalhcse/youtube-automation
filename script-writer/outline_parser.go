package main

import (
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

// Update ParseOutlinePoints to handle the format better
func (p *OutlineParser) ParseOutlinePoints(outlineResponse string) []string {
	var points []string
	lines := strings.Split(outlineResponse, "\n")

	// First pass: try to match formatted points
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Try all regex patterns (bullet -> numbered -> simple)
		if matches := p.bulletRegex.FindStringSubmatch(line); len(matches) >= 2 {
			point := p.combineMatches(matches[1], matches[2])
			if point != "" {
				points = append(points, point)
			}
		} else if matches := p.numberedRegex.FindStringSubmatch(line); len(matches) >= 2 {
			point := p.combineMatches(matches[1], matches[2])
			if point != "" {
				points = append(points, point)
			}
		} else if matches := p.simpleRegex.FindStringSubmatch(line); len(matches) >= 2 {
			point := p.combineMatches(matches[1], matches[2])
			if point != "" {
				points = append(points, point)
			}
		}
	}
	// Rest of your existing logic...
	if len(points) == 0 {
		points = p.extractMainPoints(lines)
	}

	return p.validateAndCleanPoints(points)
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

// isCapitalLetter checks if a rune is a capital letter
func (p *OutlineParser) isCapitalLetter(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

// validateAndCleanPoints validates and cleans the extracted points
func (p *OutlineParser) validateAndCleanPoints(points []string) []string {
	var cleaned []string

	for _, point := range points {
		// Clean the point
		cleaned_point := p.cleanPoint(point)

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

// cleanPoint removes unwanted characters and formats the point
func (p *OutlineParser) cleanPoint(point string) string {
	// Remove excessive whitespace
	point = strings.TrimSpace(point)

	// Remove common prefixes
	prefixes := []string{"**", "*", "-", "•"}
	for i := 1; i <= defaultSectionCount; i++ {
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
