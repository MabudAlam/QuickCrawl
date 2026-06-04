package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// autoscroll.go implements auto-scroll for lazy-loaded content in the
// /v1/scrape-core endpoint.
//
// The problem: many modern pages do not render their full content on
// first load. They use lazy-loading strategies (e.g., <img loading=lazy>,
// data-src, IntersectionObserver-driven mounts, infinite scroll panes)
// so a single snapshot of the document misses content that has not yet
// been triggered into rendering. The original /v1/scrape endpoint
// addresses this with autoScrollScript in
// internal/renderer/browser_scripts.go, which scrolls in fixed steps
// and re-snapshots. This file is the chromedp-native equivalent.
//
// Two strategies are combined into a single JS payload that runs in one
// CDP round-trip:
//
//  1. Pre-load lazy images (Strategy 1 from the design discussion).
//     Bypasses IntersectionObserver entirely by reassigning the src
//     attribute from data-src and forcing loading=eager. Often
//     finishes the job with zero scrolling.
//
//  2. Scroll + race termination (Strategy 3). After the pre-load
//     pass, we enter a scroll loop that scrolls 90% of the viewport
//     on each step. The loop terminates as soon as ANY of these
//     signals fires:
//     - height-stagnant >= StagnantLimit (page has stopped growing)
//     - end-marker visible (EndSelector found in the viewport)
//     - step count reached MaxSteps
//
// The JS takes its options as a single JSON argument so the Go side
// does not need to string-interpolate booleans/strings/numbers — the
// JSON contract is enforced by encoding/json on Go's side and JSON.parse
// on JS's side.
//
// Item-count termination (Strategy 2 from the design discussion) was
// considered and rejected: it requires knowing the item selector, which
// is page-specific information not available at scrape time. A
// heuristic-based item detector could be layered on later if real
// failures drive the need, but height-stagnation + end-marker race
// cover the common cases without false positives.

// AutoScrollOptions configures a single AutoScrollAction call.
//
// The zero value yields a sensible default behavior: scroll in 30 steps
// of 90% viewport height, 200ms pause between steps, 3 stagnant steps
// before terminating, and lazy images eagerly loaded. Window scroll is
// used (no ContainerSelector) and no early-exit marker is configured.
type AutoScrollOptions struct {
	// MaxSteps is the hard cap on the number of scroll iterations.
	// 0 falls back to 30. Each step scrolls 90% of the viewport
	// height (or 90% of ContainerSelector's clientHeight when
	// configured).
	MaxSteps int

	// PauseMs is the wait between successive scroll steps, in
	// milliseconds. 0 falls back to 200. A larger value gives the
	// page more time to render newly-visible content; a smaller
	// value finishes faster but may undershoot on slow pages.
	PauseMs int

	// StagnantLimit is the number of consecutive scroll steps during
	// which document height did not grow before terminating with
	// reason="stagnant". 0 falls back to 3. A larger value tolerates
	// slower pages; a smaller value exits earlier on a fully-loaded
	// page. With StagnantLimit=1, the loop exits on the first step
	// that did not grow the page — usually too aggressive; the
	// default 3 means three flat steps in a row.
	StagnantLimit int

	// ContainerSelector, when non-empty, scrolls the matched element
	// instead of window. Use this for SPA scroll panes (e.g., the
	// chat scroll area in a chat app, the card grid in a feed).
	// The selector is passed to document.querySelector; an empty
	// string means window scroll.
	ContainerSelector string

	// EndSelector, when non-empty, is checked each step and the
	// loop exits early with reason="endMarker" as soon as the
	// matched element is in the viewport. Use this when you know
	// the page has a sentinel element that only renders when the
	// full content is loaded (e.g., a "You've reached the end"
	// footer, a "Load more" button that disappears after the last
	// batch). An empty string disables the early-exit check.
	EndSelector string

	// LoadLazyImages controls whether the JS payload eagerly loads
	// lazy images as its first pass (before any scrolling). It runs
	// unconditionally when true. False disables the pre-load pass.
	// Note: the boolean zero value is false; callers who want the
	// default-true behavior can leave this field unconfigured —
	// the helper treats zero as "use the recommended default" and
	// substitutes true internally. This is a deliberate convenience
	// given the design we settled on (always eager-load unless the
	// caller explicitly opts out).
	LoadLazyImages bool

	// MaxHeightGrowth caps the total height growth from the page's
	// initial measured height. The loop exits with reason="maxGrowth"
	// as soon as the document (or container) scrollHeight has grown
	// by more than this many pixels from the baseline captured on
	// the first iteration. Zero means unlimited.
	//
	// This is a more meaningful "depth" control than MaxSteps for
	// pages with predictable row sizes (e.g., 80px per item, cap at
	// 100 items = MaxHeightGrowth=8000). MaxSteps caps iteration
	// count; MaxHeightGrowth caps actual progress. Use whichever
	// maps to the page's content model.
	MaxHeightGrowth int

	// MaxDurationMs caps the total wall-clock time spent in the
	// loop, measured from the first iteration's start. The loop
	// exits with reason="maxDuration" when the budget is exhausted.
	// Zero means unlimited. Useful as a defensive cap when neither
	// MaxSteps nor MaxHeightGrowth is a good fit (e.g., a page that
	// loads slowly with a known latency budget).
	MaxDurationMs int
}

