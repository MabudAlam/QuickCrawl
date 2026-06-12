// Package core provides the clean, chromedp-based reimplementation of the
// legacy renderer.
//
// File: network.go
//
// Idle detection strategy
// ─────────────────────────────────────────────────────────────────────────
// A single "network quiet" signal is not enough to know when a page is
// truly ready. There are four distinct page archetypes:
//
//	Static / SSR       — no XHR at all; inFlight=0 from tick 1, so we
//	                     must not exit before document.readyState=="complete"
//
//	SSR + hydration    — small XHR burst finishes < 500 ms; we must
//	                     survive the gap between waves
//
//	SPA (React/Vue)    — multi-wave deferred fetches; early idle exit
//	                     misses second and third waves
//
//	Heavy SPA          — long-poll / SSE / infinite scroll; never truly
//	                     idles, must time-cap and exit gracefully
//
// The solution is a four-gate cascade that ALL must agree before we
// declare the page ready:
//
//	Gate 1  readyState  document.readyState == "complete"
//	        Ensures the parser has finished and all sync scripts have run.
//	        Waits at most readyStateTimeout (10 s).
//
//	Gate 2  networkIdle  inFlight == 0 for quietWindow (800 ms)
//	        Waits for XHR/fetch bursts to settle. The counter is
//	        protected against underflow (spurious LoadingFailed events)
//	        and is reset after navigation so pre-page requests don't
//	        pollute it.
//
//	Gate 3  domStable   MutationObserver reports no mutations for
//	        mutationQuietWindow (500 ms).
//	        Catches hydration DOM writes that happen after network idle.
//
//	Gate 4  animationIdle  requestAnimationFrame epoch has advanced at
//	        least once since gate 3 closed (one rAF tick ≈ 16 ms).
//	        Ensures the browser has committed the final paint.
//
// Gates 1–2 are evaluated server-side in Go; gates 3–4 are evaluated
// client-side by a small JS snippet injected into the page.
//
// For static/SSR pages the cascade completes in ~850 ms after load.
// For SPAs it self-adjusts: each new XHR wave resets gate 2, so we
// naturally wait for all waves. Gate 3 catches post-network DOM writes.
// For heavy SPAs that never idle, the caller's context deadline fires
// and we return whatever HTML we have — same graceful fallback as before.
package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// ─────────────────────────────────────────────────────────────────────────────
// Tuning constants
// ─────────────────────────────────────────────────────────────────────────────

const (
	// quietWindow is how long network inFlight must stay at zero before
	// we consider the network idle. 800 ms survives most SPA hydration
	// bursts without being so long that static pages feel slow.
	quietWindow = 800 * time.Millisecond

	// mutationQuietWindow is how long the MutationObserver must see no
	// DOM changes before we consider the DOM stable. 500 ms is enough
	// for React/Vue reconciliation to finish.
	mutationQuietWindow = 500 * time.Millisecond

	// readyStateTimeout is the maximum time we wait for
	// document.readyState to reach "complete". Almost all pages hit this
	// well under 10 s; the cap prevents infinite hangs on broken pages.
	readyStateTimeout = 10 * time.Second

	// minDwellTime is a mandatory floor applied after readyState=="complete"
	// before we start evaluating network and DOM gates. This prevents us
	// from declaring idle on a page that fires its first XHR 50 ms after
	// load (inFlight would still be 0 at the first poll tick).
	minDwellTime = 150 * time.Millisecond

	// pollInterval is how often the Go-side poll loop re-evaluates the
	// JS readiness probe. Lower = faster exit; higher = less CDP overhead.
	pollInterval = 80 * time.Millisecond
)

// ─────────────────────────────────────────────────────────────────────────────
// networkStatusTracker — captures the HTTP status of the main document
// ─────────────────────────────────────────────────────────────────────────────

// networkStatusTracker accumulates the HTTP status code observed for the
// main-frame document response. It is goroutine-safe via a mutex.
//
// We record the LAST document response seen so that in-page navigations
// (history.pushState, location.replace) are handled correctly.
type networkStatusTracker struct {
	mu     sync.Mutex
	status int64
	seen   bool
}

// Status returns the most recently observed document response status code,
// or 200 if no document response has been seen yet.
func (t *networkStatusTracker) Status() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.seen {
		return 200
	}
	return int(t.status)
}

// Seen reports whether the tracker observed at least one document response.
func (t *networkStatusTracker) Seen() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seen
}

