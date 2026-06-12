package extractor

import (
	"os"
	"strings"
	"testing"

	quickcrawlcore "github.com/MabudAlam/quickcrawl/internal/types"
)

// TestExtractMainContentFromHtml_SplitArticleByAdWidget verifies that
// articles split into two groups of paragraphs by an in-read ad widget
// (the NewIndianExpress layout) keep BOTH groups, not just the first one.
// Regression test: previously only the first text block was kept, dropping
// every paragraph that came after the ad.
//
// Fixture is inline (not on disk) so that swapping raw.html between
// different scraped pages doesn't invalidate the regression assertion.
func TestExtractMainContentFromHtml_SplitArticleByAdWidget(t *testing.T) {
	// Inline fixture, fixture-agnostic. We build a NIE-style layout where
	// the article body is split into two groups of paragraphs by an
	// in-read ad widget. The Readability threshold is 500 chars, so we
	// generate enough prose for both groups to clear it.
	opening := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 8)
	closing := strings.Repeat("Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. ", 8)
	html := `<!doctype html><html><head><title>Sample</title></head><body>` +
		`<div class="arr--story-page-card-wrapper">` +
		`<div class="arr--element-container"><div data-test-id="text">` +
		`<p>Opening paragraph one. ` + opening + `</p>` +
		`<p>Opening paragraph two. ` + opening + `</p>` +
		`</div></div>` +
		`<div class="arr--element-container"><div data-test-id="widget" class="app-ad app-ad--story-horizontal"><div>ad</div></div></div>` +
		`</div>` +
		`<div class="arr--story-page-card-wrapper">` +
		`<div class="arr--element-container"><div data-test-id="text">` +
		`<p>Earlier, Akhilesh had posted on X. ` + closing + `</p>` +
		`<p>Countering Rai&#39;s clarification. ` + closing + `</p>` +
		`<p>Wider Sanatan community. ` + closing + `</p>` +
		`</div></div>` +
		`</div>` +
		`</body></html>`

	out := ExtractMainContentFromHtml(html)

	mustContain := []string{
		"Opening paragraph one",
		"Opening paragraph two",
		"Earlier, Akhilesh had posted on X",
		"Countering Rai",
		"Wider Sanatan community",
	}

	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("missing expected content fragment %q", s)
		}
	}

	if strings.Count(out, "Earlier, Akhilesh had posted on X") > 1 {
		t.Errorf("Group 2 paragraphs were duplicated")
	}
}

// TestExtractArticleHTML_WrapsDocumentWithHeader verifies that
// ExtractArticleHTML returns a complete HTML document (doctype, html, head,
// body) and includes the article title as an <h1>.
func TestExtractArticleHTML_WrapsDocumentWithHeader(t *testing.T) {
	raw, err := os.ReadFile("raw.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	out := ExtractArticleHTML(string(raw))

	mustContain := []string{
		"<!doctype html>",
		"<html",
		"<meta charset=\"utf-8\">",
		"<h1>",
		"<article>",
	}

	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("missing expected fragment %q", s)
		}
	}

	if !strings.HasPrefix(strings.TrimSpace(out), "<!doctype html>") {
		t.Errorf("output does not start with doctype: %q", out[:min(60, len(out))])
	}
}

// TestExtract_FormatHtmlIncludesTitleAndBody verifies that the orchestrator
// Extract() produces a FormatHtml output that wraps the body in a complete
// document with an <h1> title and that paragraphs from the middle of the
// article are present.
func TestExtract_FormatHtmlIncludesTitleAndBody(t *testing.T) {
	raw, err := os.ReadFile("raw.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	data := Extract(ExtractOptions{
		RawHTML:   string(raw),
		SourceURL: "https://example.com/article",
		Formats:   []quickcrawlcore.OutputFormat{quickcrawlcore.FormatHtml},
	})

	if data.HTML == nil || *data.HTML == "" {
		t.Fatal("expected html output")
	}
	html := *data.HTML

	if !strings.HasPrefix(strings.TrimSpace(html), "<!doctype html>") {
		t.Errorf("html output missing doctype wrapper: %q", html[:min(60, len(html))])
	}

	if !strings.Contains(html, "<h1>") {
		t.Errorf("html output missing <h1> title")
	}

	// Verify a body paragraph that lives in the middle of the article
	// (past ads, related-story sidebars, etc.) is preserved.
	if !strings.Contains(html, "Saturday Night Live") {
		t.Errorf("html output missing body content from middle of article")
	}
}