// AutoScrollResult is the JSON-decoded outcome of one AutoScrollAction
// call. The helper always populates Reason and Steps; Height is the
// document height observed on the terminating tick.
type AutoScrollResult struct {
	// Reason is one of "stagnant", "endMarker", "maxSteps",
	// "maxGrowth", "maxDuration". "error" is reserved for future
	// use; the helper currently logs and returns a zero result on
	// transport errors.
	Reason string `json:"reason"`

	// Steps is the number of iterations executed before the loop
	// exited. 0 means the loop did not run (e.g., the page had no
	// scroll height to begin with — single-screen pages).
	Steps int `json:"steps"`

	// Height is the document (or container) scrollHeight observed
	// on the terminating tick. Useful for diagnostics.
	Height int `json:"height"`
}

// autoScrollJSBody is the JavaScript payload run inside the page. It is
// an async arrow function taking a single opts argument; the Go side
// serializes the configured AutoScrollOptions as JSON and invokes
// `(autoScrollJSBody)(<json>)`. The function performs the pre-load
// pass, then enters the scroll loop, and returns a JSON object
// describing the outcome.
//
// The function is intentionally a single self-contained expression
// (not a string of inline statements) so it can be passed to
// chromedp.Evaluate without any wrapper IIFE. The trailing call
// `(opts) => { ... }(...)` is supplied by the Go wrapper.
const autoScrollJSBody = `
async (opts) => {
  // ── Pre-load lazy images (zero scroll, often finishes the job) ───
  if (opts.loadLazyImages !== false) {
    try {
      document.querySelectorAll('img[loading="lazy"]').forEach(function(img) {
        // Use getAttribute for the empty-check because img.src
        // (the IDL property) resolves to the document's base URL
        // when the src attribute is empty — making !img.src a
        // tautology for placeholder images. The raw attribute
        // value is what we want to test.
        if (img.dataset && img.dataset.src && !img.getAttribute('src')) {
          img.src = img.dataset.src;
        }
        img.loading = 'eager';
        if (img.decode) img.decode().catch(function() {});
      });
      document.querySelectorAll('img[data-src]:not([src])').forEach(function(img) {
        img.src = img.dataset.src;
      });
    } catch (_) {}
  }

  // ── Resolve scrollable element: window vs container ───────────────
  var scrollEl = null;
  if (opts.containerSelector) {
    scrollEl = document.querySelector(opts.containerSelector);
  }

  function measureHeight() {
    if (scrollEl) return scrollEl.scrollHeight;
    return Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
  }
  function stepSize() {
    var h = scrollEl ? scrollEl.clientHeight : window.innerHeight;
    return Math.floor(h * 0.9);
  }
  function scroll(dy) {
    if (scrollEl) scrollEl.scrollBy(0, dy);
    else window.scrollBy(0, dy);
  }
  function isEndVisible(sel) {
    if (!sel) return false;
    var el = document.querySelector(sel);
    if (!el) return false;
    var r = el.getBoundingClientRect();
    return r.top < window.innerHeight && r.bottom > 0;
  }

  // ── Scroll + race termination ─────────────────────────────────────
  var stagnant = 0, lastHeight = 0, i = 0;
  var maxSteps = opts.maxSteps | 0 || 30;
  var stagnantLimit = opts.stagnantLimit | 0 || 3;
  var pauseMs = opts.pauseMs | 0 || 200;
  var maxGrowth = (opts.maxHeightGrowth | 0) || 0;
  var maxDurationMs = (opts.maxDurationMs | 0) || 0;
  var startTime = Date.now();
  var baselineHeight = measureHeight();

  for (i = 0; i < maxSteps; i++) {
    var h = measureHeight();
    if (h > lastHeight) {
      stagnant = 0;
      lastHeight = h;
    } else {
      stagnant++;
      if (stagnant >= stagnantLimit) {
        return JSON.stringify({ reason: 'stagnant', steps: i, height: h });
      }
    }

    if (isEndVisible(opts.endSelector)) {
      return JSON.stringify({ reason: 'endMarker', steps: i, height: h });
    }

    if (maxGrowth > 0 && (h - baselineHeight) >= maxGrowth) {
      return JSON.stringify({ reason: 'maxGrowth', steps: i, height: h });
    }

    if (maxDurationMs > 0 && (Date.now() - startTime) >= maxDurationMs) {
      return JSON.stringify({ reason: 'maxDuration', steps: i, height: h });
    }

    scroll(stepSize());
    await new Promise(function(r) { setTimeout(r, pauseMs); });
  }

  return JSON.stringify({ reason: 'maxSteps', steps: maxSteps, height: measureHeight() });
}
`

