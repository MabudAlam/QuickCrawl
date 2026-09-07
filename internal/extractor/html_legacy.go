// Package extractor provides content extraction from HTML into various formats.
package extractor

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// HTML Flow
// 1. `Extract` captures metadata from the raw document before any mutation.
// 2. `preprocessHTML` strips the document head, removes noise, applies
//    include/exclude selectors, optionally isolates main content, and applies
//    CSS selector overrides.
// 3. `postprocessHTML` sanitizes the resulting HTML, unwraps safe wrappers,
//    removes empty/duplicate nodes, and renders a readable `<html><body>...`
//    document for the `html` response.
// 4. Markdown/plain text generation consumes the post-processed HTML so all
//    output formats share the same cleaned content tree.

// ─── Regex Patterns ────────────────────────────────────────────────────────────

// Package-level compiled regexes — compiled once, reused on every call.
var (
	noScriptRe   = regexp.MustCompile(`(?i)<noscript[^>]*>.*?</noscript>`)
	iframeRe     = regexp.MustCompile(`(?i)<iframe[^>]*>.*?</iframe>`)
	svgRe        = regexp.MustCompile(`(?i)<svg[^>]*>.*?</svg>`)
	dataImgRe    = regexp.MustCompile(`(?i)<img[^>]*src=["']data:[^"']*["'][^>]*>`)
	urlTextRe    = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>"']+\.[a-z]{2,}[^\s<>"']*`)
	buttonRe     = regexp.MustCompile(`(?si)<button[^>]*>.*?</button>`)
	whitespaceRe = regexp.MustCompile(`[ \t]{2,}`)
	newlineRe    = regexp.MustCompile(`\n\s*\n\s*\n+`)
	emptyDivRe   = regexp.MustCompile(`(?si)<div[^>]*>\s*</div>`)
	emptySpanRe  = regexp.MustCompile(`(?si)<span[^>]*>\s*</span>`)
)

// ─── Readability Selectors ────────────────────────────────────────────────────

var prioritySelectors = []string{
	"#readme .markdown-body",
	"#readme",
	".markdown-body",
	".repository-content",
	".Layout-main",
	"article",
	"main",
	"[role=\"main\"]",
}

var scoredSelectors = []string{
	"#readme", ".markdown-body", ".repository-content", ".Layout-main",
	".post-content", ".article-body", ".entry-content", ".article-content",
	".post-body", ".story-body", ".content-body", "#main-content", "#article",
	"#content", ".content", ".main", "[itemprop=\"articleBody\"]",
	"[itemprop=\"text\"]", ".main-page-content", ".js-post-body", ".s-prose",
	"#question", ".page-content", "#page-content", "[role=\"article\"]",
	".mw-parser-output", "#mw-content-text", "#bodyContent", ".mw-body-content",
}

var innerSelectors = []string{
	"#readme", ".markdown-body", ".repository-content", ".Layout-main",
	".main-page-content", ".article-content", ".post-content", ".entry-content",
	".content-body", ".article-body", "[itemprop=\"articleBody\"]", "[itemprop=\"text\"]",
	".mw-parser-output", "#mw-content-text", "#content", ".content", "article",
}

var contentHints = []string{
	"article", "content", "main", "body", "post", "story", "entry",
	"markdown", "markdown-body", "readme", "repository-content", "layout-main",
	"articlebody", "question", "answer", "mw-parser-output",
	"page-content", "text", "prose", "readme", "documentation",
}

// ─── HTML Cleaning ───────────────────────────────────────────────────────────

// HTMLPreprocessOptions controls request-specific HTML preprocessing.
type HTMLPreprocessOptions struct {
	IncludeTags   []string
	ExcludeTags   []string
	CSSSelector   *string
	BypassFilters bool
}

// ─── Readability ─────────────────────────────────────────────────────────────

