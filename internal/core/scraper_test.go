package core

import (
	"context"
	"testing"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// TestScrapeRejectsBinaryAssetURL verifies that Scrape fails fast (no fetch)
// on obvious binary asset URLs, independent of render mode.
func TestScrapeRejectsBinaryAssetURL(t *testing.T) {
	httpFetcher := NewHTTPFetcher("", nil)
	cfg := types.ScraperConfig{
		Browser: types.BrowserConfig{Mode: types.RenderModeAuto, WSURL: ""},
	}
	s, err := NewScraper(cfg, httpFetcher, nil)
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	for _, u := range []string{
		"https://example.com/a.svg",
		"https://example.com/logo.png",
		"https://example.com/video.mp4",
	} {
		_, qerr := s.Scrape(context.Background(), &types.ScrapeRequest{
			URL:     u,
			Formats: []types.OutputFormat{types.FormatMarkdown},
		})
		if qerr == nil || qerr.Code != CodeUnsupportedContent {
			t.Errorf("Scrape(%q): expected unsupported_content_type, got %v", u, qerr)
		}
	}
}
