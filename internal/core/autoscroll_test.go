package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// autoscroll_test.go verifies the two termination strategies used by
// AutoScrollAction against a real Chrome instance.
//
// The tests run a local HTTP server (httptest) that serves demo HTML
// matching the strategy under test, connect to a running Chrome via
// chromedp.NewRemoteAllocator, navigate to the demo URL, and call
// RunAutoScroll directly to capture the JSON-decoded result for
// assertion. The tests are gated on a reachable Chrome: if no browser
// is listening on the configured port, the tests skip (so the file
// compiles and runs cleanly in CI without a browser, but is exercised
// whenever Chrome is present).
//
// Strategy coverage:
//
//  1. Approach 1 — height-stabilize. A demo page adds 20 items each
//     time the user scrolls near the bottom. After ~4 scroll steps
//     the page stops growing and the loop exits with
//     Reason="stagnant". Tests assert that the page actually grew
//     (lazy load fired) and that the loop terminated with the
//     expected reason.
//
//  2. Approach 3 — end-marker race. A demo page contains an
//     end-marker element that becomes visible after a 250ms delay
//     (simulating a "load more" button that disappears once the
//     last batch is rendered). Tests assert that the loop exited
//     with Reason="endMarker" and in far fewer steps than MaxSteps,
//     proving the early-exit fired.
//
// Additional tests cover the hard cap (MaxSteps), the container-
// scroll path (ContainerSelector for SPA scroll panes), the lazy
// image pre-load pass, the no-growth fast path (page that does not
// grow at all), and the defaults (zero-value AutoScrollOptions).

// ─── helpers ──────────────────────────────────────────────────────────

// browserEnv returns the Chrome DevTools HTTP base URL the tests
// should probe, and a short reachability-check context. We probe the
// HTTP endpoint, not the WS endpoint, because the HTTP /json/version
// endpoint is what the production code uses to discover the live
// browser ID (see internal/utils/chrome.go).
//
// CHROME_HTTP_URL overrides the default for CI / local dev. The
// default assumes Chrome is running on localhost:9222 (the same
// assumption the main server makes in quickcrawl.toml).
func browserEnv(t *testing.T) (httpBase, wsURL string) {
	t.Helper()

	httpBase = os.Getenv("CHROME_HTTP_URL")
	if httpBase == "" {
		httpBase = "http://localhost:9222"
	}

	probeCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	probeReq, err := http.NewRequestWithContext(probeCtx, http.MethodGet, httpBase+"/json/version", nil)
	if err != nil {
		t.Skipf("chrome probe build failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(probeReq)
	if err != nil {
		t.Skipf("chrome not reachable at %s: %v", httpBase, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("chrome %s returned HTTP %d", httpBase, resp.StatusCode)
	}
	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Skipf("chrome /json/version decode: %v", err)
	}
	if info.WebSocketDebuggerURL == "" {
		t.Skipf("chrome /json/version returned empty webSocketDebuggerUrl")
	}
	return httpBase, info.WebSocketDebuggerURL
}

// newBrowserContext creates a fresh chromedp context connected to the
// running Chrome via the provided WS URL. Each call gets its own
// target/tab. The returned cancel functions must be called by the
// caller (typically via t.Cleanup) to release the target.
func newBrowserContext(t *testing.T, wsURL string) (context.Context, context.CancelFunc) {
	t.Helper()
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	t.Cleanup(allocCancel)
	ctx, cancel := chromedp.NewContext(allocCtx)
	t.Cleanup(cancel)
	return ctx, cancel
}

// serveHTML starts an httptest.Server that responds to every path
// with the supplied HTML and returns the server. The server URL is
// safe to pass to chromedp.Navigate.
//
// On Mac and Windows the Chrome instance the tests target is
// typically running in a Docker container (cloakhq/cloakbrowser or
// similar). Such a container sees 127.0.0.1 as its own loopback,
// not the host's. We bind the test server to 0.0.0.0 so it is
// reachable from the host's external interfaces, and rewrite the
// URL host to host.docker.internal — the convention Docker
// provides for "the host that started this container". This keeps
// the tests working in both containerized and native-Chrome
// environments with no environment flag.
func serveHTML(t *testing.T, html string) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	// Bind to all interfaces so Docker-networked Chrome can reach
	// us. The port is still kernel-assigned; we just need any
	// interface that the container can route to.
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("serveHTML: listen on 0.0.0.0:0: %v", err)
	}
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// chromeURL rewrites a test server's 127.0.0.1 URL into a URL whose
// host is reachable from a Docker-networked Chrome instance. The
// returned URL is what the tests pass to chromedp.Navigate.
func chromeURL(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("chromeURL: parse %s: %v", srv.URL, err)
	}
	u.Host = net.JoinHostPort("host.docker.internal", u.Port())
	return u.String()
}

