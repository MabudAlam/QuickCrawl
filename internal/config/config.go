// Package config is the single source of truth for application
// configuration. It loads the operator-facing *types.AppConfig from
// TOML files and environment variables, and it builds the internal
// *core.Config / *core.Scraper from that AppConfig. Anything that
// needs "given a server config, give me a working scraper" calls
// NewScraperFromConfig here rather than re-implementing the
// translation in a separate package.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// LoadAppConfig loads the application configuration.
// Configuration is loaded in order:
// 1. Defaults
// 2. TOML config file (if CONFIG env is set, or default quickcrawl.toml)
// 3. Environment variables (always override)
func LoadAppConfig() (*types.AppConfig, error) {
	configFile := strings.TrimSpace(os.Getenv("CONFIG"))
	if configFile == "" {
		configFile = "quickcrawl.toml"
	}
	return LoadAppConfigFromPath(configFile)
}

// LoadAppConfigFromPath loads the application configuration from the
// given TOML file path and then applies env-var overrides. Missing
// files are tolerated (an empty config is returned with defaults
// applied). This is the test-friendly entry point; production code
// should call LoadAppConfig.
func LoadAppConfigFromPath(configFile string) (*types.AppConfig, error) {
	cfg := &types.AppConfig{}
	cfg.Defaults()

	if err := decodeConfigFile(cfg, configFile, false); err != nil {
		return nil, err
	}

	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// decodeConfigFile loads and parses a TOML configuration file.
func decodeConfigFile(cfg *types.AppConfig, path string, required bool) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil
		}
		return fmt.Errorf("load config %s: %w", filepath.Clean(path), err)
	}
	if _, err := toml.Decode(string(data), cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", filepath.Clean(path), err)
	}
	return nil
}

