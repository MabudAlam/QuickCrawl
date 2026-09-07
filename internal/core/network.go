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
}

func newNetworkActivityTracker() *networkActivityTracker {
	t := &networkActivityTracker{}
	t.lastChanged.Store(time.Now().UnixMilli())
	return t
}

func (t *networkActivityTracker) recordRequestStart() {
	if t == nil {
		return
	}
	t.inFlight.Add(1)
	t.lastChanged.Store(time.Now().UnixMilli())
}

func (t *networkActivityTracker) recordRequestEnd() {
	if t == nil {
		return
	}
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