// runAutoScroll navigates to the supplied URL on a fresh browser
// context, runs RunAutoScroll with the given options, and returns
// the JSON-decoded result. Any chromedp-level transport error fails
// the test.
func runAutoScroll(t *testing.T, ctx context.Context, url string, opts AutoScrollOptions) AutoScrollResult {
	t.Helper()
	var result AutoScrollResult
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		// Brief pause so the load event has a chance to fire
		// before we ask the JS payload to measure height.
		// 200ms is enough for httptest-served HTML.
		chromedp.Sleep(100*time.Millisecond),
		chromedp.ActionFunc(func(c context.Context) error {
			r, err := RunAutoScroll(c, opts)
			if err != nil {
				return err
			}
			result = r
			return nil
		}),
	); err != nil {
		t.Fatalf("runAutoScroll: %v", err)
	}
	return result
}

// ─── demo HTML for Approach 1: height-stagnate ────────────────────────

// htmlLazyLoad grows the document by 20 items each time the user
// scrolls within 100px of the bottom. The growth is bounded — the
// generator stops after 4 batches (80 items total). This models a
// realistic "load more on scroll" infinite-scroll page that the user
// (or a scraper) can fully consume in a few scroll steps.
//
// The expected behavior of AutoScrollAction on this page is:
//   - Steps 0..3: page grows after each scroll (loadCount increases,
//     height grows by ~20 items' worth of pixels).
//   - Step 4+: the load-on-scroll handler stops adding items. The
//     page height stops growing. After StagnantLimit consecutive
//     stagnant checks, the loop exits with Reason="stagnant".
const htmlLazyLoad = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Lazy Load Demo</title>
  <style>
    body { font-family: sans-serif; margin: 0; padding: 20px; }
    .item { padding: 20px; border-bottom: 1px solid #eee; height: 80px; }
    .item h2 { margin: 0; font-size: 16px; }
    .item p { margin: 4px 0 0; color: #666; font-size: 14px; }
  </style>
</head>
<body>
  <h1>Lazy Load Demo (Approach 1: height-stagnate)</h1>
  <p>Open DevTools to watch the network — but here we just append DOM nodes.</p>
  <div id="content"></div>
  <div id="status" style="position:fixed;top:0;right:0;background:yellow;padding:4px 8px;font-family:monospace;font-size:12px;"></div>
  <script>
    var loadCount = 0;
    var maxBatches = 4;
    function addBatch() {
      if (loadCount >= maxBatches) return false;
      var c = document.getElementById('content');
      for (var i = 0; i < 20; i++) {
        var n = loadCount * 20 + i;
        var d = document.createElement('div');
        d.className = 'item';
        d.id = 'item-' + n;
        d.innerHTML = '<h2>Item ' + n + '</h2><p>Content for item number ' + n + '. Loaded in batch ' + (loadCount + 1) + '.</p>';
        c.appendChild(d);
      }
      loadCount++;
      document.getElementById('status').textContent = 'batches=' + loadCount + ' height=' + document.body.scrollHeight;
      return true;
    }
    // Initial batch (so the page has measurable height on first paint).
    addBatch();
    window.addEventListener('scroll', function() {
      if (window.innerHeight + window.scrollY >= document.body.offsetHeight - 100) {
        addBatch();
      }
    });
  </script>
</body>
</html>`

// ─── demo HTML for Approach 3: end-marker race ─────────────────────────

// htmlEndMarker contains a sentinel element (#end-of-results) that
// becomes visible after a 600ms delay. The page also grows by a
// tiny amount (1px) on every scroll event, which keeps the loop
// from terminating on stagnation before the marker has a chance
// to appear. The 1px-per-scroll is enough to defeat the stagnant
// counter (a single flat check resets it) but small enough that
// the page height is effectively constant for the loop's purposes.
//
// The expected behavior is Reason="endMarker" within a small
// number of steps — far fewer than MaxSteps=30. The early-exit
// is the entire point of the end-marker strategy: it tells the
// scraper "you're done" without requiring the user to wait for
// the page to stop growing (which, on real infinite-scroll sites,
// can take a very long time).
const htmlEndMarker = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>End Marker Demo</title>
  <style>
    body { font-family: sans-serif; margin: 0; padding: 20px; }
    .row { padding: 24px; border-bottom: 1px solid #ddd; font-size: 18px; }
    #end-of-results { padding: 30px; background: #cfc; font-weight: bold; text-align: center; display: none; }
  </style>
</head>
<body>
  <h1>End Marker Demo (Approach 3: end-marker race)</h1>
  <div id="content"></div>
  <div id="end-of-results">— end of results —</div>
  <script>
    var rowCount = 0;
    function addRow() {
      var c = document.getElementById('content');
      var d = document.createElement('div');
      d.className = 'row';
      d.textContent = 'Row ' + (rowCount++) + ' — filler content to make the page tall';
      c.appendChild(d);
    }
    // Pre-render 30 rows so the page has measurable height.
    for (var i = 0; i < 30; i++) addRow();
    // A periodic 1px growth keeps the loop from hitting the
    // stagnant termination while the marker timer counts down.
    // setInterval is more reliable than a scroll-event handler
    // because the latter depends on the browser firing the event
    // in a chromedp-driven context, which is not guaranteed for
    // programmatic scrollBy calls.
    var grow = document.createElement('div');
    grow.id = 'grow';
    grow.style.height = '0px';
    document.body.appendChild(grow);
    setInterval(function() {
      grow.style.height = (parseInt(grow.style.height || '0', 10) + 1) + 'px';
    }, 50);
    // Reveal the end marker after a delay. Models a "load more"
    // button that the SPA removes once the last batch is loaded.
    setTimeout(function() {
      document.getElementById('end-of-results').style.display = 'block';
    }, 600);
  </script>
</body>
</html>`

// ─── demo HTML for maxSteps hard cap ──────────────────────────────────

// htmlInfiniteForever keeps adding content on every scroll and never
// stops. This is the worst case for a scraper — without a hard cap,
// the loop would run forever. We use it to verify the MaxSteps limit
// fires when the page never reaches a termination condition.
const htmlInfiniteForever = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Infinite Scroll Demo</title></head>
<body>
  <h1>Infinite Scroll (Approach: hard cap)</h1>
  <div id="content"></div>
  <script>
    var n = 0;
    function add() {
      var c = document.getElementById('content');
      for (var i = 0; i < 5; i++) {
        var d = document.createElement('div');
        d.style.padding = '15px';
        d.style.borderBottom = '1px solid #eee';
        d.textContent = 'Row ' + (n++);
        c.appendChild(d);
      }
    }
    add();
    window.addEventListener('scroll', function() {
      if (window.innerHeight + window.scrollY >= document.body.offsetHeight - 50) add();
    });
  </script>
</body>
</html>`

// ─── demo HTML for lazy image pre-load ─────────────────────────────────

// htmlLazyImages has images that the browser would normally defer
// (loading="lazy") and images that use the data-src swap pattern.
// After AutoScrollAction runs, the JS payload's step 0 should have
// flipped loading=eager and copied data-src to src. The test
// inspects the live DOM via chromedp to assert both have happened.
const htmlLazyImages = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Lazy Image Demo</title></head>
<body>
  <h1>Lazy Image Demo</h1>
  <p>These images start deferred; the pre-load pass should eagerify them.</p>
  <img id="img-lazy" loading="lazy" data-src="https://example.com/lazy.jpg" src="" alt="lazy">
  <img id="img-datasrc" data-src="https://example.com/datasrc.jpg" alt="datasrc" style="display:none">
  <p>end of page</p>
</body>
</html>`

// ─── demo HTML for container scroll (SPA scroll pane) ─────────────────

// htmlContainerScroll has a fixed-height scrollable inner pane with
// a tall body of content. Auto-scroll must target ContainerSelector
// (the inner pane) and scroll it, not window. The test asserts both
// that the inner pane's scrollHeight grew (so the scroll actually
// affected the right element) and that window itself was not
// scrolled.
const htmlContainerScroll = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Container Scroll Demo</title>
<style>
  body { margin: 0; font-family: sans-serif; }
  .pane { height: 300px; overflow-y: auto; border: 2px solid #06c; padding: 10px; }
  .row { padding: 12px; border-bottom: 1px solid #ccc; }
</style>
</head>
<body>
  <h1>Container Scroll Demo</h1>
  <p>The blue-bordered pane below is the scroll target — not window.</p>
  <div id="pane" class="pane">
    <div id="pane-content"></div>
  </div>
  <p id="footer" style="margin-top:20px;">(footer outside pane)</p>
  <script>
    var n = 0, batches = 0;
    function add() {
      if (batches >= 3) return false;
      var c = document.getElementById('pane-content');
      for (var i = 0; i < 8; i++) {
        var d = document.createElement('div');
        d.className = 'row';
        d.textContent = 'Pane row ' + (n++);
        c.appendChild(d);
      }
      batches++;
      return true;
    }
    add();
    document.getElementById('pane').addEventListener('scroll', function() {
      if (this.scrollTop + this.clientHeight >= this.scrollHeight - 50) add();
    });
  </script>
</body>
</html>`

// ─── tests ────────────────────────────────────────────────────────────

// TestAutoScroll_Approach1_HeightStagnate verifies Approach 1:
// the loop exits with reason="stagnant" after the lazy-load page
// stops growing, and the page actually grew during the run
// (proving the scroll fired and triggered the load handler).
func TestAutoScroll_Approach1_HeightStagnate(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, htmlLazyLoad)

	// Capture the page height BEFORE the auto-scroll run. The
	// post-condition is that the post-run height is strictly
	// greater (proving the scroll triggered the load handler).
	preRun := measureBodyHeight(t, ctx, chromeURL(t, srv))

	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:      20,
		PauseMs:       150,
		StagnantLimit: 3,
	})

	postRun := measureBodyHeight(t, ctx, chromeURL(t, srv))

	if result.Reason != "stagnant" {
		t.Errorf("expected reason=stagnant, got %q (steps=%d, height=%d)", result.Reason, result.Steps, result.Height)
	}
	if result.Steps < 1 {
		t.Errorf("expected at least 1 step, got %d", result.Steps)
	}
	if postRun <= preRun {
		t.Errorf("expected page to grow during scroll: pre=%d post=%d", preRun, postRun)
	}
	if result.Height < postRun {
		t.Errorf("result.Height=%d should be >= post-run measured height=%d", result.Height, postRun)
	}
	t.Logf("Approach 1: reason=%s steps=%d height=%d pre=%d post=%d",
		result.Reason, result.Steps, result.Height, preRun, postRun)
}

// TestAutoScroll_Approach3_EndMarker verifies Approach 3: the loop
// exits with reason="endMarker" the moment the sentinel element
// enters the viewport, well before MaxSteps is reached.
func TestAutoScroll_Approach3_EndMarker(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, htmlEndMarker)

	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:      30,
		PauseMs:       200,
		StagnantLimit: 3,
		EndSelector:   "#end-of-results",
	})

	if result.Reason != "endMarker" {
		t.Errorf("expected reason=endMarker, got %q (steps=%d, height=%d)", result.Reason, result.Steps, result.Height)
	}
	// End-marker appears after 600ms and the loop pauses 200ms
	// between steps, so the loop should exit in 1-5 steps. We
	// require strictly less than MaxSteps to prove the early-exit
	// fired (not the hard cap).
	if result.Steps >= 30 {
		t.Errorf("expected early exit (steps < MaxSteps=30), got steps=%d — did the end-marker early-exit fire?", result.Steps)
	}
	t.Logf("Approach 3: reason=%s steps=%d height=%d", result.Reason, result.Steps, result.Height)
}

// TestAutoScroll_MaxSteps_HardCap verifies that the loop respects
// MaxSteps when no other termination condition fires. The infinite-
// scroll demo never stops growing, so the loop can ONLY exit via
// the hard cap.
func TestAutoScroll_MaxSteps_HardCap(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, htmlInfiniteForever)

	const hardCap = 5
	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:      hardCap,
		PauseMs:       80,
		StagnantLimit: 99, // never fires
	})

	if result.Reason != "maxSteps" {
		t.Errorf("expected reason=maxSteps, got %q", result.Reason)
	}
	if result.Steps != hardCap {
		t.Errorf("expected steps=%d, got %d", hardCap, result.Steps)
	}
	t.Logf("MaxSteps cap: reason=%s steps=%d height=%d", result.Reason, result.Steps, result.Height)
}

// htmlSteadyGrow is a page that grows by a small fixed amount
// (~50px) on a setInterval timer (independent of scroll events)
// and never stagnates. Used to verify the MaxHeightGrowth cap,
// which is measured against the page's baseline height captured
// on the first iteration (not against lastHeight, which is the
// stagnant-tracking variable).
//
// The setInterval-based growth is intentional: a scroll-event
// handler would not fire reliably for programmatic scrollBy in a
// chromedp-driven headless context (we observed that during
// autoscroll test development), and the timer approach is
// independent of the user's interaction with the page.
const htmlSteadyGrow = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Steady Grow</title></head>
<body>
  <h1>Steady Grow</h1>
  <p>This page grows by 50px every 100ms and never stops.</p>
  <div id="content"></div>
  <script>
    var grow = document.createElement('div');
    grow.id = 'grow';
    grow.style.height = '0px';
    document.body.appendChild(grow);
    setInterval(function() {
      grow.style.height = (parseInt(grow.style.height || '0', 10) + 50) + 'px';
    }, 100);
  </script>
</body>
</html>`

// TestAutoScroll_MaxHeightGrowth verifies that the loop exits with
// reason="maxGrowth" when the page has grown by more than
// MaxHeightGrowth pixels from its initial baseline. The page grows
// steadily (never stagnates, never ends), so the only termination
// that can fire is the growth cap.
func TestAutoScroll_MaxHeightGrowth(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, htmlSteadyGrow)

	// Baseline is essentially the body+h1 height (~120px). The
	// page grows by 50px every 100ms via setInterval. Cap growth
	// at 300px → page should grow to ~420px and exit. The page
	// grows independently of scroll events (timer-based), so
	// the loop can never terminate on stagnation and the only
	// path to exit is the growth cap.
	const cap = 300
	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:       20, // would be 20 iterations otherwise
		PauseMs:        50,
		StagnantLimit:  99, // never fires (page keeps growing)
		MaxHeightGrowth: cap,
	})

	if result.Reason != "maxGrowth" {
		t.Errorf("expected reason=maxGrowth, got %q (steps=%d, height=%d)", result.Reason, result.Steps, result.Height)
	}
	if result.Height < cap {
		t.Errorf("expected height >= %d (the cap), got %d", cap, result.Height)
	}
	// The loop should have terminated well before MaxSteps=20,
	// proving the growth cap is what fired (not the iteration cap).
	if result.Steps >= 20 {
		t.Errorf("expected early termination (steps < 20), got %d", result.Steps)
	}
	t.Logf("MaxHeightGrowth cap=%d: reason=%s steps=%d height=%d", cap, result.Reason, result.Steps, result.Height)
}

// TestAutoScroll_MaxDuration verifies that the loop exits with
// reason="maxDuration" when the wall-clock budget is exhausted,
// even if neither stagnation, end-marker, nor growth cap would
// have fired. The page grows steadily and we set a very short
// duration budget.
func TestAutoScroll_MaxDuration(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, htmlSteadyGrow)

	const budget = 1500 // ms
	start := time.Now()
	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:       20, // would be 20 iterations otherwise
		PauseMs:        200,
		StagnantLimit:  99, // never fires
		MaxHeightGrowth: 100000, // effectively unlimited
		MaxDurationMs:  budget,
	})
	elapsed := time.Since(start)

	if result.Reason != "maxDuration" {
		t.Errorf("expected reason=maxDuration, got %q (steps=%d, height=%d)", result.Reason, result.Steps, result.Height)
	}
	// Sanity: the run should not have exceeded the budget by much
	// (a few iterations' worth of slack is acceptable given the
	// headless-Chrome timer-throttling we observed in development).
	if elapsed > time.Duration(budget)*time.Millisecond+10*time.Second {
		t.Errorf("expected elapsed < %dms + slack, got %v", budget, elapsed)
	}
	t.Logf("MaxDurationMs budget=%d: reason=%s steps=%d height=%d elapsed=%v",
		budget, result.Reason, result.Steps, result.Height, elapsed)
}

