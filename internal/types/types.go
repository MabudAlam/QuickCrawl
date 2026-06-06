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

	"github.com/MabudAlam/quickcrawl/internal/common"
)

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
	RenderJS            *bool             `json:"renderJs,omitempty"`            // Force JavaScript rendering
	WaitFor             *int64            `json:"waitFor,omitempty"`             // Wait time in ms after page load
	IncludeTags         []string          `json:"includeTags,omitempty"`         // HTML tags to include
	ExcludeTags         []string          `json:"excludeTags,omitempty"`         // HTML tags to exclude
	JSONSchema          *json.RawMessage  `json:"jsonSchema,omitempty"`          // Schema for JSON output
	Headers             map[string]string `json:"headers,omitempty"`             // Custom HTTP headers
	CSSSelector         *string           `json:"cssSelector,omitempty"`         // Extract specific element
	Extract             *ExtractOptions   `json:"extract,omitempty"`             // LLM extraction options
	LLMExtractionPrompt *string           `json:"llmExtractionPrompt,omitempty"` // LLM extraction prompt override
	LLMResponseFormat   *string           `json:"llmResponseFormat,omitempty"`   // LLM response format name override
	TTL                 *int64            `json:"ttl,omitempty"`                 // Cache TTL in seconds (0 = bypass cache, >0 = accept cached if younger)
	// Deprecated: Browser is accepted for backward compatibility but ignored.
	// The new scraper uses chromedp only — there is a single render path.
	Browser *string `json:"browser,omitempty"` // Browser to use (lightpanda, chrome)
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

	RenderJS *bool    `json:"renderJs,omitempty"` // Force JS rendering
	WaitFor  *int64   `json:"waitFor,omitempty"`  // Wait time in ms
	// Deprecated: Browser is accepted for backward compatibility but ignored.
	// The new scraper uses chromedp only.
	Browser *string `json:"browser,omitempty"` // Browser to use (lightpanda, chrome)
}

// Defaults sets default values for optional fields.
func (r *CrawlRequest) Defaults() {
	if r.Formats == nil {
		r.Formats = []OutputFormat{FormatMarkdown}
	}
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
	Query      string         `json:"query"`                // Search query
	Region     string         `json:"region"`               // Region code (e.g., "us-en")
	Safesearch string         `json:"safesearch,omitempty"` // SafeSearch mode: "moderate", "strict", "off"
	Timelimit  string         `json:"timelimit,omitempty"`  // Time limit filter (e.g., "d" for day)
	RenderJS   *bool          `json:"renderJs,omitempty"`   // Enable JavaScript rendering (nil = auto, true = force JS, false = HTTP only)
	Formats    []OutputFormat `json:"formats"`              // Desired output formats
	Scrape     bool           `json:"scrape,omitempty"`     // Scrape each result URL and include extracted content (default: false)
}

// Defaults sets default values for optional fields.
func (r *SearchRequest) Defaults() {
	if r.Region == "" {
		r.Region = "us-en"
	}
	if r.Safesearch == "" {
		r.Safesearch = "moderate"
	}
	if r.Formats == nil {
		r.Formats = []OutputFormat{FormatMarkdown}
	}
}

// SearchResult represents a single search result with scraped content.
type SearchResult struct {
	Title       string   `json:"title"`       // Result title
	Description string   `json:"description"` // Result snippet/description
	URL         string   `json:"url"`         // Result URL
	Markdown    *string  `json:"markdown,omitempty"`
	HTML        *string  `json:"html,omitempty"`
	RawHTML     *string  `json:"rawHtml,omitempty"`
	PlainText   *string  `json:"plainText,omitempty"`
	Links       []string `json:"links,omitempty"`
	RawJSON     []byte   `json:"rawJson,omitempty"`
}

// SearchData contains the search results.
type SearchData struct {
	Results []SearchResult `json:"results"` // List of search results
}

