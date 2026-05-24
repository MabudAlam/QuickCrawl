// Package crawler provides BFS (breadth-first search) crawling capabilities.
// It crawls web pages recursively while respecting robots.txt and rate limits.
package crawler

import (
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// Maximum number of URLs to discover during a crawl.
const maxDiscoveredURLs = 5000

// CrawlOptions contains all parameters for a crawl operation.
type CrawlOptions struct {
	ID       string              // Unique crawl job ID
	Req      *types.CrawlRequest // Crawl request parameters
	Renderer interface {         // Renderer to fetch pages
		Fetch(rawURL string, headers map[string]string, renderJS *bool, waitForMs *int64, browser *string) (*types.FetchResult, *types.QuickCrawlError)
	}
	MaxConcurrency    int                     // Maximum concurrent page fetches
	RespectRobots     bool                    // Whether to follow robots.txt rules
	RequestsPerSecond float64                 // Rate limit for requests
	UserAgent         string                  // User-Agent header for requests
	StateCh           chan<- types.CrawlState // Channel for progress updates
	LLMConfig         *types.LLMConfig        // LLM configuration for extraction
	JitterFactor      float64                 // Random delay factor (0.0 to 1.0)
	StealthStrategy   utils.HeaderStrategy    // Stealth header strategy
}

// RateLimiter implements per-domain rate limiting with configurable RPS.
// It ensures that requests to a domain do not exceed the specified
// requests-per-second rate by enforcing a minimum interval between requests.
type RateLimiter struct {
	minInterval time.Duration // Minimum time between requests
	lastRequest time.Time     // When the last request was made
	lastMu      sync.Mutex    // Protects lastRequest
}

// NewRateLimiter creates a rate limiter for the given requests-per-second limit.
// If requestsPerSecond is 0 or negative, no rate limiting is applied.
func NewRateLimiter(requestsPerSecond float64) *RateLimiter {
	if requestsPerSecond < 0.0 {
		requestsPerSecond = 0.0
	}
	var minInterval time.Duration
	if requestsPerSecond > 0.0 {
		minInterval = time.Duration(1e9 / requestsPerSecond)
	}
	return &RateLimiter{
		minInterval: minInterval,
		lastRequest: time.Now().Add(-minInterval),
	}
}

// NextSleep returns the duration to wait before the next request.
// Thread-safe and respects the configured RPS limit.
func (r *RateLimiter) NextSleep() time.Duration {
	r.lastMu.Lock()
	defer r.lastMu.Unlock()

	elapsed := time.Since(r.lastRequest)
	var sleep time.Duration
	if elapsed < r.minInterval {
		sleep = r.minInterval - elapsed
	}
	r.lastRequest = time.Now().Add(sleep)
	return sleep
}

// pendingCrawlItem represents a URL to crawl with its depth level.
type pendingCrawlItem struct {
	url   string // URL to crawl
	depth uint32 // Depth in the crawl tree (0 = starting URL)
}

// crawlPageResult represents the result of crawling a single URL.
type crawlPageResult struct {
	item  pendingCrawlItem       // The URL that was crawled
	data  *types.ScrapeData      // Extracted page data
	links []string               // Discovered links on the page
	err   *types.QuickCrawlError // Error if crawl failed
}