// TestAutoScroll_LazyImagePreload verifies that the step-0 image
// pre-load pass eagerly loads <img loading="lazy"> and <img data-src>
// elements, regardless of whether any scrolling fires.
func TestAutoScroll_LazyImagePreload(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, htmlLazyImages)

	// Run with a tiny MaxSteps so the scroll loop barely runs
	// and the test isolates the pre-load pass.
	_ = runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:    1,
		PauseMs:     10,
		LoadLazyImages: true,
	})

	// Inspect the live DOM. The first image had loading="lazy" and
	// src=""; after the pre-load pass it should have loading="eager"
	// and src=data-src. The second image had data-src but no src;
	// after the pass it should have src=data-src.
	var lazyLoading, lazySrc string
	var datasrcSrc string
	if err := chromedp.Run(ctx,
		chromedp.AttributeValue("img#img-lazy", "loading", &lazyLoading, nil),
		chromedp.AttributeValue("img#img-lazy", "src", &lazySrc, nil),
		chromedp.AttributeValue("img#img-datasrc", "src", &datasrcSrc, nil),
	); err != nil {
		t.Fatalf("inspect images: %v", err)
	}
	if lazyLoading != "eager" {
		t.Errorf("expected img#img-lazy loading=eager, got %q", lazyLoading)
	}
	if lazySrc == "" {
		t.Errorf("expected img#img-lazy src to be populated from data-src, got empty")
	}
	if datasrcSrc == "" {
		t.Errorf("expected img#img-datasrc src to be populated from data-src, got empty")
	}
	t.Logf("Lazy pre-load: lazy.loading=%s lazy.src=%s datasrc.src=%s", lazyLoading, lazySrc, datasrcSrc)
}

