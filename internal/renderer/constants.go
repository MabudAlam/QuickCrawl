package renderer

import (
	"time"
)

// Size and timeout limits for the renderer.
const (
	// MaxResponseBytes is the maximum size for HTTP responses (10 MB).
	MaxResponseBytes = 10 * 1024 * 1024

	// MaxBrowserHTMLBytes is the maximum size for browser-extracted HTML (5 MB).
	// This prevents memory issues with heavy SPAs like Netflix.
	MaxBrowserHTMLBytes = 5 * 1024 * 1024

	// HTTPConnectTimeout is how long to wait for a TCP connection.
	HTTPConnectTimeout = 5 * time.Second

	// HTTPRequestTimeout is the total time limit for an HTTP request.
	HTTPRequestTimeout = 30 * time.Second

	// DefaultPageTimeout is the default time to wait for a page to load.
	DefaultPageTimeout = 30 * time.Second

	// DefaultPoolSize is the default number of concurrent browser pages.
	DefaultPoolSize = 10

	// httpMaxRetries is the number of retries for transient HTTP errors.
	httpMaxRetries = 1

	// httpRetryBackoff is the delay between retry attempts.
	httpRetryBackoff = 250 * time.Millisecond
)

// stealthHeaders are HTTP headers injected to mimic a real Chrome browser.
// These help avoid basic bot detection by making HTTP requests appear
// to come from a normal browser rather than automated tooling.
var stealthHeaders = map[string]string{
	"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
	"Accept-Language":           "en-US,en;q=0.9",
	"Sec-Ch-Ua":                 `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
	"Sec-Ch-Ua-Mobile":          "?0",
	"Sec-Ch-Ua-Platform":        `"Windows"`,
	"Sec-Fetch-Dest":            "document",
	"Sec-Fetch-Mode":            "navigate",
	"Sec-Fetch-Site":            "none",
	"Sec-Fetch-User":            "?1",
	"Upgrade-Insecure-Requests": "1",
	"Priority":                  "u=0, i",
}
