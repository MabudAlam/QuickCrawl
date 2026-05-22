package renderer

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

const stealthJSCode = `(function() {
  Object.defineProperty(navigator, 'webdriver', { get: () => false });
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
  const _origQuery = window.navigator.permissions.query.bind(window.navigator.permissions);
  window.navigator.permissions.query = (params) =>
    params.name === 'notifications'
      ? Promise.resolve({ state: Notification.permission })
      : _origQuery(params);
  const _origHTMLElement = HTMLIFrameElement.prototype.__lookupGetter__('contentWindow');
  if (_origHTMLElement) {
    Object.defineProperty(HTMLIFrameElement.prototype, 'contentWindow', {
      get: function() {
        const w = _origHTMLElement.call(this);
        if (w && !w.chrome) w.chrome = window.chrome;
        return w;
      }
    });
  }
  const _nativeToString = Function.prototype.toString;
  const _overrides = new Map();
  const _proxy = new Proxy(_nativeToString, {
    apply(target, thisArg, args) {
      const override = _overrides.get(thisArg);
      return override || _nativeToString.call(thisArg);
    }
  });
  Function.prototype.toString = _proxy;
  _overrides.set(Function.prototype.toString, 'function toString() { [native code] }');
  try {
    const _getParameter = WebGLRenderingContext.prototype.getParameter;
    WebGLRenderingContext.prototype.getParameter = function(parameter) {
      if (parameter === 37445) return 'Intel Inc.';
      if (parameter === 37446) return 'Intel Iris OpenGL Engine';
      return _getParameter.call(this, parameter);
    };
    if (typeof WebGL2RenderingContext !== 'undefined') {
      const _getParameter2 = WebGL2RenderingContext.prototype.getParameter;
      WebGL2RenderingContext.prototype.getParameter = function(parameter) {
        if (parameter === 37445) return 'Intel Inc.';
        if (parameter === 37446) return 'Intel Iris OpenGL Engine';
        return _getParameter2.call(this, parameter);
      };
    }
  } catch (_) {}
})()`

const cmpDismissJS = `
(() => {
  let clicks = 0;
  const isVisible = (el) => {
    if (!el || !el.getBoundingClientRect) return false;
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) return false;
    const s = window.getComputedStyle(el);
    return s.display !== 'none' && s.visibility !== 'hidden' && s.opacity !== '0';
  };
  const click = (el) => {
    try {
      if (!isVisible(el)) return false;
      el.click();
      clicks++;
      return true;
    } catch (_) { return false; }
  };
  const SELECTORS = [
    '#onetrust-accept-btn-handler', '.ot-accept-all',
    '#CybotCookiebotDialogBodyButtonAccept', '#CybotCookiebotDialogBodyLevelButtonAccept',
    '[data-testid="uc-accept-all-button"]', '[data-cy="uc-accept-all-button"]',
    '.sp_choice_type_11', 'button.message-component[title*="Accept" i]',
    '.qc-cmp2-summary-buttons button[mode="primary"]', '#qc-cmp2-ui button[mode="primary"]',
    '#truste-consent-button', '.cc-btn.cc-allow', '.cc-btn.cc-dismiss',
    'button[data-cmp-action="accept"]', 'button[data-accept-action="all"]',
    'button[aria-label*="Accept all" i]', 'button[aria-label*="Allow all" i]',
    '[id*="accept-cookies" i]', '[class*="accept-cookies" i]:not(input):not(textarea)',
  ];
  const tryRoot = (root) => {
    for (const sel of SELECTORS) {
      try {
        const el = root.querySelector(sel);
        if (el && click(el)) return;
      } catch (_) {}
    }
    try {
      const buttons = root.querySelectorAll('button, [role="button"], input[type="button"], input[type="submit"]');
      const PATTERNS = /^(accept all|allow all|accept cookies|i accept|agree|got it|ok|tümünü kabul et|tout accepter|alle akzeptieren|aceptar todo)$/i;
      for (const b of buttons) {
        const t = (b.innerText || b.value || b.textContent || '').trim();
        if (PATTERNS.test(t) && click(b)) return;
      }
    } catch (_) {}
  };
  tryRoot(document);
  try {
    const all = document.querySelectorAll('*');
    for (const host of all) {
      if (host.shadowRoot) tryRoot(host.shadowRoot);
    }
  } catch (_) {}
  try {
    for (const f of document.querySelectorAll('iframe')) {
      try {
        const doc = f.contentDocument || (f.contentWindow && f.contentWindow.document);
        if (doc) tryRoot(doc);
      } catch (_) {}
    }
  } catch (_) {}
  try {
    if (typeof window.__tcfapi === 'function') {
      window.__tcfapi('ping', 2, (data, ok) => {
        if (ok && data && data.cmpStatus !== 'error') {
          try { window.__tcfapi('addEventListener', 2, () => {}); } catch (_) {}
        }
      });
    }
  } catch (_) {}
  return clicks;
})()
`

