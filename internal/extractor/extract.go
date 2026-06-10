// Package extractor provides content extraction from HTML into various formats.
package extractor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

// ─── Pipeline Constants ──────────────────────────────────────────────────────

// minUsableMarkdownLen is the minimum trimmed length a markdown candidate
// must reach before we stop trying additional (more expensive) fallback
// extraction strategies. Candidates below this threshold are considered
// insufficient and trigger the next fallback.
// ExtractMetadata extracts page metadata from HTML including title, description,
// Open Graph tags, canonical URL, and language.
func ExtractMetadata(html string) ExtractedMetadata {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ExtractedMetadata{}
	}

	var meta ExtractedMetadata

	if title := doc.Find("title").First().Text(); title != "" {
		meta.Title = stringPtr(strings.TrimSpace(title))
	}

	if desc, exists := doc.Find("meta[name=\"description\"]").First().Attr("content"); exists && desc != "" {
		meta.Description = stringPtr(strings.TrimSpace(desc))
	}

	if ogTitle, exists := doc.Find("meta[property=\"og:title\"]").First().Attr("content"); exists && ogTitle != "" {
		meta.OGTitle = stringPtr(strings.TrimSpace(ogTitle))
	}
	if ogDesc, exists := doc.Find("meta[property=\"og:description\"]").First().Attr("content"); exists && ogDesc != "" {
		meta.OGDescription = stringPtr(strings.TrimSpace(ogDesc))
	}
	if ogImg, exists := doc.Find("meta[property=\"og:image\"]").First().Attr("content"); exists && ogImg != "" {
		meta.OGImage = stringPtr(strings.TrimSpace(ogImg))
	}

	if canonical, exists := doc.Find("link[rel=\"canonical\"]").First().Attr("href"); exists && canonical != "" {
		meta.CanonicalURL = stringPtr(strings.TrimSpace(canonical))
	}

	if lang, exists := doc.Find("html").First().Attr("lang"); exists && lang != "" {
		meta.Language = stringPtr(strings.TrimSpace(lang))
	}

	return meta
}

const minUsableMarkdownLen = 300

// reflowInlineLists cleans up inline lists that were incorrectly split across
// lines by markdown converters.
func reflowInlineLists(md string) string {
	re1 := regexp.MustCompile(`":\s*\n+\s*\[`)
	md = re1.ReplaceAllString(md, "\": [")

	re2 := regexp.MustCompile(`"\),\s*\n+\s*\[`)
	md = re2.ReplaceAllString(md, "\"), [")

	re3 := regexp.MustCompile(`",\s*\n+\s*([A-Z])`)
	md = re3.ReplaceAllString(md, "\", $1")

	return md
}

// stringPtr returns a pointer to the given string.
func stringPtr(s string) *string {
	return &s
}

// containsFormat returns true if the formats slice contains the target format.
func containsFormat(formats []types.OutputFormat, target types.OutputFormat) bool {
	for _, f := range formats {
		if f == target {
			return true
		}
	}
	return false
}

// createFormatSet converts a slice of OutputFormat values into a map
// for efficient O(1) lookup.
func createFormatSet(formats []types.OutputFormat) map[types.OutputFormat]bool {
	m := make(map[types.OutputFormat]bool, len(formats))
	for _, f := range formats {
		m[f] = true
	}
	return m
}

// ─── Orchestrator ─────────────────────────────────────────────────────────────