// applyEnvOverrides applies environment variable overrides to the config.
// Uses the format: <SECTION>__<KEY> (double underscore). Returns an
// error if any env value is present but unparseable (e.g. an invalid
// RENDERER__RENDER_MODE) so the process fails fast at startup.
func applyEnvOverrides(cfg *types.AppConfig) error {
	// Server configuration
	if v := envString("SERVER__HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := envUint16("SERVER__PORT"); v != nil {
		cfg.Server.Port = *v
	}
	if v := envInt64("SERVER__REQUEST_TIMEOUT_SECS"); v != nil {
		cfg.Server.RequestTimeoutSecs = *v
	}
	if v := envUint64("SERVER__RATE_LIMIT_RPS"); v != nil {
		cfg.Server.RateLimitRPS = *v
	}

	// Renderer configuration
	if v := envInt64("RENDERER__PAGE_TIMEOUT_MS"); v != nil {
		cfg.Renderer.PageTimeoutMs = *v
	}
	if v := envInt("RENDERER__POOL_SIZE"); v != nil {
		cfg.Renderer.PoolSize = *v
	}
	if v := envString("RENDERER__RENDER_MODE"); v != "" {
		mode, parseErr := types.ParseRenderMode(v)
		if parseErr != nil {
			return fmt.Errorf("RENDERER__RENDER_MODE: %w", parseErr)
		}
		cfg.Renderer.RenderMode = mode
	}
	if v := envString("RENDERER__BROWSER"); v != "" {
		cfg.Renderer.Browser = v
	}
	if v := envString("RENDERER__CHROME__WS_URL"); v != "" {
		cfg.Renderer.Chrome = &types.CdpEndpoint{WSURL: v}
	}
	if cfg.Renderer.Chrome != nil {
		if v := envStringSlice("RENDERER__CHROME__CHROME_ARGS"); v != nil {
			cfg.Renderer.Chrome.ChromeArgs = *v
		}
	}

	// Crawler configuration
	if v := envInt("CRAWLER__MAX_CONCURRENCY"); v != nil {
		cfg.Crawler.MaxConcurrency = *v
	}
	if v := envFloat64("CRAWLER__REQUESTS_PER_SECOND"); v != nil {
		cfg.Crawler.RequestsPerSecond = *v
	}
	if v := envBool("CRAWLER__RESPECT_ROBOTS_TXT"); v != nil {
		cfg.Crawler.RespectRobotsTxt = *v
	}
	if v := envString("CRAWLER__USER_AGENT"); v != "" {
		cfg.Crawler.UserAgent = v
	}
	if v := envInt("CRAWLER__DEFAULT_MAX_DEPTH"); v != nil {
		cfg.Crawler.DefaultMaxDepth = *v
	}
	if v := envInt("CRAWLER__DEFAULT_MAX_PAGES"); v != nil {
		cfg.Crawler.DefaultMaxPages = *v
	}
	if v := envInt64("CRAWLER__JOB_TTL_SECS"); v != nil {
		cfg.Crawler.JobTTLSecs = *v
	}
	if v := envBool("CRAWLER__STEALTH__ENABLED"); v != nil {
		cfg.Crawler.Stealth.Enabled = *v
	}
	if v := envFloat64("CRAWLER__STEALTH__JITTER_FACTOR"); v != nil {
		cfg.Crawler.Stealth.JitterFactor = *v
	}
	if v := envBool("CRAWLER__STEALTH__INJECT_HEADERS"); v != nil {
		cfg.Crawler.Stealth.InjectHeaders = *v
	}
	if v := envString("CRAWLER__STEALTH__STRATEGY"); v != "" {
		cfg.Crawler.Stealth.Strategy = v
	}
	

	// Extraction configuration
	if cfg.Extraction.LLM == nil {
		cfg.Extraction.LLM = &types.LLMConfig{}
	}
	if v := envString("EXTRACTION__LLM__API_KEY"); v != "" {
		cfg.Extraction.LLM.APIKey = v
	}
	if v := envString("EXTRACTION__LLM__MODEL"); v != "" {
		cfg.Extraction.LLM.Model = v
	}
	if v := envString("EXTRACTION__LLM__BASE_URL"); v != "" {
		cfg.Extraction.LLM.BaseURL = ptr(v)
	}
	if v := envUint32("EXTRACTION__LLM__MAX_TOKENS"); v != nil {
		cfg.Extraction.LLM.MaxTokens = *v
	}
	if v := envString("EXTRACTION__LLM__EXTRACTION_PROMPT"); v != "" {
		cfg.Extraction.LLM.ExtractionPrompt = v
	}
	if v := envString("EXTRACTION__LLM__RESPONSE_FORMAT"); v != "" {
		cfg.Extraction.LLM.ResponseFormat = v
	}

	// Search configuration
	if v := envString("SEARCH__BASE_URL"); v != "" {
		cfg.Search.BaseURL = v
	}
	if v := envInt("SEARCH__TIMEOUT_SECS"); v != nil {
		cfg.Search.TimeoutSecs = *v
	}
	if v := envFloat64("SEARCH__BM25F_TITLE_WEIGHT"); v != nil {
		cfg.Search.BM25FTitleWeight = *v
	}
	if v := envFloat64("SEARCH__BM25F_SNIPPET_WEIGHT"); v != nil {
		cfg.Search.BM25FSnippetWeight = *v
	}

	// Cache - REDIS_URL takes precedence, parsed for host/password/db
	if v := envString("REDIS_URL"); v != "" {
		_ = cfg.Cache.ParseRedisURL(v)
	}
	if v := envBool("CACHE__ENABLED"); v != nil {
		cfg.Cache.Enabled = *v
	}
	if v := envInt64("CACHE__TTL_DEFAULT_SECS"); v != nil {
		cfg.Cache.TTLDefaultSecs = *v
	}

	return nil
}