const shadowDOMFlattenJS = `
(() => {
  const VOID = new Set(['area','base','br','col','embed','hr','img','input','link','meta','param','source','track','wbr']);
  let hasShadow = false;
  try {
    const all = document.querySelectorAll('*');
    for (let i = 0; i < all.length; i++) {
      if (all[i].shadowRoot) { hasShadow = true; break; }
    }
  } catch (_) {}
  if (!hasShadow) return document.documentElement.outerHTML;
  const escAttr = (v) => String(v).replace(/&/g, '&amp;').replace(/"/g, '&quot;');
  const serializeAttrs = (node) => {
    let s = '';
    for (const a of node.attributes || []) s += ' ' + a.name + '="' + escAttr(a.value) + '"';
    return s;
  };
  const serialize = (node) => {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent;
    if (node.nodeType === Node.COMMENT_NODE) return '';
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    const tag = node.tagName.toLowerCase();
    const attrs = serializeAttrs(node);
    let inner = '';
    if (node.shadowRoot) {
      inner = serializeShadowRoot(node);
    } else {
      for (const child of node.childNodes) inner += serialize(child);
    }
    if (VOID.has(tag)) return '<' + tag + attrs + '>';
    return '<' + tag + attrs + '>' + inner + '</' + tag + '>';
  };
  const serializeShadowRoot = (host) => {
    let result = '';
    for (const child of host.shadowRoot.childNodes) {
      result += serializeShadowChild(child, host);
    }
    return result;
  };
  const serializeShadowChild = (node, host) => {
    if (node.nodeType === Node.TEXT_NODE) return node.textContent;
    if (node.nodeType === Node.COMMENT_NODE) return '';
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    const tag = node.tagName.toLowerCase();
    if (tag === 'style') return '';
    if (tag === 'slot') {
      const assigned = node.assignedNodes({ flatten: true });
      if (assigned.length > 0) {
        let out = '';
        for (const a of assigned) out += serialize(a);
        return out;
      }
      let fallback = '';
      for (const child of node.childNodes) fallback += serializeShadowChild(child, host);
      return fallback;
    }
    const attrs = serializeAttrs(node);
    let inner = '';
    if (node.shadowRoot) {
      inner = serializeShadowRoot(node);
    } else {
      for (const child of node.childNodes) inner += serializeShadowChild(child, host);
    }
    if (VOID.has(tag)) return '<' + tag + attrs + '>';
    return '<' + tag + attrs + '>' + inner + '</' + tag + '>';
  };
  return serialize(document.documentElement);
})()
`

const autoScrollJS = `
(() => {
  const maxSteps = 12;
  let steps = 0;
  while (steps < maxSteps) {
    window.scrollBy(0, window.innerHeight);
    steps++;
    if (document.body.scrollHeight - window.scrollY - window.innerHeight < 100) break;
  }
  return steps;
})()
`