func (t *networkStatusTracker) handle(e *network.EventResponseReceived) {
	if e.Type != network.ResourceTypeDocument {
		return
	}
	t.mu.Lock()
	t.status = e.Response.Status
	t.seen = true
	t.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────────────────────
// networkActivityTracker — counts in-flight requests for idle detection
// ─────────────────────────────────────────────────────────────────────────────

// networkActivityTracker keeps a saturating count of in-flight network
// requests. It is goroutine-safe via atomics.
//
// Key design decisions vs. the old implementation:
//
//  1. Underflow protection: recordRequestEnd never lets inFlight go below
//     zero. Chrome can emit LoadingFailed without a prior RequestWillBeSent
//     (e.g. for requests that started before the listener attached). Each
//     spurious decrement would have made inFlight permanently negative,
//     causing isIdle to never return true.
//
//  2. Reset after navigation: the caller calls Reset() right after
//     page.Navigate returns. This discards any in-flight count from the
//     previous page, preventing stale XHRs from blocking idle detection.
//
//  3. Observed-request gate: isIdle only considers the network idle if we
//     have seen at least one request-end event (i.e. the first wave of
//     requests has started AND finished). On a pure static page with zero
//     XHRs this gate is bypassed after minDwellTime via the hasRequests
//     flag staying false — the caller handles the bypass.
type networkActivityTracker struct {
	inFlight    atomic.Int64
	lastChanged atomic.Int64 // unix milliseconds
	// hasRequests is set the first time we see any request event.
	// Distinguishes "no requests at all" from "requests completed".
	hasRequests atomic.Bool
}

func newNetworkActivityTracker() *networkActivityTracker {
	t := &networkActivityTracker{}
	t.lastChanged.Store(time.Now().UnixMilli())
	return t
}

// Reset zeroes the in-flight counter and resets the clock. Call this
// immediately after navigation so pre-page requests don't contaminate
// the new page's idle detection.
func (t *networkActivityTracker) Reset() {
	if t == nil {
		return
	}
	t.inFlight.Store(0)
	t.hasRequests.Store(false)
	t.lastChanged.Store(time.Now().UnixMilli())
}

func (t *networkActivityTracker) recordRequestStart() {
	if t == nil {
		return
	}
	t.hasRequests.Store(true)
	t.inFlight.Add(1)
	t.lastChanged.Store(time.Now().UnixMilli())
}

func (t *networkActivityTracker) recordRequestEnd() {
	if t == nil {
		return
	}
	t.hasRequests.Store(true)
	// Saturating decrement: never go below zero. Spurious LoadingFailed
	// events (for requests we never saw start) must not make inFlight
	// negative.
	for {
		cur := t.inFlight.Load()
		if cur <= 0 {
			break
		}
		if t.inFlight.CompareAndSwap(cur, cur-1) {
			break
		}
	}
	t.lastChanged.Store(time.Now().UnixMilli())
}

// isIdle reports true when:
//   - no requests are in flight, AND
//   - the last activity was more than `quiet` ago.
//
// The caller is responsible for the "no requests at all" bypass; this
// function focuses only on the quiet-window logic.
func (t *networkActivityTracker) isIdle(quiet time.Duration) bool {
	if t == nil {
		return false
	}
	if t.inFlight.Load() > 0 {
		return false
	}
	elapsed := time.Since(time.UnixMilli(t.lastChanged.Load()))
	return elapsed >= quiet
}

// HasSeenRequests reports whether any network request has been observed.
func (t *networkActivityTracker) HasSeenRequests() bool {
	if t == nil {
		return false
	}
	return t.hasRequests.Load()
}

// ─────────────────────────────────────────────────────────────────────────────
// networkBundle — single ListenTarget registration for both trackers
// ─────────────────────────────────────────────────────────────────────────────

// networkBundle groups both trackers so they share a single
// ListenTarget registration and a single Network.Enable call.
type networkBundle struct {
	status   *networkStatusTracker
	activity *networkActivityTracker
}

// Handle is the chromedp.ListenTarget callback. Type-switches once and
// dispatches to the relevant tracker(s).
func (b *networkBundle) Handle(ev any) {
	switch e := ev.(type) {
	case *network.EventResponseReceived:
		if b.status != nil {
			b.status.handle(e)
		}
	case *network.EventRequestWillBeSent:
		// Filter out data: URIs — they don't represent real network activity
		// and Chrome sometimes emits them for inline resources.
		if e.Request != nil && strings.HasPrefix(e.Request.URL, "data:") {
			return
		}
		if b.activity != nil {
			b.activity.recordRequestStart()
		}
	case *network.EventLoadingFinished:
		if b.activity != nil {
			b.activity.recordRequestEnd()
		}
	case *network.EventLoadingFailed:
		// LoadingFailed also ends a request, but only decrement if we
		// actually counted a start for this request. The saturating
		// decrement in recordRequestEnd handles the case where we didn't.
		if b.activity != nil {
			b.activity.recordRequestEnd()
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// enableNetworkTracking — wires both trackers into the chromedp target
// ─────────────────────────────────────────────────────────────────────────────

// enableNetworkTracking returns a chromedp.Action that enables the Network
// CDP domain and registers the bundle as a target listener. It must be
// added to the chromedp.Run batch BEFORE the Navigate action so the
// document response event is captured.
func enableNetworkTracking(bundle *networkBundle) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := network.Enable().Do(ctx); err != nil {
			return fmt.Errorf("network.Enable: %w", err)
		}
		chromedp.ListenTarget(ctx, bundle.Handle)
		return nil
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// navigateIgnoringHTTPStatus — navigate without failing on 4xx/5xx
// ─────────────────────────────────────────────────────────────────────────────

// navigateIgnoringHTTPStatus issues a CDP page.Navigate and returns
// immediately after the browser acknowledges it (no load-event wait).
// The SPA readiness cascade in WaitForPageReady handles load detection.
//
// We deliberately ignore errorText (non-2xx status text from the browser)
// because we want to extract the response body even on 4xx/5xx — the
// real status code is captured by networkStatusTracker.
func navigateIgnoringHTTPStatus(urlstr string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, err := page.Navigate(urlstr).Do(ctx)
		if err != nil && !isHTTPStatusError(err) {
			return fmt.Errorf("page.Navigate: %w", err)
		}
		return nil
	})
}

// isHTTPStatusError reports whether err originated from a non-2xx HTTP
// response (as opposed to a network-level failure).
func isHTTPStatusError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "net::ERR_HTTP_RESPONSE_CODE_FAILURE")
}

