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
	spaContentSelectors = "main, article, [role=main], #content, #root > *, #app > *"
	spaSelectorMaxMs    = 15000
	spaSelectorTickMs   = 200
	spaBodyTextMinChars = 800
	contentStabilityMs  = 6000 // How long to wait for content to stabilize
)

// BrowserFetcher fetches pages through a browser backend exposed over CDP.
// It owns the browser-navigation flow, but not browser process management.
type BrowserFetcher struct {
	name        string
	wsURL       string
	browser     *managedBrowser
	stealth     bool
	defaultWait time.Duration
	pageTimeout time.Duration
	poolSem     *semaphore
}

// newBrowserPageFetcher creates a browser-backed PageFetcher for one CDP endpoint.
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
func (f *BrowserFetcher) SupportsJS() bool {
	return true
}

// IsAvailable checks whether the configured CDP endpoint is reachable.
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
func (f *BrowserFetcher) Fetch(rawURL string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError) {
	if f.poolSem != nil {
		f.poolSem.acquire()
		defer f.poolSem.release()
	}

	start := time.Now()
	stageStart := start
	stageTimes := make(map[string]int64)

	// Open a fresh CDP connection for this fetch.
	conn, err := dialCDPConnection(f.wsURL, 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	defer conn.Close()
	stageTimes["connect"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

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

	// Create a new browser tab and attach a CDP session to it.
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

	// Enable the CDP domains needed for navigation, DOM access, and header overrides.
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

	// Inject stealth helpers before the page's own scripts run.
	_, _ = conn.SendRecv("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": stealthScript()}, sessionID, 10*time.Second)

	// Allow the caller to override the User-Agent while keeping other headers intact.
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

	// behavior on sites that rely on blocked third-party scripts or assets.
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

	if _, err := conn.SendRecv("Page.navigate", map[string]any{"url": rawURL}, sessionID, 10*time.Second); err != nil {
		_ = conn.CloseTarget(targetID, 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}

	loadTimeout := f.pageTimeout
	// Give the page extra time to settle if the caller requested a wait.
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

	// 1. Dismiss CMP banners BEFORE waiting for content
	// (CMP banners can hide content and inflate body.innerText past SPA threshold)
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

	if waitForMs == nil {
		waitReady := waitForSpaContent(conn, sessionID, time.Duration(spaSelectorMaxMs)*time.Millisecond, networkTracker)
		if !waitReady {
			log.Printf("[browser_fetcher] SPA selector poll exhausted, waiting additional 3s for hydration")
			time.Sleep(3 * time.Second)
		}
	} else {
		time.Sleep(wait)
	}

	// 3. Capture initial HTML
	initialHTML, err := readRenderedHTMLWithShadowDOM(conn, sessionID)
	if err != nil {
		_ = conn.CloseTarget(targetID, 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}
	html := initialHTML

	if waitForMs == nil && detectLoadingPlaceholder(html) {
		if stable, stableErr := waitForPageContentToStabilizeWithShadowDOM(conn, sessionID, f.pageTimeout); stableErr == nil && stable != "" {
			html = stable
		}
	}

	autoScrollTime, revealTime := int64(0), int64(0)

	// 5. Auto-scroll for lazy-loaded content (only if explicit lazy markers AND reasonable page size)
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
			// Re-snapshot after scrolling
			if scrolledHTML, scrolledErr := readRenderedHTMLWithShadowDOM(conn, sessionID); scrolledErr == nil && scrolledHTML != "" {
				html = scrolledHTML
			}
		}
	}

	// 6. Auto-click reveal for "load more" patterns
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
			// Re-snapshot after clicking
			if revealedHTML, revealedErr := readRenderedHTMLWithShadowDOM(conn, sessionID); revealedErr == nil && revealedHTML != "" {
				html = revealedHTML
			}
		}
	}

	// 7. Challenge retry loop - wait for anti-bot challenges to resolve
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

	// Empty HTML is always a renderer failure.
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
