package renderer

import (
	"strings"
)

// detectNeedsJS determines if a page likely needs JavaScript to render properly.
// It checks for common SPA (Single Page App) patterns.
func detectNeedsJS(html string) bool {
	// Skip check for very large HTML (likely already fully rendered)
	if len(html) > 500000 {
		return false
	}

	lower := strings.ToLower(html)
	bodyLen := countBodyTextLength(lower)

	// Check for SPA indicators when body has little text
	if bodyLen < 200 {
		spaIndicators := []string{
			`id="root"`, `id="app"`, `id="__next"`, `id="__nuxt"`, `id="__gatsby"`,
			`id="svelte"`, `ng-app`, `data-reactroot`, `<script src`,
			`window.__initial_state__`, `__next_data__`, `window.__remixcontext`,
			`window.__astro`,
		}
		for _, indicator := range spaIndicators {
			if strings.Contains(lower, indicator) {
				return true
			}
		}
	}

	// Check for noscript JavaScript enable message
	if strings.Contains(lower, "<noscript>") && strings.Contains(lower, "enable javascript") {
		return true
	}

	// Check for site builders known to need JS
	if bodyLen < 500 {
		builderIndicators := []string{
			"framerusercontent.com", "webflow.io", "wixsite.com", "squarespace.com/universal",
		}
		for _, indicator := range builderIndicators {
			if strings.Contains(lower, indicator) {
				return true
			}
		}
	}

	return false
}

// detectBlockInterstitial checks if the page shows a bot challenge/interstitial.
// This includes Cloudflare, CAPTCHA, and access denied pages.
func detectBlockInterstitial(html string) bool {
	// Skip check for very large HTML
	if len(html) > 500000 {
		return false
	}

	lower := strings.ToLower(html)
	markers := []string{
		"just a moment",      // Cloudflare
		"attention required", // Cloudflare
		"cf-browser-verification",
		"cf-challenge",
		"captcha",
		"access denied",
	}

	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// detectClientSideCrash checks for application crash overlays or framework-level
// client errors that should not be treated as a successful browser render.
func detectClientSideCrash(html string) bool {
	if html == "" {
		return false
	}

	lower := strings.ToLower(html)
	markers := []string{
		"application error: a client-side exception has occurred",
		"client-side exception has occurred",
		"application error",
		"something went wrong",
		"unexpected application error",
		"uncaught exception",
	}

	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// countBodyTextLength counts the visible text characters in the HTML body.
// The input is expected to be lowercase.
func countBodyTextLength(lower string) int {
	// Find body tag start
	bodyTagStart := strings.Index(lower, "<body")
	if bodyTagStart == -1 {
		return 1000 // Default high value if no body tag
	}

	// Find end of opening body tag
	bodyStart := strings.Index(lower[bodyTagStart:], ">")
	if bodyStart == -1 {
		return 1000
	}
	bodyStart = bodyTagStart + bodyStart + 1

	// Find closing body tag
	bodyEnd := strings.Index(lower, "</body>")
	if bodyEnd == -1 {
		return 1000
	}

	// Extract body content and count visible text
	body := lower[bodyStart:bodyEnd]
	textLen := 0
	inTag := false
	for _, ch := range body {
		if ch == '<' {
			inTag = true
		} else if ch == '>' {
			inTag = false
		} else if !inTag && !strings.ContainsRune(" \t\r\n", ch) {
			textLen++
		}
	}
	return textLen
}

// hasMinimalContent checks if rendered HTML has very little content.
// This suggests JavaScript rendering failed or returned minimal content.
func hasMinimalContent(html string) bool {
	// Skip check for large HTML (likely has content)
	if len(html) > 50000 {
		return false
	}
	return countBodyTextLength(strings.ToLower(html)) < 50
}

func contentTextLength(html string) int {
	return countBodyTextLength(strings.ToLower(html))
}

// copyHeaders creates a copy of a headers map.
// Returns an empty map if input is nil or empty.
func copyHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}
