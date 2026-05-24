package renderer

import (
	"log"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// FallbackRenderer is the main rendering orchestrator that coordinates multiple
// fetch strategies. It first tries HTTP, then optionally escalates to browser-based
// rendering (Chrome DevTools Protocol) when JavaScript is needed or anti-bot
// challenges are detected.
type FallbackRenderer struct {
	http            *HTTPFetcher  // HTTP fetcher (always available)
	jsRenderers     []PageFetcher // JavaScript-capable renderers (browser backends)
	renderJSDefault *bool         // Default JavaScript rendering setting
	pageTimeoutMs   int64         // Page load timeout in milliseconds
	poolSize        int           // Browser pool size
	cleanup         func()        // Cleanup function for terminating browser processes
}

// NewFallbackRenderer creates a new FallbackRenderer with default settings.
// This is a convenience constructor that uses nil configuration and passes
// NewFallbackRenderer creates a FallbackRenderer with explicit configuration.
// It initializes the HTTP fetcher and optionally configures browser-based fetchers
// based on the configured mode (auto, chrome, lightpanda, or none).
// Browser mode is determined by RENDERER__MODE env var or renderer config.
func NewFallbackRenderer(
	userAgent string,
	stealth *types.StealthConfig,
	renderJSDefault *bool,
) (*FallbackRenderer, *types.QuickCrawlError) {
	return NewFallbackRendererWithConfig(nil, userAgent, stealth, renderJSDefault)
}

// NewFallbackRendererWithConfig creates a FallbackRenderer with explicit configuration.
// It initializes the HTTP fetcher and optionally configures browser-based fetchers
// based on the configured mode (auto, chrome, lightpanda, or none).
// Browser mode is determined by RENDERER__MODE env var or renderer config.
func NewFallbackRendererWithConfig(
	rendererCfg *types.RendererConfig,
	userAgent string,
	stealth *types.StealthConfig,
	renderJSDefault *bool,
) (*FallbackRenderer, *types.QuickCrawlError) {
	// Initialize HTTP fetcher with optional header injection for stealth
	injectHeaders := stealth != nil && stealth.InjectHeaders
	httpFetcher := NewHTTPFetcher(userAgent, injectHeaders)

	var jsRenderers []PageFetcher
	var cleanup func()

	// Load renderer configuration with defaults
	pageTimeoutMs := int64(30000)
	poolSize := 4
	browserMode := "auto"
	if rendererCfg != nil {
		browserMode = strings.ToLower(strings.TrimSpace(string(rendererCfg.Mode)))
		pageTimeoutMs = rendererCfg.PageTimeoutMs
		poolSize = rendererCfg.PoolSize
	}

	// Determine configured WebSocket URLs for each browser type
	stealthEnabled := stealth != nil && stealth.Enabled
	lightpandaWSURL := getLightPandaWSURL(rendererCfg)
	chromeWSURL := getChromeWSURL(rendererCfg)

	// Initialize browser-based renderers based on mode
	if browserMode != "none" {
		switch browserMode {
		case "lightpanda":
			jsRenderers = initializeLightPandaRenderer(lightpandaWSURL, stealthEnabled, &cleanup)

		case "chrome":
			jsRenderers = initializeChromeRenderer(rendererCfg, chromeWSURL, stealthEnabled, &cleanup)

		default: // "auto" mode - try all available browsers
			jsRenderers = initializeAutoRenderers(rendererCfg, lightpandaWSURL, chromeWSURL, stealthEnabled, &cleanup)
		}
	}

	return &FallbackRenderer{
		http:            httpFetcher,
		jsRenderers:     jsRenderers,
		renderJSDefault: renderJSDefault,
		pageTimeoutMs:   pageTimeoutMs,
		poolSize:        poolSize,
		cleanup:         cleanup,
	}, nil
}

// initializeLightPandaRenderer sets up the LightPanda browser fetcher.
// If a WebSocket URL is provided, it connects to that endpoint. Otherwise,
// it attempts to find or download the LightPanda binary and launch it natively.
func initializeLightPandaRenderer(wsURL string, stealthEnabled bool, cleanup *func()) []PageFetcher {
	if wsURL != "" {
		return append([]PageFetcher{}, newBrowserFetcher("lightpanda", wsURL, nil, stealthEnabled))
	}

	// Try to launch LightPanda natively (auto-download/find binary)
	browser, err := tryLightPandaNative()
	if err != nil {
		return nil
	}
	*cleanup = func() { browser.Close() }
	return append([]PageFetcher{}, newBrowserFetcher("lightpanda", browser.wsURL, browser, stealthEnabled))
}

// initializeChromeRenderer sets up the Chrome browser fetcher.
// If a WebSocket URL is provided, it connects to that endpoint. Otherwise,
// it attempts to find the Chrome binary and launch it with remote debugging.
func initializeChromeRenderer(rendererCfg *types.RendererConfig, wsURL string, stealthEnabled bool, cleanup *func()) []PageFetcher {
	if wsURL != "" {
		return append([]PageFetcher{}, newBrowserFetcher("chrome", wsURL, nil, stealthEnabled))
	}

	// Try to launch Chrome natively with DevTools debugging enabled
	browser, err := tryChromeNative(rendererCfg)
	if err != nil {
		return nil
	}
	*cleanup = func() { browser.Close() }
	return append([]PageFetcher{}, newBrowserFetcher("chrome", browser.wsURL, browser, stealthEnabled))
}

// initializeAutoRenderers sets up all available browser renderers in auto mode.
// It first checks for pre-configured WebSocket URLs, then falls back to
// launching browsers natively if no URLs are configured.
func initializeAutoRenderers(rendererCfg *types.RendererConfig, lightpandaWSURL, chromeWSURL string, stealthEnabled bool, cleanup *func()) []PageFetcher {
	var fetchers []PageFetcher

	// First, try to use pre-configured WebSocket URLs
	if lightpandaWSURL != "" {
		fetchers = append(fetchers, newBrowserFetcher("lightpanda", lightpandaWSURL, nil, stealthEnabled))
	}
	if chromeWSURL != "" {
		fetchers = append(fetchers, newBrowserFetcher("chrome", chromeWSURL, nil, stealthEnabled))
	}

	// If no WebSocket URLs configured, launch browsers natively
	if len(fetchers) == 0 {
		browsers, err := launchAllBrowsers(rendererCfg)
		if err != nil || len(browsers) == 0 {
			return nil
		}
		for _, browser := range browsers {
			fetchers = append(fetchers, newBrowserFetcher(browser.Name(), browser.wsURL, browser, stealthEnabled))
		}
		// Set cleanup to close all launched browsers
		*cleanup = func() {
			for _, browser := range browsers {
				browser.Close()
			}
		}
	}

	return fetchers
}

// Name returns the renderer name for debugging/logging purposes.
func (r *FallbackRenderer) Name() string {
	return "FallbackRenderer"
}

// JSRendererNames returns the configured browser renderer names in fallback order.
func (r *FallbackRenderer) JSRendererNames() []string {
	names := make([]string, 0, len(r.jsRenderers))
	for _, js := range r.jsRenderers {
		names = append(names, js.Name())
	}
	return names
}

// Fetch retrieves a URL using the most appropriate rendering strategy.
// It first tries HTTP, then optionally escalates to browser rendering based on:
// - The renderJS parameter (if non-nil, forces rendering decision)
// - Auto-detection of SPA pages that need JavaScript
// - Detection of anti-bot challenge pages (Cloudflare, CAPTCHA, etc.)
// - HTTP 401/403 responses indicating authentication requirements
//
// If preferredBrowser is specified, only that browser type will be used.
func (r *FallbackRenderer) Fetch(
	rawURL string,
	headers map[string]string,
	renderJS *bool,
	waitForMs *int64,
	preferredBrowser *string,
) (*types.FetchResult, *types.QuickCrawlError) {
	// Resolve effective renderJS setting (request override or default)
	effective := types.ResolveRenderJS(renderJS, r.renderJSDefault)
	if effective == nil && preferredBrowser != nil && *preferredBrowser != "" {
		forceJS := true
		effective = &forceJS
	}

	// Explicit JS disabled - use HTTP only
	if effective != nil && !*effective {
		return r.http.Fetch(rawURL, headers, waitForMs)
	}

	// Try HTTP first (fastest path, works for most static pages)
	result, err := r.http.Fetch(rawURL, headers, waitForMs)
	if err != nil {
		return nil, err
	}

	// Detect if JavaScript rendering is needed
	needsJS := detectNeedsJS(result.HTML)
	isBlocked := detectBlockInterstitial(result.HTML)
	isAuthBlocked := result.StatusCode == 401 || result.StatusCode == 403

	// Determine if we need to use browser rendering
	useBrowser := effective == nil && (needsJS || isBlocked || isAuthBlocked) ||
		(effective != nil && *effective)

	if useBrowser && len(r.jsRenderers) > 0 {
		log.Printf("[renderer] using browser rendering: url=%s needs_js=%v is_blocked=%v auth_blocked=%v status=%d",
			rawURL, needsJS, isBlocked, isAuthBlocked, result.StatusCode)
		return r.fetchWithBrowser(rawURL, headers, waitForMs, result, preferredBrowser)
	}

	// Add warnings for detected issues when we can't use browser
	if needsJS || isBlocked || isAuthBlocked {
		log.Printf("[renderer] HTTP result blocked: url=%s needs_js=%v is_blocked=%v auth_blocked=%v status=%d warning=%v",
			rawURL, needsJS, isBlocked, isAuthBlocked, result.StatusCode, result.Warning)
		result = appendRenderingWarning(result, needsJS, isBlocked, isAuthBlocked)
	}

	log.Printf("[renderer] using HTTP fetcher: url=%s status=%d", rawURL, result.StatusCode)
	return result, nil
}

// fetchWithBrowser attempts to fetch the URL using browser-based renderers.
// It tries each configured browser (optionally filtered by preferredBrowser)
// and returns the first good result. If all browsers fail, it returns the
// best available result with an appropriate warning.
func (r *FallbackRenderer) fetchWithBrowser(
	rawURL string,
	headers map[string]string,
	waitForMs *int64,
	httpResult *types.FetchResult,
	preferredBrowser *string,
) (*types.FetchResult, *types.QuickCrawlError) {
	headers = copyHeaders(headers)
	var thinResult *types.FetchResult

	// Filter to preferred browser if specified
	var renderers []PageFetcher
	if preferredBrowser != nil && *preferredBrowser != "" {
		for _, js := range r.jsRenderers {
			if js.Name() == *preferredBrowser {
				renderers = append(renderers, js)
			}
		}
		// Pinned renderer must be available; do not silently fall back to HTTP.
		if len(renderers) == 0 {
			return nil, types.ErrRendererError.New("preferred renderer '" + *preferredBrowser + "' not available")
		}
	} else {
		renderers = r.jsRenderers
	}

	// Try each browser renderer until one succeeds with good content
	for _, js := range renderers {
		log.Printf("[renderer] trying browser: browser=%s url=%s", js.Name(), rawURL)
		browserResult, browserErr := js.Fetch(rawURL, headers, waitForMs)
		if browserErr != nil || browserResult == nil {
			log.Printf("[renderer] browser fetch failed: browser=%s url=%s error=%v", js.Name(), rawURL, browserErr)
			continue
		}

		// Check if result is still problematic (still needs JS, still blocked, or thin content)
		stillNeedsJS := detectNeedsJS(browserResult.HTML)
		stillBlocked := detectBlockInterstitial(browserResult.HTML)
		hasCrash := detectClientSideCrash(browserResult.HTML)
		isThin := hasMinimalContent(browserResult.HTML)

		if stillNeedsJS || stillBlocked || hasCrash || isThin {
			log.Printf("[renderer] browser result not good enough: browser=%s url=%s still_needs_js=%v still_blocked=%v has_crash=%v is_thin=%v",
				js.Name(), rawURL, stillNeedsJS, stillBlocked, hasCrash, isThin)
			if thinResult == nil {
				thinResult = browserResult
			}
			continue
		}

		// Good result - add warning if needed for incomplete SPA/bot detection
		if browserResult.Warning == nil {
			if stillNeedsJS {
				warning := "SPA detected but JS rendering still looks incomplete"
				browserResult.Warning = &warning
			} else if stillBlocked {
				warning := "Anti-bot challenge detected"
				browserResult.Warning = &warning
			}
		}
		log.Printf("[renderer] browser rendering successful: browser=%s url=%s status=%d", js.Name(), rawURL, browserResult.StatusCode)
		return browserResult, nil
	}

	// All browsers failed or returned thin content, return best available
	if thinResult != nil {
		if httpResult != nil && contentTextLength(httpResult.HTML) > contentTextLength(thinResult.HTML) {
			if preferredBrowser != nil && *preferredBrowser != "" {
				warning := "preferred renderer returned low-quality content"
				thinResult.Warning = &warning
				return thinResult, nil
			}
			warning := "JS rendering returned low-quality browser content; returned richer HTTP result"
			httpResult.Warning = &warning
			return httpResult, nil
		}
		warning := "JS rendering returned thin content; falling back to best available browser result"
		thinResult.Warning = &warning
		return thinResult, nil
	}

	// All browsers failed, return HTTP result with warning
	if preferredBrowser != nil && *preferredBrowser != "" {
		return nil, types.ErrRendererError.New("preferred renderer '" + *preferredBrowser + "' failed")
	}
	warning := "JS rendering requested but browser backend failed; returned HTTP result"
	if httpResult.Warning != nil {
		warning = *httpResult.Warning + "; JS rendering backend failed"
	}
	httpResult.Warning = &warning
	return httpResult, nil
}

// appendRenderingWarning adds a warning message to the FetchResult when
// issues are detected but browser rendering is not available.
func appendRenderingWarning(result *types.FetchResult, needsJS, isBlocked, isAuthBlocked bool) *types.FetchResult {
	var warning string
	if needsJS {
		warning = "SPA detected but JS rendering not available"
	} else if isBlocked {
		warning = "Anti-bot challenge detected"
	} else if isAuthBlocked {
		warning = "Auth blocked"
	}

	if result.Warning != nil {
		*result.Warning = *result.Warning + "; " + warning
	} else {
		result.Warning = &warning
	}
	return result
}

// Close releases all resources held by the renderer, including terminating
// any browser processes that were launched natively.
func (r *FallbackRenderer) Close() {
	if r.cleanup != nil {
		r.cleanup()
	}
}

// CheckHealth returns the availability status of all fetchers.
// The map keys are fetcher names ("http", "chrome", "lightpanda")
// and values indicate whether each fetcher is currently available.
func (r *FallbackRenderer) CheckHealth() map[string]bool {
	status := map[string]bool{
		"http": r.http.IsAvailable(),
	}
	for _, js := range r.jsRenderers {
		status[js.Name()] = js.IsAvailable()
	}
	return status
}

// BrowsersInfo returns information about currently running browser instances.
// Each BrowserInfo contains the browser name and its WebSocket endpoint URL.
func (r *FallbackRenderer) BrowsersInfo() []BrowserInfo {
	var info []BrowserInfo
	for _, j := range r.jsRenderers {
		info = append(info, BrowserInfo{
			Name:  j.Name(),
			WSURL: j.(*BrowserFetcher).wsURL,
		})
	}
	return info
}

// getLightPandaWSURL retrieves the configured LightPanda WebSocket URL
// from the renderer configuration.
func getLightPandaWSURL(rendererCfg *types.RendererConfig) string {
	if rendererCfg != nil && rendererCfg.Lightpanda != nil {
		return strings.TrimSpace(rendererCfg.Lightpanda.WSURL)
	}
	return ""
}

// getChromeWSURL retrieves the configured Chrome WebSocket URL
// from the renderer configuration.
func getChromeWSURL(rendererCfg *types.RendererConfig) string {
	if rendererCfg != nil && rendererCfg.Chrome != nil {
		return strings.TrimSpace(rendererCfg.Chrome.WSURL)
	}
	return ""
}