// ExtractMainContent uses readability-style scoring to find the main content
// area of a page, trying priority selectors first, then scored selectors,
// then falling back to common element types.
func ExtractMainContent(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	// Try priority selectors first (article, main, role=main).
	bestContent := findBestContentCandidate(doc, prioritySelectors)
	if bestContent == "" {
		bestContent = findBestContentCandidate(doc, scoredSelectors)
	}
	if bestContent == "" {
		bestContent = findBestContentCandidate(doc, []string{
			"article", "main", "section", "div", "td", "blockquote", "pre", "li", "body",
		})
	}
	if bestContent != "" {
		if refined := refineInnerContent(bestContent); refined != "" {
			return refined
		}
		return bestContent
	}

	// Last resort: return the body content if it has enough text.
	body := doc.Find("body").First()
	if body.Length() > 0 {
		content, _ := body.Html()
		if hasEnoughText(content) {
			return content
		}
	}

	return html
}

// refineInnerContent drills into a selected content block and prefers a more
// specific nested content container when one exists.
func refineInnerContent(html string) string {
	current := html
	best := ""

	for depth := 0; depth < 3; depth++ {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(current))
		if err != nil {
			break
		}

		next := findBestContentCandidate(doc, innerSelectors)
		if next == "" {
			break
		}

		nextTrimmed := strings.TrimSpace(next)
		currentTrimmed := strings.TrimSpace(current)
		if len(nextTrimmed) < 150 || nextTrimmed == currentTrimmed {
			break
		}

		best = next
		current = next
	}

	return best
}

// findBestContentCandidate searches for the best content match among the
// provided selectors, returning the HTML content with the highest score.
func findBestContentCandidate(doc *goquery.Document, selectors []string) string {
	var bestScore float64
	var bestContent string

	for _, sel := range selectors {
		doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
			content, err := s.Html()
			if err != nil || len(content) < 80 {
				return
			}

			score := scoreAsContentCandidate(s, content)
			if score > bestScore {
				bestScore = score
				bestContent = content
			}
		})
	}

	return bestContent
}

// scoreAsContentCandidate calculates a content quality score based on text
// density, link density, and contextual boosts from class/id attributes.
func scoreAsContentCandidate(s *goquery.Selection, content string) float64 {
	text := strings.TrimSpace(s.Text())
	if len(text) < 50 {
		return 0
	}

	textLen := float64(len(text))
	htmlLen := float64(len(content))
	if htmlLen == 0 {
		return 0
	}

	// Text density: higher ratio of text to HTML markup indicates quality content.
	density := textLen / htmlLen

	// Link density: heavily linked content is often navigation, not main content.
	linkTextLen := float64(len(strings.TrimSpace(s.Find("a").Text())))
	linkDensity := 0.0
	if textLen > 0 {
		linkDensity = linkTextLen / textLen
	}

	score := textLen * density * density
	score *= (1.0 - minFloat64(linkDensity, 0.85))
	score *= calculateCandidateBoost(s)

	// Bonus for longer content — typically more substantive.
	if textLen > 500 {
		score *= 1.25
	}

	return score
}

// calculateCandidateBoost returns a multiplier based on class/id/role
// attributes that indicate content-rich sections.
func calculateCandidateBoost(s *goquery.Selection) float64 {
	boost := 1.0
	class, _ := s.Attr("class")
	id, _ := s.Attr("id")
	role, _ := s.Attr("role")
	haystack := strings.ToLower(class + " " + id + " " + role)

	// Boost for content-indicating keywords.
	for _, hint := range contentHints {
		if strings.Contains(haystack, hint) {
			boost += 0.25
		}
	}

	// Penalize for noise-indicating keywords.
	for _, noise := range noiseTags {
		if strings.Contains(haystack, noise) {
			boost -= 0.35
		}
	}

	// Extra boost for explicit ARIA content landmarks.
	if roleLower := strings.ToLower(role); roleLower == "main" || roleLower == "article" {
		boost += 0.4
	}

	if boost < 0.25 {
		return 0.25
	}
	return boost
}

// hasEnoughText returns true if the HTML content has sufficient text
// density (>10%) and length (>200 characters).
func hasEnoughText(html string) bool {
	return textDensity(html) > 0.1 && len(html) > 200
}

// textDensity calculates the ratio of text content to total HTML length.
func textDensity(html string) float64 {
	if len(html) == 0 {
		return 0
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return 0
	}
	text := doc.Text()
	return float64(len(strings.TrimSpace(text))) / float64(len(html))
}

// minFloat64 returns the smaller of two float64 values.
func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