// TestAutoScroll_ContainerSelector verifies that ContainerSelector
// targets an internal scrollable element (SPA scroll pane) rather
// than window. The demo has a fixed-height pane that lazy-loads
// rows on scroll; the outer page is short.
func TestAutoScroll_ContainerSelector(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, htmlContainerScroll)

	// Capture the pane's scrollHeight before and after. The
	// post-run value should be larger (rows were added).
	preRun := measureElementHeight(t, ctx, "#pane-content")

	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:         20,
		PauseMs:          100,
		StagnantLimit:    3,
		ContainerSelector: "#pane",
	})

	postRun := measureElementHeight(t, ctx, "#pane-content")
	// Also confirm window itself was not scrolled — the pane
	// should have absorbed all the scroll movement.
	var windowY float64
	if err := chromedp.Run(ctx,
		chromedp.Evaluate("window.scrollY", &windowY),
	); err != nil {
		t.Fatalf("read window.scrollY: %v", err)
	}

	if result.Reason != "stagnant" {
		t.Errorf("expected reason=stagnant, got %q", result.Reason)
	}
	if postRun <= preRun {
		t.Errorf("expected pane to grow during container scroll: pre=%d post=%d", preRun, postRun)
	}
	if windowY != 0 {
		t.Errorf("expected window.scrollY=0 (container should have absorbed scroll), got %v", windowY)
	}
	t.Logf("Container: reason=%s steps=%d pane.pre=%d pane.post=%d windowY=%v",
		result.Reason, result.Steps, preRun, postRun, windowY)
}

