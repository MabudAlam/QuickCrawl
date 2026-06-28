// Package types provides the API types, request/response structures,
// and configuration types used across the application.
package types

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/utils"
)

// RenderMode describes how a page should be fetched. It is the single
// 3-state enum used at every layer: the server-wide default
// ([renderer] render_mode in quickcrawl.toml / RENDERER__RENDER_MODE
// env var) and the per-request override (the "renderMode" field on
// ScrapeRequest, CrawlRequest, MapRequest, and the matching MCP tool
// arguments).
//
// Precedence is: per-request (non-nil) > server default > RenderModeAuto.
//
// The zero value ("") is the unset/inherit value. RenderModeAuto is
// the explicit "I want auto" value, which is semantically equivalent
// to unset for fetching behavior but is preserved as-is in cache keys
// so callers can see exactly what they asked for.
type RenderMode string

const (
	// RenderModeAuto: HTTP first, escalate to browser on anti-bot/SPA signals.
	RenderModeAuto RenderMode = "auto"
	// RenderModeBrowser: always go through the browser, never HTTP-only.
	RenderModeBrowser RenderMode = "browser"
	// RenderModeHTTP: HTTP only, never touch the browser.
	RenderModeHTTP RenderMode = "http"
)

// String returns the canonical lowercase name of the mode.
func (m RenderMode) String() string { return string(m) }

// IsValid reports whether m is one of the three recognized modes.
// The empty string is treated as invalid; callers that want to allow
// "unset" should compare against the zero value directly.
func (m RenderMode) IsValid() bool {
	switch m {
	case RenderModeAuto, RenderModeBrowser, RenderModeHTTP:
		return true
	}
	return false
}

// ParseRenderMode normalizes a raw string (from toml, env, or JSON)
// into a RenderMode. It is case-insensitive and trims whitespace.
// An empty string returns ("", nil) so callers can distinguish
// "unset" from "invalid". Unknown values return a non-nil error.
func ParseRenderMode(s string) (RenderMode, error) {
	m := RenderMode(strings.ToLower(strings.TrimSpace(s)))
	if m == "" {
		return "", nil
	}
	if !m.IsValid() {
		return "", fmt.Errorf("invalid render_mode %q: must be one of auto, browser, http", s)
	}
	return m, nil
}

// =============================================================================
// Output Format Types
// =============================================================================

// OutputFormat specifies the desired output format for scraped content.
type OutputFormat string

// Supported output formats.
const (
	FormatMarkdown   OutputFormat = "markdown"   // Markdown format (images stripped by default)
	FormatHtml       OutputFormat = "html"       // Clean HTML format
	FormatRawHtml    OutputFormat = "rawHtml"    // Raw HTML as received
	FormatPlainText  OutputFormat = "plainText"  // Plain text without markup
	FormatLinks      OutputFormat = "links"      // Just the links found on page
	FormatJson       OutputFormat = "json"       // JSON (LLM extracted)
	FormatImageLinks OutputFormat = "imageLinks" // All image src URLs
)

// UnmarshalJSON parses a JSON string into an OutputFormat.
func (f *OutputFormat) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "markdown":
		*f = FormatMarkdown
	case "html":
		*f = FormatHtml
	case "rawHtml":
		*f = FormatRawHtml
	case "plainText":
		*f = FormatPlainText
	case "links":
		*f = FormatLinks
	case "json", "extract", "llm-extract":
		*f = FormatJson
	case "imageLinks":
		*f = FormatImageLinks
	default:
		return fmt.Errorf("unknown format '%s'", s)
	}
	return nil
}

// ExtractOptions defines options for LLM-based extraction.
type ExtractOptions struct {
	Schema         json.RawMessage `json:"schema,omitempty"`         // JSON schema for structured extraction
	Prompt         string          `json:"prompt,omitempty"`         // Custom extraction prompt
	ResponseFormat string          `json:"responseFormat,omitempty"` // Custom response format name
}

// =============================================================================
// Scrape Request/Response Types
// =============================================================================

// ScrapeRequest defines the parameters for a single URL scrape operation.
type ScrapeRequest struct {
	URL                 string            `json:"url"`                           // URL to scrape
	Formats             []OutputFormat    `json:"formats"`                       // Desired output formats
	RenderMode          *RenderMode       `json:"renderMode,omitempty"`          // Per-request override: "auto" | "browser" | "http". nil = inherit server default.
	WaitFor             *int64            `json:"waitFor,omitempty"`             // Wait time in ms after page load
	IncludeTags         []string          `json:"includeTags,omitempty"`         // HTML tags to include
	ExcludeTags         []string          `json:"excludeTags,omitempty"`         // HTML tags to exclude
	JSONSchema          *json.RawMessage  `json:"jsonSchema,omitempty"`          // Schema for JSON output
	Headers             map[string]string `json:"headers,omitempty"`             // Custom HTTP headers
	CSSSelector         *string           `json:"cssSelector,omitempty"`         // Extract specific element
	Extract             *ExtractOptions   `json:"extract,omitempty"`             // LLM extraction options
	LLMExtractionPrompt *string           `json:"llmExtractionPrompt,omitempty"` // LLM extraction prompt override
	LLMResponseFormat   *string           `json:"llmResponseFormat,omitempty"`   // LLM response format name override
	TTL *int64 `json:"ttl,omitempty"` // Cache TTL in seconds (0 = bypass cache, >0 = accept cached if younger)
}

