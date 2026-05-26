// Package extractor provides content extraction from HTML into various formats.
package extractor

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	stdhtml "html"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"
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

// ─── Noise Pattern Data ───────────────────────────────────────────────────────

// semanticTags are content-bearing HTML tags that should never be removed
// even if their class/id matches noise patterns. These define the document's
// meaning and structure (headings, lists, articles, sections, etc.).
// We can remove div/span since noise detection on their class/id will decide.
var semanticTags = map[string]bool{
	"article": true, "section": true, "main": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true, "dl": true, "dt": true, "dd": true,
	"p": true, "blockquote": true, "pre": true, "code": true,
	"table": true, "thead": true, "tbody": true, "tr": true, "td": true, "th": true,
	"strong": true, "em": true, "b": true, "i": true, "u": true,
	"figure": true, "figcaption": true, "picture": true,
	"label": true, "form": true, "input": true,
}

var noisePatterns = []string{
	"sidebar", "table-of-contents", "tableofcontents", "infobox", "navbox",
	"nav-box", "navigation", "breadcrumb", "cookie", "consent", "banner",
	"disqus", "advert", "popup", "modal", "subscribe",
	"printfooter", "catlinks", "mw-panel", "mw-navigation", "sitesub",
	"jump-to-nav", "mw-editsection", "reflist", "mw-references",
	"authority-control", "mw-indicators", "sistersitebox", "mbox", "ambox",
	"ombox", "hatnote", "shortdescription", "sphinxsidebar", "sphinxfooter",
	"copyright", "dropdown", "city-selector", "location-selector",
	"lang-selector", "language-selector", "skip-to", "skip-link", "skiplinks",
	"promo", "promotional", "widget", "widgets",
	"site-footer", "site-header", "page-footer", "page-header",
	"global-nav", "global-footer", "global-header", "main-nav",
	"primary-nav", "secondary-nav", "social-share", "social-links",
	"social-icons", "follow-us", "site-map", "sitemap",
	"playground", "railway", "up.railway", "external-link",
	"sharing", "share", "follow-btn", "follow-container", "btn-follow",
	"btn-fab", "like-btn", "like-btn", "article-tags",
	"article-tags-list", "article-tags-element", "tags-link",
	"option-btn", "btn-share", "sharing-btn", "w-sharing",
	"expand-extra-info", "extra-content-cta",
	"main-icon", "icon", "fab-label", "icon-xlarge", "icon-large",
	"i-follow", "i-share", "i-facebook", "i-x", "i-whatsapp",
	"i-threads", "i-bluesky", "i-linkedin", "i-reddit", "i-flipboard",
	"is-hidden", "article-options",
}

var noiseExactTokens = []string{
	"toc", "share", "social", "related", "recommended", "comment", "footer",
}

var noisePrefixes = []string{"ad-", "ads-"}

// ─── Regex Patterns ────────────────────────────────────────────────────────────