// Extract orchestrates the full content extraction pipeline, converting
// raw HTML to the requested output formats (Markdown, HTML, plain text, etc.).
//
// Pipeline order:
//  1. Extract metadata from raw HTML before any mutation.
//  2. Preprocess HTML: strip head, remove noise, apply selectors, isolate main content.
//  3. Postprocess HTML: sanitize, unwrap wrappers, dedupe nodes, and format document output.
//  4. Convert the cleaned document into the requested output formats.
func Extract(opts ExtractOptions) *types.ScrapeData {
	// ── Step 0: PDF extraction (special case) ─────────────────────────────────
	// PDF extraction bypasses the entire HTML pipeline since there is no HTML to parse.
	// We handle it separately to short-circuit directly.
	if len(opts.RawBytes) > 0 && opts.RenderedMode != nil && strings.EqualFold(*opts.RenderedMode, "pdf") {
		return extractPDF(opts)
	}

	// ── Step 1: Metadata (before cleaning) ────────────────────────────────────
	// Extract metadata before any cleaning so we capture values even from noisy
	// sections (e.g., title in nav, description in sidebar).
	meta := ExtractMetadata(opts.RawHTML)

	// ── Step 2: Preprocess HTML ───────────────────────────────────────────────
	// Strip the head, remove obvious noise, apply selectors, and isolate the
	// main article if requested. This keeps the downstream post-process stage
	// focused on structural cleanup only.
	// contentHTML := preprocessHTML(opts.RawHTML, HTMLPreprocessOptions{
	// 	IncludeTags:      opts.IncludeTags,
	// 	ExcludeTags:      opts.ExcludeTags,
	// 	CSSSelector:      opts.CSSSelector,
	// 	SkipNoisePatterns: true,
	// })

	// The pre processing removes most of the extras from the html content

	contentHTML := ExtractMainContent(opts.RawHTML)

	// Apply noise patterns AFTER content selection to remove sidebar noise.
	// contentHTML = applyNoisePatterns(contentHTML)

	// ── Step 3: Post-process HTML ─────────────────────────────────────────────
	// Sanitizer + DOM cleanup + wrapper flattening + formatted document output.
	// contentHTML = postprocessHTML(contentHTML)

	// Debug trace: log content sizes at each pipeline stage.
	if strings.Contains(opts.SourceURL, "github.com") {
		go debugTrace(opts.RawHTML)
	}

	formatNeeded := createFormatSet(opts.Formats)

	// ── Step 4: Markdown conversion ───────────────────────────────────────────
	// Convert HTML to Markdown using html-to-markdown library.
	// Runs post-processing to strip markdown artifacts (empty anchors,
	// pilcrow ¶, section sign §, data-URI images, all images).
	var markdown *string
	if formatNeeded[types.FormatMarkdown] || formatNeeded[types.FormatJson] {
		md := HTMLToMarkdown(contentHTML)
		mdTrimmed := strings.TrimSpace(md)

		// Markdown candidate generation: only try expensive fallback strategies
		// when the primary result is below the usability threshold.
		type candidate struct {
			name    string
			content string
		}
		candidates := []candidate{{name: "primary", content: md}}
		bestLen := len(mdTrimmed)

		if bestLen < minUsableMarkdownLen {
			// Fallback 1: full clean + convert from raw HTML (bypasses include/exclude).
			fullClean := preprocessHTML(opts.RawHTML, HTMLPreprocessOptions{BypassFilters: true})
			fullClean = postprocessHTML(fullClean)
			fullMD := HTMLToMarkdown(fullClean)
			if trimmed := strings.TrimSpace(fullMD); trimmed != "" {
				candidates = append(candidates, candidate{name: "fullClean", content: fullMD})
				if l := len(trimmed); l > bestLen {
					bestLen = l
				}
			}

			// Fallback 2: structural fallback — extract tables (2+ rows) and lists (5+ items).
			if bestLen < minUsableMarkdownLen {
				structuralMD := extractStructuralFallback(opts.RawHTML)
				if trimmed := strings.TrimSpace(structuralMD); trimmed != "" {
					candidates = append(candidates, candidate{name: "structural", content: structuralMD})
					if l := len(trimmed); l > bestLen {
						bestLen = l
					}
				}
			}

			// Fallback 3: plaintext as last resort.
			if bestLen < minUsableMarkdownLen {
				plainText := HTMLToPlaintext(contentHTML)
				if strings.TrimSpace(plainText) != "" {
					candidates = append(candidates, candidate{name: "plaintext", content: plainText})
				}
			}
		}

		// Pick the best candidate by character count.
		chosenIdx := 0
		if len(candidates) > 1 {
			maxLen := len(strings.TrimSpace(candidates[0].content))
			for i := 1; i < len(candidates); i++ {
				cLen := len(strings.TrimSpace(candidates[i].content))
				if cLen > maxLen {
					maxLen = cLen
					chosenIdx = i
				}
			}
		}

		bestMD := candidates[chosenIdx].content
		bestTrimmed := strings.TrimSpace(bestMD)

		// Prepend title if not already present.
		if bestMD != "" && meta.Title != nil && !strings.Contains(bestTrimmed, *meta.Title) {
			if ogTitle := meta.OGTitle; ogTitle != nil && !strings.Contains(bestTrimmed, *ogTitle) {
				titleToPrepend := *ogTitle
				if strings.Contains(titleToPrepend, "|") {
					titleToPrepend = strings.Split(titleToPrepend, "|")[0]
				} else if idx := strings.Index(titleToPrepend, " - "); idx != -1 {
					titleToPrepend = strings.TrimSpace(titleToPrepend[:idx])
				} else if idx := strings.Index(titleToPrepend, " – "); idx != -1 {
					titleToPrepend = strings.TrimSpace(titleToPrepend[:idx])
				} else if idx := strings.Index(titleToPrepend, " — "); idx != -1 {
					titleToPrepend = strings.TrimSpace(titleToPrepend[:idx])
				}
				if titleToPrepend != "" {
					bestMD = "# " + titleToPrepend + "\n\n" + bestMD
					bestTrimmed = "# " + titleToPrepend + "\n\n" + bestTrimmed
				}
			}
		}

		// Append description if markdown is short and description is substantial.
		if len(bestTrimmed) < 1500 {
			descToAppend := ""
			if meta.Description != nil && len(*meta.Description) >= 80 {
				descToAppend = *meta.Description
			} else if meta.OGDescription != nil && len(*meta.OGDescription) >= 80 {
				descToAppend = *meta.OGDescription
			}
			if descToAppend != "" {
				prefix := descToAppend
				if len(prefix) > 120 {
					prefix = prefix[:120]
				}
				if !strings.Contains(bestTrimmed, prefix) {
					if meta.Title != nil && !strings.EqualFold(descToAppend, *meta.Title) {
						bestMD = bestMD + "\n\n" + descToAppend
					}
				}
			}
		}

		// Final cleanup: reflow inline lists and assign.
		if bestTrimmed != "" {
			bestMD = reflowInlineLists(bestMD)
			markdown = &bestMD
		} else if mdTrimmed != "" {
			markdown = &md
		} else if md != "" {
			markdown = &md
		} else if contentHTML != "" {
			markdown = &contentHTML
		} else if opts.RawHTML != "" && len(strings.TrimSpace(opts.RawHTML)) > 10 {
			rawCopy := opts.RawHTML
			markdown = &rawCopy
		}
	}

	// ── Step 5: Plain text ─────────────────────────────────────────────────────
	var plainText *string
	if formatNeeded[types.FormatPlainText] {
		pt := HTMLToPlaintext(contentHTML)
		plainText = &pt
	}

	// ── Step 6: Raw HTML (unchanged) ──────────────────────────────────────────
	var rawHTML *string
	if formatNeeded[types.FormatRawHtml] {
		raw := opts.RawHTML
		rawHTML = &raw
	}

	// ── Step 7: Clean HTML output with title and excerpt ─────────────────────
	var html *string
	if formatNeeded[types.FormatHtml] {
		extracted := ExtractHTML(opts.RawHTML)
		html = &extracted.Content
	}

	// ── Step 8: Links ─────────────────────────────────────────────────────────
	var links []string
	if formatNeeded[types.FormatLinks] {
		links = ExtractLinks(opts.RawHTML, opts.SourceURL)
	}

	// ── Step 9: Image URLs ─────────────────────────────────────────────────────
	var imageLinks []string
	if formatNeeded[types.FormatImageLinks] {
		imageLinks = ExtractImageURLs(opts.RawHTML, opts.SourceURL)
	}

	// ── Step 10: Assemble result ───────────────────────────────────────────────
	return &types.ScrapeData{
		Markdown:   markdown,
		HTML:       html,
		RawHTML:    rawHTML,
		PlainText:  plainText,
		Links:      links,
		ImageLinks: imageLinks,
		JSON:       nil,
		Metadata: types.PageMetadata{
			Title:         meta.Title,
			Description:   meta.Description,
			OGTitle:       meta.OGTitle,
			OGDescription: meta.OGDescription,
			OGImage:       meta.OGImage,
			CanonicalURL:  meta.CanonicalURL,
			SourceURL:     opts.SourceURL,
			Language:      meta.Language,
			StatusCode:    uint16(opts.StatusCode),
			RenderedMode:  opts.RenderedMode,
		},
	}
}

