package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	outlineTemplateFile   = "outline_template.txt"
	scriptTemplateFile    = "script_template.txt"
	hookIntroTemplateFile = "hook_intro_template.txt"
)

// TemplateService manages template loading and processing
type TemplateService struct {
	outlineTemplate   string
	scriptTemplate    string
	hookIntroTemplate string
}

// NewTemplateService creates a new template service
func NewTemplateService() *TemplateService {
	return &TemplateService{}
}

// LoadAllTemplates loads all required templates
func (t *TemplateService) LoadAllTemplates() error {
	templates := map[string]*string{
		outlineTemplateFile:   &t.outlineTemplate,
		scriptTemplateFile:    &t.scriptTemplate,
		hookIntroTemplateFile: &t.hookIntroTemplate,
	}

	for filename, templatePtr := range templates {
		content, err := t.loadTemplate(filename)
		if err != nil {
			return fmt.Errorf("loading template %s: %w", filename, err)
		}
		*templatePtr = content
		fmt.Printf("✓ %s loaded\n", filename)
	}

	return nil
}

func (t *TemplateService) loadTemplate(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("reading template file %s: %w", filename, err)
	}
	return string(data), nil
}

// GetOutlineTemplate returns the outline template with topic substitution
func (t *TemplateService) GetOutlineTemplate(topic string) string {
	return strings.Replace(t.outlineTemplate, "[TOPIC]", topic, -1)
}

// GetScriptTemplate returns the script template
func (t *TemplateService) GetScriptTemplate() string {
	return t.scriptTemplate
}

// GetHookIntroTemplate returns the hook and introduction template
func (t *TemplateService) GetHookIntroTemplate() string {
	return t.hookIntroTemplate
}

// BuildOutlinePrompt creates a context-aware outline prompt
func (t *TemplateService) BuildOutlinePrompt(topic string) string {
	template := t.GetOutlineTemplate(topic)

	return fmt.Sprintf(`%s

IMPORTANT REQUIREMENTS:
- Provide EXACTLY %d bullet points for the outline
- Each bullet should be formatted as: Title: Description
- No sub bullet points or nested lists in descriptions, only paragraphs

Topic: %s

Please provide the outline following the template specifications exactly.`,
		template, defaultSectionCount, topic)
}

// BuildHookIntroPrompt creates a context-aware hook and introduction prompt
func (t *TemplateService) BuildHookIntroPrompt(session *ScriptSession) string {
	return fmt.Sprintf(`%s

CONTEXT:
- Outline: %s
- Topic: %s

REQUIREMENTS:
**Hook & Introduction (300 words):**
 - Start with a relatable scenario, question, concern, or statement to hook the audience (e.g., 'Have you ever felt like your energy is fading faster than it used to?', "Tired of waking up with painful leg cramps?").  
 - Briefly introduce the topic and explain why it’s important for seniors.  
 - Mention what the video will cover (e.g., 'In this video, we’ll go over 5 simple habits to boost your energy and feel younger than ever!').  
 - End with a call to action, Ask them to like the video and subscribe to the channel for more helpful content.
 - Create smooth transition to main content
 - Maintain consistency with the hook's tone

Please write the section now, Without labeled like "Hook & Introduction:" Just continue.`,
		t.GetHookIntroTemplate(), session.Outline, session.Config.Topic)
}

// BuildSectionPrompt creates a context-aware section prompt
func (t *TemplateService) BuildSectionPrompt(session *ScriptSession, sectionNumber int, outlinePoint string) string {
	basePrompt := fmt.Sprintf(`SECTION %d GENERATION REQUEST

CURRENT OUTLINE POINT: %s

REQUIREMENTS:
- Write exactly 1000-1100 words
- Dont Start with any Section heading
- Focus specifically on the outline point: "%s"
- Avoid using bullet points, numbered lists, or section titles. Instead, use natural transitions to guide the viewer through the content.  
- Expand precisely that content—no drifting to later bullets.  
- Maintain seamless flow from previous content
- Absolutely **no** new section intros like “In this chapter…”—just continue.
- Use callbacks (“remember that crocodile‑dung sunscreen?”) for cohesion.  
- Do NOT re‑introduce the topic at each section break.  
- Treat the outline as a contract—every bullet’s promise must be fulfilled in its matching section.

Generate Section %d now, focusing on: %s`,
		sectionNumber, outlinePoint, outlinePoint, sectionNumber, outlinePoint)

	return basePrompt
}

// BuildVisualGuidancePrompt creates a visual guidance prompt
func (t *TemplateService) BuildVisualGuidancePrompt(session *ScriptSession) string {
	return fmt.Sprintf(`VISUAL GUIDANCE GENERATION

Based on the complete script content for topic: %s

REQUIREMENTS:
- EXACTLY %d visual descriptions for Hook and Introduction combined
- EXACTLY %d visual descriptions for EACH of the %d sections (%d visuals total)
- Format: "Hook Visual 1:", "Introduction Visual 1:", "Section X Visual 1:", etc.
- Brief, clear descriptions suitable for AI image generation
- Senior-friendly aesthetic: calm visuals, warm lighting, pastel colors
- Simple, minimal, realistic-stylized approach
- Focus on reassuring, gentle, and trustworthy imagery
- Avoid complex or busy compositions

VISUAL STYLE GUIDELINES:
- Warm, soft lighting
- Pastel and earth tone color palette
- Clean, uncluttered compositions
- Senior-friendly imagery (diverse older adults when people are shown)
- Professional but approachable aesthetic
- Clear, readable text elements when needed

Please provide visual guidance following this exact format."`,
		session.Config.Topic, visualImageMultiplier, visualImageMultiplier, defaultSectionCount, defaultSectionCount*visualImageMultiplier)
}