// Package-level compiled regexes — compiled once, reused on every call.
var (
	scriptRe     = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	styleRe      = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
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

var unwrapSafeTags = map[string]bool{
	"article": true, "section": true, "main": true,
	"header": true, "footer": true, "aside": true, "nav": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"p": true, "blockquote": true, "pre": true, "code": true,
	"ul": true, "ol": true, "li": true, "dl": true, "dt": true, "dd": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "td": true, "th": true,
	"figure": true, "figcaption": true,
	"form": true, "label": true, "input": true,
	"div": true, "span": true,
}

// ─── HTML Cleaning ───────────────────────────────────────────────────────────

// cleanNoise removes unwanted HTML elements using regex replacements.
// It strips script tags, style tags, noscript blocks, iframes, SVGs,
// and images with data: URIs. This is the first-pass strip done before
// any DOM-based cleaning.
func cleanNoise(html string) string {
	result := html

	result = scriptRe.ReplaceAllString(result, "")
	result = styleRe.ReplaceAllString(result, "")
	result = noScriptRe.ReplaceAllString(result, "")
	result = iframeRe.ReplaceAllString(result, "")
	result = svgRe.ReplaceAllString(result, "")
	result = dataImgRe.ReplaceAllString(result, "")
	result = urlTextRe.ReplaceAllString(result, "")
	result = buttonRe.ReplaceAllString(result, "")

	return result
}

// HTMLPreprocessOptions controls request-specific HTML preprocessing.
type HTMLPreprocessOptions struct {
	IncludeTags   []string
	ExcludeTags   []string
	CSSSelector   *string
	OnlyMain      bool
	BypassFilters bool
}

// preprocessHTML applies request-aware stripping and content selection before
// the sanitizer/formatter stage.
func preprocessHTML(rawHTML string, opts HTMLPreprocessOptions) string {
	bodyHTML := stripDocumentHead(rawHTML)
	cleaned := cleanNoise(bodyHTML)

	if !opts.BypassFilters {
		if len(opts.IncludeTags) > 0 {
			cleaned = filterBySelectors(cleaned, opts.IncludeTags)
		}
		if len(opts.ExcludeTags) > 0 {
			cleaned = removeElementsBySelectors(cleaned, opts.ExcludeTags)
		}
	}

	cleaned = applyNoisePatterns(cleaned)

	if !opts.BypassFilters && opts.OnlyMain {
		cleaned = ExtractMainContent(cleaned)
	}

	if !opts.BypassFilters && opts.CSSSelector != nil {
		cleaned = applyCSSSelector(cleaned, *opts.CSSSelector)
	}

	return cleaned
}

// stripDocumentHead removes the document <head> element after metadata has
// already been extracted. The extractor keeps metadata from the raw document
// but emits body-oriented content for downstream formats.
func stripDocumentHead(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	doc.Find("head").Remove()
	output, err := doc.Html()
	if err != nil {
		return html
	}

	return output
}

// applyNoisePatterns removes layout wrapper elements (divs/spans) that are
// clearly noise — sidebars, footers, nav blocks, cookie banners, etc. — while
// preserving all semantic content tags (headings, lists, articles, sections, etc.).
func applyNoisePatterns(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	doc.Find("*").Each(func(_ int, selection *goquery.Selection) {
		tagName := goquery.NodeName(selection)

		// Never remove semantic or structural tags — only layout wrappers (div/span).
		if semanticTags[tagName] {
			return
		}

		//Strips the content nakes so disabled it for now need moretesting
		// // Extract attributes for noise pattern matching.
		// classAttr, _ := selection.Attr("class")
		// idAttr, _ := selection.Attr("id")
		// roleAttr, _ := selection.Attr("role")

		// // Combine class + id into one lowercase string for matching.
		// combined := strings.ToLower(classAttr + " " + idAttr)

		// // Only remove if it's a noise element.
		// if isNoiseWrapper(combined, roleAttr) {
		// 	selection.Remove()
		// }
	})

	cleanedHTML, _ := doc.Html()
	return cleanedHTML
}

// isNoiseWrapper returns true if the element is a noise wrapper (e.g., sidebar,
// footer, nav container) that should be removed. It matches divs/spans with
// noisy class/id strings or landmark ARIA roles.
func isNoiseWrapper(attributes, role string) bool {
	// Check if the class/id contains any known noise phrase.
	for _, phrase := range noisePatterns {
		if strings.Contains(attributes, phrase) {
			return true
		}
	}

	// Split into individual tokens to catch token-level noise.
	// e.g., class="social-share" has token "social" matching exactNoise.
	tokens := strings.Fields(attributes)
	for _, token := range tokens {
		for _, exact := range noiseExactTokens {
			if token == exact {
				return true
			}
		}
		for _, prefix := range noisePrefixes {
			if strings.HasPrefix(token, prefix) {
				return true
			}
		}
	}

	// Check for landmark ARIA roles that indicate non-content regions.
	if role != "" {
		switch strings.ToLower(role) {
		case "navigation", "banner", "contentinfo", "complementary":
			return true
		}
	}

	return false
}

// shouldUnwrapAnonymousContainer returns true for wrapper div/span nodes that
// have no meaningful attributes and only contain block-like content.
func shouldUnwrapAnonymousContainer(s *goquery.Selection) bool {
	if len(s.Nodes) == 0 {
		return false
	}

	node := s.Nodes[0]
	tag := strings.ToLower(goquery.NodeName(s))
	if tag != "div" && tag != "span" {
		return false
	}

	if len(node.Attr) > 0 {
		return false
	}

	hasElementChild := false
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case xhtml.TextNode:
			continue
		case xhtml.CommentNode:
			continue
		case xhtml.ElementNode:
			hasElementChild = true
			if !unwrapSafeTags[strings.ToLower(child.Data)] {
				return false
			}
		default:
			return false
		}
	}

	return hasElementChild
}