// Defaults sets default values for optional fields.
func (r *ScrapeRequest) Defaults() {
	if r.Formats == nil {
		r.Formats = []OutputFormat{FormatMarkdown}
	}
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
}

// Validate checks the request against the allowed contract:
//   - URL must be present and parse cleanly with ValidateURL
//   - renderMode (if set) must be one of auto/browser/http
//   - waitFor must be in [0, 120000] ms
//   - ttl must be >= 0
//   - formats are validated by OutputFormat.UnmarshalJSON
func (r *ScrapeRequest) Validate() error {
	if strings.TrimSpace(r.URL) == "" {
		return fmt.Errorf("url is required")
	}
	if _, err := ValidateURL(r.URL); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if r.RenderMode != nil {
		if _, ok := validRenderModes[*r.RenderMode]; !ok {
			return fmt.Errorf("renderMode %q is invalid; allowed: auto, browser, http", string(*r.RenderMode))
		}
	}
	if r.WaitFor != nil {
		if *r.WaitFor < 0 || *r.WaitFor > 120000 {
			return fmt.Errorf("waitFor %d is out of range; must be between 0 and 120000 ms", *r.WaitFor)
		}
	}
	if r.TTL != nil && *r.TTL < 0 {
		return fmt.Errorf("ttl %d is invalid; must be >= 0", *r.TTL)
	}
	return nil
}

// PageMetadata contains extracted metadata about a scraped page.
type PageMetadata struct {
	Title         *string `json:"title,omitempty"`          // Page title
	Description   *string `json:"description,omitempty"`    // Meta description
	OGTitle       *string `json:"ogpTitle,omitempty"`       // Open Graph title
	OGDescription *string `json:"ogpDescription,omitempty"` // Open Graph description
	OGImage       *string `json:"ogpImage,omitempty"`       // Open Graph image URL
	CanonicalURL  *string `json:"canonicalUrl,omitempty"`   // Canonical URL
	SourceURL     string  `json:"sourceURL"`                // Actual URL scraped
	Language      *string `json:"language,omitempty"`       // Page language
	StatusCode    uint16  `json:"statusCode"`               // HTTP status code
	RenderedMode  *string `json:"renderedMode,omitempty"`   // How it was rendered (http/js)
}

// ScrapeData contains the result of a scrape operation.
type ScrapeData struct {
	Markdown   *string         `json:"markdown,omitempty"`   // Markdown conversion (images stripped)
	HTML       *string         `json:"html,omitempty"`       // Clean HTML
	RawHTML    *string         `json:"rawHtml,omitempty"`    // Raw HTML as received
	PlainText  *string         `json:"plainText,omitempty"`  // Plain text
	Links      []string        `json:"links,omitempty"`      // URLs found on page
	ImageLinks []string        `json:"imageLinks,omitempty"` // Image URLs found on page
	JSON       json.RawMessage `json:"json,omitempty"`       // LLM extracted JSON
	Warning    *string         `json:"warning,omitempty"`    // Non-fatal warning
	Metadata   PageMetadata    `json:"metadata"`             // Page metadata
}

// APIResponse wraps API responses with success/error information.
type APIResponse[T any] struct {
	Success   bool    `json:"success"`             // Whether request succeeded
	Data      *T      `json:"data,omitempty"`      // Response data
	Error     *string `json:"error,omitempty"`     // Error message
	ErrorCode *string `json:"errorCode,omitempty"` // Error code
	Warning   *string `json:"warning,omitempty"`   // Warning message
}

// APIOK creates a successful API response containing the given data.
func APIOK[T any](data T) APIResponse[T] {
	return APIResponse[T]{Success: true, Data: &data}
}

// APIErr creates an error API response with a message.
func APIErr[T any](message string) APIResponse[T] {
	return APIResponse[T]{Success: false, Error: &message}
}

// APIErrWithCode creates an error API response with message and code.
func APIErrWithCode[T any](message string, code string) APIResponse[T] {
	return APIResponse[T]{Success: false, Error: &message, ErrorCode: &code}
}

// =============================================================================
// Crawl Types
// =============================================================================

// CrawlStatus represents the current state of a crawl job.
type CrawlStatus string

// Possible crawl states.
const (
	CrawlStatusInProgress CrawlStatus = "scraping"  // Currently scraping
	CrawlStatusCompleted  CrawlStatus = "completed" // Successfully completed
	CrawlStatusFailed     CrawlStatus = "failed"    // Failed with error
)

// CrawlRequest defines parameters for a crawl operation.
type CrawlRequest struct {
	URL      string         `json:"url"`                // Starting URL
	MaxDepth *uint32        `json:"maxDepth,omitempty"` // Maximum link depth
	MaxPages *uint32        `json:"maxPages,omitempty"` // Maximum pages to crawl
	Formats  []OutputFormat `json:"formats"`            // Desired output formats

	RenderMode *RenderMode `json:"renderMode,omitempty"` // Per-request override: "auto" | "browser" | "http". nil = inherit server default.
	WaitFor    *int64      `json:"waitFor,omitempty"`    // Wait time in ms
}

// Defaults sets default values for optional fields.
func (r *CrawlRequest) Defaults() {
	if r.Formats == nil {
		r.Formats = []OutputFormat{FormatMarkdown}
	}
}

