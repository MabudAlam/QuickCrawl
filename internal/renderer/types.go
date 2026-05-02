// Package renderer provides HTTP and JavaScript browser rendering capabilities.
// It supports multiple backends: plain HTTP, LightPanda, and Chrome.
package renderer

import (
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// PageFetcher is the interface for fetching web pages.
// Implementations can use HTTP, browser automation, or both.
type PageFetcher interface {
	// Fetch retrieves a web page and returns the raw result.
	Fetch(url string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError)

	// Name returns the name of this fetcher (e.g., "http", "lightpanda").
	Name() string

	// SupportsJS returns true if this fetcher can execute JavaScript.
	SupportsJS() bool

	// IsAvailable returns true if this fetcher is ready to use.
	IsAvailable() bool
}

// BrowserInfo contains information about a running browser instance.
type BrowserInfo struct {
	// Name is the browser name (e.g., "lightpanda", "chrome").
	Name string

	// WSURL is the Chrome DevTools Protocol WebSocket URL for connecting to the browser.
	WSURL string
}