// envString reads an environment variable and trims whitespace.
func envString(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// envStringSlice parses an environment variable as a []string. Accepts
// either a JSON array (e.g. `["a","b"]`) or a comma-separated list
// (e.g. `a,b`). Returns nil when the variable is unset or empty.
func envStringSlice(key string) *[]string {
	v := envString(key)
	if v == "" {
		return nil
	}
	if strings.HasPrefix(v, "[") {
		var out []string
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil
		}
		return &out
	}
	parts := strings.Split(v, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return &parts
}

// envBool parses a boolean environment variable.
func envBool(key string) *bool {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return nil
	}
	return &parsed
}

// envInt parses an integer environment variable.
func envInt(key string) *int {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &parsed
}

// envInt64 parses an int64 environment variable.
func envInt64(key string) *int64 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

// envUint16 parses a uint16 environment variable.
func envUint16(key string) *uint16 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		return nil
	}
	value := uint16(parsed)
	return &value
}

// envUint32 parses a uint32 environment variable.
func envUint32(key string) *uint32 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return nil
	}
	value := uint32(parsed)
	return &value
}

// envUint64 parses a uint64 environment variable.
func envUint64(key string) *uint64 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

// envFloat64 parses a float64 environment variable.
func envFloat64(key string) *float64 {
	v := envString(key)
	if v == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

// ptr returns a pointer to a value.
func ptr[T any](v T) *T {
	return &v
}

// =============================================================================
// Scraper configuration
//
// The types Config, BrowserConfig, PoolConfig live in internal/types
// (alongside the operator-facing AppConfig they are projected from)
// to avoid an import cycle: internal/core imports internal/config to
// build its scraper, and internal/config imports internal/core for
// the scraper primitives it needs (NewScraper, GetCDPURL, etc.).
// Putting the types in a third package both can import breaks the
// cycle. The constructor NewScraperFromConfig lives here — it is the
// "given a server config, give me a working scraper" entry point and
// is the only place the AppConfig → types.Config projection happens.
//
// CDP URL helpers — GetCDPURL, wsURLToHTTPBase, isBrowserlessDirectWS,
// discoverCloakBrowserWSURL, VersionResponse — remain in internal/core
// (file: cdp.go) because they are pure URL manipulation utilities,
// not configuration.
// =============================================================================

// NewScraperFromConfig is the single source of truth for "given a
// server config, give me a working scraper". It is used by the HTTP
// server, the MCP server, and the CLI.
//
// The translation is:
//   - cfg.Renderer.Chrome.WSURL is honoured (auto-discovered via
//     core.GetCDPURL when possible).
//   - cfg.Renderer.RenderMode becomes types.BrowserConfig.Mode. Empty
//     string preserves the default (RenderModeAuto). Invalid values
//     fail fast.
//   - cfg.Renderer.Browser ("cloak", "browserless", "lightpanda")
//     becomes types.BrowserConfig.BrowserType.
//   - cfg.Renderer.PoolSize / PageTimeoutMs override the defaults.
//   - cfg.Crawler.Stealth.Enabled / InjectHeaders / Strategy feed
//     into the HTTP fetcher's stealth header profile.
//   - cfg.Renderer.Chrome.ChromeArgs are appended to the WSURL only
//     when Browser == "browserless" (other backends don't need the
//     ?launch= encoding).
func NewScraperFromConfig(cfg *types.AppConfig, llm *types.LLMConfig) (*core.Scraper, *core.QuickCrawlError) {
	if cfg == nil {
		cfg = &types.AppConfig{}
	}
	cfg.Defaults()

	scraperCfg := types.DefaultScraperConfig()
	switch {
	case cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) != "":
		configuredWS := strings.TrimSpace(cfg.Renderer.Chrome.WSURL)
		wsURL, err := core.GetCDPURL(configuredWS)
		if err != nil {
			scraperCfg.Browser.WSURL = configuredWS
		} else {
			scraperCfg.Browser.WSURL = wsURL
		}
	case cfg.Renderer.Chrome != nil && strings.TrimSpace(cfg.Renderer.Chrome.WSURL) == "":
		scraperCfg.Browser.WSURL = ""
	}
	if cfg.Renderer.PoolSize > 0 {
		scraperCfg.Browser.PoolSize = cfg.Renderer.PoolSize
	}
	if cfg.Renderer.PageTimeoutMs > 0 {
		scraperCfg.Browser.PageTimeout = time.Duration(cfg.Renderer.PageTimeoutMs) * time.Millisecond
	}
	scraperCfg.Browser.StealthEnabled = cfg.Crawler.Stealth.Enabled
	scraperCfg.Browser.BrowserType = cfg.Renderer.Browser

	// render_mode is the source of truth for fetch strategy:
	//   "auto"    → HTTP first, escalate to browser on anti-bot/SPA signals (default)
	//   "browser" → always go through the browser, never HTTP-only
	//   "http"    → HTTP only, never touch the browser
	// An empty string (TOML not set, no env override) preserves the
	// default (RenderModeAuto). Invalid values fail fast.
	if cfg.Renderer.RenderMode == "" {
		// keep scraperCfg.Browser.Mode = types.RenderModeAuto (from DefaultScraperConfig)
	} else if cfg.Renderer.RenderMode.IsValid() {
		scraperCfg.Browser.Mode = cfg.Renderer.RenderMode
	} else {
		return nil, core.ErrInvalidRequest.New(fmt.Sprintf("invalid render_mode %q: must be one of auto, browser, http", cfg.Renderer.RenderMode))
	}

	// Handle Chrome launch args for browserless.
	// Chrome flags are passed via the launch JSON object, URL-encoded.
	// Example: ?launch=%7B%22args%22%3A%5B...%5D%7D
	// Only encode for browserless; cloak and lightpanda don't need this encoding.
	if cfg.Renderer.Chrome != nil && len(cfg.Renderer.Chrome.ChromeArgs) > 0 {
		scraperCfg.Browser.ChromeArgs = cfg.Renderer.Chrome.ChromeArgs
		// Only encode chrome args for browserless
		if cfg.Renderer.Browser == "browserless" {
			scraperCfg.Browser.WSURL = encodeChromeArgsForBrowserless(scraperCfg.Browser.WSURL, cfg.Renderer.Chrome.ChromeArgs)
		}
	}

	var stealthProfile *utils.HeaderProfile
	if cfg.Crawler.Stealth.Enabled && cfg.Crawler.Stealth.InjectHeaders {
		profile := utils.GetHeaderProfile(utils.HeaderStrategy(cfg.Crawler.Stealth.Strategy))
		stealthProfile = &profile
	}

	httpFetcher := core.NewHTTPFetcher(cfg.Crawler.UserAgent, stealthProfile)
	return core.NewScraper(scraperCfg, httpFetcher, llm)
}

// encodeChromeArgsForBrowserless encodes Chrome launch args into the
// browserless WebSocket URL. browserless v2 accepts Chrome flags via
// the launch JSON object:
//
//	wss://host/chromium?token=TOKEN&launch=<url_encoded_json>
//
// The launch JSON format: {"args":["--flag1","--flag2",...]}.
//
// This is a small, pure URL helper — it lives here next to the only
// caller. The other CDP URL helpers (GetCDPURL, wsURLToHTTPBase,
// isBrowserlessDirectWS, discoverCloakBrowserWSURL, VersionResponse)
// remain in internal/core because they are general-purpose URL
// utilities, not configuration-translation logic.
func encodeChromeArgsForBrowserless(wsURL string, args []string) string {
	if len(args) == 0 {
		return wsURL
	}
	launchJSON, err := json.Marshal(map[string]any{"args": args})
	if err != nil {
		return wsURL
	}
	encoded := url.QueryEscape(string(launchJSON))
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return wsURL
	}
	newQuery := parsed.RawQuery
	if newQuery != "" {
		newQuery += "&"
	}
	newQuery += "launch=" + encoded
	return parsed.Scheme + "://" + parsed.Host + parsed.Path + "?" + newQuery
}