// postprocessHTML strips all non-semantic attributes and unwraps custom web
// components using a bluemonday allowlist policy. This produces clean
// HTML that preserves only meaningful structure (tables, headings, links).
//
// Elements allowed: headings, paragraphs, lists, tables, code blocks, links,
// text formatting, and structural elements.
// Attributes allowed: href (links), src (media), align (text alignment),
// dir (text direction), role (ARIA landmarks), colspan, rowspan (table structure).
//
// Everything else (class, id, style, data-*, aria-*, tabindex) is stripped.
func postprocessHTML(html string) string {
	// Build a bluemonday policy that allows only semantic elements
	// and meaningful attributes. All other attributes are removed.
	p := bluemonday.NewPolicy()

	// Structural elements.
	p.AllowElements("h1", "h2", "h3", "h4", "h5", "h6", "p", "div", "span",
		"section", "article", "main", "header", "footer", "aside", "nav")

	// Text formatting.
	p.AllowElements("strong", "em", "b", "i", "u", "mark", "small", "del", "ins",
		"sub", "sup", "blockquote")

	// Lists.
	p.AllowElements("ul", "ol", "li", "dl", "dt", "dd")

	// Tables — preserve full table structure for data extraction.
	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th")

	// Code blocks.
	p.AllowElements("pre", "code")

	// Media.
	p.AllowElements("figure", "figcaption")

	// Forms (rarely needed but preserved for completeness).
	p.AllowElements("form", "label", "input")

	// Allow meaningful attributes only. All other attributes (class, id,
	// style, data-*, aria-*, tabindex, etc.) are stripped by bluemonday.
	p.AllowAttrs("align").OnElements("div", "span", "p", "td", "th",
		"h1", "h2", "h3", "h4", "h5", "h6")
	p.AllowAttrs("dir").OnElements("html", "body", "div", "span", "p",
		"td", "th", "li", "article", "section")
	p.AllowAttrs("role").OnElements("div", "span", "section", "article",
		"main", "header", "footer", "aside", "nav", "td", "th")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")

	// Apply the policy — bluemonday strips all non-allowed elements and attributes.
	normalized := p.Sanitize(html)

	// Post-process with goquery to unwrap custom web components that bluemonday
	// doesn't handle natively (e.g., <markdown-accessibility-table>).
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(normalized))
	if err != nil {
		return normalized
	}

	// Unwrap GitHub's custom <markdown-accessibility-table> wrapper.
	// The inner <table> is preserved; the wrapper element is removed.
	doc.Find("markdown-accessibility-table").Each(func(_ int, s *goquery.Selection) {
		s.Children().Unwrap()
	})

	// Unwrap <section class="js-render-..."> diagram containers.
	// The fallback text content inside is preserved.
	doc.Find("section.js-render-needs-enrichment, section.js-render-enrichment-target").Each(
		func(_ int, s *goquery.Selection) {
			s.Children().Unwrap()
		},
	)

	// Collapse redundant <pre><code> nesting — just keep <pre>.
	doc.Find("pre code").Each(func(_ int, s *goquery.Selection) {
		s.Children().Unwrap()
	})

	// Unwrap divs/spans that contain only a single child div/span (layout wrappers).
	for i := 0; i < 5; i++ {
		changed := false
		doc.Find("div, span").Each(func(_ int, s *goquery.Selection) {
			children := s.Children()
			if children.Length() == 1 {
				child := children.First()
				childTag := goquery.NodeName(child)
				if childTag == "div" || childTag == "span" {
					childText := strings.TrimSpace(child.Text())
					grandchildren := child.Children()
					if childText == "" && grandchildren.Length() == 0 {
						return
					}
					child.Unwrap()
					changed = true
				}
			}
		})
		if !changed {
			break
		}
	}

	// Unwrap anonymous div/span containers that only group block-like content.
	for i := 0; i < 5; i++ {
		changed := false
		doc.Find("div, span").Each(func(_ int, s *goquery.Selection) {
			if shouldUnwrapAnonymousContainer(s) {
				s.Unwrap()
				changed = true
			}
		})
		if !changed {
			break
		}
	}

	// Strip <a href="..."> tags — remove entirely including text.
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		s.Remove()
	})

	// Strip <button> tags — remove entirely including text.
	doc.Find("button").Each(func(_ int, s *goquery.Selection) {
		s.Remove()
	})

	// Remove empty paragraphs that survived normalization.
	doc.Find("p").Each(func(_ int, s *goquery.Selection) {
		if strings.TrimSpace(s.Text()) == "" && s.Children().Length() == 0 {
			s.Remove()
		}
	})

	// Remove empty <div> or <span> wrappers — iterate until no more changes.
	for {
		changed := false
		doc.Find("div, span").Each(func(_ int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			children := s.Children()
			if text == "" && children.Length() == 0 {
				s.Remove()
				changed = true
			}
		})
		if !changed {
			break
		}
	}

	// Collapse nested empty divs/spans.
	for i := 0; i < 5; i++ {
		before, _ := doc.Html()
		doc.Find("div:has(> div:empty), div:has(> span:empty), span:has(> div:empty), span:has(> span:empty)").Each(func(_ int, s *goquery.Selection) {
			s.Children().Filter("div, span").Each(func(_ int, child *goquery.Selection) {
				childText := strings.TrimSpace(child.Text())
				childChildren := child.Children()
				if childText == "" && childChildren.Length() == 0 {
					child.Remove()
				}
			})
		})
		after, _ := doc.Html()
		if before == after {
			break
		}
	}

	normalized, _ = doc.Html()

	// Regex pass: strip empty tags that DOM may have missed.
	for {
		before := normalized
		normalized = emptyDivRe.ReplaceAllString(normalized, "")
		normalized = emptySpanRe.ReplaceAllString(normalized, "")
		if normalized == before {
			break
		}
	}

	normalized = whitespaceRe.ReplaceAllString(normalized, " ")
	normalized = newlineRe.ReplaceAllString(normalized, "\n\n")

	// Remove repeated sibling nodes using structural signatures.
	dedupeDoc, err := goquery.NewDocumentFromReader(strings.NewReader(normalized))
	if err == nil {
		dedupeDuplicateSiblings(dedupeDoc.Selection.Nodes[0])
		if dedupedHTML, derr := dedupeDoc.Html(); derr == nil {
			normalized = dedupedHTML
		}
	}

	// Return a formatted HTML document with a real <html>/<body> wrapper.
	bodyDoc, err := goquery.NewDocumentFromReader(strings.NewReader(normalized))
	if err != nil {
		return formatHTMLDocument(strings.TrimSpace(normalized))
	}
	if body := bodyDoc.Find("body").First(); body.Length() > 0 {
		if bodyHTML, err := body.Html(); err == nil {
			return formatHTMLDocument(strings.TrimSpace(bodyHTML))
		}
	}

	return formatHTMLDocument(strings.TrimSpace(normalized))
}

