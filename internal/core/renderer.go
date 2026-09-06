package core

import (
	"context"
	"strings"
	"sync"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/chromedp/chromedp"
)

// Renderer orchestrates HTTP and browser-based page fetching.
// It uses a RemoteAllocator connected to a pre-existing Chrome instance
// via WebSocket, avoiding the cost of spawning a new Chrome process per request.
// All browser fetches are routed through a hostPool for per-host concurrency limiting.
//
// HTTP fetching is delegated to the shared *renderer.HTTPFetcher to avoid
// duplicating the same logic. Browser (chromedp) code lives in this package.
type Renderer struct {
	http        *HTTPFetcher        // Shared HTTP fetcher
	cfg         types.BrowserConfig // Browser configuration (timeout, WS URL, etc.)
	pool        *hostPool           // Per-host concurrency limiter
	allocCtx    context.Context     // Parent context for the RemoteAllocator
	allocCancel context.CancelFunc  // Cancel function to shut down the allocator
	mu          sync.Mutex          // Protects closed flag
	closed      bool                // Prevents double-close
}

// FetchResult is the result of a page fetch (HTTP or browser). For HTTP fetches
// it is built by toCoreFetchResult from a *types.FetchResult. For browser fetches
// it is built directly inside fetchWithCDPBrowser.
type FetchResult struct {
	URL          string   // URL that was fetched
	FinalURL     string   // Final URL after redirects
	StatusCode   uint16   // HTTP status code (200 for browser unless anti-bot)
	HTML         string   // Fetched HTML content
	ContentType  string   // Content-Type header value (lowercased, no charset)
	RawBytes     []byte   // Raw bytes (for PDFs)
	RenderedWith string   // "http", "browser", or "pdf"
	Warning      *string  // Non-fatal warning (e.g. anti-bot detected)
	BlockedURLs  []string // URLs blocked by the blocklist (browser path only)
}

// hostPool limits concurrent browser fetches per-host and globally.
// It uses two semaphore levels: a global pool slot (Pool.Size) and a
// per-host slot (PerHost). This prevents any single host from
// monopolizing all browser instances.
type hostPool struct {
	mu      sync.Mutex               // Protects the host map
	pool    map[string]chan struct{} // Per-host semaphore channels
	sem     chan struct{}            // Global pool slots (size = Pool.Size)
	perHost int                      // Max concurrent fetches per host
}

// newHostPool creates a hostPool with a global semaphore of `size` slots
// and a per-host limit of `perHost` concurrent requests.
func newHostPool(size, perHost int) *hostPool {
	return &hostPool{
		pool:    make(map[string]chan struct{}),
		sem:     make(chan struct{}, size),
		perHost: perHost,
	}
}

// Acquire reserves a slot in both the global pool and the per-host pool
// for the given host. It blocks until both slots are available.
// Returns a release function that must be called when the slot is no longer needed.
func (p *hostPool) Acquire(host string) func() {
	// Get or create the per-host semaphore channel for this host.
	// Each host gets its own buffered channel of size perHost.
	p.mu.Lock()
	ch, ok := p.pool[host]
	if !ok {
		ch = make(chan struct{}, p.perHost)
		p.pool[host] = ch
	}
	p.mu.Unlock()

	// Acquire global slot first (blocks if pool is exhausted).
	p.sem <- struct{}{}
	// Then acquire per-host slot (blocks if host has too many in-flight).
	ch <- struct{}{}

	// Return a release function that must be called when done.
	return func() {
		<-ch    // Release per-host slot
		<-p.sem // Release global slot
	}
}

// NewRenderer creates a Renderer with the given configuration.
// If cfg.Browser.WSURL is non-empty, it connects to the provided Chrome
// WebSocket endpoint using chromedp's RemoteAllocator. This allows
// reusing a persistent Chrome instance rather than spawning one per request.
//
// The caller passes in a shared *HTTPFetcher so the HTTP code path
// is identical to /v1/scrape — no duplicated logic.
func NewRenderer(cfg types.ScraperConfig, httpFetcher *HTTPFetcher) (*Renderer, *QuickCrawlError) {
	var allocCtx context.Context
	var allocCancel context.CancelFunc

	// Only set up RemoteAllocator if a WebSocket URL is provided.
	// If not, browser fetches will fail with ErrBrowserNotAvailable.
	if cfg.Browser.WSURL != "" {
		// NewRemoteAllocator connects to an already-running Chrome instance.
		// All chromedp contexts created from this allocator reuse the same Chrome.
		// Use NoModifyURL to prevent chromedp from trying to auto-discover the
		// WebSocket URL via /json/version, which fails when the URL has no
		// explicit port (e.g., wss://host.domain.com/devtools/browser/...).
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), cfg.Browser.WSURL, chromedp.NoModifyURL)
	}

	r := &Renderer{
		http:        httpFetcher,
		cfg:         cfg.Browser,
		pool:        newHostPool(cfg.Pool.Size, cfg.Pool.PerHost),
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
	}

	return r, nil
}

// extractHost pulls the host portion from a URL string.
// E.g. "https://docs.tinyfish.ai/path?query" -> "docs.tinyfish.ai".
// The host string is used as the key in the per-host concurrency pool.
func extractHost(rawURL string) string {
	if i := strings.Index(rawURL, "://"); i != -1 {
		rest := rawURL[i+3:]
		if j := strings.Index(rest, "/"); j != -1 {
			return rest[:j]
		}
		return rest
	}
	return rawURL
}

