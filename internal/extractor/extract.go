package extractor

import (
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// ─── HTML Cleaner ──────────────────────────────────────────────────────────────

var (
	scriptRe   = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	styleRe    = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
	noScriptRe = regexp.MustCompile(`(?i)<noscript[^>]*>.*?</noscript>`)
	iframeRe   = regexp.MustCompile(`(?i)<iframe[^>]*>.*?</iframe>`)
	svgRe      = regexp.MustCompile(`(?i)<svg[^>]*>.*?</svg>`)
	dataImgRe  = regexp.MustCompile(`(?i)<img[^>]*src=["']data:[^"']*["'][^>]*>`)
)

var noisePatterns = []string{
	"sidebar", "table-of-contents", "tableofcontents", "infobox", "navbox",
	"nav-box", "navigation", "breadcrumb", "cookie", "consent", "banner",
	"disqus", "advert", "popup", "modal", "newsletter", "subscribe",
	"printfooter", "catlinks", "mw-panel", "mw-navigation", "sitesub",
	"jump-to-nav", "mw-editsection", "reflist", "mw-references",
	"authority-control", "mw-indicators", "sistersitebox", "mbox", "ambox",
	"ombox", "hatnote", "shortdescription", "sphinxsidebar", "sphinxfooter",
	"copyright", "dropdown", "city-selector", "location-selector",
}

var noiseExactTokens = []string{
	"toc", "share", "social", "related", "recommended", "comment", "footer",
}

var noisePrefixes = []string{"ad-", "ads-"}

var structuralElements = map[string]bool{
	"html": true, "head": true, "body": true, "main": true,
}

// CleanHTML removes unwanted HTML elements and optionally filters to main content.
// It strips scripts, styles, noscript, iframes, SVGs, and data URI images.
// When onlyMainContent is true, it also removes nav, footer, header, aside, menu,
// and select elements, plus applies noise pattern filtering.
// All structural removals are done through goquery to avoid partial tag issues.
func CleanHTML(html string, onlyMainContent bool, includeTags, excludeTags []string) string {
	result := html

	result = scriptRe.ReplaceAllString(result, "")
	result = styleRe.ReplaceAllString(result, "")
	result = noScriptRe.ReplaceAllString(result, "")
	result = iframeRe.ReplaceAllString(result, "")
	result = svgRe.ReplaceAllString(result, "")
	result = dataImgRe.ReplaceAllString(result, "")

	if onlyMainContent {
		result = applyNoisePatterns(result)
	}

	if len(includeTags) > 0 {
		result = filterBySelectors(result, includeTags)
	}

	if len(excludeTags) > 0 {
		result = removeElementsBySelectors(result, excludeTags)
	}

	return result
}

// applyNoisePatterns removes elements matching common noise patterns like
// sidebars, navigation, footers, and other non-content elements.
func applyNoisePatterns(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		tag := goquery.NodeName(s)
		if structuralElements[tag] {
			return
		}

		class, _ := s.Attr("class")
		id, _ := s.Attr("id")
		role, _ := s.Attr("role")

		combined := strings.ToLower(class + " " + id)

		isNoise := false
		for _, pattern := range noisePatterns {
			if strings.Contains(combined, pattern) {
				isNoise = true
				break
			}
		}

		if !isNoise {
			tokens := strings.Fields(class + " " + id)
			for _, tok := range tokens {
				for _, exact := range noiseExactTokens {
					if tok == exact {
						isNoise = true
						break
					}
				}
				if isNoise {
					break
				}
				for _, pre := range noisePrefixes {
					if strings.HasPrefix(tok, pre) {
						isNoise = true
						break
					}
				}
				if isNoise {
					break
				}
			}
		}

		if !isNoise && role != "" {
			roleLower := strings.ToLower(role)
			if roleLower == "contentinfo" || roleLower == "navigation" ||
				roleLower == "banner" || roleLower == "complementary" {
				isNoise = true
			}
		}

		if isNoise {
			s.Remove()
		}
	})

	output, _ := doc.Html()
	return output
}