// Validate checks the request against the allowed contract:
//   - URL must be present and parse cleanly with ValidateURL
//   - maxDepth in [0, 100] if set (negative rejected)
//   - maxPages in [1, 100] if set (zero and negatives rejected)
//   - renderMode (if set) must be one of auto/browser/http
//   - waitFor in [0, 120000] ms
//   - json format is not allowed on /v1/crawl (caller must use /v1/scrape)
func (r *CrawlRequest) Validate() error {
	if strings.TrimSpace(r.URL) == "" {
		return fmt.Errorf("url is required")
	}
	if _, err := ValidateURL(r.URL); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if r.MaxDepth != nil {
		if *r.MaxDepth < 0 {
			return fmt.Errorf("maxDepth %d is invalid; must be >= 0", *r.MaxDepth)
		}
		if *r.MaxDepth > 100 {
			return fmt.Errorf("maxDepth %d is out of range; must be between 0 and 100", *r.MaxDepth)
		}
	}
	if r.MaxPages != nil {
		if *r.MaxPages < 1 {
			return fmt.Errorf("maxPages %d is out of range; must be between 1 and 100", *r.MaxPages)
		}
		if *r.MaxPages > 100 {
			return fmt.Errorf("maxPages %d is out of range; must be between 1 and 100", *r.MaxPages)
		}
	}
	if r.RenderMode != nil {
		if _, ok := validRenderModes[*r.RenderMode]; !ok {
			return fmt.Errorf("renderMode %q is invalid; allowed: auto, browser, http", string(*r.RenderMode))
		}
	}
	if r.WaitFor != nil {
		if *r.WaitFor < 0 || *r.WaitFor > 120000 {
			return fmt.Errorf("waitFor %d is out of range; must be between 0 and 120000 ms", *r.WaitFor)
		}
	}
	for _, f := range r.Formats {
		if f == FormatJson {
			return fmt.Errorf("'json' format is not supported on /v1/crawl. Use /v1/scrape for LLM-based JSON extraction")
		}
	}
	return nil
}

// CrawlState represents the current state of a crawl job.
type CrawlState struct {
	ID        string       `json:"id,omitempty"`    // Unique job ID
	Success   bool         `json:"success"`         // Whether job succeeded
	Status    CrawlStatus  `json:"status"`          // Current status
	Total     uint32       `json:"total"`           // Total URLs discovered
	Completed uint32       `json:"completed"`       // Pages successfully scraped
	Data      []ScrapeData `json:"data"`            // Scraped page data
	Error     *string      `json:"error,omitempty"` // Error message if failed
}

// CrawlStartResponse is returned when a crawl job is started.
type CrawlStartResponse struct {
	Success bool   `json:"success"` // Always true if job started
	ID      string `json:"id"`      // Unique job ID for status checks
}

// =============================================================================
// Map Types
// =============================================================================

// MapRequest defines parameters for URL discovery.
type MapRequest struct {
	URL        string `json:"url"`                  // Starting URL
	MaxDepth   *int   `json:"maxDepth,omitempty"`   // Maximum link depth
	UseSitemap *bool  `json:"useSitemap,omitempty"` // Whether to check sitemap
	Timeout    *int   `json:"timeout,omitempty"`    // Timeout in milliseconds
}

// Defaults sets default values for optional fields.
func (r *MapRequest) Defaults() {
	if r.UseSitemap == nil {
		v := true
		r.UseSitemap = &v
	}
}

// Validate checks the request against the allowed contract:
//   - URL must be present and parse cleanly with ValidateURL
//   - maxDepth in [0, 100] if set (negative rejected)
//   - timeout in (0, 600000] ms if set
func (r *MapRequest) Validate() error {
	if strings.TrimSpace(r.URL) == "" {
		return fmt.Errorf("url is required")
	}
	if _, err := ValidateURL(r.URL); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if r.MaxDepth != nil {
		if *r.MaxDepth < 0 {
			return fmt.Errorf("maxDepth %d is invalid; must be >= 0", *r.MaxDepth)
		}
		if *r.MaxDepth > 100 {
			return fmt.Errorf("maxDepth %d is out of range; must be between 0 and 100", *r.MaxDepth)
		}
	}
	if r.Timeout != nil {
		if *r.Timeout <= 0 || *r.Timeout > 600000 {
			return fmt.Errorf("timeout %d is out of range; must be between 1 and 600000 ms", *r.Timeout)
		}
	}
	return nil
}

// MapData contains discovered URLs.
type MapData struct {
	Links []string `json:"links"` // Discovered URLs
}

// MapResponse is returned by the map endpoint.
type MapResponse struct {
	Success bool     `json:"success"`         // Whether operation succeeded
	Data    *MapData `json:"data,omitempty"`  // Discovered URLs
	Error   *string  `json:"error,omitempty"` // Error message
}

// =============================================================================
// Search Types
// =============================================================================