// applyAutoScrollDefaults fills in the documented defaults on a copy
// of opts and returns the effective configuration. Mutating the
// caller's struct would be surprising, so we always work on a copy.
func applyAutoScrollDefaults(opts AutoScrollOptions) AutoScrollOptions {
	eff := opts
	if eff.MaxSteps <= 0 {
		eff.MaxSteps = 30
	}
	if eff.PauseMs <= 0 {
		eff.PauseMs = 200
	}
	if eff.StagnantLimit <= 0 {
		eff.StagnantLimit = 3
	}
	// LoadLazyImages: the caller almost always wants this on,
	// and the cost on pages without lazy images is one no-op
	// querySelectorAll. We default to true and let callers
	// explicitly opt out by setting it to false. The JS uses
	// !== false so the JSON-serialized zero value (false)
	// disables it; we therefore substitute true here when the
	// caller did not set it.
	eff.LoadLazyImages = true
	return eff
}

// buildAutoScrollExpression assembles the full JS expression to
// evaluate: an IIFE that invokes the payload with the JSON-serialized
// options. The expression is `(payload)(<json>)`. We define the JSON
// shape inline (lowercase tags) so the Go side and the JS side
// agree on the field names without relying on Go's default
// capitalization.
func buildAutoScrollExpression(opts AutoScrollOptions) (string, error) {
	optsForJS := struct {
		MaxSteps          int    `json:"maxSteps"`
		PauseMs           int    `json:"pauseMs"`
		StagnantLimit     int    `json:"stagnantLimit"`
		ContainerSelector string `json:"containerSelector"`
		EndSelector       string `json:"endSelector"`
		LoadLazyImages    bool   `json:"loadLazyImages"`
		MaxHeightGrowth   int    `json:"maxHeightGrowth"`
		MaxDurationMs     int    `json:"maxDurationMs"`
	}{
		MaxSteps:          opts.MaxSteps,
		PauseMs:           opts.PauseMs,
		StagnantLimit:     opts.StagnantLimit,
		ContainerSelector: opts.ContainerSelector,
		EndSelector:       opts.EndSelector,
		LoadLazyImages:    opts.LoadLazyImages,
		MaxHeightGrowth:   opts.MaxHeightGrowth,
		MaxDurationMs:     opts.MaxDurationMs,
	}
	optsJSON, err := json.Marshal(optsForJS)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s)(%s)", autoScrollJSBody, string(optsJSON)), nil
}