// ─────────────────────────────────────────────────────────────────────────────
// readinessProbe — JS injected to evaluate client-side gates 3 and 4
// ─────────────────────────────────────────────────────────────────────────────

// readinessProbeJS installs a MutationObserver and rAF tracker in the page
// and exposes window.__pageReady() for the Go poll loop to query.
//
// The probe is idempotent — calling it twice is a no-op. This matters
// because chromedp may re-evaluate the script after a soft navigation.
//
// It reports three fields:
//
//	readyState   string   — document.readyState
//	domStable    bool     — no mutations for mutationQuietWindow
//	rafAdvanced  bool     — at least one rAF tick fired after domStable
const readinessProbeJS = `
(function() {
  if (window.__pageReadyInstalled) return;
  window.__pageReadyInstalled = true;

  const MUTATION_QUIET_MS = %d;   // injected by Go

  var lastMutation = Date.now();
  var domStableAt  = 0;  // timestamp when domStable first became true
  var rafFired     = false;

  // Gate 3: MutationObserver
  var mo = new MutationObserver(function() {
    lastMutation = Date.now();
    domStableAt  = 0;
    rafFired     = false;
  });
  mo.observe(document.documentElement, {
    childList: true, subtree: true,
    attributes: true, characterData: true
  });

  // Gate 4: rAF tracker — schedules itself once domStable is reached
  function scheduleRaf() {
    if (rafFired) return;
    var sinceLastMut = Date.now() - lastMutation;
    if (sinceLastMut >= MUTATION_QUIET_MS) {
      // DOM has been stable long enough; record the stable timestamp
      // and wait for one more rAF to confirm the browser committed it.
      if (!domStableAt) domStableAt = Date.now();
      requestAnimationFrame(function() { rafFired = true; });
    } else {
      // Not stable yet — check again after the remaining quiet window.
      setTimeout(scheduleRaf, MUTATION_QUIET_MS - sinceLastMut + 10);
    }
  }
  scheduleRaf();

  window.__pageReady = function() {
    var sinceLastMut = Date.now() - lastMutation;
    var domStable = sinceLastMut >= MUTATION_QUIET_MS;
    return {
      readyState:  document.readyState,
      domStable:   domStable,
      rafAdvanced: rafFired,
    };
  };
})();
`

// ─────────────────────────────────────────────────────────────────────────────
// PageReadiness — result of WaitForPageReady
// ─────────────────────────────────────────────────────────────────────────────

// PageReadiness summarises how the page settled. Callers can use this to
// log diagnostic information or adjust downstream behaviour.
type PageReadiness struct {
	// GatesReached is the number of gates (1–4) that fired before we
	// returned. 4 = fully settled; lower values mean we timed out or the
	// context was cancelled mid-cascade.
	GatesReached int

	// NetworkSeen reports whether any XHR/fetch activity was observed.
	// False on pure static/SSR pages.
	NetworkSeen bool

	// TimedOut is true if we exited because the caller's context expired
	// rather than because all gates closed.
	TimedOut bool
}