// SearchRequest defines parameters for a search operation.
type SearchRequest struct {
	Query      string         `json:"query"`                // Search query (required)
	Region     string         `json:"region,omitempty"`     // Region code, e.g. "us-en". Mapped to SearXNG language.
	Language   string         `json:"language,omitempty"`   // SearXNG language code, e.g. "en", "auto", "all".
	TimeRange  string         `json:"timeRange,omitempty"`  // SearXNG time range: "day", "week", "month", "year".
	Categories string         `json:"categories,omitempty"` // Comma-separated SearXNG categories, e.g. "general,news".
	Page       int            `json:"page,omitempty"`       // SearXNG pageno, default 1.
	UseBM25    bool           `json:"use_bm25,omitempty"`   // Use BM25 scoring algorithm (default: false)
	RenderMode *RenderMode    `json:"renderMode,omitempty"` // Per-request override: "auto" | "browser" | "http". nil = inherit server default.
	Formats    []OutputFormat `json:"formats,omitempty"`    // Desired output formats when scrape=true.
	Scrape     bool           `json:"scrape,omitempty"`     // Scrape each result URL and include extracted content (default: false).
}

// Defaults sets default values for optional fields.
func (r *SearchRequest) Defaults() {
	if r.Region == "" && r.Language == "" {
		r.Language = "auto"
	}
	if r.Categories == "" {
		r.Categories = "general"
	}
	if r.Page <= 0 {
		r.Page = 1
	}
	if r.Scrape && r.Formats == nil {
		r.Formats = []OutputFormat{FormatMarkdown}
	}
}

// Valid SearXNG time_range values. Empty string is treated as "no filter".
var validTimeRanges = map[string]struct{}{
	"":     {},
	"day":   {},
	"week":  {},
	"month": {},
	"year":  {},
}

// Valid SearXNG category tokens.
var validCategories = map[string]struct{}{
	"general":      {},
	"images":       {},
	"videos":       {},
	"news":         {},
	"map":          {},
	"music":        {},
	"it":           {},
	"science":      {},
	"social media": {},
	"files":        {},
	"code":         {},
}

// Valid SearXNG safesearch levels.
var validSafesearch = map[string]struct{}{
	"":  {},
	"0": {},
	"1": {},
	"2": {},
}

// Valid SearXNG render mode overrides accepted by the API.
var validRenderModes = map[RenderMode]struct{}{
	"":            {},
	RenderModeAuto:   {},
	RenderModeBrowser: {},
	RenderModeHTTP:   {},
}

// Validate returns an error if any field holds a value outside the allowed
// SearXNG contract. Empty optional fields are accepted.
func (r *SearchRequest) Validate() error {
	if strings.TrimSpace(r.Query) == "" {
		return fmt.Errorf("query is required")
	}
	if r.TimeRange != "" {
		if _, ok := validTimeRanges[r.TimeRange]; !ok {
			return fmt.Errorf("timeRange %q is invalid; allowed: day, week, month, year", r.TimeRange)
		}
	}
	if r.Categories != "" {
		for _, raw := range strings.Split(r.Categories, ",") {
			cat := strings.TrimSpace(raw)
			if cat == "" {
				continue
			}
			if _, ok := validCategories[cat]; !ok {
				return fmt.Errorf("categories contains invalid value %q; allowed: general, images, videos, news, map, music, it, science, \"social media\", files, code", cat)
			}
		}
	}

	if r.Page < 0 || r.Page > 1000 {
		return fmt.Errorf("page %d is out of range; must be between 0 and 1000", r.Page)
	}
	if r.RenderMode != nil {
		if _, ok := validRenderModes[*r.RenderMode]; !ok {
			return fmt.Errorf("renderMode %q is invalid; allowed: auto, browser, http", string(*r.RenderMode))
		}
	}
	return nil
}

// SearchResult represents a single search result with optional scraped content.
type SearchResult struct {
	Position  int      `json:"position"`                   // 1-based position in the result set
	Score     float64  `json:"score"`                       // Native score from search engine
	BM25Score float64  `json:"bm25_score,omitempty"`       // BM25 relevance score (set when use_bm25=true)
	Title     string   `json:"title"`                      // Result title
	URL       string   `json:"url"`                        // Result URL
	SiteName  string   `json:"site_name,omitempty"`        // Hostname extracted from URL.
	Snippet   string   `json:"snippet,omitempty"`           // Search snippet / description.
	Engine    string   `json:"-"`                          // Internal only, not exposed in API response
	Published string   `json:"published_date,omitempty"`   // ISO 8601 publish date if available.
	Markdown  *string  `json:"markdown,omitempty"`
	HTML      *string  `json:"html,omitempty"`
	RawHTML   *string  `json:"raw_html,omitempty"`
	PlainText *string  `json:"plain_text,omitempty"`
	Links     []string `json:"links,omitempty"`
	RawJSON   []byte   `json:"raw_json,omitempty"`
}

// SearchData contains the search results.
type SearchData struct {
	Results []SearchResult `json:"results"` // List of search results
}

// SearchResponse is returned by the search endpoint. Matches the public
// "firecrawl-style" flat shape: {query, results, total_results, page}.
type SearchResponse struct {
	Query        string         `json:"query"`        // Echo of the search query.
	Results      []SearchResult `json:"results"`      // List of search results.
	TotalResults int            `json:"total_results"`// Number of results returned.
	Page         int            `json:"page"`         // 0-based page index.
}

// =============================================================================
// Internal Types
// =============================================================================

// FetchResult contains the raw result from a fetcher (HTTP or browser).
type FetchResult struct {
	URL               string  // URL that was fetched
	FinalURL          *string // Final URL after redirects
	StatusCode        uint16  // HTTP status code
	HTML              string  // Fetched HTML content
	ContentType       *string // Content-Type header value
	RawBytes          []byte  // Raw bytes (for PDFs)
	RenderedWith      *string // How it was rendered (http/browser)
	Warning           *string // Non-fatal warning
	CapturedResponses []CapturedNetworkResponse
}