// dedupeDuplicateSiblings removes consecutive sibling nodes that are structurally
// identical. This is a generic cleanup pass that catches duplicated wrappers or
// repeated empty blocks without special-casing any tag name.
func dedupeDuplicateSiblings(node *xhtml.Node) {
	if node == nil {
		return
	}

	var prevSig string
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		dedupeDuplicateSiblings(child)

		sig := canonicalNodeSignature(child)
		if sig != "" && sig == prevSig {
			node.RemoveChild(child)
		} else {
			prevSig = sig
		}
		child = next
	}
}

func canonicalNodeSignature(node *xhtml.Node) string {
	if node == nil {
		return ""
	}

	switch node.Type {
	case xhtml.TextNode:
		text := normalizeInlineText(node.Data)
		return "text:" + text
	case xhtml.ElementNode:
		var b strings.Builder
		b.WriteString("el:")
		b.WriteString(strings.ToLower(node.Data))
		if len(node.Attr) > 0 {
			attrs := make([]xhtml.Attribute, 0, len(node.Attr))
			for _, attr := range node.Attr {
				if strings.TrimSpace(attr.Key) == "" {
					continue
				}
				attrs = append(attrs, attr)
			}
			sort.Slice(attrs, func(i, j int) bool {
				if attrs[i].Key == attrs[j].Key {
					return attrs[i].Val < attrs[j].Val
				}
				return attrs[i].Key < attrs[j].Key
			})
			for _, attr := range attrs {
				b.WriteString("|")
				b.WriteString(strings.ToLower(attr.Key))
				b.WriteString("=")
				b.WriteString(strings.TrimSpace(attr.Val))
			}
		}
		b.WriteString(">")
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			sig := canonicalNodeSignature(child)
			if sig != "" {
				b.WriteString(sig)
				b.WriteByte('|')
			}
		}
		return b.String()
	default:
		return ""
	}
}

