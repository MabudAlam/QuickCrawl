package extractor

import (
	"strings"
	"testing"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

func BenchmarkHTMLToMarkdown(b *testing.B) {
	html := `<html>
<body>
<article>
<h1>Heading One</h1>
<p>First paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
<h2>Subheading</h2>
<p>Second paragraph with a <a href="https://example.com">link</a>.</p>
<ul><li>Item one</li><li>Item two</li></ul>
<pre><code>code block</code></pre>
</article>
</body>
</html>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HTMLToMarkdown(html)
	}
}

func BenchmarkHTMLToPlaintext(b *testing.B) {
	html := `<html>
<body>
<article>
<h1>Heading One</h1>
<p>First paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
<h2>Subheading</h2>
<p>Second paragraph with a <a href="https://example.com">link</a>.</p>
<ul><li>Item one</li><li>Item two</li></ul>
<pre><code>code block</code></pre>
</article>
</body>
</html>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HTMLToPlaintext(html)
	}
}

func BenchmarkExtractMainContent(b *testing.B) {
	html := `<html>
<body>
<nav class="sidebar">Navigation menu with lots of links</nav>
<header>Site header</header>
<main>
<article>
<h1>Important Article Title</h1>
<p>This is the first paragraph of the article with substantial content that explains the main topic.</p>
<p>Second paragraph with more detailed information about the subject matter being discussed.</p>
<p>Third paragraph that continues the discussion and provides additional context and details.</p>
</article>
</main>
<footer>Site footer with copyright info</footer>
</body>
</html>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractMainContent(html)
	}
}

func BenchmarkExtractLinks(b *testing.B) {
	html := `<html>
<body>
<nav>
<a href="https://example.com/page1">Page 1</a>
<a href="https://example.com/page2">Page 2</a>
<a href="https://example.com/page3">Page 3</a>
</nav>
<article>
<a href="https://example.com/article1">Article 1</a>
<a href="https://example.com/article2">Article 2</a>
<a href="/relative/path">Relative Link</a>
<a href="#section">Anchor Link</a>
</article>
<footer>
<a href="https://example.com/about">About</a>
<a href="https://example.com/contact">Contact</a>
</footer>
</body>
</html>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractLinks(html, "https://example.com")
	}
}

func BenchmarkExtractMetadata(b *testing.B) {
	html := `<!DOCTYPE html>
<html>
<head>
<title>Test Page Title</title>
<meta name="description" content="This is a test page description for SEO purposes">
<meta name="keywords" content="test, page, benchmark">
<meta property="og:title" content="Social Media Title">
<meta property="og:description" content="Social media description">
<link rel="canonical" href="https://example.com/canonical">
</head>
<body>
<p>Content</p>
</body>
</html>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractMetadata(html)
	}
}

func BenchmarkConvertIndentedToFencedCode(b *testing.B) {
	md := `Paragraph before

    code line one
    code line two
    code line three

Paragraph after`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertIndentedToFencedCode(md)
	}
}

func TestHTMLToMarkdown(t *testing.T) {
	html := `<h1>Title</h1><p>Paragraph with <strong>bold</strong> text.</p>`
	md := HTMLToMarkdown(html)
	if md == "" {
		t.Error("Expected markdown output, got empty string")
	}
}

func TestHTMLToPlaintext(t *testing.T) {
	html := `<h1>Title</h1><p>Paragraph with <strong>bold</strong> text.</p>`
	text := HTMLToPlaintext(html)
	if text == "" {
		t.Error("Expected plaintext output, got empty string")
	}
}

func TestExtractMainContentPrefersArticleOverSidebar(t *testing.T) {
	html := `<html><body>
<nav class="sidebar">
<a href="/a">Home</a>
<a href="/b">About</a>
<a href="/c">Docs</a>
</nav>
<main>
<article class="article-body">
<h1>Main title</h1>
<p>This is the important article body with substantial content and multiple sentences.</p>
<p>It should win over the sidebar even if the sidebar appears early in the DOM.</p>
</article>
</main>
</body></html>`

	content := ExtractMainContent(html)
	if !strings.Contains(content, "important article body") {
		t.Fatalf("expected article content to be selected, got: %s", content)
	}
	if strings.Contains(content, "Home") || strings.Contains(content, "About") || strings.Contains(content, "Docs") {
		t.Fatalf("expected sidebar noise to be excluded, got: %s", content)
	}
}

func TestExtractHtmlPreservesSemanticTagsThroughWrapperUnwrap(t *testing.T) {
	html := `<html><body><div><div><h1>Title</h1><p>Body text with enough length to survive cleaning.</p></div></div></body></html>`

	data := Extract(ExtractOptions{
		RawHTML:   html,
		SourceURL: "https://example.com",
		Formats:   []types.OutputFormat{types.FormatHtml},
	})

	if data.HTML == nil {
		t.Fatal("expected HTML output")
	}

	got := *data.HTML
	if !strings.Contains(got, "<h1>Title</h1>") {
		t.Fatalf("expected heading to remain in HTML output, got: %s", got)
	}
	if !strings.Contains(got, "<p>Body text with enough length to survive cleaning.</p>") {
		t.Fatalf("expected paragraph to remain in HTML output, got: %s", got)
	}
	if strings.Contains(got, "<div><div>") {
		t.Fatalf("expected nested wrapper divs to be unwrapped, got: %s", got)
	}
}

func TestExtractPDFTextHeuristic(t *testing.T) {
	raw := []byte("%PDF-1.4\n1 0 obj\n(Hello) Tj\n(World) Tj\nendobj\n")
	got := ExtractPDFText(raw)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Fatalf("expected PDF heuristic to recover text, got: %q", got)
	}
}