// CapturedNetworkResponse stores an XHR/fetch response body captured during a
// browser render.
type CapturedNetworkResponse struct {
	URL           string  `json:"url"`
	RequestID     string  `json:"requestId"`
	Status        uint16  `json:"status"`
	MimeType      *string `json:"mimeType,omitempty"`
	BodySizeBytes int     `json:"bodySizeBytes"`
	Body          *string `json:"body,omitempty"`
}

// CdpEndpoint defines a Chrome DevTools Protocol endpoint.
type CdpEndpoint struct {
	WSURL      string   `toml:"ws_url" json:"wsUrl"`           // WebSocket URL
	ChromeArgs []string `toml:"chrome_args" json:"chromeArgs"` // Chrome launch flags
}

// BrowserInfo contains information about a running browser instance.
// The Name and WSURL are surfaced via the /health endpoint and the
// MCP tool output.
type BrowserInfo struct {
	Name  string `json:"name"`  // Browser name (e.g., "chrome")
	WSURL string `json:"wsUrl"` // Chrome DevTools Protocol WebSocket URL
}

// =============================================================================
// Configuration Types
// =============================================================================

// RendererConfig configures the rendering subsystem.
type RendererConfig struct {
	PageTimeoutMs int64        `toml:"page_timeout_ms" json:"pageTimeoutMs"`  // Page load timeout
	PoolSize      int          `toml:"pool_size" json:"poolSize"`             // Browser pool size
	RenderMode    RenderMode   `toml:"render_mode" json:"renderMode"`         // Render mode: auto, http, browser (empty = inherit)
	Browser       string       `toml:"browser" json:"browser"`                // Browser: cloak, browserless, lightpanda
	Chrome        *CdpEndpoint `toml:"chrome" json:"chrome"`                  // Chrome config
}

// Defaults sets default values for unset fields.
// Also normalizes RenderMode (case + whitespace) so downstream code
// never has to handle "AUTO" or " browser " as a separate case.
func (c *RendererConfig) Defaults() {
	if c.PageTimeoutMs == 0 {
		c.PageTimeoutMs = 30000
	}
	if c.PoolSize == 0 {
		c.PoolSize = 4
	}
	if c.RenderMode != "" {
		c.RenderMode = RenderMode(strings.ToLower(strings.TrimSpace(string(c.RenderMode))))
	}
}

// StealthConfig configures stealth/bot-detection evasion.
type StealthConfig struct {
	Enabled       bool    `toml:"enabled" json:"enabled"`              // Enable stealth mode
	JitterFactor  float64 `toml:"jitter_factor" json:"jitterFactor"`   // Random delay factor
	InjectHeaders bool   `toml:"inject_headers" json:"injectHeaders"` // Inject browser headers
	Strategy      string `toml:"strategy" json:"strategy"`             // Header strategy: modern_browser, mobile_device, bot_friendly
}

// Defaults sets default values for unset fields.
func (s *StealthConfig) Defaults() {
	s.JitterFactor = 0.2
	s.InjectHeaders = true
	if s.Strategy == "" {
		s.Strategy = "modern_browser"
	}
}

// CrawlerConfig configures the crawling subsystem.
type CrawlerConfig struct {
	MaxConcurrency    int           `toml:"max_concurrency" json:"maxConcurrency"`        // Max concurrent crawls
	RequestsPerSecond float64       `toml:"requests_per_second" json:"requestsPerSecond"` // Rate limit
	RespectRobotsTxt  bool          `toml:"respect_robots_txt" json:"respectRobotsTxt"`   // Follow robots.txt
	UserAgent         string        `toml:"user_agent" json:"userAgent"`                  // User agent string
	DefaultMaxDepth   int           `toml:"default_max_depth" json:"defaultMaxDepth"`     // Default max depth
	DefaultMaxPages   int           `toml:"default_max_pages" json:"defaultMaxPages"`     // Default max pages
	JobTTLSecs        int64         `toml:"job_ttl_secs" json:"jobTtlSecs"`               // Job TTL in seconds
	Stealth           StealthConfig `toml:"stealth" json:"stealth"`                       // Stealth settings
}

// Defaults sets default values for unset fields.
func (c *CrawlerConfig) Defaults() {
	if c.MaxConcurrency == 0 {
		c.MaxConcurrency = 10
	}
	if c.RequestsPerSecond == 0 {
		c.RequestsPerSecond = 10.0
	}
	if c.UserAgent == "" {
		c.UserAgent = ""
	}
	if c.DefaultMaxDepth == 0 {
		c.DefaultMaxDepth = 2
	}
	if c.DefaultMaxPages == 0 {
		c.DefaultMaxPages = 100
	}
	if c.JobTTLSecs == 0 {
		c.JobTTLSecs = 3600
	}
	c.Stealth.Defaults()
}

// LLMConfig configures LLM-based extraction.
type LLMConfig struct {
	APIKey           string  `toml:"api_key" json:"apiKey"`                     // API key
	Model            string  `toml:"model" json:"model"`                        // Model name
	BaseURL          *string `toml:"base_url" json:"baseUrl"`                   // Custom API base URL
	MaxTokens        uint32  `toml:"max_tokens" json:"maxTokens"`               // Max tokens in response
	ExtractionPrompt string  `toml:"extraction_prompt" json:"extractionPrompt"` // System prompt for extraction
	ResponseFormat   string  `toml:"response_format" json:"responseFormat"`     // Response format name
}

