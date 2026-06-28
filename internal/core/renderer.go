package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
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
	http        *HTTPFetcher // Shared HTTP fetcher
	cfg         types.BrowserConfig   // Browser configuration (timeout, WS URL, etc.)
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

// ellipsis clips s to at most n bytes and appends "..." if the
// original was longer. Used for debug logging where we want a
// short, single-line representation of a long string.
func ellipsis(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// isAntiBotPage checks page HTML for generic anti-bot challenge markers.
// This includes Cloudflare "Just a Moment" pages, CAPTCHA pages, and generic
// access denied messages. It is a simple string search and does not detect
// vendor-specific blocks (Akamai, PerimeterX, Datadome, etc. are only in
// the original renderer).
//
// The marker set is intentionally broad — false positives are cheap (a
// block page is returned with a warning instead of waiting 15s for the
// SPA poll to time out and 3s for autoscroll to give up). Real content
// pages very rarely contain these substrings in their first 1KB of
// body text.
func isAntiBotPage(html string) bool {
	html = strings.ToLower(html)
	markers := []string{
		// Cloudflare variants — covers "Just a Moment" challenges,
		// generic blocks, and the per-incident "Ray ID" error pages.
		"just a moment",
		"attention required",
		"cf-browser-verification",
		"cf-challenge",
		"checking your browser before accessing",
		"this site is using a security service to protect itself",
		"ray id",                      // Cloudflare incident id on error pages
		"cloudflare",                  // generic CF identifier
		"blocked by network security", // Reddit's Cloudflare-block text
		"your request has been blocked",
		// Generic / vendor-agnostic. We intentionally omit the
		// very common word "forbidden" — it false-positives on
		// book titles and code comments, and 403 status codes
		// already cover the legitimate-block case at the HTTP
		// level. "captcha" is kept because it is a strong
		// anti-bot signal and rarely appears in regular content.
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
//   - mode=nil              → inherit server default (cfg.Mode)
//   - mode=auto             → HTTP first, then check and escalate to browser if needed
//   - mode=browser          → uses fetchWithCDPBrowser (full browser, JavaScript rendered)
//   - mode=http             → uses shared *renderer.HTTPFetcher (no JavaScript)
//
// In auto mode, the escalation decision is based on:
//   - HTTP failure + browser available → escalate
//   - PDF response → return HTTP result
//   - SPA shell detected (framework markers, builder platforms, dense scripts)
//   - Cloudflare/anti-bot challenge page detected
//   - Soft-block status code (401, 403, 404, 405, 406, 410, 412, 429, 451, 500, 503)
//   - Thin HTML response (body text < 200 chars)
func (renderer *Renderer) FetchOrchestrator(ctx context.Context, rawURL string, headers map[string]string, mode *types.RenderMode, waitMs int64) (*FetchResult, *QuickCrawlError) {
	// Precedence: per-request mode (non-nil) > server-wide cfg.Mode > RenderModeAuto.
	// The server config ([renderer] render_mode in quickcrawl.toml, or
	// RENDERER__RENDER_MODE env var) is the *default* for every request.
	// A caller that passes mode explicitly always wins — there is no
	// policy gate. This is intentional: render_mode is a default, not a
	// restriction, and the per-request knob exists precisely to opt out
	// of that default.
	forceBrowser := false
	forceHTTP := false
	effective := renderer.cfg.Mode
	if mode != nil {
		effective = *mode
	}
	switch effective {
	case types.RenderModeBrowser:
		forceBrowser = true
	case types.RenderModeHTTP:
		forceHTTP = true
	}

	if forceHTTP {
		waitForMs := waitMs
		typesResult, typesErr := renderer.http.Fetch(rawURL, headers, &waitForMs)
		if typesErr != nil {
			return nil, typesErr
		}
		return toCoreFetchResult(typesResult, rawURL), nil
	}

	if forceBrowser {
		return renderer.fetchWithCDPBrowser(ctx, rawURL, headers, waitMs)
	}

	// AUTO MODE: HTTP first, then check and escalate if needed
	waitForMs := waitMs
	typesResult, typesErr := renderer.http.Fetch(rawURL, headers, &waitForMs)
	if typesErr != nil {
		// HTTP failed but browser is available → try browser
		if renderer.allocCtx != nil {
			return renderer.fetchWithCDPBrowser(ctx, rawURL, headers, waitMs)
		}
		return nil, typesErr
	}

	result := toCoreFetchResult(typesResult, rawURL)

	// PDF early return — don't try to browser-render PDFs
	if isPDFContentType(result.ContentType) {
		return result, nil
	}

	// Check for escalation triggers
	needsEscalation := false
	escalationReason := ""

	// 1. SPA shell detected
	if needsJSRendering(result.HTML) {
		needsEscalation = true
		escalationReason = "SPA shell detected"
	}

	// 2. Soft-block status code
	if !needsEscalation && isSoftBlockStatus(result.StatusCode) {
		needsEscalation = true
		escalationReason = fmt.Sprintf("soft-block status HTTP %d", result.StatusCode)
	}

	// 3. Thin content (body text < 200 chars on 2xx)
	if !needsEscalation && result.StatusCode >= 200 && result.StatusCode < 300 {
		if looksLikeThinHTML(result.HTML) {
			needsEscalation = true
			escalationReason = "thin HTML content"
		}
	}

	// 4. Anti-bot challenge pages (Cloudflare, generic bot wall, vendor blocks)
	if !needsEscalation && result.StatusCode >= 200 && result.StatusCode < 300 {
		if looksLikeCloudflareChallenge(result.HTML) {
			needsEscalation = true
			escalationReason = "Cloudflare challenge detected"
		} else if looksLikeGenericBotWall(result.HTML) {
			needsEscalation = true
			escalationReason = "generic anti-bot wall detected"
		} else if vendor := looksLikeVendorBlock(result.HTML); vendor != "" {
			needsEscalation = true
			escalationReason = "anti-bot vendor block: " + vendor
		}
	}

	if needsEscalation && renderer.allocCtx != nil {
		w := "auto-escalated to browser: " + escalationReason
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

	// Step 2: Acquire a concurrency slot for this host.
	// This prevents overwhelming any single origin with parallel browser fetches.
	host := extractHost(rawURL)
	release := renderer.pool.Acquire(host)
	defer release()

	allocCtx := renderer.allocCtx

	// Step 4: Create a new chromedp context.
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
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	// Step 5: Apply the page timeout to the browser context.
	// All chromedp actions within runCtx will fail if they exceed this deadline.
	runCtx, cancel := context.WithTimeout(browserCtx, renderer.cfg.PageTimeout)
	defer cancel()

	// Step 5a: Determine the readiness budget after navigation.
	// The browser-side waiting is now driven by WaitForSPAReady (see
	// internal/core/spa.go) which polls for content readiness instead
	// of sleeping for a fixed duration. The budget is the SPA poll
	// timeout; it is bounded by the overall PageTimeout above.
	//
	// In default mode (waitMs == 0) we give the poll the same 15s
	// the previous renderer uses for SPA readiness.
	//
	// In explicit mode (waitMs > 0) the caller has asked for a fixed
	// wait duration. We honor that as a single Sleep rather than the
	// poll, matching the previous renderer's `else { time.Sleep(wait) }`
	// behavior.
	spaTimeout := 15 * time.Second
	if waitMs > 0 {
		spaTimeout = time.Duration(waitMs) * time.Millisecond
	}

	// Variables populated by the chromedp action sequence below.
	var finalURL string
	var statusCode uint16
	var headHTML string
	var bodyHTML string
	var spaResult SPAReadinessResult

	// tracker records blocked request URLs for per-request logging.
	tracker := &blockedTracker{}

	// blockDetected is set by the early anti-bot check (a quick
	// OuterHTML of <body> right after Navigate) when the page
	// looks like a Cloudflare / generic anti-bot challenge. When
	// true, the SPA readiness poll and the autoscroll loop are
	// skipped — they would both waste ~5-15s of budget on a page
	// that will never hydrate. The final OuterHTMLs still run so
	// the block page itself is returned (it is informative: it
	// tells the caller what challenge they hit).
	var blockDetected bool
	var blockWarning string

	// networkBundle groups the HTTP status tracker (for capturing the
	// main document response's status code) and the network activity
	// tracker (for SPA network-idle fast exit). The bundle's Handle
	// method dispatches events to both trackers. See
	// internal/core/network.go for the full design rationale.
	networkBundle := &networkBundle{
		status:   &networkStatusTracker{},
		activity: newNetworkActivityTracker(),
	}
	// Convenience alias for the existing call sites that read .Status().
	networkTracker := networkBundle.status

	// Stage timing — collects per-action millisecond costs so we can
	// attribute residual gaps between /scrape and /scrape-core to a
	// specific stage. Logged at the end of the chromedp.Run in
	// map[k:v k:v] format.
	stageTimes := map[string]int64{}
	stageStart := time.Now()

	// Step 5b: Build the chromedp action sequence.
	//
	// enableNetworkTracking is inserted FIRST. It enables the
	// Network CDP domain and registers a typed-event listener that
	// captures the document response's status code AND tracks
	// in-flight requests for the SPA network-idle fast exit. Without
	// this, the chromedp high-level APIs do not surface the response
	// status — Navigate only signals "load event fired", not "server
	// returned status N". See internal/core/network.go for details.
	//
	// stealthInjectionAction is inserted SECOND. It registers our stealth
	// payload with Page.addScriptToEvaluateOnNewDocument, which makes the
	// browser run the script on every new document it loads — and crucially
	// runs it BEFORE the page's own scripts. Anti-bot systems inspect
	// navigator.webdriver, navigator.plugins and other fingerprinting
	// surfaces during page load; if we patch them after the page's scripts
	// have already run, the bot detection has already fired. The action is
	// best-effort (errors are swallowed) — see internal/core/stealth.go
	// for the trade-off discussion.
	//
	// Navigate loads the URL in the browser tab. We use a custom
	// navigateIgnoringHTTPStatus action instead of chromedp.Navigate
	// because the latter aborts on any non-2xx response (returns
	// "page load error net::ERR_HTTP_RESPONSE_CODE_FAILURE"), which
	// would prevent us from ever capturing 4xx/5xx status codes via
	// the networkTracker. The custom action issues the raw
	// page.Navigate CDP call, ignores the errorText field, and
	// waits for Page.loadEventFired before returning. See
	// internal/core/network.go for the full design rationale.
	//
	// dismissCookieBannersAction is conditionally inserted after Navigate,
	// only in default mode (waitMs == 0). Explicit waitForMs implies the
	// caller is managing timing themselves, so we skip auto-dismiss to
	// avoid surprising them with hidden clicks. The
	// dismiss step itself is best-effort and never aborts the run.
	//
	// The next action is either the SPA readiness poll (default mode) or
	// a single Sleep (explicit waitMs). The poll polls every 200ms for
	// content selectors + body text + optional JS predicate, exiting
	// early on success or timing out at spaTimeout. See
	// internal/core/spa.go:WaitForSPAReady for details.
	//
	// AutoScrollAction is appended after the readiness check (in both
	// default and explicit-waitMs modes) and before the HTML extract.
	// It is best-effort and never aborts the scrape. The default
	// AutoScrollOptions yield 30 steps of 90% viewport scroll, 200ms
	// pause, 3 stagnant-step limit, and eager lazy-image loading. See
	// internal/core/autoscroll.go for the design rationale.
	//
	// Location captures the final URL after any client-side redirects.
	//
	// OuterHTML extracts the serialized HTML of the head and body elements
	// as plain strings (not CDP's compressed JSON encoding).
	//
	// ActionFunc is a catch-all that runs a custom function — here it
	// reads the captured network status from the tracker (set by the
	// listener registered in enableNetworkStatusTracking). If the listener
	// never saw a document response (extremely rare, e.g. browser closed
	// mid-navigation), the tracker returns 200 by default.
	actions := []chromedp.Action{
		enableNetworkTracking(networkBundle),
		// Browser request blocklist. Drops analytics, ads, and
		// trackers at the Fetch CDP domain. Uses the hardcoded
		// globalBlockedPatterns list (32 patterns) defined in
		// internal/core/blocklist.go — the same list the engine
		// ships with. See internal/core/fetch_block.go for the
		// full design.
		fetchBlockAction(tracker),
		// Stealth injection is conditional on the server config. When
		// stealth.enabled=false (the default), the action is a no-op and
		// the Page.addScriptToEvaluateOnNewDocument CDP call is skipped,
		// saving ~30-50ms per request. See internal/core/stealth.go.
		stealthInjectionAction(renderer.cfg.StealthEnabled),
		navigateIgnoringHTTPStatus(rawURL),
		// No-op action that captures the navigate time. We can't time
		// navigateIgnoringHTTPStatus from inside because it's a custom
		// Action, not an ActionFunc — so we let stageStart keep ticking
		// through it and capture the total in this no-op.
		chromedp.ActionFunc(func(ctx context.Context) error {
			stageTimes["navigate"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			return nil
		}),
		// Early anti-bot short-circuit: a single-signal check on the
		// HTTP status captured by the network tracker. 4xx/5xx for
		// the main document strongly suggests a challenge or block
		// page — a real content page very rarely returns 403 for the
		// document itself, and the cost of a false positive ("return
		// a 403 page with a warning") is the same as the unoptimized
		// path.
		//
		// The previous implementation also did an OuterHTML("body")
		// + 250ms sleep here, but both were redundant: the SPA poll
		// already reads the body (via OuterHTML inside runPollTick),
		// and the post-extraction isAntiBotPage check at the bottom
		// of fetchWithCDPBrowser catches any status-200 anti-bot
		// pages we missed. Removing this saves ~50-100ms per
		// request on the common static/SSR case.
		//
		// The block page is still captured in the final OuterHTMLs
		// below and surfaced via Warning.
		chromedp.ActionFunc(func(ctx context.Context) error {
			stageTimes["antiBot"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			if st := networkTracker.Status(); st >= 400 && st < 600 {
				blockDetected = true
				blockWarning = fmt.Sprintf("blocked by anti-bot protection (server returned HTTP %d on initial document; SPA poll and auto-scroll skipped)", st)
			}
			return nil
		}),
	}
	if waitMs == 0 {
		// Use the fast variant: pre-check for known banner selectors
		// (cheap, ~5ms) and skip the full dismiss IIFE (~50-200ms) when
		// no banner is detected. The full script still runs when the
		// pre-check finds a match. See internal/core/cookies.go.
		//
		// We wrap the dismiss action in an ActionFunc so we can capture
		// its wall-clock cost in stageTimes["bannerDismiss"].
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			stageTimes["preNav"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			_ = dismissCookieBannersFastAction().Do(ctx)
			stageTimes["bannerDismiss"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			return nil
		}))
		// Wrap WaitForSPAReady in an ActionFunc so the polling runs
		// inside the same chromedp.Run batch as Navigate and the
		// subsequent OuterHTML extractions — they all share the
		// browser context that Run initializes.
		//
		// The SPA poll is skipped when blockDetected — the page is
		// a challenge, no amount of polling will change that, and
		// the default 15s budget is pure waste.
		//
		// The network activity tracker is passed through so the poll
		// can fast-exit when the network has been idle for 500ms —
		// mirroring the original /v1/scrape renderer's exit at
		// internal/renderer/browser_render_helpers.go:97. For static/
		// SSR pages the load event fires, all assets settle, and
		// the poll returns true on its first tick instead of waiting
		// for the SPA budget.
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			stageTimes["preNav"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			if blockDetected {
				// Mark as timed-out so the post-run warning
				// path (which already exists for the
				// "thin content" case) does not produce a
				// redundant message. The blockWarning set
				// above is the primary signal.
				spaResult = SPAReadinessResult{State: StateTimeout}
				return nil
			}
			res, err := WaitForSPAReady(ctx, SPAReadinessOptions{
				URL:            rawURL,
				Timeout:        spaTimeout,
				NetworkTracker: networkBundle.activity,
				// Selectors, MinBodyText, PollInterval, Predicate
				// left zero — WaitForSPAReady substitutes the
				// original renderer's default selector set
				// ("main, article, [role=main], #content,
				// #root > *, #app > *") and 800-char text
				// threshold when no conditions are configured.
			})
			stageTimes["spaPoll"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			spaResult = res
			return err
		}))
	} else {
		actions = append(actions, chromedp.Sleep(time.Duration(waitMs)*time.Millisecond))
	}
	// AutoScrollAction runs in both default and explicit-waitMs modes
	// so that pages requiring scroll-to-load trigger correctly under
	// both timing models. EndSelector is left empty in v1 (no
	// page-specific sentinel hint); a future ScrapeRequest field can
	// thread a caller-provided selector through to AutoScrollOptions.
	//
	// Skipped on block pages for the same reason as the SPA poll:
	// the challenge page has no lazy-loaded content to scroll
	// into view, and the loop would just hit the stagnant
	// termination after a few steps.
	if !blockDetected {
		actions = append(actions, AutoScrollAction(AutoScrollOptions{
			MaxSteps: 5,
		}))
	}
	// Combined extraction: read the final URL, head HTML, and body
	// HTML in a single JavaScript round-trip. The previous code used
	// three separate chromedp actions (chromedp.Location + two
	// chromedp.OuterHTML calls), each of which is its own CDP
	// command. Combining them into one Evaluate saves ~2 CDP
	// round-trips per request — roughly 30-100ms on the common
	// static/SSR case. The chromedp.Evaluate action returns a
	// three-element JSON array which we unmarshal into the local
	// headHTML/bodyHTML/finalURL variables.
	//
	// We use index 0 (head) and 1 (body) instead of named keys to
	// minimize JSON size over the wire; the return type is declared
	// as []any so the unmarshaler accepts the heterogeneous array.
	actions = append(actions,
		chromedp.ActionFunc(func(ctx context.Context) error {
			stageTimes["preExtract"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			var extracted []any
			if err := chromedp.Evaluate(`
				(() => {
					const head = document.head ? document.head.outerHTML : '';
					const body = document.body ? document.body.outerHTML : '';
					return [head, body, window.location.href];
				})()
			`, &extracted).Do(ctx); err != nil {
				return err
			}
			if len(extracted) >= 1 {
				if s, ok := extracted[0].(string); ok {
					headHTML = s
				}
			}
			if len(extracted) >= 2 {
				if s, ok := extracted[1].(string); ok {
					bodyHTML = s
				}
			}
			if len(extracted) >= 3 {
				if s, ok := extracted[2].(string); ok {
					finalURL = s
				}
			}
			statusCode = uint16(networkTracker.Status())
			stageTimes["extract"] = time.Since(stageStart).Milliseconds()
			stageStart = time.Now()
			return nil
		}),
	)

	// Detect chrome-error://chromewebdata/ — the URL Chrome redirects to
	// when the main document navigation fails (e.g. 4xx/5xx HTTP status,
	// DNS failure, connection refused). We set statusCode to 502 in this
	// case as a fallback when the network tracker didn't capture the
	// real status (which is a known chromedp limitation discussed in
	// internal/core/network.go).
	//
	// The actual HTTP status code from the server (e.g. 404 vs 503) is
	// not available without deeper chromedp/CDP plumbing; 502 is the
	// closest semantic match since it indicates "we got a response but
	// it was an error" without claiming a specific server status.
	if strings.HasPrefix(finalURL, "chrome-error://") && !networkTracker.Seen() {
		statusCode = 502
	}

	runStart := time.Now()
	err := chromedp.Run(runCtx, actions...)
	stageTimes["chromedpRun"] = time.Since(runStart).Milliseconds()
	// Per-stage timing log. Useful for attributing residual gaps to
	// a specific stage (anti-bot check, banner dismiss, SPA poll,
	// extraction). Only shown at debug level (LOG_LEVEL=debug).
	utils.Log.Debug("browser timing",
		"url", rawURL,
		"total_ms", stageTimes["chromedpRun"],
	)

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

	// Step 6: Build the result and check for anti-bot pages.
	result := &FetchResult{
		URL:          rawURL,
		FinalURL:     finalURL,
		StatusCode:   statusCode,
		HTML:         headHTML + bodyHTML,
		RenderedWith: "browser",
		BlockedURLs:  tracker.get(),
	}

	// Step 6: Detect generic anti-bot challenge pages (Cloudflare, CAPTCHA, etc.).
	// This uses simple string matching on page content.
	//
	// blockDetected was set by the early anti-bot check right after
	// Navigate. We prefer the early warning (more diagnostic detail,
	// set before we burned time on the SPA/autoscroll skip) but fall
	// back to the late check as a safety net for any markers the early
	// scan might have missed.
	if blockDetected && blockWarning != "" {
		w := blockWarning
		result.Warning = &w
	} else if isAntiBotPage(result.HTML) {
		w := "blocked by anti-bot protection"
		result.Warning = &w
	}

	// Step 7: Surface a "thin content" warning when the SPA readiness
	// poll timed out. The original renderer emits this when
	// waitForSpaContent returns false after exhausting its budget; we
	// mirror that with StateTimeout from WaitForSPAReady. The warning
	// is non-fatal — the snapshot is still returned, since partial
	// content is usually better than no content.
	if waitMs == 0 && spaResult.State == StateTimeout {
		w := fmt.Sprintf("SPA readiness timeout after %s (text=%d chars, %d polls)",
			spaResult.Duration.Round(100*time.Millisecond),
			spaResult.BodyTextLength,
			spaResult.PollCount)
		result.Warning = &w
	}

	// Diagnostic: log the SPA exit reason for /scrape-core requests
	// when debug is on. Useful to understand which path fired
	// (network-idle, text-threshold, selector match) on static
	// pages — a key signal when comparing /scrape-core to /scrape.
	utils.Log.Debug("SPA exit",
		"state", spaResult.State,
		"selector", spaResult.MatchedSelector,
		"text_len", spaResult.BodyTextLength,
		"polls", spaResult.PollCount,
		"duration", spaResult.Duration.Round(10*time.Millisecond),
	)

	return result, nil
}