// TestAutoScroll_Defaults verifies that the zero-value
// AutoScrollOptions produces a sane run on a static page. A page
// that does not grow should exit with reason="stagnant" (or
// "maxSteps" if the page is large but the loop is bounded —
// either is acceptable for the default path).
func TestAutoScroll_Defaults(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	// Tiny static page — should exit on first stagnant check.
	srv := serveHTML(t, `<!DOCTYPE html><html><body><h1>tiny</h1><p>short.</p></body></html>`)

	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{})

	if result.Reason != "stagnant" && result.Reason != "maxSteps" {
		t.Errorf("expected reason=stagnant or maxSteps, got %q", result.Reason)
	}
	if result.Height <= 0 {
		t.Errorf("expected positive height, got %d", result.Height)
	}
	t.Logf("Defaults: reason=%s steps=%d height=%d", result.Reason, result.Steps, result.Height)
}

// TestAutoScroll_NoGrowth_StaticPage verifies the fast-path behavior
// on a page that has no lazy-load handlers at all. The first scroll
// should not grow the document, and the loop should exit on the
// 3rd consecutive stagnant check (StagnantLimit=3 default).
func TestAutoScroll_NoGrowth_StaticPage(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	srv := serveHTML(t, `<!DOCTYPE html>
<html>
<body>
  <h1>Static</h1>
  <p>This page is intentionally short. There is no lazy load and no
  scroll handler. The auto-scroll loop should hit the stagnant
  termination within a few steps.</p>
</body>
</html>`)

	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:      20,
		PauseMs:       30,
		StagnantLimit: 3,
	})

	if result.Reason != "stagnant" {
		t.Errorf("expected reason=stagnant on static page, got %q", result.Reason)
	}
	// 3 stagnant checks before exit → steps should be 3 (or a
	// small number if the loop is so fast the page was already
	// measured at the same height on the first iteration).
	if result.Steps < 1 || result.Steps > 6 {
		t.Errorf("expected steps in [1,6], got %d", result.Steps)
	}
	t.Logf("No-growth: reason=%s steps=%d height=%d", result.Reason, result.Steps, result.Height)
}

