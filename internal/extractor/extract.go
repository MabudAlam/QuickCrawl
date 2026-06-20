// Package extractor provides content extraction from HTML into various formats.
package extractor

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/dom"
	"github.com/markusmobius/go-trafilatura"
)

const minUsableMarkdownLen = 300

// contentResult holds the extracted body HTML and optional title/excerpt.
type contentResult struct {
	BodyHTML string
	Title    string
	Excerpt  string
	DocHTML  string // full document with wrapper
}

// ─── Public API ───────────────────────────────────────────────────────────────

// Extract orchestrates the full content extraction pipeline, converting
// raw HTML to the requested output formats (Markdown, HTML, plain text, etc.).
func Extract(opts ExtractOptions) *types.ScrapeData {
	if len(opts.RawBytes) > 0 && opts.RenderedMode != nil && strings.EqualFold(*opts.RenderedMode, "pdf") {
		return extractPDF(opts)
	}

	meta := ExtractMetadata(opts.RawHTML)
	content := extractContent(opts)

	formatNeeded := createFormatSet(opts.Formats)
	markdown := convertToMarkdown(content.BodyHTML, meta, opts, formatNeeded)
	plainText := convertToPlainText(content.BodyHTML, formatNeeded)
	html := convertToHTML(content.DocHTML, meta, formatNeeded)
	links := extractLinks(opts.RawHTML, opts.SourceURL, formatNeeded)
	imageLinks := extractImageLinks(opts.RawHTML, opts.SourceURL, formatNeeded)

	return &types.ScrapeData{
		Markdown:   markdown,
		HTML:       html,
		RawHTML:    rawHTML(opts, formatNeeded),
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

// ─── Content Extraction Router ────────────────────────────────────────────────

func extractContent(opts ExtractOptions) contentResult {
	extractor := opts.ExtractorType
	if extractor == "" {
		extractor = ExtractorTrafilatura
	}

	switch extractor {
	case ExtractorTrafilatura:
		return extractWithTrafilatura(opts)
	case ExtractorReadability:
		return extractWithReadabilityAlgo(opts)
	case ExtractorLegacy:
		return extractWithLegacy(opts)
	default:
		return extractWithTrafilatura(opts)
	}
}

// ─── Trafilatura Extractor ─────────────────────────────────────────────────────

func extractWithTrafilatura(opts ExtractOptions) contentResult {
	tOpts := trafilatura.Options{

		EnableFallback: true,
		Config:         trafilatura.DefaultConfig(),
	}
	if opts.SourceURL != "" {
		if u, err := url.Parse(opts.SourceURL); err == nil {
			tOpts.OriginalURL = u
		}
	}

	result, err := trafilatura.Extract(strings.NewReader(opts.RawHTML), tOpts)
	if err == nil && result != nil && result.ContentNode != nil {
		bodyHTML := dom.OuterHTML(result.ContentNode)
		docHTML := dom.OuterHTML(trafilatura.CreateReadableDocument(result))
		return contentResult{BodyHTML: bodyHTML, DocHTML: docHTML}
	}

	return extractWithReadabilityAlgo(opts)
}

// ─── Readability Extractor ────────────────────────────────────────────────────

func extractWithReadabilityAlgo(opts ExtractOptions) contentResult {
	article := ExtractArticleWithMetadata(opts.RawHTML)
	bodyHTML := article.Content
	docHTML := FormatDocument(article.Title, article.Excerpt, article.Content)
	return contentResult{BodyHTML: bodyHTML, Title: article.Title, Excerpt: article.Excerpt, DocHTML: docHTML}
}

// ─── Legacy Extractor ─────────────────────────────────────────────────────────

func extractWithLegacy(opts ExtractOptions) contentResult {
	bodyHTML := preprocessRawHTML(opts.RawHTML, HTMLPreprocessOptionInterface{
		IncludeTags: opts.IncludeTags,
		ExcludeTags: opts.ExcludeTags,
		CSSSelector: opts.CSSSelector,
	})
	bodyHTML = ExtractMainContent(bodyHTML)
	bodyHTML = postprocessRawHTML(bodyHTML)
	return contentResult{BodyHTML: bodyHTML, DocHTML: bodyHTML}
}

// ─── Output Converters ────────────────────────────────────────────────────────

func convertToMarkdown(bodyHTML string, meta ExtractedMetadata, opts ExtractOptions, formatNeeded map[types.OutputFormat]bool) *string {
	if !formatNeeded[types.FormatMarkdown] && !formatNeeded[types.FormatJson] {
		return nil
	}

	candidates := buildMarkdownCandidates(bodyHTML, opts)
	best := selectBestMarkdownCandidate(candidates)
	if best == "" {
		return nil
	}

	best = prependArticleHeader(best, meta)
	best = appendDescriptionIfNeeded(best, meta)
	best = reflowInlineLists(best)
	return &best
}

func buildMarkdownCandidates(bodyHTML string, opts ExtractOptions) []string {
	candidates := []string{HTMLToMarkdown(bodyHTML)}
	if len(strings.TrimSpace(candidates[0])) < minUsableMarkdownLen {
		if fullClean := postprocessRawHTML(preprocessRawHTML(opts.RawHTML, HTMLPreprocessOptionInterface{BypassFilters: true})); fullClean != "" {
			candidates = append(candidates, HTMLToMarkdown(fullClean))
		}
		if structural := extractStructuralFallback(opts.RawHTML); structural != "" {
			candidates = append(candidates, structural)
		}
		if plain := HTMLToPlaintext(bodyHTML); strings.TrimSpace(plain) != "" {
			candidates = append(candidates, plain)
		}
	}
	return candidates
}

func selectBestMarkdownCandidate(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	best := candidates[0]
	bestLen := len(strings.TrimSpace(best))
	for _, c := range candidates[1:] {
		if l := len(strings.TrimSpace(c)); l > bestLen {
			best, bestLen = c, l
		}
	}
	return best
}

func prependArticleHeader(md string, meta ExtractedMetadata) string {
	header := articleHeaderMarkdown(meta)
	if header == "" {
		return md
	}
	firstLine := strings.SplitN(header, "\n", 2)[0]
	if firstLine != "" && !strings.HasPrefix(strings.TrimSpace(md), strings.TrimPrefix(firstLine, "# ")) {
		return header + "\n" + md
	}
	return md
}

func appendDescriptionIfNeeded(md string, meta ExtractedMetadata) string {
	if len(strings.TrimSpace(md)) >= 1500 {
		return md
	}
	desc := descriptionToAppend(meta)
	if desc == "" {
		return md
	}
	if !strings.Contains(strings.TrimSpace(md), desc[:min(120, len(desc))]) {
		if meta.Title == nil || !strings.EqualFold(desc, *meta.Title) {
			return md + "\n\n" + desc
		}
	}
	return md
}

func descriptionToAppend(meta ExtractedMetadata) string {
	if meta.Description != nil && len(*meta.Description) >= 80 {
		return *meta.Description
	}
	if meta.OGDescription != nil && len(*meta.OGDescription) >= 80 {
		return *meta.OGDescription
	}
	return ""
}

func convertToPlainText(bodyHTML string, formatNeeded map[types.OutputFormat]bool) *string {
	if !formatNeeded[types.FormatPlainText] {
		return nil
	}
	pt := HTMLToPlaintext(bodyHTML)
	return &pt
}

func rawHTML(opts ExtractOptions, formatNeeded map[types.OutputFormat]bool) *string {
	if !formatNeeded[types.FormatRawHtml] {
		return nil
	}
	raw := opts.RawHTML
	return &raw
}

func convertToHTML(docHTML string, meta ExtractedMetadata, formatNeeded map[types.OutputFormat]bool) *string {
	if !formatNeeded[types.FormatHtml] {
		return nil
	}
	htmlStr := injectMetadataHeader(docHTML, meta)
	return &htmlStr
}

func injectMetadataHeader(htmlStr string, meta ExtractedMetadata) string {
	if strings.Contains(htmlStr, `class="byline"`) || strings.Contains(htmlStr, `class="dateline"`) {
		return htmlStr
	}
	header := articleHeaderHTML(meta)
	if header == "" {
		return htmlStr
	}
	header = stripH1FromHeader(header)
	if header == "" {
		return htmlStr
	}
	if idx := strings.Index(htmlStr, "<h1>"); idx >= 0 {
		end := strings.Index(htmlStr[idx:], "</h1>") + len("</h1>")
		return htmlStr[:idx+end] + "\n" + header + htmlStr[idx+end:]
	}
	if bodyIdx := strings.Index(htmlStr, "<body>"); bodyIdx >= 0 {
		insertAt := bodyIdx + len("<body>")
		return htmlStr[:insertAt] + "\n" + header + htmlStr[insertAt:]
	}
	return htmlStr
}

func extractLinks(rawHTML, sourceURL string, formatNeeded map[types.OutputFormat]bool) []string {
	if !formatNeeded[types.FormatLinks] {
		return nil
	}
	return ExtractLinks(rawHTML, sourceURL)
}

func extractImageLinks(rawHTML, sourceURL string, formatNeeded map[types.OutputFormat]bool) []string {
	if !formatNeeded[types.FormatImageLinks] {
		return nil
	}
	return ExtractImageURLs(rawHTML, sourceURL)
}

// ─── Markdown Helpers ─────────────────────────────────────────────────────────

func reflowInlineLists(md string) string {
	md = regexp.MustCompile(`":\s*\n+\s*\[`).ReplaceAllString(md, "\": [")
	md = regexp.MustCompile(`"\),\s*\n+\s*\[`).ReplaceAllString(md, "\"), [")
	md = regexp.MustCompile(`",\s*\n+\s*([A-Z])`).ReplaceAllString(md, "\", $1")
	return md
}

// ─── Article Header Builders ──────────────────────────────────────────────────

func articleHeaderMarkdown(meta ExtractedMetadata) string {
	var b strings.Builder
	title := resolveTitle(meta)
	if title != "" {
		b.WriteString("# " + title + "\n")
	}
	if meta.Author != nil && *meta.Author != "" {
		b.WriteString("*By " + *meta.Author + "*\n")
	}
	if meta.PublishedTime != nil || meta.ModifiedTime != nil {
		b.WriteString("*")
		if meta.PublishedTime != nil && *meta.PublishedTime != "" {
			b.WriteString("Published " + formatDateForDisplay(*meta.PublishedTime))
		}
		if meta.ModifiedTime != nil && *meta.ModifiedTime != "" &&
			(meta.PublishedTime == nil || *meta.ModifiedTime != *meta.PublishedTime) {
			if meta.PublishedTime != nil {
				b.WriteString("; ")
			}
			b.WriteString("Updated " + formatDateForDisplay(*meta.ModifiedTime))
		}
		b.WriteString("*\n")
	}
	return b.String()
}

func articleHeaderHTML(meta ExtractedMetadata) string {
	var b strings.Builder
	title := escapeHTML(resolveTitle(meta))
	if title != "" {
		b.WriteString("    <h1>" + title + "</h1>\n")
	}
	if meta.Author != nil && *meta.Author != "" {
		b.WriteString("    <p class=\"byline\">By " + escapeHTML(*meta.Author) + "</p>\n")
	}
	if meta.PublishedTime != nil || meta.ModifiedTime != nil {
		b.WriteString("    <p class=\"dateline\">")
		if meta.PublishedTime != nil && *meta.PublishedTime != "" {
			b.WriteString("Published " + escapeHTML(formatDateForDisplay(*meta.PublishedTime)))
		}
		if meta.ModifiedTime != nil && *meta.ModifiedTime != "" &&
			(meta.PublishedTime == nil || *meta.ModifiedTime != *meta.PublishedTime) {
			if meta.PublishedTime != nil {
				b.WriteString("; ")
			}
			b.WriteString("Updated " + escapeHTML(formatDateForDisplay(*meta.ModifiedTime)))
		}
		b.WriteString("</p>\n")
	}
	return b.String()
}

func resolveTitle(meta ExtractedMetadata) string {
	if meta.OGTitle != nil && *meta.OGTitle != "" {
		title := *meta.OGTitle
		if idx := strings.IndexAny(title, "|\u2013\u2014-"); idx > 0 {
			title = strings.TrimSpace(title[:idx])
		}
		return title
	}
	if meta.Title != nil {
		return *meta.Title
	}
	return ""
}

// ─── Utility Functions ────────────────────────────────────────────────────────

func formatDateForDisplay(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.UTC().Format("02 Jan 2006, 15:04 MST")
}

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

func createFormatSet(formats []types.OutputFormat) map[types.OutputFormat]bool {
	m := make(map[types.OutputFormat]bool, len(formats))
	for _, f := range formats {
		m[f] = true
	}
	return m
}

func stringPtr(s string) *string {
	return &s
}

func containsFormat(formats []types.OutputFormat, target types.OutputFormat) bool {
	for _, f := range formats {
		if f == target {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Fallback Extractors ──────────────────────────────────────────────────────

func extractStructuralFallback(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	var results []string
	doc.Find("table").Each(func(_ int, s *goquery.Selection) {
		if s.Find("tr").Length() >= 2 {
			if chunk, _ := s.Html(); len(strings.TrimSpace(chunk)) >= 40 {
				if md := strings.TrimSpace(HTMLToMarkdown(chunk)); md != "" {
					results = append(results, md)
				}
			}
		}
	})
	doc.Find("ul, ol").Each(func(_ int, s *goquery.Selection) {
		if s.Find("li").Length() >= 5 {
			if chunk, _ := s.Html(); len(strings.TrimSpace(chunk)) >= 40 {
				if md := strings.TrimSpace(HTMLToMarkdown(chunk)); md != "" {
					results = append(results, md)
				}
			}
		}
	})
	return strings.Join(results, "\n\n")
}

func extractPDF(opts ExtractOptions) *types.ScrapeData {
	text := ExtractPDFText(opts.RawBytes)
	var markdown, plainText *string
	if containsFormat(opts.Formats, types.FormatMarkdown) || containsFormat(opts.Formats, types.FormatJson) {
		if strings.TrimSpace(text) != "" {
			markdown = &text
		}
	}
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
