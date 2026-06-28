// Package core's cdp.go contains pure Chrome DevTools Protocol URL
// helpers. They are general-purpose utilities — not configuration —
// so they live here rather than in internal/config.
package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// GetCDPURL resolves a Chrome DevTools Protocol endpoint URL. Given any of
// ws://, wss://, http://, or https://, it queries the browser's
// /json/version and returns the live webSocketDebuggerUrl. Browserless v2
// / commercial CDP endpoints that serve a WebSocket directly (token= query
// param or /chromium|/firefox|/webkit path) are passed through unchanged.
func GetCDPURL(baseURL string) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	// Browserless v2 / commercial CDP endpoints serve a WebSocket directly
	// and don't expose /json/version. Detect these and use the URL as-is.
	if isBrowserlessDirectWS(baseURL) {
		return baseURL, nil
	}

	httpBase, ok := httpBaseFor(baseURL)
	if !ok {
		return "", fmt.Errorf("invalid base URL: %s", baseURL)
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

// isBrowserlessDirectWS returns true for commercial / browserless-style CDP
// endpoints that serve a WebSocket directly and don't expose /json/version.
// Such URLs either carry a token= query parameter or use a browser-named path.
func isBrowserlessDirectWS(url string) bool {
	if strings.Contains(url, "token=") {
		return true
	}
	return strings.Contains(url, "/chromium") || strings.Contains(url, "/firefox") || strings.Contains(url, "/webkit")
}

// httpBaseFor returns the HTTP base URL to query for /json/version.
// Accepts http://, https:// (returned as-is), ws:// (→ http://),
// and wss:// (→ https://). Path is stripped.
func httpBaseFor(rawURL string) (string, bool) {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(lower, "http://"),
		strings.HasPrefix(lower, "https://"):
		schemeEnd := strings.Index(lower, "://") + 3
		idx := strings.Index(rawURL[schemeEnd:], "/")
		if idx == -1 {
			return rawURL, true
		}
		return rawURL[:schemeEnd+idx], true
	case strings.HasPrefix(lower, "ws://"):
		return "http://" + stripPath(rawURL[len("ws://"):]), true
	case strings.HasPrefix(lower, "wss://"):
		return "https://" + stripPath(rawURL[len("wss://"):]), true
	}
	return "", false
}

func stripPath(hostAndPath string) string {
	if i := strings.Index(hostAndPath, "/"); i != -1 {
		return hostAndPath[:i]
	}
	return hostAndPath
}