// extractStructuralFallback extracts content from tables with ≥2 rows and
// lists with ≥5 items. This is a fallback for pages where readability scoring
// fails to find the main content but the data is embedded in structured elements
// (e.g., salary tables, job listings).
func extractStructuralFallback(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	var results []string

	// Extract tables with at least 2 rows.
	doc.Find("table").Each(func(_ int, s *goquery.Selection) {
		rows := s.Find("tr")
		if rows.Length() >= 2 {
			htmlChunk, _ := s.Html()
			if len(strings.TrimSpace(htmlChunk)) >= 40 {
				md := HTMLToMarkdown(htmlChunk)
				if strings.TrimSpace(md) != "" {
					results = append(results, md)
				}
			}
		}
	})

	// Extract lists with at least 5 items.
	doc.Find("ul, ol").Each(func(_ int, s *goquery.Selection) {
		items := s.Find("li")
		if items.Length() >= 5 {
			htmlChunk, _ := s.Html()
			if len(strings.TrimSpace(htmlChunk)) >= 40 {
				md := HTMLToMarkdown(htmlChunk)
				if strings.TrimSpace(md) != "" {
					results = append(results, md)
				}
			}
		}
	})

	if len(results) == 0 {
		return ""
	}
	return strings.Join(results, "\n\n")
}

