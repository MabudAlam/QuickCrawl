package renderer

import (
	"log"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// FallbackRenderer is the main rendering orchestrator that coordinates multiple
// fetch strategies using a chain pattern:
//   - renderJs=false: HTTP only
//   - renderJs=true: HTTP → Lightpanda → Chrome (all, pick best)
//   - renderJs=auto: HTTP first, then chain to browsers only if needed
type FallbackRenderer struct {
	http            *HTTPFetcher  // HTTP fetcher (always available)
	jsRenderers     []PageFetcher // JavaScript-capable renderers (browser backends)
	renderJSDefault *bool         // Default JavaScript rendering setting
	pageTimeoutMs   int64         // Page load timeout in milliseconds
	poolSize        int           // Browser pool size
	cleanup         func()        // Cleanup function for terminating browser processes
}

// fetchChainItem represents a single step in the rendering chain
type fetchChainItem struct {
	name    string      // Name for logging
	fetcher PageFetcher // The actual fetcher (HTTP or browser)
}

// NewFallbackRenderer creates a renderer with the default browser configuration.
func NewFallbackRenderer(
	userAgent string,
	stealth *types.StealthConfig,
	renderJSDefault *bool,
) (*FallbackRenderer, *types.QuickCrawlError) {
	return NewFallbackRendererWithConfig(nil, userAgent, stealth, renderJSDefault)
}

// NewFallbackRendererWithConfig creates a renderer using explicit renderer settings.
func NewFallbackRendererWithConfig(
	rendererCfg *types.RendererConfig,
	userAgent string,
	stealth *types.StealthConfig,
	renderJSDefault *bool,
) (*FallbackRenderer, *types.QuickCrawlError) {
	// Determine if stealth headers should be injected
	var stealthProfile *utils.HeaderProfile
	stealthEnabled := stealth != nil && stealth.Enabled && stealth.InjectHeaders
	if stealthEnabled {
		// Get a header profile based on the configured strategy (modern_browser, mobile_device, bot_friendly)
		profile := utils.GetHeaderProfile(utils.HeaderStrategy(stealth.Strategy))
		stealthProfile = &profile
	}

	// Initialize HTTP fetcher with stealth profile (nil if stealth is disabled)
	httpFetcher := NewHTTPFetcher(userAgent, stealthProfile)

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
	stealthEnabled = stealth != nil && stealth.Enabled
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
func initializeLightPandaRenderer(wsURL string, stealthEnabled bool, cleanup *func()) []PageFetcher {
	if wsURL != "" {
		return append([]PageFetcher{}, newBrowserPageFetcher("lightpanda", wsURL, nil, stealthEnabled))
	}

	// Launch LightPanda locally only when no WS endpoint is configured.
	browser, err := startLightPandaBrowser()
	if err != nil {
		return nil
	}
	*cleanup = func() { browser.Close() }
	return append([]PageFetcher{}, newBrowserPageFetcher("lightpanda", browser.wsURL, browser, stealthEnabled))
}

// initializeChromeRenderer sets up the Chrome browser fetcher.
func initializeChromeRenderer(rendererCfg *types.RendererConfig, wsURL string, stealthEnabled bool, cleanup *func()) []PageFetcher {
	if wsURL != "" {
		return append([]PageFetcher{}, newBrowserPageFetcher("chrome", wsURL, nil, stealthEnabled))
	}

	// Launch Chrome locally only when no WS endpoint is configured.
	browser, err := startChromeBrowser(rendererCfg)
	if err != nil {
		return nil
	}
	*cleanup = func() { browser.Close() }
	return append([]PageFetcher{}, newBrowserPageFetcher("chrome", browser.wsURL, browser, stealthEnabled))
}

// initializeAutoRenderers sets up all available browser renderers in auto mode.
// It first checks for pre-configured WebSocket URLs, then falls back to
// launching browsers natively if no URLs are configured.
func initializeAutoRenderers(rendererCfg *types.RendererConfig, lightpandaWSURL, chromeWSURL string, stealthEnabled bool, cleanup *func()) []PageFetcher {
	var fetchers []PageFetcher

	// First, try to use pre-configured WebSocket URLs
	if lightpandaWSURL != "" {
		fetchers = append(fetchers, newBrowserPageFetcher("lightpanda", lightpandaWSURL, nil, stealthEnabled))
	}
	if chromeWSURL != "" {
		fetchers = append(fetchers, newBrowserPageFetcher("chrome", chromeWSURL, nil, stealthEnabled))
	}

	// If no WebSocket URLs configured, launch browsers natively
	if len(fetchers) == 0 {
		browsers, err := launchAvailableBrowserBackends(rendererCfg)
		if err != nil || len(browsers) == 0 {
			return nil
		}
		for _, browser := range browsers {
			fetchers = append(fetchers, newBrowserPageFetcher(browser.Name(), browser.wsURL, browser, stealthEnabled))
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

// Fetch retrieves a URL using a chain pattern:
//   - renderJs=false: HTTP only
//   - renderJs=true: HTTP → Lightpanda → Chrome (all in chain, pick best)
//   - renderJs=auto: HTTP first, chain to browser only if HTTP result is bad
//
// If preferredBrowser is specified, only that browser type will be used.
func (r *FallbackRenderer) Fetch(
	rawURL string,
	headers map[string]string,
	renderJS *bool,
	waitForMs *int64,
	preferredBrowser *string,
) (*types.FetchResult, *types.QuickCrawlError) {
	// Resolve effective renderJS setting
	effective := types.ResolveRenderJS(renderJS, r.renderJSDefault)
	if effective == nil && preferredBrowser != nil && *preferredBrowser != "" {
		forceJS := true
		effective = &forceJS
	}

	// Explicit JS disabled - HTTP only
	if effective != nil && !*effective {
		return r.http.Fetch(rawURL, headers, waitForMs)
	}

	// Build the fetch chain based on settings
	chain := r.buildFetchChain(effective, preferredBrowser)
	if len(chain) == 0 {
		return r.http.Fetch(rawURL, headers, waitForMs)
	}

	// Execute the chain
	return r.executeFetchChain(rawURL, headers, waitForMs, chain, effective)
}

// buildFetchChain constructs the chain of fetchers based on renderJS and preferredBrowser settings
func (r *FallbackRenderer) buildFetchChain(effective *bool, preferredBrowser *string) []fetchChainItem {
	var chain []fetchChainItem

	// Determine if we should include HTTP in the chain
	includeHTTP := effective == nil || (effective != nil && !*effective)
	isForcedJS := effective != nil && *effective

	// Determine which browsers to use
	var browsers []PageFetcher
	if preferredBrowser != nil && *preferredBrowser != "" {
		// Use only the preferred browser
		for _, js := range r.jsRenderers {
			if js.Name() == *preferredBrowser {
				browsers = append(browsers, js)
			}
		}
	} else {
		browsers = r.jsRenderers
	}

	// Build chain based on renderJS setting
	if includeHTTP && !isForcedJS {
		// Auto mode: HTTP first in chain, browsers only if HTTP is bad
		chain = append(chain, fetchChainItem{name: "http", fetcher: r.http})
	}

	// Add browsers to chain
	for _, browser := range browsers {
		chain = append(chain, fetchChainItem{
			name:    browser.Name(),
			fetcher: browser,
		})
	}

	return chain
}

// executeFetchChain runs each fetcher in order, applying heuristics to decide continue/stop
func (r *FallbackRenderer) executeFetchChain(
	rawURL string,
	headers map[string]string,
	waitForMs *int64,
	chain []fetchChainItem,
	effective *bool,
) (*types.FetchResult, *types.QuickCrawlError) {
	isForcedJS := effective != nil && *effective
	var bestResult *types.FetchResult
	var bestResultQuality *ContentQuality

	for i, item := range chain {
		log.Printf("[renderer] trying fetcher: name=%s url=%s chain_pos=%d", item.name, rawURL, i)

		result, err := item.fetcher.Fetch(rawURL, headers, waitForMs)
		if err != nil || result == nil {
			log.Printf("[renderer] fetcher failed: name=%s url=%s error=%v", item.name, rawURL, err)
			continue
		}

		quality := assessContentQuality(result)

		// If content is good, return it immediately (for auto mode)
		if quality.isGood() && !isForcedJS {
			log.Printf("[renderer] good content found: name=%s url=%s", item.name, rawURL)
			return result, nil
		}

		// Track best result
		if bestResult == nil || quality.isBetterThan(bestResultQuality) {
			bestResult = result
			bestResultCopy := quality
			bestResultQuality = &bestResultCopy
		}

		// For forced JS mode, try all fetchers and pick the best
		if isForcedJS {
			continue
		}

		// For auto mode with HTTP result, check if we should chain to next
		if item.name == "http" && quality.isGood() {
			// HTTP gave good content, no need for browsers
			return result, nil
		}

		// Continue to next in chain
	}

	// Return best result we found
	if bestResult != nil {
		if !isForcedJS {
			return bestResult, nil
		}
		// For forced JS, add warning if content is thin
		if bestResultQuality != nil && bestResultQuality.isThin {
			warning := "JS rendering returned thin content"
			bestResult.Warning = &warning
		}
		return bestResult, nil
	}

	// All fetchers failed
	return nil, types.ErrRendererError.New("all fetchers in chain failed")
}

// ContentQuality holds the quality assessment of fetched content
type ContentQuality struct {
	isThin      bool
	isBlocked   bool
	isCrash     bool
	isPlaceholder bool
	vendorBlock string // non-empty if blocked by specific vendor
	reason      string
}

// isGood returns true if content is usable
func (c *ContentQuality) isGood() bool {
	return !c.isThin && !c.isBlocked && !c.isCrash && !c.isPlaceholder && c.vendorBlock == ""
}

// isBetterThan compares two content qualities, returns true if this is better
func (c *ContentQuality) isBetterThan(other *ContentQuality) bool {
	if other == nil {
		return true
	}
	// Prefer non-thin content
	if c.isThin && !other.isThin {
		return false
	}
	if !c.isThin && other.isThin {
		return true
	}
	// Prefer blocked content over other issues
	if c.isBlocked && !other.isBlocked {
		return false
	}
	if !c.isBlocked && other.isBlocked {
		return true
	}
	return false
}

// assessContentQuality evaluates the quality of a fetch result
func assessContentQuality(result *types.FetchResult) ContentQuality {
	q := ContentQuality{}

	q.isThin = pageLooksLikeThinHTML(result.HTML)
	q.isBlocked = pageHasBlockInterstitial(result.HTML)
	_, q.isCrash = pageLooksLikeFailedRender(result.HTML)
	q.isPlaceholder = pageLooksLikeLoadingPlaceholder(result.HTML)
	q.vendorBlock = pageLooksLikeVendorBlock(result.HTML)
	q.isBlocked = q.isBlocked || q.vendorBlock != ""

	return q
}

// appendRenderingWarning adds a warning message to the FetchResult when
// issues are detected but browser rendering is not available.
func appendRenderingWarning(result *types.FetchResult, needsJS, isBlocked, isAuthBlocked, isThin bool) *types.FetchResult {
	var warning string
	if needsJS {
		warning = "SPA detected but JS rendering not available"
	} else if isBlocked {
		warning = "Anti-bot challenge detected"
	} else if isAuthBlocked {
		warning = "Auth blocked"
	} else if isThin {
		warning = "HTTP result was thin and JS rendering was not used"
	}

	if result.Warning != nil {
		*result.Warning = *result.Warning + "; " + warning
	} else {
		result.Warning = &warning
	}
	return result
}

func isSoftBlockedStatus(statusCode uint16) bool {
	switch statusCode {
	case 401, 403, 404, 405, 406, 410, 412, 429, 451, 500, 503:
		return true
	default:
		return false
	}
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