// filterBySelectors extracts and concatenates text content from elements
// matching the provided CSS selectors.
func filterBySelectors(html string, selectors []string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	var parts []string
	for _, sel := range selectors {
		doc.Find(sel).Each(func(i int, s *goquery.Selection) {
			parts = append(parts, s.Text())
		})
	}

	if len(parts) == 0 {
		return html
	}
	return strings.Join(parts, "\n")
}

// removeElementsBySelectors removes all elements matching the provided
// CSS selectors from the HTML document.
func removeElementsBySelectors(html string, selectors []string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	for _, sel := range selectors {
		doc.Find(sel).Each(func(i int, s *goquery.Selection) {
			s.Remove()
		})
	}

	output, _ := doc.Html()
	return output
}

// ─── Readability ────────────────────────────────────────────────────────────────

var prioritySelectors = []string{"article", "main", "[role=\"main\"]"}

var scoredSelectors = []string{
	".post-content", ".article-body", ".entry-content", ".article-content",
	".post-body", ".story-body", ".content-body", "#main-content", "#article",
	"#content", ".content", ".main", "[itemprop=\"articleBody\"]",
	"[itemprop=\"text\"]", ".main-page-content", ".js-post-body", ".s-prose",
	"#question", ".page-content", "#page-content", "[role=\"article\"]",
	".mw-parser-output", "#mw-content-text", "#bodyContent", ".mw-body-content",
}

var innerSelectors = []string{
	".main-page-content", ".article-content", ".post-content", ".entry-content",
	".content-body", ".article-body", "[itemprop=\"articleBody\"]", "[itemprop=\"text\"]",
	".mw-parser-output", "#mw-content-text", "#content", ".content", "article",
}

var contentHints = []string{
	"article", "content", "main", "body", "post", "story", "entry",
	"markdown", "articlebody", "question", "answer", "mw-parser-output",
	"page-content", "text", "prose", "readme", "documentation",
}

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

// ExtractMainContent uses readability-style scoring to find the main content
// area of a page, trying priority selectors first, then scored selectors,
// then falling back to common element types.
func ExtractMainContent(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

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
		return bestContent
	}

	body := doc.Find("body").First()
	if body.Length() > 0 {
		content, _ := body.Html()
		if hasEnoughText(content) {
			return content
		}
	}

	return html
}

// findBestContentCandidate searches for the best content match among the
// provided selectors, returning the HTML content with the highest score.
func findBestContentCandidate(doc *goquery.Document, selectors []string) string {
	var bestScore float64
	var bestContent string

	for _, sel := range selectors {
		doc.Find(sel).Each(func(i int, s *goquery.Selection) {
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

	density := textLen / htmlLen
	linkTextLen := float64(len(strings.TrimSpace(s.Find("a").Text())))
	linkDensity := 0.0
	if textLen > 0 {
		linkDensity = linkTextLen / textLen
	}

	score := textLen * density * density
	score *= (1.0 - minFloat64(linkDensity, 0.85))
	score *= calculateCandidateBoost(s)

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

	for _, hint := range contentHints {
		if strings.Contains(haystack, hint) {
			boost += 0.25
		}
	}

	for _, noise := range noisePatterns {
		if strings.Contains(haystack, noise) {
			boost -= 0.35
		}
	}

	if roleLower := strings.ToLower(role); roleLower == "main" || roleLower == "article" {
		boost += 0.4
	}

	if boost < 0.25 {
		return 0.25
	}
	return boost
}

// ExtractLinks extracts all HTTP/HTTPS links from the HTML document,
// resolving them relative to the baseURL and deduplicating results.
func ExtractLinks(html, baseURL string) []string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return extractLinksUsingRegex(html, baseURL)
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return extractLinksUsingRegex(html, baseURL)
	}

	var links []string
	seen := make(map[string]bool)

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		if strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "mailto:") ||
			strings.HasPrefix(href, "data:") ||
			strings.HasPrefix(href, "tel:") ||
			strings.HasPrefix(href, "blob:") ||
			strings.HasPrefix(href, "#") {
			return
		}

		absoluteURL, err := base.Parse(href)
		if err != nil {
			return
		}

		absStr := absoluteURL.String()
		if seen[absStr] {
			return
		}
		seen[absStr] = true
		links = append(links, absStr)
	})

	return links
}

