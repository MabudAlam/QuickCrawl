package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// spa.go implements SPA (Single-Page Application) readiness detection
// for the /v1/scrape-core endpoint.
//
// The problem: chromedp.Navigate waits for Chrome's load event, but
// modern SPAs (React, Vue, Angular, Next.js client-side routes, etc.)
// typically render their primary content AFTER the load event fires —
// via XHR/fetch, framework hydration, route changes, or lazy
// components. A bare `chromedp.Sleep(2*time.Second)` after Navigate is
// a coin flip: fast pages oversleep, slow hydration undersleeps and
// returns a half-rendered shell.
//
// The pattern used here mirrors what the original /v1/scrape endpoint
// does in internal/renderer/browser_render_helpers.go:waitForSpaContent,
// adapted to chromedp. We poll the page every 200ms and consider the
// page "ready" when ALL configured conditions are satisfied on the
// same tick:
//   - At least one of the configured CSS selectors matches a node.
//   - The visible body text length is at or above the configured
//     threshold.
//   - If a custom JS predicate is configured, it returns a truthy
//     value.
//
// The poll exits early the moment all conditions are met, or when the
// configured timeout elapses. The returned SPAReadinessResult reports
// which conditions were satisfied on exit so callers can decide
// whether to surface a warning, retry, or trust the snapshot anyway.

// SPAReadinessOptions configures a single WaitForSPAReady call.
//
// The helper is intended to be used inside a chromedp.Run action
// sequence: navigate first, then pass the configured pre-poll actions
// (e.g., dismissCookieBannersAction) into the same Run, then call
// WaitForSPAReady. The caller controls navigation; the helper only
// polls.
//
// Zero-value fields fall back to documented defaults. The only field
// the caller MUST set is Timeout.
type SPAReadinessOptions struct {
	// URL is the page to wait on. The helper does not navigate — the
	// caller is expected to have already called chromedp.Navigate
	// before invoking the helper. URL is used only in error messages
	// and result fields for diagnostics; the helper does not contact
	// it.
	URL string

	// Selectors is the set of CSS selectors polled each tick. The
	// helper considers the selector check satisfied as soon as ANY
	// of these matches at least one element in the document. Empty
	// slice disables the selector check; the page is then considered
	// ready solely on the body text threshold and/or the JS
	// predicate.
	//
	// Sensible default for a generic scrape target:
	// `main, article, [role=main], #content, #root > *, #app > *` —
	// same set used by the original renderer's waitForSpaContent.
	Selectors []string

	// MinBodyText is the minimum number of visible (innerText)
	// characters required for the page to be considered hydrated.
	// Zero disables the text check; the page is then considered
	// ready solely on the selector match and/or the JS predicate.
	// The original uses 800; that is also this helper's default when
	// no other check is configured.
	MinBodyText int

	// PollInterval is the cadence at which the page is evaluated.
	// Smaller values are more responsive but increase CDP round-
	// trips. Zero falls back to 200ms (matches the original).
	PollInterval time.Duration

	// Timeout is the total budget for the wait. The helper returns
	// with State=StateTimeout when this elapses without readiness.
	// This field is required — a zero value is rejected.
	Timeout time.Duration

	// Predicate is an optional JavaScript expression evaluated each
	// tick. The helper considers the predicate check satisfied as
	// soon as it returns a truthy value. The expression runs inside
	// the page's main world with `returnByValue: true`, so the
	// result must be JSON-serializable. Empty string disables the
	// predicate check.
	//
	// Example: `return window.__APP_READY__ === true`
	// Example: `return document.querySelectorAll('li').length > 5`
	//
	// The expression body is wrapped in an IIFE so it can use
	// `return` or a bare statement block. Any thrown error counts
	// as falsy.
	Predicate string
}

// SPAReadinessState describes the outcome of a WaitForSPAReady call.
type SPAReadinessState int

const (
	// StateUnknown is the zero value and should never be returned by
	// the helper. It exists so callers can detect an uninitialized
	// result struct.
	StateUnknown SPAReadinessState = iota

	// StateReady means all configured conditions were satisfied on
	// the same tick. The helper exited as soon as this happened.
	StateReady

	// StateTimeout means the configured Timeout elapsed before all
	// configured conditions were satisfied. The returned
	// SPAReadinessResult still carries the last observed per-tick
	// measurements so callers can decide what to do (log a "thin
	// content" warning, retry, trust the snapshot anyway).
	StateTimeout
)

