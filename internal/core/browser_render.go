package core

// Browser (chromedp/CDP) page rendering. This is the real browser fetch — the
// counterpart of the plain-HTTP fetch in http.go. Keeping it in its own file
// lets renderer.go stay a thin orchestrator (mode switch + shared plumbing).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/chromedp/chromedp"
)

// fetchWithCDPBrowser loads rawURL in a headless Chrome browser via chromedp:
// navigate, wait for JavaScript to render the page, then read back the HTML.
//
// It reuses one persistent Chrome instance (the RemoteAllocator) and just opens
// a fresh tab per request, which is far cheaper than spawning a browser process
// each time. A per-host concurrency slot stops any single origin from hogging
// the browser.
func (renderer *Renderer) fetchWithCDPBrowser(ctx context.Context, rawURL string, headers map[string]string, waitMs int64) (*FetchResult, *QuickCrawlError) {
	// No browser configured (no CDP WS URL) → nothing to do.
	if renderer.allocCtx == nil {
		return nil, ErrBrowserNotAvailable.New("no browser WS URL configured")
	}

	// Limit concurrent browser loads per host.
	release := renderer.pool.Acquire(extractHost(rawURL))
	defer release()

	// Open a tab on the shared Chrome and give this request a hard deadline.
	browserCtx, cancelTab := chromedp.NewContext(renderer.allocCtx)
	defer cancelTab()
	runCtx, cancel := context.WithTimeout(browserCtx, renderer.cfg.PageTimeout)
	defer cancel()

	// Network CDP listeners capture the page's real HTTP status (chromedp's
	// Navigate doesn't surface it) and track in-flight requests so the
	// readiness poll below can fast-exit once the page goes idle.
	bundle := &networkBundle{
		status:   &networkStatusTracker{},
		activity: newNetworkActivityTracker(),
	}
	blocked := &blockedTracker{}

	// Wait budget for JS to render: 15s by default, or the caller's waitMs.
	budget := 15 * time.Second
	if waitMs > 0 {
		budget = time.Duration(waitMs) * time.Millisecond
	}

	// Order matters: enable status/block/stealth BEFORE navigating so nothing
	// on the page escapes them. navigateIgnoringHTTPStatus (instead of
	// chromedp.Navigate) deliberately doesn't abort on 4xx/5xx responses.
	actions := []chromedp.Action{
		enableNetworkTracking(bundle),
		fetchBlockAction(blocked),
		stealthInjectionAction(renderer.cfg.StealthEnabled),
		navigateIgnoringHTTPStatus(rawURL),
	}

	spaResult := SPAReadinessResult{State: StateReady}
	if waitMs == 0 {
		// Default: dismiss cookie banners, then poll until the SPA looks
		// rendered (or the budget runs out). Error/challenge pages are skipped
		// so we don't burn the budget on something that will never hydrate.
		actions = append(actions,
			dismissCookieBannersFastAction(),
			chromedp.ActionFunc(func(ctx context.Context) error {
				if st := bundle.status.Status(); st >= 400 && st < 600 {
					return nil
				}
				res, err := WaitForSPAReady(ctx, SPAReadinessOptions{
					URL:            rawURL,
					Timeout:        budget,
					NetworkTracker: bundle.activity,
				})
				spaResult = res
				return err
			}),
		)
	} else {
		// Explicit waitFor: caller asked for a fixed delay, so just sleep.
		actions = append(actions, chromedp.Sleep(time.Duration(waitMs)*time.Millisecond))
	}

	var headHTML, bodyHTML, finalURL string
	// Scroll to trigger lazy-loaded content, then grab final URL + <head>/<body>
	// in a single JS round-trip (cheaper than several CDP commands).
	actions = append(actions,
		AutoScrollAction(AutoScrollOptions{MaxSteps: 5}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var v []any
			if err := chromedp.Evaluate(`
				(() => {
					const head = document.head ? document.head.outerHTML : '';
					const body = document.body ? document.body.outerHTML : '';
					return [head, body, window.location.href];
				})()
			`, &v).Do(ctx); err != nil {
				return err
			}
			if len(v) > 0 {
				headHTML, _ = v[0].(string)
			}
			if len(v) > 1 {
				bodyHTML, _ = v[1].(string)
			}
			if len(v) > 2 {
				finalURL, _ = v[2].(string)
			}
			return nil
		}),
	)

	if err := chromedp.Run(runCtx, actions...); err != nil {
		if strings.Contains(err.Error(), "context deadline") || strings.Contains(err.Error(), "context canceled") {
			return nil, ErrTimeout.New(fmt.Sprintf("page load timed out for %s", rawURL))
		}
		return nil, ErrRendererError.Wrap(err)
	}

	if finalURL == "" {
		finalURL = rawURL
	}

	// If navigation landed on Chrome's error page and no status was captured,
	// report a 502 (a response came back but it was an error).
	statusCode := uint16(bundle.status.Status())
	if strings.HasPrefix(finalURL, "chrome-error://") && !bundle.status.Seen() {
		statusCode = 502
	}

	result := &FetchResult{
		URL:          rawURL,
		FinalURL:     finalURL,
		StatusCode:   statusCode,
		HTML:         headHTML + bodyHTML,
		RenderedWith: "browser",
		BlockedURLs:  blocked.get(),
	}

	// Surface a warning when the result looks like a challenge/block page or
	// the JS never finished rendering (so partial content is returned knowingly).
	if isAntiBotPage(result.HTML) {
		w := "blocked by anti-bot protection"
		result.Warning = &w
	} else if waitMs == 0 && spaResult.State == StateTimeout {
		w := fmt.Sprintf("page may be incomplete: SPA readiness timeout after %s",
			spaResult.Duration.Round(100*time.Millisecond))
		result.Warning = &w
	}

	utils.Log.Debug("browser fetch", "url", rawURL, "status", statusCode, "html_bytes", len(result.HTML))
	return result, nil
}