const autoClickRevealJS = `
(() => {
  const maxClicks = 5;
  const buttons = document.querySelectorAll('button, [role="button"], a');
  let clicks = 0;
  for (const btn of buttons) {
    if (clicks >= maxClicks) break;
    const text = (btn.innerText || btn.textContent || '').trim().toLowerCase();
    if (text.match(/load more|show more|continue|read more|see all|view more/i)) {
      btn.click();
      clicks++;
    }
  }
  return clicks;
})()
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
	stageStart := start
	stageTimes := make(map[string]int64)

	// Connect to browser via WebSocket
	conn, err := connectCDP(f.wsURL, 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	defer conn.Close()
	stageTimes["connect"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// Create a new browser page/tab
	targetRes, err := conn.SendRecv("Target.createTarget", map[string]any{"url": "about:blank"}, "", 10*time.Second)
	if err != nil {
		return nil, types.ErrRendererError.New(err.Error())
	}
	targetID := utils.ExtractJSONStringField(targetRes, "targetId")
	if targetID == "" {
		return nil, types.ErrRendererError.New("CDP target id missing")
	}
	stageTimes["createTarget"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// Attach to the target to get control
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
	stageTimes["enableDomains"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// Inject stealth scripts to avoid bot detection
	if f.stealth {
		conn.SendRecv("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": getStealthScript()}, sessionID, 10*time.Second)
	}

	// Set custom User-Agent if provided
	if ua, ok := headers["User-Agent"]; ok && ua != "" {
		conn.SendRecv("Network.setUserAgentOverride", map[string]any{"userAgent": ua}, sessionID, 10*time.Second)
	}

	// Set extra HTTP headers
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
			conn.SendRecv("Network.setExtraHTTPHeaders", map[string]any{"headers": extra}, sessionID, 10*time.Second)
		}
	}
	stageTimes["setupHeaders"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

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
	stageTimes["navigate+wait"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	// Additional wait after page load
	wait := f.defaultWait
	if waitForMs != nil && *waitForMs > 0 {
		wait = time.Duration(*waitForMs) * time.Millisecond
	}
	time.Sleep(wait)

	// Get initial HTML snapshot to evaluate conditions
	initialHTML, err := getRenderedHTMLShadowDOM(conn, sessionID)
	if err != nil {
		conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
		return nil, types.ErrRendererError.New(err.Error())
	}
	stageTimes["getHTML"] = time.Since(stageStart).Milliseconds()
	stageStart = time.Now()

	html := initialHTML
	cmpDismissTime, autoScrollTime, revealTime, stableTime, challengeRetryTime := int64(0), int64(0), int64(0), int64(0), int64(0)

	// Run CMP/cookie consent auto-dismiss after page load
	// Only run if CMP banner is detected - checking for common banner selectors
	// to avoid unnecessary JS execution on pages without consent management
	if f.stealth && !isAntiBotChallengePage(html) && !isLoadingPlaceholder(html) {
		hasCMPBanner := strings.Contains(html, "onetrust") ||
			strings.Contains(html, "cookiebot") ||
			strings.Contains(html, "usercentrics") ||
			strings.Contains(html, "cookieconsent") ||
			strings.Contains(html, "cc-window") ||
			strings.Contains(html, "cmp-wrapper") ||
			strings.Contains(html, "qc-cmp2") ||
			strings.Contains(html, "truste-consent")

		if hasCMPBanner {
			cmpStart := time.Now()
			conn.SendRecv("Runtime.evaluate", map[string]any{
				"expression":            getCMPDismissScript(),
				"awaitPromise":          true,
				"includeCommandLineAPI": true,
			}, sessionID, 5*time.Second)
			time.Sleep(500 * time.Millisecond)
			cmpDismissTime = time.Since(cmpStart).Milliseconds()
		}
	}
	stageTimes["cmpDismiss"] = cmpDismissTime
	stageStart = time.Now()

	// Run auto-scroll and click-to-reveal for lazy-loaded content
	// Only run when explicit lazy-load or reveal markers are detected.
	// This matches CRW's behavior: auto-scroll is gated on markers like
	// `loading="lazy"`, `data-src=`, `infinite-scroll`, etc.
	if f.stealth && waitForMs == nil && !isAntiBotChallengePage(html) && !isLoadingPlaceholder(html) {
		hasLazyMarkers := strings.Contains(html, `loading="lazy"`) ||
			strings.Contains(html, "data-src=") ||
			strings.Contains(html, "infinite-scroll") ||
			strings.Contains(html, "lazy-load")

		if hasLazyMarkers && len(html) < 200000 {
			autoScrollStart := time.Now()
			conn.SendRecv("Runtime.evaluate", map[string]any{
				"expression":            getAutoScrollScript(),
				"awaitPromise":          true,
				"includeCommandLineAPI": true,
			}, sessionID, 5*time.Second)
			time.Sleep(500 * time.Millisecond)
			autoScrollTime = time.Since(autoScrollStart).Milliseconds()
		}

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
			conn.SendRecv("Runtime.evaluate", map[string]any{
				"expression":            getAutoClickRevealScript(),
				"awaitPromise":          true,
				"includeCommandLineAPI": true,
			}, sessionID, 5*time.Second)
			time.Sleep(500 * time.Millisecond)
			revealTime = time.Since(revealStart).Milliseconds()
		}
	}
	stageTimes["autoScroll"] = autoScrollTime
	stageTimes["reveal"] = revealTime
	stageStart = time.Now()

	// If page still loading, wait for content to stabilize
	if waitForMs == nil && isLoadingPlaceholder(html) {
		stabilityStart := time.Now()
		if stable, stableErr := waitForContentStabilityShadowDOM(conn, sessionID, f.pageTimeout); stableErr == nil && stable != "" {
			html = stable
		}
		stableTime = time.Since(stabilityStart).Milliseconds()
	}
	stageTimes["stableWait"] = stableTime
	stageStart = time.Now()

	// Retry on challenge pages (Cloudflare)
	if isAntiBotChallengePage(html) {
		for attempt := 1; attempt <= 3; attempt++ {
			challengeStart := time.Now()
			time.Sleep(3 * time.Second)
			retryHTML, retryErr := getRenderedHTMLShadowDOM(conn, sessionID)
			if retryErr != nil {
				conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
				return nil, types.ErrRendererError.New(retryErr.Error())
			}
			html = retryHTML
			challengeRetryTime += time.Since(challengeStart).Milliseconds()
			if !isAntiBotChallengePage(html) {
				break
			}
		}
	}
	stageTimes["challengeRetry"] = challengeRetryTime
	stageStart = time.Now()

	// Ensure we got content
	if html == "" {
		conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
		return nil, types.ErrRendererError.New("Empty HTML from CDP renderer")
	}

	// Clean up the browser page
	conn.SendRecv("Target.closeTarget", map[string]any{"targetId": targetID}, "", 5*time.Second)
	stageTimes["closeTarget"] = time.Since(stageStart).Milliseconds()

	totalTime := time.Since(start).Milliseconds()

	// Log detailed timing breakdown
	log.Printf("[browser_fetcher] Fetch timing breakdown for %s (total=%dms): %+v", rawURL, totalTime, stageTimes)

	renderedMethod := f.name
	return &types.FetchResult{
		URL:          rawURL,
		StatusCode:   statusCode,
		HTML:         html,
		ContentType:  ptr("text/html"),
		RawBytes:     nil,
		RenderedWith: &renderedMethod,
		TimeTaken:    uint64(totalTime),
	}, nil
}

// getStealthScript returns the stealth JavaScript code injected into every new page.
// This script masks automation indicators to avoid bot detection.
func getStealthScript() string {
	return stealthJSCode
}

func getCMPDismissScript() string {
	return cmpDismissJS
}

func getShadowDOMFlattenScript() string {
	return shadowDOMFlattenJS
}

func getAutoScrollScript() string {
	return autoScrollJS
}

func getAutoClickRevealScript() string {
	return autoClickRevealJS
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
	if len(html) > MaxBrowserHTMLBytes {
		html = html[:MaxBrowserHTMLBytes]
	}
	return html, nil
}

// getRenderedHTMLShadowDOM extracts HTML with Shadow DOM flattened.
// This serializes web components and slot projections for proper content extraction.
func getRenderedHTMLShadowDOM(conn *cdpConnection, sessionID string) (string, error) {
	evalRes, err := conn.SendRecv("Runtime.evaluate", map[string]any{
		"expression":            getShadowDOMFlattenScript(),
		"returnByValue":         true,
		"awaitPromise":          true,
		"includeCommandLineAPI": true,
	}, sessionID, 10*time.Second)
	if err != nil {
		return "", err
	}
	html := extractCDPResultValue(evalRes)
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

// waitForContentStabilityShadowDOM waits for content stability using shadow DOM flattening.
func waitForContentStabilityShadowDOM(conn *cdpConnection, sessionID string, timeout time.Duration) (string, error) {
	budget := 6 * time.Second
	if timeout > 0 && timeout < budget {
		budget = timeout
	}
	deadline := time.Now().Add(budget)
	prevLen := 0
	stableTicks := 0
	lastHTML := ""

	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		html, err := getRenderedHTMLShadowDOM(conn, sessionID)
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
		strings.Contains(lower, "attention required") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "please verify") ||
		strings.Contains(lower, "checking your browser") ||
		strings.Contains(lower, "pardon our interruption") ||
		strings.Contains(lower, "_pxappid") ||
		strings.Contains(lower, "captcha-delivery") ||
		strings.Contains(lower, "_incapsula_resource") ||
		strings.Contains(lower, "incapsula incident id") ||
		strings.Contains(lower, "sucuri website firewall") ||
		strings.Contains(lower, "generated by cloudfront")
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
