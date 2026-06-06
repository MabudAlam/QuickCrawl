// Package core provides the clean, chromedp-based reimplementation of the
// legacy renderer.
//
// File: fetch_block.go
//
// This file wires the blocklist into the browser's Fetch domain. The
// approach mirrors the engine's chromium renderer (engine/internal/render/chrome/renderer.go):
// we call fetch.Enable() to opt into the Fetch CDP domain, then register
// a chromedp.ListenTarget listener that reacts to every
// fetch.EventRequestPaused event. For each paused request we ask the
// blocklist whether the URL or resource type should be blocked, and
// either fail the request (network.ErrorReasonAborted) or continue it.
//
// Why Fetch domain and not network.SetBlockedURLs?
//
//   - SetBlockedURLs is a single CDP call that takes a flat list of URL
//     strings matched by Chrome's own substring matcher. It supports
//     neither resource-type filtering nor the rich pattern syntax the
//     blocklist already provides (exact, wildcard, case-sensitive and
//     case-insensitive regexp).
//
//   - The Fetch domain pauses every sub-request and gives us a
//     callback. The callback can decide per-request based on URL
//     pattern, resource type, or any future dimension. The cost is one
//     extra goroutine per sub-request, but sub-requests are short-lived
//     (typically <500ms) and goroutine creation on Linux is cheap
//     (~2us).
//
// Where the action fits in the action sequence:
//
//  1. enableNetworkTracking (network.Enable + ListenTarget for status/activity)
//  2. fetchBlockAction (fetch.Enable + ListenTarget for blocklist)
//  3. stealthInjectionAction
//  4. navigateIgnoringHTTPStatus
//  5. SPA poll / Sleep / extract
//
// fetch.Enable must run BEFORE navigate. The Fetch domain is opt-in like
// Network — without fetch.Enable, no fetch.EventRequestPaused events are
// emitted. The listener must be attached before the first sub-request
// fires, which happens during navigation. Putting the action at index 1
// (right after enableNetworkTracking) satisfies both constraints.
//
// Performance notes:
//
//   - The listener is registered with chromedp.ListenTarget on the same
//     target as the network tracker. chromedp dispatches events on a
//     single goroutine; if we used a blocking CDP command in the
//     handler it would serialize all events. Instead we spawn a
//     goroutine per paused request so the listener returns immediately
//     and the next event is delivered without delay.
//
//   - FailRequest and ContinueRequest both run on a 2-second timeout
//     derived from the parent ctx. If the parent context is cancelled
//     (chromedp.Run returned), the timeout fires immediately and the
//     goroutine exits without leaking.
//
//   - The blocklist is built once per request (customPatterns + global
//     patterns + resource types) and shared across all sub-requests.
//     Building it takes ~10-30us for the default global list of 32
//     patterns; this is negligible compared to the 1-3ms cost of a
//     Fetch.requestPaused round-trip.
package core

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type blockedTracker struct {
	mu   sync.Mutex
	urls []string
}

func (t *blockedTracker) add(url string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.urls = append(t.urls, url)
}

func (t *blockedTracker) get() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.urls))
	copy(out, t.urls)
	return out
}

// fetchBlockAction returns a chromedp.Action that enables the Fetch CDP
// domain and registers a request-interception listener. The listener
// evaluates the hardcoded global blocklist (globalBlockedPatterns in
// internal/core/blocklist.go) against every sub-request issued by the
// page and either fails it (network.ErrorReasonAborted) or continues it
// unmodified.
//
// The blocklist is built once per request from the package-level
// globalBlockedPatterns slice. There is no per-request or per-config
// pattern override — the global list is the entire policy. Operators
// who need to extend the list edit globalBlockedPatterns in place.
//
// tracker is used to record the URLs of blocked requests for logging.
// Pass nil to discard tracking.
func fetchBlockAction(tracker *blockedTracker) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		bl := NewBlocklist(nil)

		if err := fetch.Enable().Do(ctx); err != nil {
			return err
		}

		chromedp.ListenTarget(ctx, func(ev interface{}) {
			paused, ok := ev.(*fetch.EventRequestPaused)
			if !ok {
				return
			}

			go func(p *fetch.EventRequestPaused) {
				handleBlockedRequest(ctx, bl, p, tracker)
			}(paused)
		})

		return nil
	})
}

func handleBlockedRequest(parent context.Context, bl *Blocklist, p *fetch.EventRequestPaused, tracker *blockedTracker) {
	cmdCtx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	c := chromedp.FromContext(cmdCtx)
	execCtx := cdp.WithExecutor(cmdCtx, c.Target)

	blockedByURL := bl.IsBlocked(p.Request.URL)
	blockedByResourceType := bl.IsResourceTypeBlocked(string(p.ResourceType))

	if blockedByURL || blockedByResourceType {
		if tracker != nil {
			tracker.add(p.Request.URL)
		}
		_ = fetch.FailRequest(p.RequestID, network.ErrorReasonAborted).Do(execCtx)
		return
	}

	_ = fetch.ContinueRequest(p.RequestID).Do(execCtx)
}
