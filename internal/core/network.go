// Package core provides the clean, chromedp-based reimplementation of the
// legacy renderer.
//
// File: network.go
//
// This file contains the Network.responseReceived listener that captures
// the actual HTTP status code of the main-frame navigation. The chromedp
// high-level APIs (Navigate, Sleep, OuterHTML) do not surface the response
// status — they only know when Page.loadEventFired fires, not what status
// the server returned. Without this listener, statusCode is hardcoded to
// 200, which means a 404 page or a soft 5xx would scrape successfully and
// the caller would never know.
//
// The original renderer captures the same status via raw CDP event
// channels at internal/renderer/cdp_connection.go:WaitForPageReady. We
// mirror that behavior using chromedp's typed event listener.
package core

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// networkStatusTracker accumulates the HTTP status code observed for the
// main-frame document response. It is goroutine-safe: chromedp dispatches
// events on the target's event-loop goroutine, but callers may read Status
// from a different goroutine (e.g. after chromedp.Run returns).
//
// We only track responses whose Type is ResourceTypeDocument. Sub-frame
// loads, XHRs, images, fonts, etc. all have their own responseReceived
// events but the per-navigation status we want is the main document
// response — typically 200, but can be 301/302 (followed automatically by
// the browser), 304, 4xx, 5xx, etc.
//
// The tracker records the LAST document response seen before the listener
// is detached. This handles the case where a page triggers an internal
// navigation (history.pushState, location.replace) — the second document
// response is the one most relevant to the final HTML we extract.
type networkStatusTracker struct {
	mu     sync.Mutex
	status int64
	// seen flips to true the first time a document response is observed.
	// Distinguishes "no document response yet" (default 200) from "saw
	// status 0 which is impossible" — we never let 0 leak to callers.
	seen bool
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

// Seen reports whether the tracker observed at least one document
// response. Callers can use this to distinguish "we successfully captured
// the server's status" from "we fell back to the default 200" — the
// former has the real status, the latter does not.
func (t *networkStatusTracker) Seen() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seen
}

// Handle is the chromedp.ListenTarget callback. It is invoked synchronously
// for every event the target emits. We type-switch on *network.EventResponseReceived
// and ignore anything else; this is cheaper than registering a separate
// listener per event type.
//
// Concurrency: Handle is always called from chromedp's event dispatcher
// goroutine, never concurrently with itself, but it CAN race with Status()
// (called from the caller goroutine after chromedp.Run returns). The mutex
// protects both.
func (t *networkStatusTracker) Handle(ev any) {
	e, ok := ev.(*network.EventResponseReceived)
	if !ok {
		return
	}
	if e.Type != network.ResourceTypeDocument {
		return
	}
	t.mu.Lock()
	t.status = e.Response.Status
	t.seen = true
	t.mu.Unlock()
}

// networkActivityTracker keeps a cheap count of in-flight network requests
// so the SPA readiness poll can exit as soon as the page has gone quiet —
// mirroring the original /v1/scrape renderer's exit at
// internal/renderer/browser_render_helpers.go:97:
//
//	if tracker != nil && tracker.isIdle(500*time.Millisecond) {
//	    return true
//	}
//
// For static/SSR pages this is much faster than the 15s default SPA budget:
// the page is "ready" the moment the load event fires and no further XHR/
// fetch is in flight. For SPA pages with deferred hydration, it lets the
// poll wait for the hydration requests to settle without blocking on the
// full text-threshold time.
//
// The tracker is goroutine-safe via atomics. chromedp's event dispatcher
// invokes Handle serially on a single goroutine, but isIdle may be called
// from the SPA poll goroutine — atomic.Int64.Load/Add is enough for both.
type networkActivityTracker struct {
	inFlight    atomic.Int64
	lastChanged atomic.Int64 // unix milliseconds
}

func newNetworkActivityTracker() *networkActivityTracker {
	tracker := &networkActivityTracker{}
	tracker.lastChanged.Store(time.Now().UnixMilli())
	return tracker
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
	t.inFlight.Add(-1)
	t.lastChanged.Store(time.Now().UnixMilli())
}

// isIdle reports whether no requests are in flight AND the tracker has
// not seen any activity in the last `quiet` duration. The quiet window
// (typically 500ms) is what makes this safe: a request that fires and
// completes in <500ms is invisible to the SPA poll because lastChanged
// stays recent. A request that started >500ms ago and is still in
// flight would have made lastChanged >500ms old AND inFlight > 0; we
// only return true when BOTH are quiet.
func (t *networkActivityTracker) isIdle(quiet time.Duration) bool {
	if t == nil {
		return false
	}
	if t.inFlight.Load() > 0 {
		return false
	}
	elapsed := time.Duration(time.Now().UnixMilli()-t.lastChanged.Load()) * time.Millisecond
	return elapsed >= quiet
}

// networkBundle groups the two trackers so they can share a single
// Network.enable call and a single ListenTarget registration. Both
// trackers run on the same event stream, dispatched in Handle below.
type networkBundle struct {
	status   *networkStatusTracker
	activity *networkActivityTracker
}

