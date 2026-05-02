package crawler

import (
	"testing"
)

func TestParsesRobotsTxt(t *testing.T) {
	text := `
User-agent: *
Disallow: /admin/
Disallow: /private/

Sitemap: https://example.com/sitemap.xml
`
	robots := ParseRobotsTxt(text)
	if robots.IsAllowed("/admin/page") {
		t.Error("Expected /admin/page to be disallowed")
	}
	if !robots.IsAllowed("/public/page") {
		t.Error("Expected /public/page to be allowed")
	}
	if len(robots.Sitemaps) != 1 || robots.Sitemaps[0] != "https://example.com/sitemap.xml" {
		t.Errorf("Expected sitemap, got %v", robots.Sitemaps)
	}
}

func TestHandlesEdgeCases(t *testing.T) {
	text := "User-agent:\nDisallow:\nSitemap:\n"
	robots := ParseRobotsTxt(text)
	if !robots.IsAllowed("/anything") {
		t.Error("Expected /anything to be allowed")
	}
	if len(robots.Sitemaps) != 0 {
		t.Errorf("Expected no sitemaps, got %v", robots.Sitemaps)
	}
}

func TestWildcardPatternMatching(t *testing.T) {
	text := "User-agent: *\nDisallow: /*.pdf\n"
	robots := ParseRobotsTxt(text)
	if robots.IsAllowed("/document.pdf") {
		t.Error("Expected /document.pdf to be disallowed")
	}
	if robots.IsAllowed("/path/to/file.pdf") {
		t.Error("Expected /path/to/file.pdf to be disallowed")
	}
	if !robots.IsAllowed("/document.html") {
		t.Error("Expected /document.html to be allowed")
	}
}

func TestDollarEndAnchor(t *testing.T) {
	text := "User-agent: *\nDisallow: /*.pdf$\n"
	robots := ParseRobotsTxt(text)
	if robots.IsAllowed("/document.pdf") {
		t.Error("Expected /document.pdf to be disallowed")
	}
	if !robots.IsAllowed("/document.pdf?query=1") {
		t.Error("Expected /document.pdf?query=1 to be allowed")
	}
}

func TestAllowOverridesDisallow(t *testing.T) {
	text := `
User-agent: *
Disallow: /private/
Allow: /private/public-page
`
	robots := ParseRobotsTxt(text)
	if robots.IsAllowed("/private/secret") {
		t.Error("Expected /private/secret to be disallowed")
	}
	if !robots.IsAllowed("/private/public-page") {
		t.Error("Expected /private/public-page to be allowed")
	}
}

func TestSpecificityLongerPatternWins(t *testing.T) {
	text := `
User-agent: *
Disallow: /
Allow: /public/
`
	robots := ParseRobotsTxt(text)
	if robots.IsAllowed("/private") {
		t.Error("Expected /private to be disallowed")
	}
	if !robots.IsAllowed("/public/page") {
		t.Error("Expected /public/page to be allowed")
	}
}

func TestEqualLengthAllowWins(t *testing.T) {
	text := `
User-agent: *
Disallow: /path
Allow: /path
`
	robots := ParseRobotsTxt(text)
	if !robots.IsAllowed("/path") {
		t.Error("Expected /path to be allowed")
	}
}
