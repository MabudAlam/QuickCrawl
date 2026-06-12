// Package extractor provides content extraction from HTML into various formats.
package extractor

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
)

// ─── Pipeline Constants ──────────────────────────────────────────────────────

// minUsableMarkdownLen is the minimum trimmed length a markdown candidate
// must reach before we stop trying additional (more expensive) fallback
// extraction strategies. Candidates below this threshold are considered
// insufficient and trigger the next fallback.
// ExtractMetadata extracts page metadata from HTML including title, description,
// author, published/updated timestamps, Open Graph tags, canonical URL, and language.
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

	if author, exists := doc.Find("meta[name=\"author\"]").First().Attr("content"); exists && author != "" {
		meta.Author = stringPtr(strings.TrimSpace(author))
	} else if author, exists := doc.Find("meta[property=\"article:author\"]").First().Attr("content"); exists && author != "" {
		meta.Author = stringPtr(strings.TrimSpace(author))
	} else if author, exists := doc.Find("meta[name=\"byl\"]").First().Attr("content"); exists && author != "" {
		meta.Author = stringPtr(strings.TrimSpace(author))
	}

	if pubTime, exists := doc.Find("meta[property=\"article:published_time\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	} else if pubTime, exists := doc.Find("meta[name=\"pubdate\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	} else if pubTime, exists := doc.Find("meta[name=\"date\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	} else if pubTime, exists := doc.Find("meta[itemprop=\"datePublished\"]").First().Attr("content"); exists && pubTime != "" {
		meta.PublishedTime = stringPtr(strings.TrimSpace(pubTime))
	}

	if updTime, exists := doc.Find("meta[property=\"article:modified_time\"]").First().Attr("content"); exists && updTime != "" {
		meta.ModifiedTime = stringPtr(strings.TrimSpace(updTime))
	} else if updTime, exists := doc.Find("meta[name=\"lastmod\"]").First().Attr("content"); exists && updTime != "" {
		meta.ModifiedTime = stringPtr(strings.TrimSpace(updTime))
	} else if updTime, exists := doc.Find("meta[itemprop=\"dateModified\"]").First().Attr("content"); exists && updTime != "" {
		meta.ModifiedTime = stringPtr(strings.TrimSpace(updTime))
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

// articleHeaderMarkdown builds the leading `# Title` / `*By Author*` /
// `*Published …; Updated …*` block prepended to markdown output. Lines
// that are empty (no author, no dates) are skipped so the result is
// tight. Returns an empty string if no header info is available.
func articleHeaderMarkdown(meta ExtractedMetadata) string {
	var b strings.Builder

	var title string
	if meta.OGTitle != nil && *meta.OGTitle != "" {
		title = *meta.OGTitle
	} else if meta.Title != nil {
		title = *meta.Title
	}
	if idx := strings.IndexAny(title, "|\u2013\u2014-"); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n")
	}

	if meta.Author != nil && *meta.Author != "" {
		b.WriteString("*By ")
		b.WriteString(*meta.Author)
		b.WriteString("*\n")
	}

	if meta.PublishedTime != nil || meta.ModifiedTime != nil {
		b.WriteString("*")
		if meta.PublishedTime != nil && *meta.PublishedTime != "" {
			b.WriteString("Published ")
			b.WriteString(formatDateForDisplay(*meta.PublishedTime))
		}
		if meta.ModifiedTime != nil && *meta.ModifiedTime != "" &&
			(meta.PublishedTime == nil || *meta.ModifiedTime != *meta.PublishedTime) {
			if meta.PublishedTime != nil {
				b.WriteString("; ")
			}
			b.WriteString("Updated ")
			b.WriteString(formatDateForDisplay(*meta.ModifiedTime))
		}
		b.WriteString("*\n")
	}

	return b.String()
}

// articleHeaderHTML builds an HTML block matching the markdown header:
//   <h1>Title</h1>
//   <p class="byline">By Author</p>
//   <p class="dateline">Published …; Updated …</p>
// Empty parts are omitted. Returns empty string if nothing to render.
func articleHeaderHTML(meta ExtractedMetadata) string {
	var b strings.Builder

	var title string
	if meta.OGTitle != nil && *meta.OGTitle != "" {
		title = *meta.OGTitle
	} else if meta.Title != nil {
		title = *meta.Title
	}
	if idx := strings.IndexAny(title, "|\u2013\u2014-"); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}

	if title != "" {
		b.WriteString("    <h1>")
		b.WriteString(escapeHTML(title))
		b.WriteString("</h1>\n")
	}

	if meta.Author != nil && *meta.Author != "" {
		b.WriteString("    <p class=\"byline\">By ")
		b.WriteString(escapeHTML(*meta.Author))
		b.WriteString("</p>\n")
	}

	if meta.PublishedTime != nil || meta.ModifiedTime != nil {
		b.WriteString("    <p class=\"dateline\">")
		if meta.PublishedTime != nil && *meta.PublishedTime != "" {
			b.WriteString("Published ")
			b.WriteString(escapeHTML(formatDateForDisplay(*meta.PublishedTime)))
		}
		if meta.ModifiedTime != nil && *meta.ModifiedTime != "" &&
			(meta.PublishedTime == nil || *meta.ModifiedTime != *meta.PublishedTime) {
			if meta.PublishedTime != nil {
				b.WriteString("; ")
			}
			b.WriteString("Updated ")
			b.WriteString(escapeHTML(formatDateForDisplay(*meta.ModifiedTime)))
		}
		b.WriteString("</p>\n")
	}

	return b.String()
}

// formatDateForDisplay converts an ISO 8601 timestamp into a human-readable
// form. The input is preserved verbatim if it can't be parsed.
func formatDateForDisplay(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.UTC().Format("02 Jan 2006, 15:04 MST")
}

// stripH1FromHeader removes the leading <h1>...</h1> line from a header
// fragment produced by articleHeaderHTML. The remaining byline and dateline
// are returned (with leading/trailing whitespace trimmed).
func stripH1FromHeader(header string) string {
	lines := strings.Split(header, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "<h1>") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
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

	flag := true

	contentHTML := ""
	bodyHTML := ""
	if flag == true {
		article := ExtractArticleWithMetadata(opts.RawHTML)
		bodyHTML = article.Content
		contentHTML = FormatDocument(article.Title, article.Excerpt, article.Content)
	} else {
		// ── Step 2: Preprocess HTML ───────────────────────────────────────────────
		// Strip the head, remove obvious noise, apply selectors, and isolate the
		// main article if requested. This keeps the downstream post-process stage
		// focused on structural cleanup only.
		bodyHTML = preprocessRawHTML(opts.RawHTML, HTMLPreprocessOptionInterface{
			IncludeTags: opts.IncludeTags,
			ExcludeTags: opts.ExcludeTags,
			CSSSelector: opts.CSSSelector,
		})

		// The pre processing removes most of the extras from the html content

		// bodyHTML = ExtractMainContent(bodyHTML)

		// ── Step 3: Post-process HTML ─────────────────────────────────────────────
		// Sanitizer + DOM cleanup + wrapper flattening + formatted document output.
		bodyHTML = postprocessRawHTML(bodyHTML)
		contentHTML = bodyHTML

		// Debug trace: log content sizes at each pipeline stage.
		if strings.Contains(opts.SourceURL, "github.com") {
			go debugTrace(opts.RawHTML)
		}
	}

	formatNeeded := createFormatSet(opts.Formats)

	// ── Step 4: Markdown conversion ───────────────────────────────────────────
	// Convert HTML to Markdown using html-to-markdown library.
	// Runs post-processing to strip markdown artifacts (empty anchors,
	// pilcrow ¶, section sign §, data-URI images, all images).
	var markdown *string
	if formatNeeded[types.FormatMarkdown] || formatNeeded[types.FormatJson] {
		md := HTMLToMarkdown(bodyHTML)
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
			fullClean := preprocessRawHTML(opts.RawHTML, HTMLPreprocessOptionInterface{BypassFilters: true})
			fullClean = postprocessRawHTML(fullClean)
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
				plainText := HTMLToPlaintext(bodyHTML)
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

		// Prepend the article header (title, byline, dates) so the markdown
		// output matches the structure of a published article. We only
		// prepend if the body doesn't already start with the same title text
		// (i.e. the readability pass kept it).
		if bestMD != "" {
			header := articleHeaderMarkdown(meta)
			if header != "" {
				headerFirstLine := strings.SplitN(header, "\n", 2)[0]
				if headerFirstLine == "" || !strings.HasPrefix(bestTrimmed, strings.TrimPrefix(headerFirstLine, "# ")) {
					bestMD = header + "\n" + bestMD
					bestTrimmed = header + "\n" + bestTrimmed
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
		pt := HTMLToPlaintext(bodyHTML)
		plainText = &pt
	}

	// ── Step 6: Raw HTML (unchanged) ──────────────────────────────────────────
	var rawHTML *string
	if formatNeeded[types.FormatRawHtml] {
		raw := opts.RawHTML
		rawHTML = &raw
	}

	// ── Step 7: Clean HTML output ─────────────────────────────────────────────
	var html *string
	if formatNeeded[types.FormatHtml] {
		// Inject byline + dateline into the HTML output if not already
		// present. We do this here (after FormatDocument) so it works for
		// both the flag==true (wrapped) and flag==false (raw) paths.
		htmlStr := contentHTML
		if !strings.Contains(htmlStr, `class="byline"`) && !strings.Contains(htmlStr, `class="dateline"`) {
			headerExtra := articleHeaderHTML(meta)
			if headerExtra != "" {
				// Drop the <h1> from the injected header so we don't
				// duplicate the title (FormatDocument already adds one).
				headerExtra = stripH1FromHeader(headerExtra)
				if headerExtra != "" {
					if idx := strings.Index(htmlStr, "<h1>"); idx >= 0 {
						end := strings.Index(htmlStr[idx:], "</h1>") + len("</h1>")
						htmlStr = htmlStr[:idx+end] + "\n" + headerExtra + htmlStr[idx+end:]
					} else {
						// No existing <h1>; insert at the top of <body>.
						if bodyIdx := strings.Index(htmlStr, "<body>"); bodyIdx >= 0 {
							insertAt := bodyIdx + len("<body>")
							htmlStr = htmlStr[:insertAt] + "\n" + headerExtra + htmlStr[insertAt:]
						}
					}
				}
			}
		}
		html = &htmlStr
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
	cleaned := cleanNoiseMethid(afterHead)

	// Step 3: applyNoisePatterns alone
	noised := applyNoisePattern(cleaned)

	// Step 4: postprocessHTML (full)
	post := postprocessRawHTML(noised)

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
