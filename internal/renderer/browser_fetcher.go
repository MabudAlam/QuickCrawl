package renderer

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

const stealthJSCode = `Object.defineProperty(navigator, 'webdriver', { get: () => false });
if (!window.chrome) {
  window.chrome = { runtime: {}, loadTimes: function(){}, csi: function(){} };
}
Object.defineProperty(navigator, 'plugins', {
  get: () => {
    const arr = [
      { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer' },
      { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
      { name: 'Native Client', filename: 'internal-nacl-plugin' },
    ];
    arr.item = (i) => arr[i];
    arr.namedItem = (n) => arr.find(p => p.name == n);
    arr.refresh = () => {};
    return arr;
  }
});
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
const originalQuery = window.navigator.permissions.query.bind(window.navigator.permissions);
window.navigator.permissions.query = (params) =>
  params.name === 'notifications'
    ? Promise.resolve({ state: Notification.permission })
    : originalQuery(params);
const origHTMLElement = HTMLIFrameElement.prototype.__lookupGetter__('contentWindow');
if (origHTMLElement) {
  Object.defineProperty(HTMLIFrameElement.prototype, 'contentWindow', {
    get: function() {
      const w = origHTMLElement.call(this);
      if (w && !w.chrome) w.chrome = window.chrome;
      return w;
    }
  });
}
const nativeToString = Function.prototype.toString;
const overrides = new Map();
const proxy = new Proxy(nativeToString, {
  apply(target, thisArg, args) {
    const override = overrides.get(thisArg);
    return override || nativeToString.call(thisArg);
  }
});
Function.prototype.toString = proxy;
`

// BrowserFetcher fetches pages using Chrome DevTools Protocol via WebSocket.
// It can execute JavaScript and handle SPA rendering.
type BrowserFetcher struct {
	name        string          // Browser name (e.g., "lightpanda", "chrome")
	wsURL       string          // WebSocket URL for CDP connection
	browser     *managedBrowser // Managed browser process (if we launched it)
	stealth     bool            // Whether to inject stealth scripts
	defaultWait time.Duration   // Default wait after page load
	pageTimeout time.Duration   // Maximum time to wait for page load
	poolSem     *semaphore      // Concurrency limiter
}

// semaphore limits concurrent operations using a buffered channel.
type semaphore struct {
	mu     sync.Mutex
	sem    chan struct{}
	closed bool
}

// newSemaphore creates a semaphore with the given limit.
func newSemaphore(limit int) *semaphore {
	return &semaphore{
		sem: make(chan struct{}, limit),
	}
}

// acquire waits for a slot to become available.
func (s *semaphore) acquire() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.sem <- struct{}{}
}

// release frees a slot back to the pool.
func (s *semaphore) release() {
	select {
	case <-s.sem:
	default:
	}
}

// close permanently disables the semaphore.
func (s *semaphore) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	close(s.sem)
}

// globalPools holds semaphores for each WebSocket URL to limit concurrency.
var globalPools sync.Map

// getPool returns or creates a semaphore for the given WebSocket URL.
func getPool(wsURL string, size int) *semaphore {
	if pool, ok := globalPools.Load(wsURL); ok {
		return pool.(*semaphore)
	}
	pool := newSemaphore(size)
	globalPools.Store(wsURL, pool)
	return pool
}

// newBrowserFetcher creates a new browser-based page fetcher.
func newBrowserFetcher(name, wsURL string, browser *managedBrowser, stealth bool) *BrowserFetcher {
	return &BrowserFetcher{
		name:        name,
		wsURL:       wsURL,
		browser:     browser,
		stealth:     stealth,
		defaultWait: 2 * time.Second,
		pageTimeout: 30 * time.Second,
		poolSem:     getPool(wsURL, 10),
	}
}

// Name returns the browser type name.
func (f *BrowserFetcher) Name() string {
	return f.name
}

// SupportsJS returns true since browser can execute JavaScript.
func (f *BrowserFetcher) SupportsJS() bool {
	return true
}