// formatHTMLDocument wraps body content in a full HTML document and renders it
// with consistent indentation so the response is easy to read and stable.
func formatHTMLDocument(bodyHTML string) string {
	bodyHTML = strings.TrimSpace(bodyHTML)
	if bodyHTML == "" {
		return "<html>\n  <body></body>\n</html>"
	}

	ctx := &xhtml.Node{Type: xhtml.ElementNode, Data: "body"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(bodyHTML), ctx)
	if err != nil {
		return "<html>\n  <body>\n" + indentText(bodyHTML, 4) + "\n  </body>\n</html>"
	}

	var b strings.Builder
	b.WriteString("<html>\n  <body>\n")
	renderHTMLNodes(&b, nodes, 4)
	b.WriteString("  </body>\n</html>")
	return strings.TrimSpace(b.String())
}

func renderHTMLNodes(b *strings.Builder, nodes []*xhtml.Node, indent int) {
	for _, node := range nodes {
		renderHTMLNode(b, node, indent)
	}
}

func renderHTMLNode(b *strings.Builder, node *xhtml.Node, indent int) {
	switch node.Type {
	case xhtml.TextNode:
		text := normalizeInlineText(node.Data)
		if text == "" {
			return
		}
		writeIndent(b, indent)
		b.WriteString(stdhtml.EscapeString(text))
		b.WriteByte('\n')
	case xhtml.ElementNode:
		tag := strings.ToLower(node.Data)
		if isVoidTag(tag) {
			writeIndent(b, indent)
			b.WriteByte('<')
			b.WriteString(tag)
			writeAttrs(b, node)
			b.WriteString(">\n")
			return
		}

		inlineOnly := nodeHasOnlyInlineContent(node)
		writeIndent(b, indent)
		b.WriteByte('<')
		b.WriteString(tag)
		writeAttrs(b, node)

		if inlineOnly {
			b.WriteByte('>')
			renderInlineChildren(b, node.FirstChild)
			b.WriteString("</")
			b.WriteString(tag)
			b.WriteString(">\n")
			return
		}

		b.WriteString(">\n")
		renderHTMLChildren(b, node.FirstChild, indent+2)
		writeIndent(b, indent)
		b.WriteString("</")
		b.WriteString(tag)
		b.WriteString(">\n")
	}
}

