package crawler

import (
	"testing"
)

func TestParsesUrlset(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/page1</loc></url>
  <url><loc>https://example.com/page2</loc></url>
</urlset>`
	urls := ParseSitemap(xml)
	if len(urls) != 2 {
		t.Errorf("Expected 2 URLs, got %d", len(urls))
	}
	if urls[0] != "https://example.com/page1" {
		t.Errorf("Expected first URL to be https://example.com/page1, got %s", urls[0])
	}
	if urls[1] != "https://example.com/page2" {
		t.Errorf("Expected second URL to be https://example.com/page2, got %s", urls[1])
	}
}
