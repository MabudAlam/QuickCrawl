package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// chrome.go provides helpers for discovering a running Chrome instance's
// current Chrome DevTools Protocol WebSocket URL.
//
// Chrome assigns each browser session a random UUID (the "browser ID") and
// exposes it at /devtools/browser/<id>. Hardcoding the ID in config breaks
// the moment the browser restarts — the previous ID is no longer valid
// and the WebSocket upgrade returns 404. The /json/version endpoint
// always returns the current webSocketDebuggerUrl, so it is the
// canonical discovery path used by Puppeteer, Playwright, and chromedp
// itself.

// chromeVersionResponse mirrors the JSON shape of GET /json/version on
// a Chrome DevTools endpoint. Only the fields we need are decoded.
type chromeVersionResponse struct {
	Browser             string `json:"Browser"`
	ProtocolVersion     string `json:"Protocol-Version"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// DiscoverBrowserWSURL queries a Chrome DevTools HTTP endpoint for the
// current browser session's webSocketDebuggerUrl.
//
// baseWSURL is the configured WebSocket URL of the form
// "ws://host:port/devtools/browser/<id>" or simply "ws://host:port". If
// the URL contains a /devtools/browser/<id> suffix it is stripped and
// replaced with the live value reported by the browser; otherwise the
// configured URL is returned as-is (the caller is asserting a specific
// session).
//
// The function uses a short HTTP timeout (5s) so a dead Chrome does not
// block server startup. On failure the original input URL is returned
// alongside the error, so callers can choose to either fail-fast
// (treat as fatal) or fall back (log and continue with the stale URL).
func DiscoverBrowserWSURL(ctx context.Context, baseWSURL string) (string, error) {
	if strings.TrimSpace(baseWSURL) == "" {
		return "", fmt.Errorf("empty browser WS URL")
	}

	// Convert ws:// -> http:// and wss:// -> https:// for the HTTP probe.
	// Strip any /devtools/browser/<id> suffix so the URL points at the
	// host root, which is what /json/version is served from.
	httpBase, err := wsToHTTPBase(baseWSURL)
	if err != nil {
		return baseWSURL, fmt.Errorf("invalid WS URL %q: %w", baseWSURL, err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, httpBase+"/json/version", nil)
	if err != nil {
		return baseWSURL, fmt.Errorf("build request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return baseWSURL, fmt.Errorf("probe %s: %w", httpBase+"/json/version", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return baseWSURL, fmt.Errorf("probe %s returned HTTP %d", httpBase+"/json/version", resp.StatusCode)
	}

	var info chromeVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return baseWSURL, fmt.Errorf("decode /json/version: %w", err)
	}
	if info.WebSocketDebuggerURL == "" {
		return baseWSURL, fmt.Errorf("/json/version returned empty webSocketDebuggerUrl")
	}
	return info.WebSocketDebuggerURL, nil
}

// wsToHTTPBase converts a ws:// or wss:// URL into the corresponding
// http:// or https:// host root, stripping any /devtools/* path. It is
// the inverse of the WS URL form used by Chrome DevTools.
//
// Examples:
//
//	ws://localhost:9222/devtools/browser/abc-123  -> http://localhost:9222
//	wss://chrome.example.com/devtools/browser/x   -> https://chrome.example.com
//	ws://localhost:9222                            -> http://localhost:9222
func wsToHTTPBase(wsURL string) (string, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return "", fmt.Errorf("unsupported scheme %q (expected ws or wss)", u.Scheme)
	}
	// Strip any path that looks like a DevTools endpoint; we want the
	// host root so we can hit /json/version. The path may be empty
	// (host-only URL), or it may already include /devtools/browser/<id>.
	if idx := strings.Index(u.Path, "/devtools"); idx != -1 {
		u.Path = u.Path[:idx]
	}
	return u.String(), nil
}