// extractLinksUsingRegex extracts links using regex fallback when HTML
// parsing fails. It resolves URLs relative to baseURL.
func extractLinksUsingRegex(html, baseURL string) []string {
	var links []string
	seen := make(map[string]bool)

	re := regexp.MustCompile(`<a[^>]+href=["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(html, -1)

	base, _ := url.Parse(baseURL)
	if base == nil {
		return links
	}

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		href := strings.TrimSpace(m[1])
		if href == "" || strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "mailto:") ||
			strings.HasPrefix(href, "data:") ||
			strings.HasPrefix(href, "tel:") ||
			strings.HasPrefix(href, "blob:") ||
			strings.HasPrefix(href, "#") {
			continue
		}

		absoluteURL, err := base.Parse(href)
		if err != nil {
			continue
		}

		absStr := absoluteURL.String()
		if seen[absStr] {
			continue
		}
		seen[absStr] = true
		links = append(links, absStr)
	}

	return links
}

// hasEnoughText returns true if the HTML content has sufficient text
// density (>10%) and length (>200 characters).
func hasEnoughText(html string) bool {
	return textDensity(html) > 0.1 && len(html) > 200
}

// minFloat64 returns the smaller of two float64 values.
func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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

func stringPtr(s string) *string {
	return &s
}

// Extract orchestrates the full content extraction pipeline, converting
// raw HTML to the requested output formats (Markdown, HTML, plain text, etc.)
// with optional chunking and filtering.
func Extract(opts ExtractOptions) *types.ScrapeData {
	if len(opts.RawBytes) > 0 && opts.RenderedMode != nil && strings.EqualFold(*opts.RenderedMode, "pdf") {
		return extractPDF(opts)
	}

	meta := ExtractMetadata(opts.RawHTML)

	cleaned := CleanHTML(opts.RawHTML, opts.OnlyMainContent, opts.IncludeTags, opts.ExcludeTags)

	formatNeeded := createFormatSet(opts.Formats)

	selectedHTML := ""
	if opts.CSSSelector != nil {
		selectedHTML = applyCSSSelector(cleaned, *opts.CSSSelector)
	} else if opts.XPath != nil {
		selectedHTML = applyXPath(cleaned, *opts.XPath)
	}

	contentHTML := cleaned
	if selectedHTML != "" {
		contentHTML = selectedHTML
	}

	if opts.OnlyMainContent && opts.CSSSelector == nil && opts.XPath == nil {
		mainContent := ExtractMainContent(contentHTML)
		if mainContent != "" {
			contentHTML = mainContent
		}
	}

	var markdown *string
	if formatNeeded[types.FormatMarkdown] || formatNeeded[types.FormatJson] {
		md := HTMLToMarkdown(contentHTML)
		mdTrimmed := strings.TrimSpace(md)
		suspiciouslyShort := opts.CSSSelector == nil && opts.XPath == nil && len(mdTrimmed) < 100 && len(opts.RawHTML) > 5000

		if suspiciouslyShort {
			var fallbackMD string
			if opts.OnlyMainContent {
				fromCleaned := HTMLToMarkdown(cleaned)
				basicCleaned := CleanHTML(opts.RawHTML, false, opts.IncludeTags, opts.ExcludeTags)
				basicMD := HTMLToMarkdown(basicCleaned)

				if len(strings.TrimSpace(fromCleaned)) >= len(strings.TrimSpace(basicMD)) {
					fallbackMD = fromCleaned
				} else {
					fallbackMD = basicMD
				}
			} else {
				fallbackMD = HTMLToMarkdown(opts.RawHTML)
			}

			if len(strings.TrimSpace(fallbackMD)) < 100 && len(opts.RawHTML) > 5000 {
				plain := HTMLToPlaintext(contentHTML)
				if strings.TrimSpace(plain) == "" {
					plain = HTMLToPlaintext(cleaned)
				}
				if strings.TrimSpace(plain) == "" {
					plain = HTMLToPlaintext(opts.RawHTML)
				}
				if strings.TrimSpace(plain) != "" {
					markdown = &plain
				} else if strings.TrimSpace(fallbackMD) != "" {
					markdown = &fallbackMD
				}
			} else if strings.TrimSpace(fallbackMD) != "" {
				markdown = &fallbackMD
			} else if mdTrimmed != "" {
				markdown = &md
			}
		} else if mdTrimmed != "" {
			markdown = &md
		} else if md != "" {
			markdown = &md
		} else if contentHTML != "" {
			markdown = &contentHTML
		} else if cleaned != "" && len(strings.TrimSpace(cleaned)) > 10 {
			markdown = &cleaned
		} else if opts.RawHTML != "" && len(strings.TrimSpace(opts.RawHTML)) > 10 {
			rawCopy := opts.RawHTML
			markdown = &rawCopy
		}
	}

	var plainText *string
	if formatNeeded[types.FormatPlainText] {
		pt := HTMLToPlaintext(contentHTML)
		plainText = &pt
	}

	var rawHTML *string
	if formatNeeded[types.FormatRawHtml] {
		raw := opts.RawHTML
		rawHTML = &raw
	}

	var html *string
	if formatNeeded[types.FormatHtml] {
		html = &contentHTML
	}

	var links []string
	if formatNeeded[types.FormatLinks] {
		links = ExtractLinks(opts.RawHTML, opts.SourceURL)
	}

	var chunks []types.ChunkResult
	if opts.ChunkStrategy != nil && markdown != nil && len(strings.TrimSpace(*markdown)) > 0 {
		rawChunks := ChunkText(*markdown, opts.ChunkStrategy)

		if opts.Query != nil && opts.FilterMode != nil && len(strings.TrimSpace(*opts.Query)) > 0 && len(rawChunks) > 0 {
			topK := 5
			if opts.TopK != nil {
				topK = *opts.TopK
			}
			filtered := FilterChunksScored(rawChunks, *opts.Query, opts.FilterMode, topK)
			chunks = filtered
		} else {
			for i, chunk := range rawChunks {
				var score *float64
				chunks = append(chunks, types.ChunkResult{
					Content: chunk,
					Score:   score,
					Index:   i,
				})
			}
			if opts.TopK != nil && *opts.TopK < len(chunks) {
				chunks = chunks[:*opts.TopK]
			}
		}
	}

	var orphanWarning *string
	if opts.ChunkStrategy == nil && (opts.Query != nil || opts.FilterMode != nil) {
		warning := "'query' and 'filterMode' require 'chunkStrategy' to be set. These parameters were ignored."
		orphanWarning = &warning
	}

	return &types.ScrapeData{
		Markdown:  markdown,
		HTML:      html,
		RawHTML:   rawHTML,
		PlainText: plainText,
		Links:     links,
		JSON:      nil,
		Chunks:    chunks,
		Warning:   orphanWarning,
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
			TimeTaken:     opts.TimeTaken,
		},
	}
}

func containsFormat(formats []types.OutputFormat, target types.OutputFormat) bool {
	for _, f := range formats {
		if f == target {
			return true
		}
	}
	return false
}

// createFormatSet converts a slice of OutputFormat values into a map
// for efficient lookup.
func createFormatSet(formats []types.OutputFormat) map[types.OutputFormat]bool {
	m := make(map[types.OutputFormat]bool, len(formats))
	for _, f := range formats {
		m[f] = true
	}
	return m
}

// applyCSSSelector extracts HTML content matching a CSS selector.
func applyCSSSelector(html, selector string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	el := doc.Find(selector).First()
	if el.Length() > 0 {
		content, _ := el.Html()
		return content
	}
	return ""
}

// applyXPath applies an XPath expression to extract content, converting
// the XPath to a CSS selector internally where possible.
func applyXPath(html, xpath string) string {
	xpath = strings.TrimSpace(xpath)
	if xpath == "" {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	css, filter := convertXPathToCSS(xpath)
	if css == "" {
		return ""
	}

	if strings.HasPrefix(css, "unsupported:") {
		log.Printf("xpath: unsupported expression type: %s", css)
		return ""
	}

	var parts []string
	doc.Find(css).Each(func(i int, s *goquery.Selection) {
		if filter != nil && !filter(s) {
			return
		}
		if inner, err := s.Html(); err == nil && inner != "" {
			parts = append(parts, inner)
		} else {
			parts = append(parts, s.Text())
		}
	})

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// convertXPathToCSS converts a limited subset of XPath expressions to CSS selectors.
// Supports: @id=, @class=, @attr=, and contains(@class, ...) and contains(@id, ...).
// Returns "unsupported:<reason>" for unsupported expressions.
func convertXPathToCSS(xpath string) (string, func(*goquery.Selection) bool) {
	path := strings.TrimSpace(xpath)
	path = strings.TrimPrefix(path, ".")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimPrefix(path, "/")

	if path == "" {
		return "", nil
	}

	if idx := strings.Index(path, "/"); idx != -1 {
		path = path[:idx]
	}

	tag := "*"
	predicate := ""
	if idx := strings.Index(path, "["); idx != -1 {
		tag = path[:idx]
		predicate = strings.TrimSuffix(path[idx+1:], "]")
	} else {
		tag = path
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = "*"
	}

	if strings.HasPrefix(tag, "@") {
		tag = "*"
	}

	var filter func(*goquery.Selection) bool
	css := tag

	if predicate == "" {
		return css, nil
	}

	predicate = strings.TrimSpace(predicate)
	switch {
	case strings.HasPrefix(predicate, "@id="):
		val := strings.Trim(predicate[len("@id="):], `"'`)
		if val == "" {
			return "", nil
		}
		if css == "*" {
			css = "#" + escapeCSSIdentifier(val)
		} else {
			css = tag + "#" + escapeCSSIdentifier(val)
		}
	case strings.HasPrefix(predicate, "@class="):
		val := strings.Trim(predicate[len("@class="):], `"'`)
		if val == "" {
			return "", nil
		}
		if css == "*" {
			css = "." + escapeCSSIdentifier(val)
		} else {
			css = tag + "." + escapeCSSIdentifier(val)
		}
	case strings.HasPrefix(predicate, "@") && strings.Contains(predicate, "="):
		parts := strings.SplitN(predicate[1:], "=", 2)
		if len(parts) != 2 {
			return "", nil
		}
		attr := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if attr == "" || val == "" {
			return "", nil
		}
		if css == "*" {
			css = "*" + "[" + attr + "=\"" + val + "\"]"
		} else {
			css = tag + "[" + attr + "=\"" + val + "\"]"
		}
	default:
		if strings.HasPrefix(predicate, "contains(@class,") {
			needle := extractQuotedValue(predicate)
			if needle == "" {
				return "", nil
			}
			filter = func(sel *goquery.Selection) bool {
				class, _ := sel.Attr("class")
				return strings.Contains(class, needle)
			}
		} else if strings.HasPrefix(predicate, "contains(@id,") {
			needle := extractQuotedValue(predicate)
			if needle == "" {
				return "", nil
			}
			filter = func(sel *goquery.Selection) bool {
				id, _ := sel.Attr("id")
				return strings.Contains(id, needle)
			}
		} else {
			return "unsupported:predicate:" + predicate, nil
		}
	}

	return css, filter
}

// extractQuotedValue extracts a quoted string value from a predicate expression.
func extractQuotedValue(s string) string {
	start := strings.IndexAny(s, `"'`)
	if start == -1 {
		return ""
	}
	quote := s[start]
	end := strings.IndexRune(s[start+1:], rune(quote))
	if end == -1 {
		return ""
	}
	return s[start+1 : start+1+end]
}

// escapeCSSIdentifier escapes a CSS identifier by replacing spaces with dots.
func escapeCSSIdentifier(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", ".")
	return value
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
		Chunks:    nil,
		Warning:   nil,
		Metadata: types.PageMetadata{
			SourceURL:    opts.SourceURL,
			StatusCode:   uint16(opts.StatusCode),
			RenderedMode: opts.RenderedMode,
			TimeTaken:    opts.TimeTaken,
		},
	}
}
