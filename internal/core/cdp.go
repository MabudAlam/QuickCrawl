// Package core's cdp.go contains pure Chrome DevTools Protocol URL
// helpers. They are general-purpose utilities — not configuration —
// so they live here rather than in internal/config.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// VersionResponse is the JSON shape returned by a browser's
// /json/version endpoint.
type VersionResponse struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	V8Version            string `json:"V8-Version"`
	WebKitVersion        string `json:"WebKit-Version"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// GetCDPURL resolves a Chrome DevTools Protocol endpoint URL. Given a
// ws:// or wss:// URL, it queries the browser's /json/version and
// returns the live webSocketDebuggerUrl. Browserless v2 / commercial
// CDP endpoints that serve a WebSocket directly (token= query param
// or /chromium|/firefox|/webkit path) are passed through unchanged.
func GetCDPURL(baseURL string) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	// Browserless v2 / commercial CDP endpoints serve a WebSocket directly
	// and don't expose /json/version. Detect these and use the URL as-is.
	if isBrowserlessDirectWS(baseURL) {
		return baseURL, nil
	}

	httpBase, ok := wsURLToHTTPBase(baseURL)
	if !ok {
		return "", fmt.Errorf("invalid ws URL: %s", baseURL)
	}

	resp, err := http.Get(httpBase + "/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var version VersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return "", err
	}

	if version.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("webSocketDebuggerUrl not found")
	}

	return version.WebSocketDebuggerURL, nil
}

// discoverCloakBrowserWSURL queries the CloakBrowser /json/version endpoint to
// recover the current browser WebSocket URL. CloakBrowser creates a new Chrome
// instance for each new WebSocket connection, so the URL must be discovered
// fresh for each request. This function is called per-request, not at startup.
func discoverCloakBrowserWSURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	httpBase, ok := wsURLToHTTPBase(baseURL)
	if !ok {
		return baseURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpBase+"/json/version", nil)
	if err != nil {
		utils.Log.Info("cloak discovery: failed to build request", "base", httpBase, "error", err)
		return baseURL
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		utils.Log.Info("cloak discovery: failed to reach /json/version", "base", httpBase, "error", err)
		return baseURL
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		utils.Log.Info("cloak discovery: /json/version returned non-200", "base", httpBase, "status", resp.StatusCode)
		return baseURL
	}

	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		utils.Log.Info("cloak discovery: failed to decode response", "base", httpBase, "error", err)
		return baseURL
	}

	if payload.WebSocketDebuggerURL == "" {
		utils.Log.Info("cloak discovery: response missing webSocketDebuggerUrl", "base", httpBase)
		return baseURL
	}

	return payload.WebSocketDebuggerURL
}

// isBrowserlessDirectWS returns true for commercial / browserless-style CDP
// endpoints that serve a WebSocket directly and don't expose /json/version.
// Such URLs either carry a token= query parameter or use a browser-named path.
func isBrowserlessDirectWS(url string) bool {
	if strings.Contains(url, "token=") {
		return true
	}
	return strings.Contains(url, "/chromium") || strings.Contains(url, "/firefox") || strings.Contains(url, "/webkit")
}

// wsURLToHTTPBase converts a ws:// or wss:// URL to its http:// or
// https:// counterpart, stripping any path. Returns ("", false) if
// the input is not a ws/wss URL.
func wsURLToHTTPBase(wsURL string) (string, bool) {
	lower := strings.ToLower(wsURL)
	var schemeLen int
	var httpScheme string
	switch {
	case strings.HasPrefix(lower, "ws://"):
		schemeLen = len("ws://")
		httpScheme = "http://"
	case strings.HasPrefix(lower, "wss://"):
		schemeLen = len("wss://")
		httpScheme = "https://"
	default:
		return "", false
	}
	rest := wsURL[schemeLen:]
	if i := strings.Index(rest, "/"); i != -1 {
		rest = rest[:i]
	}
	return httpScheme + rest, true
}
