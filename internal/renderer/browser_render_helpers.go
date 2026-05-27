package renderer

import (
	"encoding/json"
	"strconv"
	"sync/atomic"
	"time"
)

// extractRuntimeEvaluationValue pulls the string value out of a Runtime.evaluate response.
func extractRuntimeEvaluationValue(raw json.RawMessage) string {
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

// readRenderedHTML extracts the full rendered HTML document from the current page.
func readRenderedHTML(conn *cdpConnection, sessionID string) (string, error) {
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
	html := extractRuntimeEvaluationValue(evalRes)
	if len(html) > MaxBrowserHTMLBytes {
		html = html[:MaxBrowserHTMLBytes]
	}
	return html, nil
}

// readRenderedHTMLWithShadowDOM extracts HTML with Shadow DOM flattened into the output.
func readRenderedHTMLWithShadowDOM(conn *cdpConnection, sessionID string) (string, error) {
	evalRes, err := conn.SendRecv("Runtime.evaluate", map[string]any{
		"expression":            shadowDOMFlattenScript(),
		"returnByValue":         true,
		"awaitPromise":          true,
		"includeCommandLineAPI": true,
	}, sessionID, 10*time.Second)
	if err != nil {
		return "", err
	}
	html := extractRuntimeEvaluationValue(evalRes)
	if len(html) > MaxBrowserHTMLBytes {
		html = html[:MaxBrowserHTMLBytes]
	}
	return html, nil
}

// waitForSpaContent waits for a mounted SPA content root and enough visible
// body text to appear before we snapshot the page for extraction.
func waitForSpaContent(conn *cdpConnection, sessionID string, timeout time.Duration, tracker *networkActivityTracker) bool {
	deadline := time.Now().Add(timeout)
	expr := `(function() {
		if (!document.querySelector(` + "`" + spaContentSelectors + "`" + `)) return "-1";
		const text = (document.body && document.body.innerText) || '';
		return String(text.trim().length);
	})()`

	for time.Now().Before(deadline) {
		evalRes, err := conn.SendRecv("Runtime.evaluate", map[string]any{
			"expression":            expr,
			"returnByValue":         true,
			"awaitPromise":          true,
			"includeCommandLineAPI": true,
		}, sessionID, 2*time.Second)
		if err == nil {
			if len := extractRuntimeEvaluationValue(evalRes); len != "" {
				// The expression returns either "-1" (selector missing) or a
				// decimal length. Any string >= the threshold means the page is
				// actually hydrated enough to snapshot.
				if n, parseErr := strconv.Atoi(len); parseErr == nil && n >= spaBodyTextMinChars {
					return true
				}
				if tracker != nil && tracker.isIdle(500*time.Millisecond) {
					return true
				}
			}
		}
		time.Sleep(spaSelectorTickMs * time.Millisecond)
	}
	return false
}

// waitForPageContentToStabilize waits for page content to stop changing significantly.
func waitForPageContentToStabilize(conn *cdpConnection, sessionID string, timeout time.Duration) (string, error) {
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
		html, err := readRenderedHTML(conn, sessionID)
		if err != nil {
			return "", err
		}
		currLen := len(html)
		placeholderGone := !detectLoadingPlaceholder(html)
		if isContentLengthStable(prevLen, currLen, placeholderGone) {
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

// waitForPageContentToStabilizeWithShadowDOM waits for content stability using shadow DOM flattening.
func waitForPageContentToStabilizeWithShadowDOM(conn *cdpConnection, sessionID string, timeout time.Duration) (string, error) {
	budget := 8 * time.Second
	if timeout > 0 && timeout < budget {
		budget = timeout
	}
	deadline := time.Now().Add(budget)
	prevLen := 0
	stableTicks := 0
	lastHTML := ""

	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		html, err := readRenderedHTMLWithShadowDOM(conn, sessionID)
		if err != nil {
			return "", err
		}
		currLen := len(html)
		placeholderGone := !detectLoadingPlaceholder(html)
		if isContentLengthStable(prevLen, currLen, placeholderGone) {
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

// isContentLengthStable allows a small amount of variation in page size while waiting for dynamic content.
func isContentLengthStable(prevLen, currLen int, placeholderGone bool) bool {
	if prevLen == 0 || !placeholderGone {
		return false
	}
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

// detectAntiBotChallengePage checks for Cloudflare, CAPTCHA, and similar interstitials.
func detectAntiBotChallengePage(html string) bool {
	return pageHasBlockInterstitial(html) || pageLooksLikeGenericBotWall(html)
}

// detectLoadingPlaceholder checks for loading shells commonly emitted by JS apps.
func detectLoadingPlaceholder(html string) bool {
	return pageLooksLikeLoadingPlaceholder(html)
}

// ptr returns a pointer to a copy of v.
func ptr[T any](v T) *T {
	return &v
}

// networkActivityTracker keeps a cheap count of in-flight network requests so
// the SPA readiness poll can exit once the page has gone quiet.
type networkActivityTracker struct {
	inFlight    atomic.Int64
	lastChanged atomic.Int64
}

func newNetworkActivityTracker() *networkActivityTracker {
	tracker := &networkActivityTracker{}
	tracker.lastChanged.Store(time.Now().UnixMilli())
	return tracker
}

func (t *networkActivityTracker) recordRequestStart() {
	t.inFlight.Add(1)
	t.lastChanged.Store(time.Now().UnixMilli())
}

func (t *networkActivityTracker) recordRequestEnd() {
	t.inFlight.Add(-1)
	t.lastChanged.Store(time.Now().UnixMilli())
}

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

// runNetworkIdlePump updates the tracker from Network.* CDP events.
func runNetworkIdlePump(events <-chan cdpEvent, sessionID string, tracker *networkActivityTracker, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.SessionID != sessionID {
				continue
			}
			switch ev.Method {
			case "Network.requestWillBeSent":
				tracker.recordRequestStart()
			case "Network.loadingFinished", "Network.loadingFailed":
				tracker.recordRequestEnd()
			}
		}
	}
}