// IsAvailable checks if the browser WebSocket endpoint is reachable.
func (f *BrowserFetcher) IsAvailable() bool {
	if strings.TrimSpace(f.wsURL) == "" {
		return false
	}
	conn, err := connectCDP(f.wsURL, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Fetch loads a URL in the browser and returns the rendered HTML.
func (f *BrowserFetcher) Fetch(rawURL string, headers map[string]string, waitForMs *int64) (*types.FetchResult, *types.QuickCrawlError) {
	// Limit concurrent fetches per browser
	if f.poolSem != nil {
		f.poolSem.acquire()
		defer f.poolSem.release()
	}

	start := time.Now()

	// Connect to browser via WebSocket
	conn, err := connectCDP(f.wsURL, 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	defer conn.Close()

	// Create a new browser page/tab
	targetRes, err := conn.SendRecv("Target.createTarget", map[string]any{"url": "about:blank"}, "", 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	targetID := extractJSONStringField(targetRes, "targetId")
	if targetID == "" {
		return nil, types.ErrRendererError.New("CDP target id missing")
	}

	// Attach to the target to get control
	attachRes, err := conn.SendRecv("Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "", 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	sessionID := extractJSONStringField(attachRes, "sessionId")
	if sessionID == "" {
		return nil, types.ErrRendererError.New("CDP session id missing")
	}

	// Enable CDP domains we need
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
			conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
			return nil, types.ErrRendererError.New(err.Error())
		}
	}

	// Inject stealth scripts to avoid bot detection
	if f.stealth {
		conn.SendRecv("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": getStealthScript()}, sessionID, 10*time.Second)
	}

	// Set custom User-Agent if provided
	if ua, ok := headers["User-Agent"]; ok && ua != "" {
		conn.SendRecv("Network.setUserAgentOverride", map[string]any{"userAgent": ua}, sessionID, 10*time.Second)
	}

	// Set extra HTTP headers
	if len(headers) > 0 {
		extra := make(map[string]string, len(headers))
		for k, v := range headers {
			if strings.EqualFold(k, "User-Agent") {
				continue
			}
			extra[k] = v
		}
		if len(extra) > 0 {
			conn.SendRecv("Network.setExtraHTTPHeaders", map[string]any{"headers": extra}, sessionID, 10*time.Second)
		}
	}

	// Navigate to the target URL
	if _, err := conn.SendRecv("Page.navigate", map[string]any{"url": rawURL}, sessionID, 10*time.Second); err != nil {
		conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}

	// Wait for page to load
	loadTimeout := f.pageTimeout
	if waitForMs != nil && *waitForMs > 0 {
		loadTimeout += time.Duration(*waitForMs) * time.Millisecond
	}

	statusCode, err := conn.WaitForPageReady(sessionID, loadTimeout)
	if err != nil {
		conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}

	// Additional wait after page load
	wait := f.defaultWait
	if waitForMs != nil && *waitForMs > 0 {
		wait = time.Duration(*waitForMs) * time.Millisecond
	}
	time.Sleep(wait)

	// Extract HTML from the page
	html, err := getRenderedHTML(conn, sessionID)
	if err != nil {
		conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}

	// If page still loading, wait for content to stabilize
	if waitForMs == nil && isLoadingPlaceholder(html) {
		if stable, stableErr := waitForContentStability(conn, sessionID, f.pageTimeout); stableErr == nil && stable != "" {
			html = stable
		}
	}

	// Retry on challenge pages (Cloudflare)
	if isAntiBotChallengePage(html) {
		for attempt := 1; attempt <= 3; attempt++ {
			time.Sleep(3 * time.Second)
			retryHTML, retryErr := getRenderedHTML(conn, sessionID)
			if retryErr != nil {
				conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
				return nil, types.ErrRendererError.New(retryErr.Error())
			}
			html = retryHTML
			if !isAntiBotChallengePage(html) {
				break
			}
		}
	}

	// Ensure we got content
	if html == "" {
		conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
		return nil, types.ErrRendererError.New("Empty HTML from CDP renderer")
	}

	// Clean up the browser page
	conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)

	renderedMethod := f.name
	return &types.FetchResult{
		URL:          rawURL,
		StatusCode:   statusCode,
		HTML:         html,
		ContentType:  ptr("text/html"),
		RawBytes:     nil,
		RenderedWith: &renderedMethod,
		TimeTaken:    uint64(time.Since(start).Milliseconds()),
	}, nil
}

// getStealthScript returns the stealth JavaScript code injected into every new page.
// This script masks automation indicators to avoid bot detection.
func getStealthScript() string {
	return stealthJSCode
}

// extractJSONStringField extracts a string field from a JSON object.
func extractJSONStringField(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// extractCDPResultValue extracts the result value from a CDP Runtime.evaluate response.
func extractCDPResultValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if res, ok := m["result"].(map[string]any); ok {
		if val, ok := res["value"].(string); ok {
			return val
		}
	}
	return ""
}

// getRenderedHTML extracts the full rendered HTML document.
// Returning the full document keeps browser output consistent with HTTP fetches,
// which lets downstream heuristics and content extraction operate on the same shape.
func getRenderedHTML(conn *cdpConnection, sessionID string) (string, error) {
	expression := `(function() {
		if (document.documentElement) {
			return document.documentElement.outerHTML;
		}
		if (document.body) {
			return document.body.outerHTML;
		}
		return '';
	})()`
	evalRes, err := conn.SendRecv("Runtime.evaluate", map[string]any{
		"expression":            expression,
		"returnByValue":         true,
		"awaitPromise":          true,
		"includeCommandLineAPI": true,
	}, sessionID, 10*time.Second)
	if err != nil {
		return "", err
	}
	html := extractCDPResultValue(evalRes)
	// Limit HTML size to prevent memory issues with heavy SPAs
	if len(html) > MaxBrowserHTMLBytes {
		html = html[:MaxBrowserHTMLBytes]
	}
	return html, nil
}

// waitForContentStability waits for page content to stop changing significantly.
// It polls the page periodically and returns when content has stabilized.
func waitForContentStability(conn *cdpConnection, sessionID string, timeout time.Duration) (string, error) {
	budget := 2 * time.Second
	if timeout > 0 && timeout < budget {
		budget = timeout
	}
	deadline := time.Now().Add(budget)
	prevLen := 0
	stableTicks := 0
	lastHTML := ""

	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		html, err := getRenderedHTML(conn, sessionID)
		if err != nil {
			return "", err
		}
		currLen := len(html)
		placeholderGone := !isLoadingPlaceholder(html)
		if hasContentStabilized(prevLen, currLen, placeholderGone) {
			stableTicks++
			if stableTicks >= 2 {
				return html, nil
			}
		} else {
			stableTicks = 0
		}
		prevLen = currLen
		lastHTML = html
	}
	return lastHTML, nil
}