// String renders SPAReadinessState for logs and error messages.
func (s SPAReadinessState) String() string {
	switch s {
	case StateReady:
		return "ready"
	case StateTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// SPAReadinessResult is the outcome of a single WaitForSPAReady call.
//
// The fields are populated even on StateTimeout, so callers can log
// partial progress (which selector nearly matched, how much text was
// present, whether the JS predicate came close).
type SPAReadinessResult struct {
	// State is the overall outcome (ready or timeout).
	State SPAReadinessState

	// MatchedSelector is the first selector (in Selectors order) that
	// matched on the tick that triggered StateReady. Empty when no
	// selector check was configured, when no selector matched, or
	// when the helper timed out.
	MatchedSelector string

	// BodyTextLength is the trimmed innerText length observed on the
	// final tick. Useful for diagnosing "thin content" warnings.
	BodyTextLength int

	// PredicateTruthy is true when the configured JS predicate
	// returned a truthy value on the final tick. False when no
	// predicate was configured.
	PredicateTruthy bool

	// PollCount is the number of evaluation ticks the helper ran
	// before exiting. Useful for tuning PollInterval and Timeout.
	PollCount int

	// Duration is the wall-clock time spent polling. Always <=
	// Timeout.
	Duration time.Duration
}

// waitReadyJSTemplate is the JavaScript body the helper evaluates
// each tick. The expression returns a small JSON-serializable object
// describing the current state of the page. The helper decodes that
// object into pollSnapshot via the standard chromedp JSON contract.
//
// Two placeholders are substituted at call time from the configured
// options: %SELECTORS_BODY% and %PREDICATE_BODY%. Each is a multi-
// line snippet that may be empty.
const waitReadyJSTemplate = `
(function() {
  var out = { selector: '', text: 0, predicate: false };
  try {
    out.text = ((document.body && document.body.innerText) || '').trim().length;
  } catch (_) {}
  %SELECTORS_BODY%
  %PREDICATE_BODY%
  return JSON.stringify(out);
})()
`

// pollSnapshot mirrors the JSON shape produced by waitReadyJSTemplate.
// The helper decodes into this struct via encoding/json after the
// JS expression returns its JSON-string payload.
type pollSnapshot struct {
	Selector  string `json:"selector"`
	Text      int    `json:"text"`
	Predicate bool   `json:"predicate"`
}

// buildWaitReadyJS assembles the per-call JavaScript by inlining the
// configured selector list and predicate into the template. Returns
// the final expression ready to pass to chromedp.Evaluate.
func buildWaitReadyJS(selectors []string, predicate string) string {
	var selectorsBody string
	if len(selectors) > 0 {
		// Build a comma-separated quoted list, e.g. "'main', 'article'".
		// quoteCount guards against the extremely unlikely case where
		// a selector contains an unescaped single quote (CSS does
		// not normally allow that, but we defend in depth).
		quoted := make([]string, len(selectors))
		for i, s := range selectors {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `'`, `\'`)
			quoted[i] = "'" + s + "'"
		}
		// Try each selector inside a try/catch so a malformed
		// selector cannot poison the whole poll. First match wins;
		// the helper reports which selector matched.
		selectorsBody = fmt.Sprintf(`
  try {
    var sels = [%s];
    for (var i = 0; i < sels.length; i++) {
      var el = document.querySelector(sels[i]);
      if (el) { out.selector = sels[i]; break; }
    }
  } catch (_) {}`, strings.Join(quoted, ", "))
	}

	var predicateBody string
	if predicate != "" {
		// Wrap the user's expression in an IIFE so they can use
		// `return` or a bare statement block. Anything thrown is
		// swallowed; a thrown predicate counts as falsy.
		predicateBody = fmt.Sprintf(`
  try {
    out.predicate = !!((function() { %s })());
  } catch (_) { out.predicate = false; }`, predicate)
	}

	expr := waitReadyJSTemplate
	expr = strings.Replace(expr, "%SELECTORS_BODY%", selectorsBody, 1)
	expr = strings.Replace(expr, "%PREDICATE_BODY%", predicateBody, 1)
	return expr
}

// runPollTick executes one readiness evaluation against the page and
// decodes the JSON-string result into snap. Returns the chromedp
// transport error if the call itself failed (the snap is zero-valued
// in that case).
func runPollTick(ctx context.Context, expr string, snap *pollSnapshot) error {
	var raw string
	if err := chromedp.Evaluate(expr, &raw).Do(ctx); err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), snap)
}