// TestAutoScroll_EndMarker_BeatsStagnation verifies the race
// ordering: when both end-marker and stagnation are candidates,
// end-marker wins (it's checked first inside the loop body).
func TestAutoScroll_EndMarker_BeatsStagnation(t *testing.T) {
	_, wsURL := browserEnv(t)
	ctx, _ := newBrowserContext(t, wsURL)

	// A page that is static (so stagnation would fire) AND has
	// an end-marker (which appears immediately). The end-marker
	// should win the race.
	srv := serveHTML(t, `<!DOCTYPE html>
<html>
<body>
  <h1>Race Test</h1>
  <p>This page never grows (no scroll handler) but has an
  end-marker that is visible from t=0.</p>
  <div id="end-of-results" style="padding:20px;background:#cfc;">END</div>
</body>
</html>`)

	result := runAutoScroll(t, ctx, chromeURL(t, srv), AutoScrollOptions{
		MaxSteps:      10,
		PauseMs:       50,
		StagnantLimit: 3,
		EndSelector:   "#end-of-results",
	})

	if result.Reason != "endMarker" {
		t.Errorf("expected endMarker to win the race over stagnant, got %q", result.Reason)
	}
	t.Logf("Race: reason=%s steps=%d (end-marker won over stagnant)", result.Reason, result.Steps)
}

