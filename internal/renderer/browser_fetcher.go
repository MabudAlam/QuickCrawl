// Package renderer provides browser-backed page fetching via Chrome DevTools Protocol (CDP).
// It handles JavaScript execution, SPA hydration, anti-bot challenges, and network capture.
package renderer

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// SPA readiness defaults. These mirror the Rust renderer's "selector + text"
// readiness gate closely enough to avoid snapshotting an empty shell.
const (
	// spaContentSelectors is the CSS selector list used to detect meaningful
	// page content in SPAs that don't use traditional full-page loads.
	spaContentSelectors = "main, article, [role=main], #content, #root > *, #app > *"

	// spaSelectorMaxMs is the maximum time to poll for SPA content readiness.
	spaSelectorMaxMs = 15000

	// spaSelectorTickMs is the polling interval when checking for SPA content.
	spaSelectorTickMs = 200

	// spaBodyTextMinChars is the minimum character count in body text required
	// to consider the SPA as having hydrated meaningful content.
	spaBodyTextMinChars = 800

	// contentStabilityMs is how long the page must remain stable before
	// we consider the content fully loaded (no more network requests or DOM mutations).
	contentStabilityMs = 6000
)

// BrowserFetcher fetches pages through a browser backend exposed over CDP.
// It owns the browser-navigation flow, but not browser process management.
// All navigation, header injection, SPA handling, and content extraction
// happens inside Fetch(); the caller receives the final rendered HTML.
type BrowserFetcher struct {
	name        string        // Backend name for diagnostics and API responses.
	wsURL       string        // WebSocket URL of the CDP endpoint.
	browser     *managedBrowser // Reference to the managed browser process.
	stealth     bool          // Whether to inject stealth scripts to mask automation.
	defaultWait time.Duration // Default post-navigation wait before snapshotting.
	pageTimeout time.Duration // Maximum time allowed for full page load.
	poolSem     *semaphore    // Concurrency limiter per origin.
}

// newBrowserPageFetcher creates a browser-backed PageFetcher for one CDP endpoint.
// It initializes a semaphore with capacity 10 to limit concurrent fetches per origin.
func newBrowserPageFetcher(name, wsURL string, browser *managedBrowser, stealth bool) *BrowserFetcher {
	return &BrowserFetcher{
		name:        name,
		wsURL:       wsURL,
		browser:     browser,
		stealth:     stealth,
		defaultWait: 5 * time.Second,
		pageTimeout: 30 * time.Second,
		poolSem:     browserPoolForURL(wsURL, 10),
	}
}

// Name returns the backend name used for diagnostics and API responses.
func (f *BrowserFetcher) Name() string {
	return f.name
}

// SupportsJS reports that a browser backend can execute JavaScript.
// This is always true for BrowserFetcher since it uses a real browser.
func (f *BrowserFetcher) SupportsJS() bool {
	return true
}