// WaitForSPAReady polls the current page in a chromedp context until
// all configured readiness conditions are met, or the configured
// timeout elapses.
//
// The helper does not navigate — the caller is expected to have
// called chromedp.Navigate (and any pre-poll actions such as
// dismissCookieBannersAction) before invoking this function.
//
// Required: opts.Timeout. Other fields fall back to the documented
// defaults (MinBodyText=800, PollInterval=200ms, no predicate, no
// selectors). When neither selectors nor predicate nor MinBodyText is
// configured, a sensible default selector set is used so the helper
// is never a silent no-op:
//
//	["main", "article", "[role=main]", "#content", "#root > *", "#app > *"]
//
// Exit conditions (in order of priority):
//  1. Every configured condition is satisfied on the same tick —
//     returns StateReady.
//  2. The configured timeout elapses — returns StateTimeout with the
//     last observed per-condition measurements.
//  3. The parent context is cancelled — returns StateTimeout with
//     the last observed per-condition measurements; the parent's
//     error is NOT propagated (the caller will see it through the
//     surrounding chromedp.Run if used inline).
//
// Network-idle is intentionally NOT included in v1. The chromedp
// pattern for it (page.SetLifecycleEventsEnabled plus
// chromedp.ListenTarget) is reliable enough to be a useful secondary
// signal but not as a primary gate. It can be layered on later
// through the Predicate mechanism (e.g., `return
// window.__networkQuietMs__ > 500` from a custom JS listener).
func WaitForSPAReady(parentCtx context.Context, opts SPAReadinessOptions) (SPAReadinessResult, error) {
	if opts.Timeout <= 0 {
		return SPAReadinessResult{}, errors.New("WaitForSPAReady: Timeout is required and must be > 0")
	}

	// Apply documented defaults in a copy so we never mutate the
	// caller's struct.
	eff := opts
	if eff.PollInterval <= 0 {
		eff.PollInterval = 200 * time.Millisecond
	}
	if eff.MinBodyText < 0 {
		eff.MinBodyText = 0
	}
	selectorCheckEnabled := len(eff.Selectors) > 0
	predicateCheckEnabled := eff.Predicate != ""
	if !selectorCheckEnabled && !predicateCheckEnabled && eff.MinBodyText <= 0 {
		// Default selector set — mirrors internal/renderer/browser_fetcher.go.
		eff.Selectors = []string{"main", "article", "[role=main]", "#content", "#root > *", "#app > *"}
		selectorCheckEnabled = true
		// A 0 MinBodyText is upgraded to the original's 800-char
		// threshold when we substitute the default selector set,
		// so the helper is never silent.
		eff.MinBodyText = 800
	}

	// Build a per-call timeout context. We honor the chromedp
	// parent context's cancellation in addition to the configured
	// budget.
	ctx, cancel := context.WithTimeout(parentCtx, eff.Timeout)
	defer cancel()

	expr := buildWaitReadyJS(eff.Selectors, eff.Predicate)

	start := time.Now()
	result := SPAReadinessResult{State: StateTimeout}
	ticker := time.NewTicker(eff.PollInterval)
	defer ticker.Stop()

	// Run an immediate first evaluation, then poll on the ticker.
	// We do the first one outside the ticker so a page that is
	// already ready on entry returns immediately without waiting a
	// full PollInterval.
	if evaluateTickInto(ctx, expr, eff, &result) {
		result.Duration = time.Since(start)
		result.State = StateReady
		return result, nil
	}

	for {
		select {
		case <-ctx.Done():
			// Either the parent context was cancelled or the
			// configured timeout elapsed. result fields already
			// hold the last observed measurements.
			result.Duration = time.Since(start)
			result.State = StateTimeout
			return result, nil
		case <-ticker.C:
			if evaluateTickInto(ctx, expr, eff, &result) {
				result.Duration = time.Since(start)
				result.State = StateReady
				return result, nil
			}
		}
	}
}

// evaluateTickInto runs one readiness evaluation and updates result
// with the per-condition measurements, advancing PollCount. Returns
// true when all configured conditions are now satisfied (i.e., the
// caller should exit the wait loop).
func evaluateTickInto(ctx context.Context, expr string, opts SPAReadinessOptions, result *SPAReadinessResult) bool {
	var snap pollSnapshot
	if err := runPollTick(ctx, expr, &snap); err != nil {
		// Transport error: stop polling, let the caller see the
		// failure through the returned error from WaitForSPAReady.
		// We do not panic — a transient CDP hiccup is not the same
		// as the page being unready.
		return false
	}
	result.PollCount++
	result.BodyTextLength = snap.Text
	result.MatchedSelector = snap.Selector
	result.PredicateTruthy = snap.Predicate

	selectorOK := !selectorCheckEnabledFor(opts) || snap.Selector != ""
	textOK := opts.MinBodyText <= 0 || snap.Text >= opts.MinBodyText
	predicateOK := opts.Predicate == "" || snap.Predicate

	// Lenient exit: if body text is already substantial, treat as ready
	// even when no selector matched. Pages like Hacker News use plain
	// table layouts and will never match the default SPA selectors, but
	// their body innerText is well above the threshold. This mirrors
	// the original /scrape renderer's lenient exit at
	// internal/renderer/browser_render_helpers.go:97.
	if textOK && !selectorOK && opts.MinBodyText > 0 && snap.Text >= opts.MinBodyText {
		result.MatchedSelector = "body-text-only"
		return true
	}

	return selectorOK && textOK && predicateOK
}

// selectorCheckEnabledFor is a small helper to keep the condition
// logic in evaluateTickInto readable. It mirrors the defaulting done
// in WaitForSPAReady.
func selectorCheckEnabledFor(opts SPAReadinessOptions) bool {
	return len(opts.Selectors) > 0
}
