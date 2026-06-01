package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/renderer"
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
	http        *renderer.HTTPFetcher // Shared HTTP fetcher from internal/renderer
	cfg         BrowserConfig         // Browser configuration (timeout, WS URL, etc.)
	pool        *hostPool             // Per-host concurrency limiter
	allocCtx    context.Context       // Parent context for the RemoteAllocator
	allocCancel context.CancelFunc    // Cancel function to shut down the allocator
	mu          sync.Mutex            // Protects closed flag
	closed      bool                  // Prevents double-close
}

// FetchResult is the result of a page fetch (HTTP or browser). For HTTP fetches
// it is built by toCoreFetchResult from a *types.FetchResult. For browser fetches
// it is built directly inside fetchWithCDPBrowser.
type FetchResult struct {
	URL          string  // URL that was fetched
	FinalURL     string  // Final URL after redirects
	StatusCode   uint16  // HTTP status code (200 for browser unless anti-bot)
	HTML         string  // Fetched HTML content
	ContentType  string  // Content-Type header value (lowercased, no charset)
	RawBytes     []byte  // Raw bytes (for PDFs)
	RenderedWith string  // "http", "browser", or "pdf"
	TimeTakenMs  uint64  // Time taken in milliseconds
	Warning      *string // Non-fatal warning (e.g. anti-bot detected)
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
// The caller passes in a shared *renderer.HTTPFetcher so the HTTP code path
// is identical to /v1/scrape — no duplicated logic.
func NewRenderer(cfg Config, httpFetcher *renderer.HTTPFetcher) (*Renderer, *QuickCrawlError) {
	var allocCtx context.Context
	var allocCancel context.CancelFunc

	// Only set up RemoteAllocator if a WebSocket URL is provided.
	// If not, browser fetches will fail with ErrBrowserNotAvailable.
	if cfg.Browser.WSURL != "" {
		// NewRemoteAllocator connects to an already-running Chrome instance.
		// All chromedp contexts created from this allocator reuse the same Chrome.
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), cfg.Browser.WSURL)
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

// isAntiBotPage checks page HTML for generic anti-bot challenge markers.
// This includes Cloudflare "Just a Moment" pages, CAPTCHA pages, and generic
// access denied messages. It is a simple string search and does not detect
// vendor-specific blocks (Akamai, PerimeterX, Datadome, etc. are only in
// the original renderer).
func isAntiBotPage(html string) bool {
	html = strings.ToLower(html)
	markers := []string{
		"just a moment",
		"attention required",
		"cf-browser-verification",
		"cf-challenge",
		"captcha",
		"access denied",
	}
	for _, m := range markers {
		if strings.Contains(html, m) {
			return true
		}
	}
	return false
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
// It is the main entry point for fetching a URL.
//
//   - renderJS=false → uses shared *renderer.HTTPFetcher (no JavaScript)
//   - renderJS=true  → uses fetchWithCDPBrowser (full browser, JavaScript rendered)
func (renderer *Renderer) FetchOrchestrator(ctx context.Context, rawURL string, headers map[string]string, renderJS bool, waitMs int64) (*FetchResult, *QuickCrawlError) {
	if !renderJS {
		// Delegate to the shared HTTP fetcher used by /v1/scrape.
		// waitForMs is passed for interface compatibility (HTTP fetches ignore it).
		waitForMs := waitMs
		typesResult, typesErr := renderer.http.Fetch(rawURL, headers, &waitForMs)
		if typesErr != nil {
			return nil, convertTypesError(typesErr)
		}
		return toCoreFetchResult(typesResult, rawURL), nil
	}
	return renderer.fetchWithCDPBrowser(ctx, rawURL, headers, waitMs)
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
		URL:         r.URL,
		StatusCode:  r.StatusCode,
		HTML:        r.HTML,
		RawBytes:    r.RawBytes,
		TimeTakenMs: r.TimeTaken,
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

// convertTypesError maps a *types.QuickCrawlError into the local *core.QuickCrawlError
// so the public surface of this package keeps its own error type.
func convertTypesError(e *types.QuickCrawlError) *QuickCrawlError {
	if e == nil {
		return nil
	}
	return &QuickCrawlError{
		Message: e.Message,
		Code:    ErrorCode(string(e.Code)),
	}
}

// fetchWithCDPBrowser loads a URL in a headless Chrome browser via chromedp.
// It acquires a per-host concurrency slot, creates an isolated browser context,
// navigates to the URL, waits for JavaScript to render, and extracts the HTML.
//
// Steps:
//  1. Validate that a browser is available (RemoteAllocator was set up).
//  2. Acquire a per-host concurrency slot from the pool.
//  3. Create a new chromedp context from the RemoteAllocator with
//     chromedp.WithNewBrowserContext(). This creates a fresh Chrome
//     BrowserContext (incognito-like partition) for the request and a
//     new target/tab inside it. Cookies, localStorage, IndexedDB, cache,
//     and service workers are fully isolated from other concurrent fetches.
//     The BrowserContext is disposed automatically when cancelBrowser is
//     called (deferred below), via chromedp's built-in target.DisposeBrowserContext.
//  4. Apply a timeout to the browser context.
//  5. Run the chromedp action sequence:
//     - Navigate to the URL
//     - Sleep for waitDuration to allow JS to execute
//     - Capture the final URL (after any redirects)
//     - Extract <head> and <body> HTML
//  6. Check for anti-bot challenge pages and set a warning if detected.
//  7. Return the FetchResult with HTML, timing, and metadata.
func (renderer *Renderer) fetchWithCDPBrowser(ctx context.Context, rawURL string, headers map[string]string, waitMs int64) (*FetchResult, *QuickCrawlError) {
	// Step 1: Reject if no browser is configured.
	if renderer.allocCtx == nil {
		return nil, ErrBrowserNotAvailable.New("no browser WS URL configured")
	}

	start := time.Now()

	// Step 2: Acquire a concurrency slot for this host.
	// This prevents overwhelming any single origin with parallel browser fetches.
	host := extractHost(rawURL)
	release := renderer.pool.Acquire(host)
	defer release()

	// Step 3: Create a new chromedp context.
	// chromedp.NewContext creates a new browser tab (target) from the allocator.
	// The RemoteAllocator reuses the persistent Chrome connection.
	//
	// Note on isolation: we previously passed chromedp.WithNewBrowserContext()
	// here for per-request storage isolation (cookies, localStorage, etc.).
	// That option is temporarily disabled because chromedp v0.15.1's generated
	// Target.createTarget params include deprecated bool fields (newWindow,
	// background, forTab, hidden, focus) that Chrome 95+ rejects with
	// "Failed to open new tab - no browser is open (-32000)". The standard
	// Chrome DevTools Protocol no longer lists those fields, but cdproto has
	// not been regenerated. We track the workaround in internal/core/TODO.md
	// (or as an inline note). To re-enable, either (a) patch the cdproto
	// CreateTargetParams struct to add `omitzero` to the deprecated bool
	// fields, or (b) drop in a vendored copy of the latest protocol types,
	// or (c) issue the createBrowserContext + createTarget via raw CDP and
	// attach with chromedp.WithTargetID. All three options preserve the
	// WithNewBrowserContext() call at this site — no other code change.
	browserCtx, cancelBrowser := chromedp.NewContext(renderer.allocCtx)
	defer cancelBrowser()

	// Step 4: Apply the page timeout to the browser context.
	// All chromedp actions within runCtx will fail if they exceed this deadline.
	runCtx, cancel := context.WithTimeout(browserCtx, renderer.cfg.PageTimeout)
	defer cancel()

	// Step 5a: Determine the wait duration after navigation.
	// If waitMs is not provided or is 0, default to 2 seconds.
	// This gives SPAs time to hydrate and lazy-load content.
	waitDuration := time.Duration(waitMs) * time.Millisecond
	if waitDuration == 0 {
		waitDuration = 2 * time.Second
	}

	// Variables populated by the chromedp action sequence below.
	var finalURL string
	var statusCode uint16
	var headHTML string
	var bodyHTML string

	// Step 5b: Build the chromedp action sequence.
	//
	// Navigate loads the URL in the browser tab. chromedp waits for the
	// Page.loadEventFired event by default (or DomContentLoaded if configured).
	//
	// dismissCookieBannersAction is conditionally inserted after Navigate,
	// only in default mode (waitMs == 0). It mirrors the original renderer's
	// behavior at internal/renderer/browser_fetcher.go:296 — explicit
	// waitForMs implies the caller is managing timing themselves, so we
	// skip auto-dismiss to avoid surprising them with hidden clicks. The
	// dismiss step itself is best-effort and never aborts the run.
	//
	// Sleep waits for the specified duration after navigation. This is a
	// naive approach compared to SPA readiness polling — it does not check
	// whether content has actually loaded.
	//
	// Location captures the final URL after any client-side redirects.
	//
	// OuterHTML extracts the serialized HTML of the head and body elements
	// as plain strings (not CDP's compressed JSON encoding).
	//
	// ActionFunc is a catch-all that runs a custom function — here it sets
	// statusCode to 200. Note: actual HTTP status is not captured from the
	// browser; chromedp does not provide this without network monitoring.
	actions := []chromedp.Action{
		chromedp.Navigate(rawURL),
	}
	if waitMs == 0 {
		actions = append(actions, dismissCookieBannersAction())
	}
	actions = append(actions,
		chromedp.Sleep(waitDuration),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("head", &headHTML, chromedp.ByQuery),
		chromedp.OuterHTML("body", &bodyHTML, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			statusCode = 200
			return nil
		}),
	)

	err := chromedp.Run(runCtx, actions...)

	// Step 5c: Handle errors from the chromedp run.
	if err != nil {
		// Distinguish timeout/cancellation from other errors.
		// Timeout errors indicate the page load exceeded PageTimeout.
		if strings.Contains(err.Error(), "context deadline") || strings.Contains(err.Error(), "context canceled") {
			return nil, ErrTimeout.New(fmt.Sprintf("page load timed out for %s", rawURL))
		}
		return nil, ErrRendererError.Wrap(err)
	}

	// Fall back to original URL if no redirect occurred.
	if finalURL == "" {
		finalURL = rawURL
	}

	elapsed := time.Since(start)

	// Step 6: Build the result and check for anti-bot pages.
	result := &FetchResult{
		URL:          rawURL,
		FinalURL:     finalURL,
		StatusCode:   statusCode,
		HTML:         headHTML + bodyHTML,
		RenderedWith: "browser",
		TimeTakenMs:  uint64(elapsed.Milliseconds()),
	}

	// Step 6: Detect generic anti-bot challenge pages (Cloudflare, CAPTCHA, etc.).
	// This uses simple string matching on page content.
	if isAntiBotPage(result.HTML) {
		w := "blocked by anti-bot protection"
		result.Warning = &w
	}

	return result, nil
}