// Close releases the RemoteAllocator and prevents further browser fetches.
// It is safe to call multiple times (idempotent via the closed flag).
func (renderer *Renderer) Close() error {
	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	if renderer.closed {
		return nil
	}
	renderer.closed = true
	if renderer.allocCancel != nil {
		renderer.allocCancel()
	}
	return nil
}

// FetchOrchestrator decides whether to use HTTP fetching or browser rendering.
// It is the single entry point for fetching a URL.
//
// Effective mode precedence: per-request mode (non-nil) > server-wide cfg.Mode.
//   - http:    plain HTTP fetch only — never the browser (hard opt-out).
//   - browser: render directly in the browser (top of the ladder).
//   - auto:    fetch over HTTP first, then escalate to the browser when the
//     response needs JavaScript.
func (renderer *Renderer) FetchOrchestrator(ctx context.Context, rawURL string, headers map[string]string, mode *types.RenderMode, waitMs int64) (*FetchResult, *QuickCrawlError) {
	// Precedence: per-request mode (non-nil) > server-wide cfg.Mode.
	// render_mode is a default, not a restriction; the per-request knob exists
	// precisely to opt out of that default. There is no policy gate.
	effective := renderer.cfg.Mode
	if mode != nil {
		effective = *mode
	}

	switch effective {
	case types.RenderModeHTTP:
		return renderer.fetchHTTPOnly(ctx, rawURL, headers, waitMs)
	case types.RenderModeBrowser:
		return renderer.fetchWithCDPBrowser(ctx, rawURL, headers, waitMs)
	default: // RenderModeAuto
		return renderer.fetchAuto(ctx, rawURL, headers, waitMs)
	}
}

// fetchHTTPOnly performs a plain HTTP fetch. It never touches the browser.
func (renderer *Renderer) fetchHTTPOnly(ctx context.Context, rawURL string, headers map[string]string, waitMs int64) (*FetchResult, *QuickCrawlError) {
	waitForMs := waitMs
	typesResult, typesErr := renderer.http.Fetch(ctx, rawURL, headers, &waitForMs)
	if typesErr != nil {
		return nil, typesErr
	}
	return toCoreFetchResult(typesResult, rawURL), nil
}

// fetchAuto fetches over plain HTTP, then runs the escalation preflight
// (escalationReason) on the response. When the preflight finds a reason the page
// needs a real browser — and one is available — the URL is re-fetched in the
// browser. A PDF response is never browser-rendered.
func (renderer *Renderer) fetchAuto(ctx context.Context, rawURL string, headers map[string]string, waitMs int64) (*FetchResult, *QuickCrawlError) {
	waitForMs := waitMs
	typesResult, typesErr := renderer.http.Fetch(ctx, rawURL, headers, &waitForMs)
	if typesErr != nil {
		// HTTP failed but a browser is available → try the browser.
		if renderer.allocCtx != nil {
			return renderer.fetchWithCDPBrowser(ctx, rawURL, headers, waitMs)
		}
		return nil, typesErr
	}

	result := toCoreFetchResult(typesResult, rawURL)

	// PDFs are never browser-rendered.
	if isPDFContentType(result.ContentType) {
		return result, nil
	}

	// Preflight: escalate when the response signals JS is needed.
	if reason := escalationReason(result.HTML, result.StatusCode); reason != "" && renderer.allocCtx != nil {
		w := "auto-escalated to browser: " + reason
		result.Warning = &w
		return renderer.fetchWithCDPBrowser(ctx, rawURL, headers, waitMs)
	}

	return result, nil
}

// toCoreFetchResult adapts a *types.FetchResult (from internal/renderer) into
// the simpler *core.FetchResult used internally by this package.
//
// Differences in shape:
//   - types.FetchResult has *string for FinalURL/ContentType/RenderedWith/Warning
//   - core.FetchResult uses plain strings (empty = "not set")
func toCoreFetchResult(r *types.FetchResult, rawURL string) *FetchResult {
	if r == nil {
		return &FetchResult{URL: rawURL, RenderedWith: "http"}
	}
	out := &FetchResult{
		URL:        r.URL,
		StatusCode: r.StatusCode,
		HTML:       r.HTML,
		RawBytes:   r.RawBytes,
	}
	if r.FinalURL != nil {
		out.FinalURL = *r.FinalURL
	} else {
		out.FinalURL = r.URL
	}
	if r.ContentType != nil {
		out.ContentType = *r.ContentType
	}
	if r.RenderedWith != nil {
		out.RenderedWith = *r.RenderedWith
	} else {
		out.RenderedWith = "http"
	}
	if r.Warning != nil {
		w := *r.Warning
		out.Warning = &w
	}
	return out
}

// toTypesFetchResult is the inverse of toCoreFetchResult: it adapts a
// *core.FetchResult back into a *types.FetchResult for the legacy crawl
// pipeline (and any other consumer that expects the types shape). Fields
// are mapped 1:1; *string fields become pointers to local string copies.
func toTypesFetchResult(r *FetchResult) *types.FetchResult {
	if r == nil {
		return nil
	}
	out := &types.FetchResult{
		URL:        r.URL,
		StatusCode: r.StatusCode,
		HTML:       r.HTML,
		RawBytes:   r.RawBytes,
	}
	finalURL := r.FinalURL
	if finalURL == "" {
		finalURL = r.URL
	}
	out.FinalURL = &finalURL
	contentType := r.ContentType
	if contentType != "" {
		out.ContentType = &contentType
	}
	rendered := r.RenderedWith
	if rendered == "" {
		rendered = "http"
	}
	out.RenderedWith = &rendered
	if r.Warning != nil {
		w := *r.Warning
		out.Warning = &w
	}
	return out
}