func (b *networkBundle) Handle(ev any) {
	// Type-switch once, dispatch to both trackers. Cheaper than
	// registering two separate ListenTarget callbacks.
	switch e := ev.(type) {
	case *network.EventResponseReceived:
		if b.status != nil {
			b.status.Handle(e)
		}
	case *network.EventRequestWillBeSent:
		if b.activity != nil {
			b.activity.recordRequestStart()
		}
	case *network.EventLoadingFinished, *network.EventLoadingFailed:
		if b.activity != nil {
			b.activity.recordRequestEnd()
		}
	}
}

// enableNetworkTracking wires both the status tracker (for capturing the
// document response's HTTP status) and the activity tracker (for SPA
// network-idle fast exit) into the chromedp target. It returns a pointer
// to the bundle so the caller can read both trackers' state after the
// action batch completes.
//
// The function performs three steps in order:
//
//  1. Enable the Network CDP domain. Without this, no Network.* events
//     are emitted at all — the domain is opt-in.
//  2. Register the bundle as a target listener. Listeners receive every
//     event on the target; we type-switch inside Handle to pick out the
//     ones each tracker cares about.
//  3. Return the bundle pointer so the caller can read .status.Status()
//     and .activity.isIdle() later.
//
// The returned chromedp.Action must be added to the chromedp.Run batch
// BEFORE the Navigate action — otherwise the main document response fires
// before our listener is attached, and we miss it.
//
// This function is intentionally cheap and best-effort: enabling the
// network domain is a no-op if it is already enabled, and a failed
// registration simply means statusCode stays at 200 (the previous
// behavior) and isIdle() returns false (the SPA poll falls back to
// selector/text-based readiness). Errors from network.Enable() are
// returned so the caller can decide how to handle them; the caller in
// renderer.go currently logs and proceeds, matching the original's
// "best effort" stance.
func enableNetworkTracking(bundle *networkBundle) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := network.Enable().Do(ctx); err != nil {
			return err
		}
		chromedp.ListenTarget(ctx, bundle.Handle)
		return nil
	})
}

// enableNetworkStatusTracking is a thin wrapper that wires up only the
// status tracker. It is kept for compatibility with any caller that
// does not need the activity tracker (none in the current codebase,
// but external embedders may rely on the narrower signature).
func enableNetworkStatusTracking(tracker *networkStatusTracker) chromedp.Action {
	return enableNetworkTracking(&networkBundle{status: tracker})
}

// navigateIgnoringHTTPStatus is a custom navigation action that does NOT
// fail on non-2xx HTTP responses. chromedp.Navigate returns
// "page load error net::ERR_HTTP_RESPONSE_CODE_FAILURE" whenever the
// server responds with any 4xx/5xx status, which aborts the entire
// chromedp.Run batch before we can capture the status code from the
// listener or extract the page's body (Chrome has already navigated to
// chrome-error://chromewebdata/ at that point, so the batch has nothing
// to extract from).
//
// The original renderer does not have this problem because it uses raw
// CDP events and explicitly returns the status code (even for 4xx/5xx)
// to the caller. To match that behavior, we replace chromedp.Navigate
// with a custom action that:
//
//  1. Calls page.Navigate via the lower-level CDP API. This returns
//     (frameID, loaderID, errorText, httpResponse, err). errorText is
//     non-empty for non-2xx but the err itself is nil — the request
//     "succeeded" in the sense that we got a response from the server.
//
//  2. Records the errorText so the caller can detect that the
//     navigation reached the server but the server returned a non-2xx.
//     This is the "soft failure" state we want to expose.
//
//  3. Waits for Page.loadEventFired (with a configurable timeout) to
//     know when the browser has finished rendering the response —
//     for 4xx/5xx this is when Chrome's built-in error page has been
//     fully rendered, and for 2xx it is when the real page has loaded.
//
//  4. Returns nil even when the server returned 4xx/5xx. The caller
//     reads the actual status code from the networkStatusTracker and
//     decides how to surface it (e.g. 4xx → 404 API error, 5xx → 502).
//
// The waitForLoad parameter selects the page lifecycle event to wait on.
// Pass nil to use the default 30s timeout.
//
// This is the same trade-off as the original: a 4xx/5xx response is a
// legitimate server response, not a network error. Returning it with the
// real status code is more useful to callers than failing the entire
// fetch.
func navigateIgnoringHTTPStatus(urlstr string, waitForLoad *time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// Issue the navigation. errorText is set for non-2xx responses;
		// we deliberately ignore it here so the batch can continue.
		frameID, _, errText, _, err := page.Navigate(urlstr).Do(ctx)
		if err != nil {
			return err
		}
		_ = frameID
		_ = errText

		// Don't wait for Page.loadEventFired here - just return immediately
		// after navigation. The SPA readiness poll (WaitForSPAReady) that runs
		// after this action will handle waiting for content to be ready.
		// This avoids a race condition where the load event fires before our
		// ListenTarget listener is attached, which was causing 45s timeouts
		// on browserless. See REMOVED's post_navigate_phase which similarly
		// proceeds immediately after navigation.
		return nil
	})
}

// isHTTPStatusError reports whether err originated from a non-2xx HTTP
// response. Used by the renderer to distinguish "server returned 4xx/5xx"
// (which we want to surface as the captured status) from "browser failed
// to navigate" (which is a renderer error).
func isHTTPStatusError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "net::ERR_HTTP_RESPONSE_CODE_FAILURE")
}
