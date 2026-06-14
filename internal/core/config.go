package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

type Config struct {
	Browser BrowserConfig
	Pool    PoolConfig
}

type BrowserConfig struct {
	Mode           BrowserMode
	BrowserType    string // "browserless", "cloak", "lightpanda"
	WSURL          string
	NumBrowsers    int
	PageTimeout    time.Duration
	PoolSize       int
	StealthEnabled bool     // When true, register anti-fingerprint JS on every page. When false, the call is skipped entirely.
	ChromeArgs     []string // Chrome launch flags for browserless
}

type BrowserMode string

const (
	BrowserModeAuto     BrowserMode = "auto"
	BrowserModeChrome   BrowserMode = "chrome"
	BrowserModeHTTPOnly BrowserMode = "http"
)

type PoolConfig struct {
	Size    int
	PerHost int
}

func DefaultConfig() Config {
	return Config{
		Browser: BrowserConfig{
			Mode:        BrowserModeAuto,
			WSURL:       "ws://localhost:9222",
			NumBrowsers: 4,
			PageTimeout: 60 * time.Second,
			PoolSize:    10,
		},
		Pool: PoolConfig{
			Size:    4,
			PerHost: 10,
		},
	}
}

// NewScraperFromConfig is a convenience constructor that builds a
// *Scraper from the legacy *types.AppConfig. It is the single source of
// truth for "given a server config, give me a working scraper" and is
// used by the HTTP server, MCP server, and CLI.
//
// The cfg.Renderer.Chrome.WSURL is honoured. cfg.Renderer.Mode and
// cfg.Renderer.Lightpanda are accepted for backward-compat but the
// new scraper uses chromedp only — those fields have no effect.
func NewScraperFromConfig(cfg *types.AppConfig, llm *types.LLMConfig) (*Scraper, *QuickCrawlError) {
	if cfg == nil {
		cfg = &types.AppConfig{}
	}
	cfg.Defaults()

	coreCfg := DefaultConfig()
	switch {
	case cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) != "":
		configuredWS := strings.TrimSpace(cfg.Renderer.Chrome.WSURL)
		wsURL, err := GetCDPURL(configuredWS)
		if err != nil {
			coreCfg.Browser.WSURL = configuredWS
		} else {
			coreCfg.Browser.WSURL = wsURL
		}
	case cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) == "":
		coreCfg.Browser.WSURL = ""
	}
	if cfg.Renderer.PoolSize > 0 {
		coreCfg.Browser.PoolSize = cfg.Renderer.PoolSize
	}
	if cfg.Renderer.PageTimeoutMs > 0 {
		coreCfg.Browser.PageTimeout = time.Duration(cfg.Renderer.PageTimeoutMs) * time.Millisecond
	}
	coreCfg.Browser.StealthEnabled = cfg.Crawler.Stealth.Enabled
	coreCfg.Browser.BrowserType = cfg.Renderer.Browser

	// Handle Chrome launch args for browserless.
	// Chrome flags are passed via the launch JSON object, URL-encoded.
	// Example: ?launch=%7B%22args%22%3A%5B...%5D%7D
	// Only encode for browserless; cloak and lightpanda don't need this encoding.
	if cfg.Renderer.Chrome != nil && len(cfg.Renderer.Chrome.ChromeArgs) > 0 {
		coreCfg.Browser.ChromeArgs = cfg.Renderer.Chrome.ChromeArgs
		// Only encode chrome args for browserless
		if cfg.Renderer.Browser == "browserless" {
			coreCfg.Browser.WSURL = encodeChromeArgsToURL(coreCfg.Browser.WSURL, cfg.Renderer.Chrome.ChromeArgs)
		}
	}

	var stealthProfile *utils.HeaderProfile
	if cfg.Crawler.Stealth.Enabled && cfg.Crawler.Stealth.InjectHeaders {
		profile := utils.GetHeaderProfile(utils.HeaderStrategy(cfg.Crawler.Stealth.Strategy))
		stealthProfile = &profile
	}

	httpFetcher := NewHTTPFetcher(cfg.Crawler.UserAgent, stealthProfile)
	return NewScraper(coreCfg, httpFetcher, llm)
}

type VersionResponse struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	V8Version            string `json:"V8-Version"`
	WebKitVersion        string `json:"WebKit-Version"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

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

// encodeChromeArgsToURL encodes Chrome launch args into the browserless WebSocket URL.
// browserless v2 accepts Chrome flags via the launch JSON object:
// wss://host/chromium?token=TOKEN&launch=<url_encoded_json>
//
// The launch JSON format:
//
//	{"args":["--flag1","--flag2",...]}
func encodeChromeArgsToURL(wsURL string, args []string) string {
	if len(args) == 0 {
		return wsURL
	}

	// Build the launch JSON
	launchJSON := map[string]any{"args": args}
	jsonBytes, err := json.Marshal(launchJSON)
	if err != nil {
		return wsURL
	}

	// URL-encode the JSON
	encoded := url.QueryEscape(string(jsonBytes))

	// Parse the existing URL
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return wsURL
	}

	// Build new query string
	newQuery := parsed.RawQuery
	if newQuery != "" {
		newQuery += "&"
	}
	newQuery += "launch=" + encoded

	// Reconstruct URL manually
	return parsed.Scheme + "://" + parsed.Host + parsed.Path + "?" + newQuery
}