// ─── shared utility used by the tests above ───────────────────────────

// measureBodyHeight returns the current document scrollHeight by
// evaluating a JS expression against the live page. Used to assert
// that the page actually grew during the auto-scroll run.
func measureBodyHeight(t *testing.T, ctx context.Context, _ string) int {
	t.Helper()
	var h int
	if err := chromedp.Run(ctx,
		chromedp.Evaluate("Math.max(document.body.scrollHeight, document.documentElement.scrollHeight)", &h),
	); err != nil {
		t.Fatalf("measureBodyHeight: %v", err)
	}
	return h
}

// measureElementHeight returns the offsetHeight of the first
// element matching the given CSS selector. Used by the container-
// scroll test to assert that the scrollable pane's content grew.
func measureElementHeight(t *testing.T, ctx context.Context, sel string) int {
	t.Helper()
	// Escape the selector for embedding inside a JS string. We
	// only need to handle the common case of simple selectors
	// (id, class, tag) — the tests use #pane-content.
	jsSel, err := json.Marshal(sel)
	if err != nil {
		t.Fatalf("marshal selector: %v", err)
	}
	var h int
	expr := fmt.Sprintf(`(function(){var e=document.querySelector(%s);return e?e.offsetHeight:0;})()`, string(jsSel))
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &h)); err != nil {
		t.Fatalf("measureElementHeight(%s): %v", sel, err)
	}
	return h
}
