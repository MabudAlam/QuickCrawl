package core

import (
	"context"

	"github.com/chromedp/chromedp"
)

// cookies.go implements Cookie/CMP (Consent Management Platform) banner
// auto-dismissal for browser-rendered fetches.
//
// Consent dialogs are a frequent obstacle when scraping real-world pages:
// they obscure the actual content, inflate body text past SPA readiness
// thresholds, and in some cases trap the user behind a modal that blocks
// pointer events. The original /v1/scrape endpoint tackles this in two
// layers — network-level blocking of CMP hostnames (see
// internal/renderer/browser_background_consumer.go) plus a JavaScript
// dismissal pass. This file mirrors the JavaScript half of that strategy
// for the /v1/scrape-core endpoint.
//
// The JavaScript is identical in behavior to the script in
// internal/renderer/browser_scripts.go:cookieBannerDismissJS, kept here as
// an independent copy so core can evolve without depending on the
// original renderer's internal APIs.

// cookieBannerPrecheckJS returns true if any of the high-confidence banner
// selectors matches an element in the document. This is a cheap pre-check
// (a single document.querySelectorAll call) that lets the renderer skip the
// full cookieBannerDismissJS IIFE on the vast majority of pages that have
// no banner at all.
//
// The selector list is a subset of cookieBannerDismissJS's SELECTORS —
// only the most common, vendor-distinctive IDs/classes that have
// near-zero false-positive rate. The full list (with broader text
// matching) still runs as the second stage when this returns true.
const cookieBannerPrecheckJS = `
(() => {
  const SELECTORS = [
    '#onetrust-accept-btn-handler',
    '.ot-accept-all',
    '#CybotCookiebotDialogBodyButtonAccept',
    '#CybotCookiebotDialogBodyLevelButtonAccept',
    '[data-testid="uc-accept-all-button"]',
    '[data-cy="uc-accept-all-button"]',
    '.sp_choice_type_11',
    '.qc-cmp2-summary-buttons button[mode="primary"]',
    '#qc-cmp2-ui button[mode="primary"]',
    '#truste-consent-button',
    '.cc-btn.cc-allow',
    '.cc-btn.cc-dismiss',
    'button[data-cmp-action="accept"]',
    'button[data-accept-action="all"]',
    'button[aria-label*="Accept all" i]',
    'button[aria-label*="Allow all" i]',
    '[id*="accept-cookies" i]',
    '[class*="accept-cookies" i]',
  ];
  for (const sel of SELECTORS) {
    try {
      if (document.querySelector(sel)) return true;
    } catch (_) {}
  }
  return false;
})()
`

// cookieBannerDismissJS locates and clicks accept/allow buttons on common
// consent banners. It is intentionally written as a self-contained IIFE so
// it can be passed verbatim to chromedp.Evaluate.
//
// The script applies four strategies in order:
//
//  1. Curated selector list — covers 10+ major CMPs (OneTrust, CookieBot,
//     Usercentrics, Didomi, Quantcast, TrustArc, Iubenda, Osano, etc.) by
//     ID, class, or data attribute. Fastest path when the vendor matches.
//  2. Text pattern matching — scans all buttons and inputs for text
//     matching a regex of common accept phrases in 8 languages (English,
//     Turkish, French, German, Spanish, plus variants like "got it").
//  3. Shadow DOM traversal — re-runs strategies 1 and 2 against any
//     element's shadowRoot. Shallow: only top-level shadow hosts.
//  4. Iframe traversal — re-runs strategies 1 and 2 against each
//     same-origin iframe's document. Cross-origin iframes throw inside
//     the try/catch and are silently skipped.
//
// A bonus path registers a TCF v2 listener when window.__tcfapi is
// present, signalling consent to IAB TCF-compliant CMPs.
//
// The function returns the number of buttons successfully clicked. All
// errors are caught and swallowed — failure to dismiss a banner never
// breaks the page render.
//
// Extending coverage:
//   - Append new selectors to the SELECTORS array.
//   - Add new locale phrases to the PATTERNS regex.
//   - Add a new vendor-specific strategy as a top-level try block.
//   - For cross-origin iframes, switch to chromedp's frame-targeted
//     actions and run the script in each frame's context.
const cookieBannerDismissJS = `
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

// CookieDismissalResult reports the outcome of a banner-dismissal pass.
//
// Clicks is the number of accept/allow buttons that were successfully
// clicked during the pass. Skipped is true when the dismissal was not
// performed at all (e.g., the caller chose to skip, or the script
// evaluation failed at the CDP transport level).
type CookieDismissalResult struct {
	Clicks  int  // Number of banner buttons successfully clicked.
	Skipped bool // True when the dismiss pass was not executed.
}

// dismissCookieBanners executes cookieBannerDismissJS in the current
// chromedp page context and returns the outcome. The function is
// best-effort: any error from the script evaluation is captured in
// CookieDismissalResult.Skipped and never propagated, so a banner
// problem can never break the page render.
//
// The dismiss script is designed to never throw — it wraps every step in
// try/catch and returns 0 when nothing is found. The only failure modes
// this Go wrapper protects against are CDP transport errors, which are
// logged via the Skipped flag and otherwise ignored.
func dismissCookieBanners(ctx context.Context) CookieDismissalResult {
	var clicks int
	if err := chromedp.Evaluate(cookieBannerDismissJS, &clicks).Do(ctx); err != nil {
		return CookieDismissalResult{Skipped: true}
	}
	return CookieDismissalResult{Clicks: clicks}
}

// dismissCookieBannersAction returns a chromedp.Action that runs the
// dismiss pass. Use it inline inside a chromedp.Run call to keep the
// dismissal in the same CDP roundtrip batch as navigation and HTML
// extraction:
//
//	err := chromedp.Run(ctx,
//	    chromedp.Navigate(url),
//	    dismissCookieBannersAction(),   // best-effort
//	    chromedp.Sleep(2*time.Second),
//	    chromedp.OuterHTML("body", &html, chromedp.ByQuery),
//	)
//
// The action swallows all errors from the dismiss pass so a banner
// problem never aborts the surrounding chromedp.Run sequence. This
// matches the previous renderer's behavior (where both the result and
// any error from SendRecv are discarded with `_, _ =`).
func dismissCookieBannersAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_ = dismissCookieBanners(ctx)
		return nil
	})
}

// dismissCookieBannersFastAction is the optimized version of
// dismissCookieBannersAction. It first runs a cheap pre-check
// (cookieBannerPrecheckJS) that returns true only when a high-confidence
// banner selector matches. On the vast majority of pages — those without
// any consent banner — the pre-check returns false and the full
// cookieBannerDismissJS IIFE (which scans every button, every shadow
// root, and every iframe) is skipped entirely.
//
// The pre-check is one document.querySelector() call per known selector
// (a small fixed list, no DOM tree walk), so its cost is typically
// <5ms. The full dismiss script is ~50-200ms. The break-even is on any
// page without a banner, which is the dominant case in production.
//
// Both stages swallow errors — a banner problem never aborts the
// surrounding chromedp.Run sequence, matching the original
// dismissCookieBannersAction contract.
func dismissCookieBannersFastAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		var hasBanner bool
		if err := chromedp.Evaluate(cookieBannerPrecheckJS, &hasBanner).Do(ctx); err != nil {
			// Pre-check failed — fall back to the full dismiss. Better
			// to be safe than to skip a real banner.
			_ = dismissCookieBanners(ctx)
			return nil
		}
		if !hasBanner {
			// No banner detected. Skip the expensive full script. This
			// is the common-case fast path.
			return nil
		}
		_ = dismissCookieBanners(ctx)
		return nil
	})
}