// Defaults sets default values for unset fields.
func (l *LLMConfig) Defaults() {
	if l.Model == "" {
		l.Model = "gpt-4o-mini"
	}
	if l.MaxTokens == 0 {
		l.MaxTokens = 4096
	}
	if l.ExtractionPrompt == "" {
		l.ExtractionPrompt = "You are a data extraction assistant. Extract structured data from the user's content according to the provided JSON schema. Return ONLY the JSON object that matches the schema."
	}
	if l.ResponseFormat == "" {
		l.ResponseFormat = "extracted_data"
	}
}

// ExtractionConfig configures the extraction subsystem.
type ExtractionConfig struct {
	LLM *LLMConfig `toml:"llm" json:"llm"` // LLM settings
}

// Defaults sets default values for unset fields.
func (e *ExtractionConfig) Defaults() {
}

// CacheConfig configures the Redis cache.
type CacheConfig struct {
	Enabled      bool   `toml:"enabled" json:"enabled"`             // Enable/disable caching
	RedisURL     string `toml:"redis_url" json:"redisUrl"`           // Redis connection URL
	Password    string `toml:"password" json:"password"`             // Redis password
	DB           int    `toml:"db" json:"db"`                       // Redis database number
	TTLDefaultSecs int64 `toml:"ttl_default_secs" json:"ttlDefaultSecs"` // Default TTL in seconds (0 = no cache)
}

// ParseRedisURL populates RedisURL, Password, and DB from a standard redis:// URI.
// This allows hosting platforms to provide a single REDIS_URL environment variable.
func (c *CacheConfig) ParseRedisURL(uri string) error {
	if uri == "" {
		return nil
	}
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid redis url: %w", err)
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return fmt.Errorf("invalid redis scheme: %s", u.Scheme)
	}
	c.RedisURL = u.Host
	if u.User != nil {
		c.Password, _ = u.User.Password()
		if u.User.Username() != "" && u.User.Username() != "default" {
		}
	}
	if u.Path != "" && u.Path != "/" {
		db, err := strconv.Atoi(strings.TrimPrefix(u.Path, "/"))
		if err == nil {
			c.DB = db
		}
	}
	return nil
}

// Defaults sets default values for unset fields.
func (c *CacheConfig) Defaults() {
	if c.TTLDefaultSecs == 0 {
		c.TTLDefaultSecs = 3600
	}
	if c.Enabled && c.RedisURL != "" && !strings.HasPrefix(c.RedisURL, "redis://") && !strings.HasPrefix(c.RedisURL, "rediss://") {
		return
	}
	if c.Enabled && c.RedisURL != "" && (strings.HasPrefix(c.RedisURL, "redis://") || strings.HasPrefix(c.RedisURL, "rediss://")) {
		_ = c.ParseRedisURL(c.RedisURL)
	}
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Host               string `toml:"host" json:"host"`                               // Listen address
	Port               uint16 `toml:"port" json:"port"`                               // Listen port
	RequestTimeoutSecs int64  `toml:"request_timeout_secs" json:"requestTimeoutSecs"` // Request timeout
	RateLimitRPS       uint64 `toml:"rate_limit_rps" json:"rateLimitRps"`             // Rate limit RPS
}

// Defaults sets default values for unset fields.
func (s *ServerConfig) Defaults() {
	if s.Host == "" {
		s.Host = "0.0.0.0"
	}
	if s.Port == 0 {
		s.Port = 3000
	}
	if s.RequestTimeoutSecs == 0 {
		s.RequestTimeoutSecs = 60
	}
	if s.RateLimitRPS == 0 {
		s.RateLimitRPS = 10
	}
}

// AppConfig is the root configuration structure.
type AppConfig struct {
	Server     ServerConfig     `toml:"server" json:"server"`         // Server settings
	Renderer   RendererConfig   `toml:"renderer" json:"renderer"`     // Renderer settings
	Crawler    CrawlerConfig    `toml:"crawler" json:"crawler"`       // Crawler settings
	Extraction ExtractionConfig `toml:"extraction" json:"extraction"` // Extraction settings
	Cache      CacheConfig      `toml:"cache" json:"cache"`           // Cache settings
	Search     SearchConfig     `toml:"search" json:"search"`         // Search engine settings
}

// Defaults sets default values for all subsystems.
func (c *AppConfig) Defaults() {
	c.Server.Defaults()
	c.Renderer.Defaults()
	c.Crawler.Defaults()
	c.Extraction.Defaults()
	c.Cache.Defaults()
	c.Search.Defaults()
}

// SearchConfig configures the search backend used by the /v1/search endpoint.
// SearchConfig configures the SearXNG search backend.
// Only SearXNG is supported as the search engine.
type SearchConfig struct {
	// BaseURL is the SearXNG instance root (no trailing slash). The
	// search client appends /search?q=...&format=json.
	BaseURL string `toml:"base_url" json:"baseUrl"`

	// TimeoutSecs is the request timeout for the upstream search call.
	TimeoutSecs int `toml:"timeout_secs" json:"timeoutSecs"`

	// BM25F weights for re-ranking. Matches in higher-weight fields
	// (typically the title) contribute more to the final score.
	BM25FTitleWeight   float64 `toml:"bm25f_title_weight" json:"bm25fTitleWeight"`
	BM25FSnippetWeight float64 `toml:"bm25f_snippet_weight" json:"bm25fSnippetWeight"`
}

