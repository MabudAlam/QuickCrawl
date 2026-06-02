package core

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

type Config struct {
	Browser BrowserConfig
	Pool    PoolConfig
}

type BrowserConfig struct {
	Mode            BrowserMode
	WSURL           string
	NumBrowsers     int
	PageTimeout     time.Duration
	PoolSize        int
	StealthEnabled  bool // When true, register anti-fingerprint JS on every page. When false, the call is skipped entirely.
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
	if cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) != "" {
		configuredWS := strings.TrimSpace(cfg.Renderer.Chrome.WSURL)
		if discovered := discoverChromeWSURL(configuredWS); discovered != "" {
			coreCfg.Browser.WSURL = discovered
		} else {
			coreCfg.Browser.WSURL = configuredWS
		}
	}
	if cfg.Renderer.PoolSize > 0 {
		coreCfg.Browser.PoolSize = cfg.Renderer.PoolSize
	}
	if cfg.Renderer.PageTimeoutMs > 0 {
		coreCfg.Browser.PageTimeout = time.Duration(cfg.Renderer.PageTimeoutMs) * time.Millisecond
	}
	coreCfg.Browser.StealthEnabled = cfg.Crawler.Stealth.Enabled

	var stealthProfile *utils.HeaderProfile
	if cfg.Crawler.Stealth.Enabled && cfg.Crawler.Stealth.InjectHeaders {
		profile := utils.GetHeaderProfile(utils.HeaderStrategy(cfg.Crawler.Stealth.Strategy))
		stealthProfile = &profile
	}

	httpFetcher := renderer.NewHTTPFetcher(cfg.Crawler.UserAgent, stealthProfile)
	return NewScraper(coreCfg, httpFetcher, llm)
}

// chromeVersionResponse is the subset of the Chrome DevTools /json/version
// payload we need. The endpoint returns a few extra fields (V8 version,
// protocol version, user agent) which we discard.
type chromeVersionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// discoverChromeWSURL queries the Chrome DevTools /json/version endpoint to
// recover the current browser WebSocket URL. Chrome assigns a fresh browser
// ID on every restart, so a hardcoded ws://.../devtools/browser/<uuid> URL
// in the config becomes stale the moment Chrome is restarted. Without this
// discovery step, the chromedp RemoteAllocator would attempt to dial a
// session ID that no longer exists and every /v1/scrape with renderJs=true
// would fail with a fast "context deadline" error.
func discoverChromeWSURL(configuredWSURL string) string {
	base, ok := wsURLToHTTPBase(configuredWSURL)
	if !ok {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/json/version", nil)
	if err != nil {
		log.Printf("[core] chrome discovery: failed to build request for %s: %v", base, err)
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[core] chrome discovery: failed to reach %s/json/version: %v (using configured WS URL)", base, err)
		return ""
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[core] chrome discovery: %s/json/version returned HTTP %d (using configured WS URL)", base, resp.StatusCode)
		return ""
	}

	var payload chromeVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		log.Printf("[core] chrome discovery: failed to decode response: %v (using configured WS URL)", err)
		return ""
	}
	if payload.WebSocketDebuggerURL == "" {
		log.Printf("[core] chrome discovery: response missing webSocketDebuggerUrl (using configured WS URL)")
		return ""
	}
	if payload.WebSocketDebuggerURL != configuredWSURL {
		log.Printf("[core] chrome discovery: configured WS URL was stale; using live URL %s", payload.WebSocketDebuggerURL)
	}
	return payload.WebSocketDebuggerURL
}

// wsURLToHTTPBase converts a Chrome DevTools WebSocket URL to its HTTP origin
// counterpart. ws://host:port/path -> http://host:port. Returns ok=false for
// URLs that don't look like ws:// or wss:// so the caller can fall back to
// the raw configured value.
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