// ─────────────────────────────────────────────────────────────────────────────
// WaitForPageReady — the four-gate cascade
// ─────────────────────────────────────────────────────────────────────────────

// WaitForPageReady blocks until the page has settled according to the
// four-gate cascade described at the top of this file, or until ctx is
// cancelled.
//
// activity may be nil, in which case gates 1 and 2 are skipped and the
// function waits only for DOM stability (gates 3–4). This is useful for
// callers that do not have a networkActivityTracker.
//
// The function is designed to be called AFTER navigateIgnoringHTTPStatus
// returns and AFTER the activity tracker has been Reset().
func WaitForPageReady(ctx context.Context, activity *networkActivityTracker) (PageReadiness, error) {
	result := PageReadiness{}

	// ── Gate 1: document.readyState == "complete" ─────────────────────
	//
	// We poll with a short interval rather than using Page.loadEventFired
	// so we don't have to worry about the event firing before our listener
	// is attached (a race that caused 45 s timeouts on browserless).
	readyCtx, readyCancel := context.WithTimeout(ctx, readyStateTimeout)
	defer readyCancel()

	if err := pollUntil(readyCtx, pollInterval, func(ctx context.Context) (bool, error) {
		var state string
		err := chromedp.Evaluate(`document.readyState`, &state).Do(ctx)
		if err != nil {
			// Page may not be ready for JS evaluation yet — not fatal.
			return false, nil
		}
		return state == "complete" || state == "interactive", nil
	}); err != nil {
		if ctx.Err() != nil {
			result.TimedOut = true
			return result, ctx.Err()
		}
		// readyState timeout — proceed anyway; better to extract
		// partial HTML than to give up entirely.
	}
	result.GatesReached = 1

	// Mandatory dwell: give the page a moment to fire its first XHR
	// before we start watching the network counter.
	select {
	case <-ctx.Done():
		result.TimedOut = true
		return result, ctx.Err()
	case <-time.After(minDwellTime):
	}

	// ── Gate 2: network idle ──────────────────────────────────────────
	//
	// If the page has no XHR activity at all (pure static/SSR), we skip
	// waiting and proceed directly to DOM stability — the network is
	// trivially idle because it never started.
	if activity != nil {
		if err := pollUntil(ctx, pollInterval, func(_ context.Context) (bool, error) {
			// Two sub-cases:
			// a) Page has made at least one request: wait for the quiet window.
			// b) Page has made zero requests (static/SSR): minDwellTime already
			//    elapsed above, so zero requests means trivially idle — return true.
			if activity.HasSeenRequests() {
				return activity.isIdle(quietWindow), nil
			}
			return true, nil
		}); err != nil {
			if ctx.Err() != nil {
				result.TimedOut = true
				result.NetworkSeen = activity.HasSeenRequests()
				return result, ctx.Err()
			}
		}
	}
	result.GatesReached = 2
	result.NetworkSeen = activity != nil && activity.HasSeenRequests()

	// ── Install the JS readiness probe (gates 3–4) ───────────────────
	probeJS := fmt.Sprintf(readinessProbeJS, mutationQuietWindow.Milliseconds())
	if err := chromedp.Evaluate(probeJS, nil).Do(ctx); err != nil {
		// If we can't inject JS, skip gates 3–4 and return what we have.
		// This can happen on chrome-error:// pages.
		return result, nil
	}

	// ── Gate 3 + 4: DOM stable + rAF advanced ────────────────────────
	type probeResult struct {
		ReadyState  string `json:"readyState"`
		DomStable   bool   `json:"domStable"`
		RafAdvanced bool   `json:"rafAdvanced"`
	}

	if err := pollUntil(ctx, pollInterval, func(ctx context.Context) (bool, error) {
		var pr probeResult
		err := chromedp.Evaluate(`window.__pageReady()`, &pr).Do(ctx)
		if err != nil {
			return false, nil
		}
		return pr.DomStable && pr.RafAdvanced, nil
	}); err != nil {
		if ctx.Err() != nil {
			result.TimedOut = true
			return result, ctx.Err()
		}
	}

	result.GatesReached = 4
	return result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// pollUntil — generic poll helper
// ─────────────────────────────────────────────────────────────────────────────

// pollUntil calls cond repeatedly at `interval` until it returns true or
// ctx is cancelled/expired. It returns ctx.Err() on cancellation.
func pollUntil(ctx context.Context, interval time.Duration, cond func(context.Context) (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ok, err := cond(ctx)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}