// RunAutoScroll executes the auto-scroll loop against the current
// chromedp target and returns the JSON-decoded result. It is the
// lower-level function used by AutoScrollAction and by tests that
// want to assert on Reason/Steps/Height without going through the
// swallowed-error wrapper.
//
// The supplied ctx must be a chromedp executor context (i.e., it has
// been initialized by chromedp.NewContext or is itself the executor
// passed into a chromedp.ActionFunc).
//
// Errors are returned for transport failures (CDP error, JS exception)
// and for malformed result payloads. AutoScrollAction wraps this
// function and logs-and-swallows both error classes; tests that want
// to assert on the result should call RunAutoScroll directly.
func RunAutoScroll(ctx context.Context, opts AutoScrollOptions) (AutoScrollResult, error) {
	eff := applyAutoScrollDefaults(opts)

	expr, err := buildAutoScrollExpression(eff)
	if err != nil {
		return AutoScrollResult{}, fmt.Errorf("autoscroll: marshal opts: %w", err)
	}

	// AwaitPromise=true is required because the payload is an async
	// function; without it, Evaluate returns the unresolved Promise.
	// ReturnByValue=true is not strictly required (we return a
	// string from the payload, which chromedp unmarshals via the
	// JSON contract), but being explicit guards against future
	// changes that return objects.
	var rawResult string
	if err := chromedp.Evaluate(expr, &rawResult,
		func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			p.AwaitPromise = true
			p.ReturnByValue = true
			return p
		},
	).Do(ctx); err != nil {
		return AutoScrollResult{}, fmt.Errorf("autoscroll: evaluate: %w", err)
	}

	var result AutoScrollResult
	if err := json.Unmarshal([]byte(rawResult), &result); err != nil {
		return AutoScrollResult{}, fmt.Errorf("autoscroll: decode result %q: %w", rawResult, err)
	}
	return result, nil
}

// AutoScrollAction returns a chromedp.Action that runs the auto-scroll
// loop in the current target.
//
// The action is best-effort: a transport error or a JS exception is
// logged and the action returns nil rather than failing the scrape.
// A scroll that fails to run is far less costly than a snapshot that
// is never returned.
//
// The action must be invoked AFTER the page has been navigated to and
// AFTER any pre-scroll readiness check (e.g., WaitForSPAReady). It is
// not safe to run it before the document has loaded — there would be
// nothing to scroll, and the JS would observe document.body.scrollHeight
// = 0. The intended insertion point in fetchWithCDPBrowser is between
// the SPA poll and the OuterHTML extraction.
//
// Example usage inside a chromedp.Run batch:
//
//	chromedp.Run(ctx,
//	    navigateIgnoringHTTPStatus(url, nil),
//	    WaitForSPAReady(...) wrapped in ActionFunc,
//	    AutoScrollAction(AutoScrollOptions{ EndSelector: ".end-marker" }),
//	    chromedp.OuterHTML("body", &html, chromedp.ByQuery),
//	)
func AutoScrollAction(opts AutoScrollOptions) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		result, err := RunAutoScroll(ctx, opts)
		if err != nil {
			utils.Log.Info("autoscroll error", "error", err)
			return nil
		}
		utils.Log.Info("autoscroll result", "reason", result.Reason, "steps", result.Steps, "height", result.Height)
		return nil
	})
}