// Defaults sets the default timeout and BM25F weights. BaseURL must be
// set separately.
func (s *SearchConfig) Defaults() {
	if s.TimeoutSecs == 0 {
		s.TimeoutSecs = 30
	}
	if s.BM25FTitleWeight == 0 {
		s.BM25FTitleWeight = 2.0
	}
	if s.BM25FSnippetWeight == 0 {
		s.BM25FSnippetWeight = 1.0
	}
}

// Validate checks if the search config is valid. Returns an error
// with configuration guidance if BaseURL is empty.
func (s *SearchConfig) Validate() error {
	if strings.TrimSpace(s.BaseURL) == "" {
		return fmt.Errorf(
			"search.base_url is required and cannot be empty; " +
				"set it in quickcrawl.toml ([search] base_url) " +
				"or via the SEARCH__BASE_URL environment variable",
		)
	}
	return nil
}

// HTTPClientConfig holds HTTP client settings.
type HTTPClientConfig struct {
	ConnectTimeout time.Duration // Connection timeout
	Timeout        time.Duration // Request timeout
	UserAgent      string        // User agent string
}

// ValidateSafeURL checks if a URL is safe for crawling.
func ValidateSafeURL(u *url.URL) error {
	return utils.ValidateSafeURL(u)
}

// ValidateURL parses and validates a URL string.
func ValidateURL(urlStr string) (*url.URL, error) {
	return utils.ValidateURL(urlStr)
}

// BuiltinUAPool is a pool of real browser user agents.
var BuiltinUAPool = utils.BuiltinUAPool

// GetBuiltinUAPool returns a copy of the built-in UA pool.
var GetBuiltinUAPool = utils.GetBuiltinUAPool

// =============================================================================
// Scraper-side configuration
//
// These types are the internal "what the scraper actually consumes" view
// of the operator-facing AppConfig. They live in internal/types (not in
// internal/core or internal/config) to avoid an import cycle: internal/core
// imports internal/config to wire up the scraper, and internal/config
// imports internal/core for the scraper primitives (NewScraper,
// GetCDPURL, etc.) it needs. Putting the data-only types in a third
// package that both can import breaks the cycle. The constructor
// NewScraperFromConfig — the AppConfig → ScraperConfig projection —
// lives in internal/config and is the only caller of these types.
// =============================================================================

// ScraperConfig is the scraper-side view of the configuration. It is
// a strict subset of AppConfig, projected down to the fields the
// scraper runtime actually uses.
type ScraperConfig struct {
	Browser BrowserConfig
	Pool    PoolConfig
}

// BrowserConfig is the scraper's view of the renderer subsystem. It
// carries everything the chromedp allocator and the per-host pool need
// to know about a browser, plus the render_mode that gates the
// HTTP-vs-browser branch in FetchOrchestrator.
type BrowserConfig struct {
	Mode           RenderMode
	BrowserType    string // "browserless", "cloak", "lightpanda"
	WSURL          string
	NumBrowsers    int
	PageTimeout    time.Duration
	PoolSize       int
	StealthEnabled bool     // When true, register anti-fingerprint JS on every page. When false, the call is skipped entirely.
	ChromeArgs     []string // Chrome launch flags for browserless
}

// PoolConfig is the per-host concurrency pool config.
type PoolConfig struct {
	Size    int
	PerHost int
}