func renderHTMLChildren(b *strings.Builder, child *xhtml.Node, indent int) {
	for n := child; n != nil; n = n.NextSibling {
		renderHTMLNode(b, n, indent)
	}
}

func renderInlineChildren(b *strings.Builder, child *xhtml.Node) {
	for n := child; n != nil; n = n.NextSibling {
		switch n.Type {
		case xhtml.TextNode:
			text := normalizeInlineText(n.Data)
			if text != "" {
				b.WriteString(stdhtml.EscapeString(text))
			}
		case xhtml.ElementNode:
			tag := strings.ToLower(n.Data)
			b.WriteByte('<')
			b.WriteString(tag)
			writeAttrs(b, n)
			if isVoidTag(tag) {
				b.WriteString(">")
				continue
			}
			b.WriteByte('>')
			renderInlineChildren(b, n.FirstChild)
			b.WriteString("</")
			b.WriteString(tag)
			b.WriteByte('>')
		}
	}
}

func writeAttrs(b *strings.Builder, node *xhtml.Node) {
	for _, attr := range node.Attr {
		if strings.TrimSpace(attr.Key) == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(attr.Key)
		b.WriteString(`="`)
		b.WriteString(stdhtml.EscapeString(attr.Val))
		b.WriteByte('"')
	}
}

func writeIndent(b *strings.Builder, indent int) {
	if indent <= 0 {
		return
	}
	b.WriteString(strings.Repeat(" ", indent))
}

func indentText(text string, indent int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	prefix := strings.Repeat(" ", indent)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func normalizeInlineText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}

	collapsed := strings.Join(strings.Fields(s), " ")
	if collapsed == "" {
		return ""
	}

	runes := []rune(s)
	if len(runes) > 0 && unicode.IsSpace(runes[0]) {
		collapsed = " " + collapsed
	}
	if len(runes) > 0 && unicode.IsSpace(runes[len(runes)-1]) {
		collapsed += " "
	}

	return collapsed
}

func nodeHasOnlyInlineContent(node *xhtml.Node) bool {
	hasContent := false
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case xhtml.TextNode:
			if strings.TrimSpace(child.Data) != "" {
				hasContent = true
			}
		case xhtml.ElementNode:
			hasContent = true
			if !isInlineTag(strings.ToLower(child.Data)) {
				return false
			}
		case xhtml.CommentNode:
			continue
		default:
			return false
		}
	}
	return hasContent
}

func isInlineTag(tag string) bool {
	switch tag {
	case "a", "abbr", "b", "code", "del", "em", "i", "ins", "mark", "q", "s", "small", "span", "strong", "sub", "sup", "u", "label":
		return true
	default:
		return false
	}
}

func isVoidTag(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

// filterBySelectors extracts and concatenates text content from elements
// matching the provided CSS selectors. Used when IncludeTags is specified.
func filterBySelectors(html string, selectors []string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	var parts []string
	for _, sel := range selectors {
		doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
			parts = append(parts, s.Text())
		})
	}

	if len(parts) == 0 {
		return html
	}
	return strings.Join(parts, "\n")
}

// removeElementsBySelectors removes all elements matching the provided
// CSS selectors from the HTML document. Used when ExcludeTags is specified.
func removeElementsBySelectors(html string, selectors []string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	for _, sel := range selectors {
		doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
			s.Remove()
		})
	}

	output, _ := doc.Html()
	return output
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
	for _, noise := range noisePatterns {
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