// IsAvailable checks whether the configured CDP endpoint is reachable.
// It attempts a short-dial TCP connection to the WebSocket URL.
func (f *BrowserFetcher) IsAvailable() bool {
	if strings.TrimSpace(f.wsURL) == "" {
		return false
	}
	conn, err := dialCDPConnection(f.wsURL, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Fetch loads a page in the browser, waits for it to settle, and returns rendered HTML.
// It performs the full browser flow: target creation, navigation, header setup,
// dynamic content heuristics, and final HTML extraction.
//
// The fetch pipeline is:
//
//	1. Dial CDP WebSocket and create a new browser tab.
//	2. Enable required CDP domains (Page, Runtime, Network).
//	3. Inject stealth scripts and apply caller-supplied headers.
//	4. Start background pumps for network idle detection and response capture.
//	5. Navigate to the target URL and wait for Page.load event.
//	6. Dismiss cookie/CMP banners if no explicit wait was requested.
//	7. Poll for SPA content readiness or wait for explicit duration.
//	8. Capture initial HTML snapshot.
//	9. Auto-scroll to trigger lazy-loaded content if markers detected.
//	10. Auto-click "load more" / "show more" buttons if reveal markers detected.
//	11. Retry on anti-bot challenge pages (up to 3 times with 3s delays).
//	12. Return final HTML with timing breakdown and captured XHR/Fetch responses.
func (f *BrowserFetcher) Fetch(rawURL string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError) {
	// Limit concurrent fetches per origin to avoid overwhelming the browser.
	if f.poolSem != nil {
		f.poolSem.acquire()
		defer f.poolSem.release()
	}

	start := time.Now()
	stageStart := start
	stageTimes := make(map[string]int64)

	// ─── Step 1: Open CDP connection ───────────────────────────────────────────
	conn, err := dialCDPConnection(f.wsURL, 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	defer conn.Close()
	stageTimes["connect"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// ─── Step 2: Create isolated browser context (Chrome only) ──────────────────
	// Firefox does not support browser contexts; all tabs share the default context.
	var browserContextID string
	if strings.EqualFold(f.name, "chrome") {
		browserContextID, err = conn.CreateBrowserContext(10 * time.Second)
		if err != nil {
			return nil, types.ErrRendererError.New(err.Error())
		}
		defer func() {
			if browserContextID != "" {
				_ = conn.DisposeBrowserContext(browserContextID, 5*time.Second)
			}
		}()
	}

	// ─── Step 3: Create a new browser tab and attach a CDP session ──────────────
	targetParams := map[string]any{"url": "about:blank"}
	if browserContextID != "" {
		targetParams["browserContextId"] = browserContextID
	}
	targetRes, err := conn.SendRecv("Target.createTarget", targetParams, "", 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	targetID := utils.ExtractJSONStringField(targetRes, "targetId")
	if targetID == "" {
		return nil, types.ErrRendererError.New("CDP target id missing")
	}
	stageTimes["createTarget"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	attachRes, err := conn.SendRecv("Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "", 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	sessionID := utils.ExtractJSONStringField(attachRes, "sessionId")
	if sessionID == "" {
		return nil, types.ErrRendererError.New("CDP session id missing")
	}
	stageTimes["attachTarget"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// ─── Step 4: Enable required CDP domains ─────────────────────────────────────
	// Page: enables navigation and DOM events.
	// Runtime: enables JavaScript evaluation.
	// Network: enables network monitoring and header overrides.
	commands := []struct {
		method string
		params map[string]any
	}{
		{"Page.enable", map[string]any{}},
		{"Runtime.enable", map[string]any{}},
		{"Network.enable", map[string]any{}},
	}
	for _, cmd := range commands {
		if _, err := conn.SendRecv(cmd.method, cmd.params, sessionID, 10*time.Second); err != nil {
			_ = conn.CloseTarget(targetID, 5*time.Second)
			return nil, types.ErrRendererError.New(err.Error())
		}
	}
	stageTimes["enableDomains"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// ─── Step 5: Inject stealth helpers ─────────────────────────────────────────
	// These scripts run before any page scripts to mask automation signatures
	// (webdriver flag, navigator properties, etc.).
	_, _ = conn.SendRecv("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": stealthScript()}, sessionID, 10*time.Second)

	// ─── Step 6: Apply caller-supplied headers ────────────────────────────────────
	// User-Agent is set via setUserAgentOverride to affect all requests.
	// All other headers are set via setExtraHTTPHeaders for the navigation request.
	if ua, ok := headers["User-Agent"]; ok && ua != "" {
		_, _ = conn.SendRecv("Network.setUserAgentOverride", map[string]any{"userAgent": ua}, sessionID, 10*time.Second)
	}

	var extra map[string]string
	if len(headers) > 0 {
		extra = make(map[string]string, len(headers))
		for k, v := range headers {
			if strings.EqualFold(k, "User-Agent") {
				continue
			}
			extra[k] = v
		}
		if len(extra) > 0 {
			_, _ = conn.SendRecv("Network.setExtraHTTPHeaders", map[string]any{"headers": extra}, sessionID, 10*time.Second)
		}
	}
	stageTimes["setupHeaders"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// ─── Step 7: Start background CDP event pumps ────────────────────────────────
	// These run for the lifetime of the fetch and handle:
	//   - Network idle detection: triggers when all pending requests settle.
	//   - Network response capture: stores XHR/Fetch responses with JSON bodies.
	//   - Request interception: blocks tracking/ads hosts and resource types.
	//   - Auth handling: responds to HTTP auth challenges (currently disabled).
	pageEvents := conn.Subscribe()
	idleEvents := conn.Subscribe()
	captureEvents := conn.Subscribe()
	interceptEvents := conn.Subscribe()
	authEvents := conn.Subscribe()
	networkTracker := newNetworkActivityTracker()
	done := make(chan struct{})
	defer close(done)

	go runNetworkIdlePump(idleEvents, sessionID, networkTracker, done)

	var capturedResponses []capturedNetworkResponse
	var capturedMu sync.Mutex
	go runNetworkCapturePump(conn, captureEvents, sessionID, &capturedResponses, &capturedMu, done)

	// Request interception and auth handling are compiled in but disabled by default.
	// They can be enabled by setting interceptEnabled or authEnabled to true.
	// Blocking is useful on sites with noisy third-party scripts; auth is useful
	// on sites behind HTTP Basic/Digest auth.
	interceptEnabled := false
	authEnabled := false
	if interceptEnabled || authEnabled {
		enableParams := map[string]any{
			"patterns": []map[string]string{},
		}
		if interceptEnabled {
			enableParams["patterns"] = []map[string]string{{"urlPattern": "*"}}
		}
		if authEnabled {
			enableParams["handleAuthRequests"] = true
		}
		if _, err := conn.SendRecv("Fetch.enable", enableParams, sessionID, 10*time.Second); err != nil {
			return nil, types.ErrRendererError.New(err.Error())
		}
		defer func() {
			_, _ = conn.SendRecv("Fetch.disable", map[string]any{}, sessionID, 5*time.Second)
		}()
		if interceptEnabled {
			blocklist := newDefaultRequestBlocklist()
			go runFetchInterceptionPump(conn, interceptEvents, sessionID, blocklist, done)
		}
		if authEnabled {
			go runFetchAuthPump(conn, authEvents, sessionID, nil, done)
		}
	}

	// ─── Step 8: Navigate to target URL ─────────────────────────────────────────
	if _, err := conn.SendRecv("Page.navigate", map[string]any{"url": rawURL}, sessionID, 10*time.Second); err != nil {
		_ = conn.CloseTarget(targetID, 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}

	// Extend timeout if caller requested extra wait time for JS-heavy pages.
	loadTimeout := f.pageTimeout
	if waitForMs != nil && *waitForMs > 0 {
		loadTimeout += time.Duration(*waitForMs) * time.Millisecond
	}

	statusCode, err := conn.WaitForPageReady(pageEvents, sessionID, loadTimeout)
	if err != nil {
		_ = conn.CloseTarget(targetID, 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}
	stageTimes["navigate+wait"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	wait := f.defaultWait
	if waitForMs != nil && *waitForMs > 0 {
		wait = time.Duration(*waitForMs) * time.Millisecond
	}

	// ─── Step 9: Dismiss CMP / cookie banners ───────────────────────────────────
	// CMP banners can hide page content and inflate body.innerText past the SPA
	// readiness threshold. We only auto-dismiss when no explicit wait was given,
	// since explicit waits imply the caller is managing timing themselves.
	cmpDismissTime := int64(0)
	if waitForMs == nil && !detectAntiBotChallengePage("") {
		cmpDismissStart := time.Now()
		_, _ = conn.SendRecv("Runtime.evaluate", map[string]any{
			"expression":            cookieBannerDismissScript(),
			"awaitPromise":          true,
			"includeCommandLineAPI": true,
		}, sessionID, 2*time.Second)
		cmpDismissTime = time.Since(cmpDismissStart).Milliseconds()
	}
	stageTimes["cmpDismiss"] = cmpDismissTime

	// ─── Step 10: Wait for SPA content or explicit duration ────────────────────
	// If an explicit wait time was given, sleep for that duration.
	// Otherwise, poll for SPA content readiness (selector + text threshold).
	if waitForMs == nil {
		waitReady := waitForSpaContent(conn, sessionID, time.Duration(spaSelectorMaxMs)*time.Millisecond, networkTracker)
		if !waitReady {
			log.Printf("[browser_fetcher] SPA selector poll exhausted, waiting additional 3s for hydration")
			time.Sleep(3 * time.Second)
		}
	} else {
		time.Sleep(wait)
	}

	// ─── Step 11: First HTML snapshot ───────────────────────────────────────────
	initialHTML, err := readRenderedHTMLWithShadowDOM(conn, sessionID)
	if err != nil {
		_ = conn.CloseTarget(targetID, 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}
	html := initialHTML

	// If the snapshot looks like a loading placeholder, wait for content to stabilize.
	if waitForMs == nil && detectLoadingPlaceholder(html) {
		if stable, stableErr := waitForPageContentToStabilizeWithShadowDOM(conn, sessionID, f.pageTimeout); stableErr == nil && stable != "" {
			html = stable
		}
	}

	autoScrollTime, revealTime := int64(0), int64(0)

	// ─── Step 12: Auto-scroll for lazy-loaded content ────────────────────────────
	// Scroll to the bottom of the page to trigger intersection-observer-based
	// lazy loading. Only runs when lazy-load markers are present and the page
	// is reasonably small (<200KB), to avoid scrolling megabytes of hidden content.
	if waitForMs == nil && !detectAntiBotChallengePage(html) && !detectLoadingPlaceholder(html) {
		hasLazyMarkers := strings.Contains(html, `loading="lazy"`) ||
			strings.Contains(html, "data-src=") ||
			strings.Contains(html, "infinite-scroll") ||
			strings.Contains(html, "lazy-load")

		if hasLazyMarkers && len(html) < 200000 {
			autoScrollStart := time.Now()
			_, _ = conn.SendRecv("Runtime.evaluate", map[string]any{
				"expression":            autoScrollScript(),
				"awaitPromise":          true,
				"includeCommandLineAPI": true,
			}, sessionID, 5*time.Second)
			time.Sleep(500 * time.Millisecond)
			autoScrollTime = time.Since(autoScrollStart).Milliseconds()

			// Re-snapshot after scrolling to capture lazy-loaded images/iframes.
			if scrolledHTML, scrolledErr := readRenderedHTMLWithShadowDOM(conn, sessionID); scrolledErr == nil && scrolledHTML != "" {
				html = scrolledHTML
			}
		}
	}

	// ─── Step 13: Auto-click "load more" / "show more" / "reveal" buttons ───────
	// Many sites hide long-form content behind disclosure buttons. We detect
	// common aria-expanded patterns and CSS class names and click them.
	if waitForMs == nil && !detectAntiBotChallengePage(html) && !detectLoadingPlaceholder(html) {
		hasRevealMarkers := strings.Contains(html, `aria-expanded="false"`) ||
			strings.Contains(html, "load-more") ||
			strings.Contains(html, "show-more") ||
			strings.Contains(strings.ToLower(html), ">load more<") ||
			strings.Contains(strings.ToLower(html), ">show more<") ||
			strings.Contains(strings.ToLower(html), ">read more<") ||
			strings.Contains(strings.ToLower(html), ">show full<") ||
			strings.Contains(strings.ToLower(html), ">view all<")

		if hasRevealMarkers && len(html) < 200000 {
			revealStart := time.Now()
			_, _ = conn.SendRecv("Runtime.evaluate", map[string]any{
				"expression":            autoClickRevealScript(),
				"awaitPromise":          true,
				"includeCommandLineAPI": true,
			}, sessionID, 5*time.Second)
			time.Sleep(500 * time.Millisecond)
			revealTime = time.Since(revealStart).Milliseconds()

			// Re-snapshot after clicking to capture revealed content.
			if revealedHTML, revealedErr := readRenderedHTMLWithShadowDOM(conn, sessionID); revealedErr == nil && revealedHTML != "" {
				html = revealedHTML
			}
		}
	}

	// ─── Step 14: Anti-bot challenge retry loop ───────────────────────────────────
	// Some sites show a challenge page (CAPTCHA, JS challenge, etc.) before
	// rendering actual content. Retry up to 3 times with 3s delays to wait
	// for the challenge to resolve (e.g., user solving a CAPTCHA, challenge
	// timing out, or JavaScript challenge completing).
	challengeRetryTime := int64(0)
	if detectAntiBotChallengePage(html) {
		for attempt := 1; attempt <= 3; attempt++ {
			challengeStart := time.Now()
			time.Sleep(3 * time.Second)
			retryHTML, retryErr := readRenderedHTMLWithShadowDOM(conn, sessionID)
			if retryErr != nil {
				_ = conn.CloseTarget(targetID, 5*time.Second)
				return nil, types.ErrRendererError.New(retryErr.Error())
			}
			html = retryHTML
			challengeRetryTime += time.Since(challengeStart).Milliseconds()
			if !detectAntiBotChallengePage(html) {
				break
			}
		}
	}

	stageTimes["autoScroll"] = autoScrollTime
	stageTimes["reveal"] = revealTime
	stageTimes["challengeRetry"] = challengeRetryTime
	stageStart = time.Now()

	// ─── Step 15: Final validation and cleanup ───────────────────────────────────
	if html == "" {
		_ = conn.CloseTarget(targetID, 5*time.Second)
		return nil, types.ErrRendererError.New("Empty HTML from CDP renderer")
	}

	_ = conn.CloseTarget(targetID, 5*time.Second)
	stageTimes["closeTarget"] = time.Since(stageStart).Milliseconds()

	totalTime := time.Since(start).Milliseconds()
	log.Printf("[browser_fetcher] Fetch timing breakdown for %s (total=%dms): %+v", rawURL, totalTime, stageTimes)

	renderedMethod := f.name
	return &types.FetchResult{
		URL:               rawURL,
		StatusCode:        statusCode,
		HTML:              html,
		ContentType:       ptr("text/html"),
		RawBytes:          nil,
		RenderedWith:      &renderedMethod,
		TimeTaken:         uint64(totalTime),
		CapturedResponses: toCapturedNetworkResponses(capturedResponses),
	}, nil
}