// hasContentStabilized checks if content length has stabilized.
// It allows 5% variation in content length to account for minor dynamic changes.
func hasContentStabilized(prevLen, currLen int, placeholderGone bool) bool {
	if prevLen == 0 || !placeholderGone {
		return false
	}
	// Allow 5% variation in content length
	tolerance := prevLen / 20
	if tolerance < 500 {
		tolerance = 500
	}
	diff := currLen - prevLen
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// isAntiBotChallengePage checks if the page shows a Cloudflare or similar anti-bot challenge.
func isAntiBotChallengePage(html string) bool {
	if len(html) > 500000 {
		return false
	}
	lower := strings.ToLower(html)
	return strings.Contains(lower, "just a moment") ||
		strings.Contains(lower, "cf-browser-verification") ||
		strings.Contains(lower, "cf-challenge-running") ||
		strings.Contains(lower, "challenge-platform") ||
		(strings.Contains(lower, "challenge") && strings.Contains(lower, "cloudflare")) ||
		strings.Contains(lower, "attention required")
}

// isLoadingPlaceholder checks if the page shows a loading state placeholder.
// This indicates the page content has not yet been fully rendered.
func isLoadingPlaceholder(html string) bool {
	if len(html) > 500000 {
		return false
	}
	lower := strings.ToLower(html)
	markers := []string{
		"loading",
		"spinner",
		"skeleton",
		"please enable javascript",
		"enable javascript to view",
		`id="root"></div>`,
		`id="app"></div>`,
		`id="__next"></div>`,
		`id="__nuxt"></div>`,
		`id="__gatsby"></div>`,
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}