// DefaultScraperConfig is the scraper-side default. Operator overrides
// come in via config.NewScraperFromConfig (which projects an AppConfig
// into this shape) or via direct core.NewScraper construction in tests.
func DefaultScraperConfig() ScraperConfig {
	return ScraperConfig{
		Browser: BrowserConfig{
			Mode:        RenderModeAuto,
			WSURL:       "",
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

type BrandRequest struct {
	URL string `json:"url"`
}

type BrandResponse struct {
	Success bool       `json:"success"`
	Domain  string    `json:"domain"`
	Brand   *BrandData `json:"brand,omitempty"`
}

type BrandData struct {
	Domain     string           `json:"domain,omitempty"`
	Title     string           `json:"title,omitempty"`
	Name      string           `json:"name,omitempty"`
	Tagline   string           `json:"tagline,omitempty"`
	Description string         `json:"description,omitempty"`
	Colors    []BrandColor     `json:"colors,omitempty"`
	Logos     []BrandLogo      `json:"logos,omitempty"`
	Backdrops []BrandBackdrop  `json:"backdrops,omitempty"`
	Address   *BrandAddress    `json:"address,omitempty"`
	Socials   []SocialLink    `json:"socials,omitempty"`
	Links     *BrandLinks     `json:"links,omitempty"`
	PrimaryLanguage string    `json:"primary_language,omitempty"`
	Fonts     *BrandFonts     `json:"fonts,omitempty"`
	Styleguide *BrandStyleguide `json:"styleguide,omitempty"`
}

// BrandFonts is the typography signal extracted from a rendered page.
// Fonts holds per-font usage stats (which elements use the font, how many
// elements/words, what % of the page). FontLinks maps font display name
// to the actual woff2/woff/ttf file URLs grouped by weight.
type BrandFonts struct {
	Fonts     []BrandFont           `json:"fonts"`
	FontLinks map[string]BrandFontLink `json:"fontLinks"`
}

type BrandFont struct {
	Font           string   `json:"font"`
	Uses           []string `json:"uses"`
	Fallbacks      []string `json:"fallbacks"`
	NumElements    int      `json:"num_elements"`
	NumWords       int      `json:"num_words"`
	PercentElements int     `json:"percent_elements"`
	PercentWords   int      `json:"percent_words"`
}

type BrandFontLink struct {
	Type        string            `json:"type"` // "google" | "custom" | "adobe" | "system"
	Files       map[string]string `json:"files"` // weight -> url
	DisplayName string            `json:"displayName,omitempty"`
	Category    string            `json:"category,omitempty"`
}

// BrandStyleguide captures computed-style design tokens for the page.
// Typography headings/paragraphs carry real font-family, size, weight,
// line-height and letter-spacing. Components carry the actual rendered
// button/card CSS so a downstream style guide can re-use them verbatim.
type BrandStyleguide struct {
	Mode            string                  `json:"mode"` // "light" | "dark"
	Colors          BrandStyleguideColors   `json:"colors"`
	Typography      BrandStyleguideTypography `json:"typography"`
	ElementSpacing  map[string]string       `json:"elementSpacing"`
	Shadows         map[string]string       `json:"shadows"`
	Components      BrandStyleguideComponents `json:"components"`
	FontLinks       map[string]BrandFontLink `json:"fontLinks"`
}

type BrandStyleguideColors struct {
	Accent     string `json:"accent"`
	Background string `json:"background"`
	Text       string `json:"text"`
}

type BrandStyleguideTypography struct {
	Headings map[string]BrandTextStyle `json:"headings"`
	P        BrandTextStyle            `json:"p"`
}

type BrandTextStyle struct {
	FontFamily    string   `json:"fontFamily"`
	FontSize      string   `json:"fontSize"`
	FontWeight    int      `json:"fontWeight"`
	LineHeight    string   `json:"lineHeight"`
	LetterSpacing string   `json:"letterSpacing"`
	FontFallbacks []string `json:"fontFallbacks"`
}

type BrandStyleguideComponents struct {
	Button BrandButtonVariants `json:"button"`
	Card   BrandCardStyle      `json:"card"`
}

type BrandButtonVariants struct {
	Primary   BrandButtonStyle `json:"primary"`
	Secondary BrandButtonStyle `json:"secondary"`
	Link      BrandButtonStyle `json:"link"`
}

type BrandButtonStyle struct {
	BackgroundColor string   `json:"backgroundColor"`
	Color           string   `json:"color"`
	BorderColor     string   `json:"borderColor"`
	BorderRadius    string   `json:"borderRadius"`
	BorderWidth     string   `json:"borderWidth"`
	BorderStyle     string   `json:"borderStyle"`
	Padding         string   `json:"padding"`
	FontSize        string   `json:"fontSize"`
	FontWeight      int      `json:"fontWeight"`
	MinWidth        string   `json:"minWidth"`
	MinHeight       string   `json:"minHeight"`
	TextDecoration  string   `json:"textDecoration"`
	BoxShadow       string   `json:"boxShadow"`
	FontFallbacks   []string `json:"fontFallbacks"`
	FontFamily      string   `json:"fontFamily"`
	CSS             string   `json:"css"`
}

type BrandCardStyle struct {
	BackgroundColor string `json:"backgroundColor"`
	BorderColor     string `json:"borderColor"`
	BorderRadius    string `json:"borderRadius"`
	BorderWidth     string `json:"borderWidth"`
	BorderStyle     string `json:"borderStyle"`
	Padding         string `json:"padding"`
	BoxShadow       string `json:"boxShadow"`
	TextColor       string `json:"textColor"`
	CSS             string `json:"css"`
}

type BrandColor struct {
	Hex string `json:"hex"`
	Name string `json:"name"`
}

type BrandLogo struct {
	URL        string           `json:"url"`
	Format     string           `json:"format,omitempty"`
	Sizes      []int            `json:"sizes,omitempty"`
	Mode       string           `json:"mode,omitempty"`
	Colors     []BrandColor     `json:"colors,omitempty"`
	Resolution *ImageResolution `json:"resolution,omitempty"`
}

type BrandBackdrop struct {
	URL        string          `json:"url"`
	Colors     []BrandColor    `json:"colors,omitempty"`
	Resolution *ImageResolution `json:"resolution,omitempty"`
}

type ImageResolution struct {
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	AspectRatio float64 `json:"aspect_ratio"`
}

type BrandAddress struct {
	City         string `json:"city,omitempty"`
	Country      string `json:"country,omitempty"`
	CountryCode  string `json:"country_code,omitempty"`
	StateProvince string `json:"state_province,omitempty"`
	StateCode   string `json:"state_code,omitempty"`
	PostalCode  string `json:"postal_code,omitempty"`
}

type SocialLink struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type BrandLinks struct {
	Careers  string `json:"careers,omitempty"`
	Contact  string `json:"contact,omitempty"`
	Pricing  string `json:"pricing,omitempty"`
	Terms    string `json:"terms,omitempty"`
	Privacy  string `json:"privacy,omitempty"`
	Blog     string `json:"blog,omitempty"`
	Login    string `json:"login,omitempty"`
	Signup   string `json:"signup,omitempty"`
}