// SearchResponse is returned by the search endpoint.
type SearchResponse struct {
	Success bool        `json:"success"`         // Whether operation succeeded
	Data    *SearchData `json:"data,omitempty"`  // Search results
	Error   *string     `json:"error,omitempty"` // Error message
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
	WSURL string `toml:"ws_url" json:"wsUrl"` // WebSocket URL
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
	PageTimeoutMs int64        `toml:"page_timeout_ms" json:"pageTimeoutMs"` // Page load timeout
	PoolSize      int          `toml:"pool_size" json:"poolSize"`            // Browser pool size
	Chrome        *CdpEndpoint `toml:"chrome" json:"chrome"`                 // Chrome config
}

// Defaults sets default values for unset fields.
func (c *RendererConfig) Defaults() {
	if c.PageTimeoutMs == 0 {
		c.PageTimeoutMs = 30000
	}
	if c.PoolSize == 0 {
		c.PoolSize = 4
	}
}

// StealthConfig configures stealth/bot-detection evasion.
type StealthConfig struct {
	Enabled       bool    `toml:"enabled" json:"enabled"`              // Enable stealth mode
	JitterFactor  float64 `toml:"jitter_factor" json:"jitterFactor"`   // Random delay factor
	InjectHeaders bool    `toml:"inject_headers" json:"injectHeaders"` // Inject browser headers
	Strategy      string  `toml:"strategy" json:"strategy"`            // Header strategy: modern_browser, mobile_device, bot_friendly
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
}

// Defaults sets default values for all subsystems.
func (c *AppConfig) Defaults() {
	c.Server.Defaults()
	c.Renderer.Defaults()
	c.Crawler.Defaults()
	c.Extraction.Defaults()
	c.Cache.Defaults()
}

// HTTPClientConfig holds HTTP client settings.
type HTTPClientConfig struct {
	ConnectTimeout time.Duration // Connection timeout
	Timeout        time.Duration // Request timeout
	UserAgent      string        // User agent string
}

// =============================================================================
// Error Type Aliases (re-exported from common)
// =============================================================================

// Error codes (alias from common).
const (
	CodeHttp              = common.CodeHttp
	CodeTargetUnreachable = common.CodeTargetUnreachable
	CodeInvalidURL        = common.CodeInvalidURL
	CodeInvalidRequest    = common.CodeInvalidRequest
	CodeRendererError     = common.CodeRendererError
	CodeExtractionErr     = common.CodeExtractionErr
	CodeCrawlError        = common.CodeCrawlError
	CodeTimeout           = common.CodeTimeout
	CodeConfigError       = common.CodeConfigError
	CodeNotFound          = common.CodeNotFound
	CodeRateLimited       = common.CodeRateLimited
	CodeInternalErr       = common.CodeInternalErr
	CodeForbidden         = common.CodeForbidden
)

// Error factories (alias from common).
var (
	ErrHttp              = common.ErrHttp
	ErrTargetUnreachable = common.ErrTargetUnreachable
	ErrInvalidURL        = common.ErrInvalidURL
	ErrInvalidRequest    = common.ErrInvalidRequest
	ErrRendererError     = common.ErrRendererError
	ErrExtraction        = common.ErrExtraction
	ErrCrawl             = common.ErrCrawl
	ErrTimeout           = common.ErrTimeout
	ErrConfig            = common.ErrConfig
	ErrNotFound          = common.ErrNotFound
	ErrRateLimited       = common.ErrRateLimited
	ErrInternal          = common.ErrInternal
	ErrForbidden         = common.ErrForbidden
)

// Type aliases for common types.
type (
	QuickCrawlError        = common.QuickcrawlError
	QuickCrawlErrorFactory = common.QuickcrawlErrorFactory
	ErrorCode              = common.ErrorCode
)

// ValidateSafeURL checks if a URL is safe for crawling.
func ValidateSafeURL(u *url.URL) error {
	return common.ValidateSafeURL(u)
}

// ValidateURL parses and validates a URL string.
func ValidateURL(urlStr string) (*url.URL, error) {
	return common.ValidateURL(urlStr)
}

// BuiltinUAPool is a pool of real browser user agents.
var BuiltinUAPool = common.BuiltinUAPool

// GetBuiltinUAPool returns a copy of the built-in UA pool.
var GetBuiltinUAPool = common.GetBuiltinUAPool
