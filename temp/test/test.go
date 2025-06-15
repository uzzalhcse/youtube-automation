package main

import (
	"fmt"
	"regexp"
	"strings"
)

func main() {
	outlineResponse := `Okay, here's an outline for a long-form video script on improving sleep and energy levels for seniors, designed for the "Wisderly" channel.

*   **Understanding the Sleep-Energy Connection for Seniors:** Exploring the unique sleep challenges faced by older adults and why quality sleep is crucial for physical and mental well-being. This section will cover the common sleep disturbances that affect seniors, such as insomnia, frequent awakenings, and changes in sleep cycles. We'll explain how poor sleep impacts energy levels, mood, cognitive function, and overall health, potentially exacerbating existing conditions like arthritis or heart problems. The goal is to emphasize the vital link between restorative sleep and a vibrant, active lifestyle in later years, motivating viewers to prioritize sleep improvement.

*   **The Evening Routine Roadmap: Simple Steps for a Restful Night:** A practical guide to building a relaxing and effective pre-sleep routine. We'll present a step-by-step approach, including creating a calming bedtime environment (dim lights, comfortable temperature), implementing a "digital sunset" (reducing screen time), incorporating relaxation techniques (gentle stretching, meditation, deep breathing), and optimizing evening meals and hydration. Focus will be on habits that are easy to incorporate into daily life and specifically beneficial for older adults, with adaptations offered for varying physical abilities and lifestyles. We will also suggest specific timing for each activity, creating a structured yet flexible roadmap for viewers to follow.

*   **Foods, Supplements, and Lifestyle Tweaks: Maximizing Your Sleep Potential:** Diving into specific dietary choices, supplements (with necessary precautions and disclaimers), and lifestyle adjustments that can significantly impact sleep quality. We'll discuss foods that promote sleep (e.g., tart cherries, chamomile tea), those to avoid (e.g., caffeine, alcohol), and the importance of balanced nutrition. The section will also address the potential benefits and risks of common sleep supplements like melatonin and magnesium, emphasizing the need for consulting with a healthcare professional. Finally, we will discuss the impact of regular physical activity (done earlier in the day) and consistent sleep-wake schedules on regulating the body's natural sleep-wake cycle.

`

	lines := strings.Split(outlineResponse, "\n")

	// Universal pattern: match bullet points with **bold** title and optional colon after
	re := regexp.MustCompile(`^\s*[\*\-•]\s+\*\*(.+?)\*\*:?[\s–-]*?(.*)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) > 2 {
			title := strings.TrimSpace(matches[1])
			description := strings.TrimSpace(matches[2])
			fmt.Println("Title:", title)
			fmt.Println("Description:", description)
			fmt.Println()
		}
	}
}