// extractPDF handles PDF extraction when raw bytes are provided with a PDF
// renderedMethod indicator.
func extractPDF(opts ExtractOptions) *types.ScrapeData {
	text := ExtractPDFText(opts.RawBytes)

	var markdown *string
	if containsFormat(opts.Formats, types.FormatMarkdown) || containsFormat(opts.Formats, types.FormatJson) {
		if strings.TrimSpace(text) != "" {
			markdown = &text
		}
	}

	var plainText *string
	if containsFormat(opts.Formats, types.FormatPlainText) {
		if strings.TrimSpace(text) != "" {
			plainText = &text
		}
	}

	var rawHTML *string
	if containsFormat(opts.Formats, types.FormatRawHtml) {
		raw := opts.RawHTML
		rawHTML = &raw
	}

	return &types.ScrapeData{
		Markdown:  markdown,
		HTML:      nil,
		RawHTML:   rawHTML,
		PlainText: plainText,
		Links:     nil,
		JSON:      nil,
		Warning:   nil,
		Metadata: types.PageMetadata{
			SourceURL:    opts.SourceURL,
			StatusCode:   uint16(opts.StatusCode),
			RenderedMode: opts.RenderedMode,
		},
	}
}

// debugTrace logs content sizes at each pipeline stage.
func debugTrace(rawHTML string) {
	// Step 1: strip head
	afterHead := func(h string) string {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(h))
		if doc != nil {
			doc.Find("head").Remove()
			out, _ := doc.Html()
			return out
		}
		return h
	}(rawHTML)

	// Step 2: cleanNoise
	cleaned := cleanNoise(afterHead)

	// Step 3: applyNoisePatterns alone
	noised := applyNoisePatterns(cleaned)

	// Step 4: postprocessHTML (full)
	post := postprocessHTML(noised)

	// Step 5: markdown
	md := HTMLToMarkdown(post)

	fmt.Printf("trace: raw=%d stripHead=%d cleanNoise=%d noisePat=%d post=%d md=%d\n",
		len(rawHTML), len(afterHead), len(cleaned), len(noised), len(post), len(md))
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
