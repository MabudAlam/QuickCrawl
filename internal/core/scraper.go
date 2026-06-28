//Scrape Joins the API /scrape
//It validates and applies the waitMs and renderMode and
//then calls the renderer

package core

import (
	"context"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/extractor"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
)

type Scraper struct {
	renderer *Renderer
	llm      *llmExtractor
	llmCfg   *types.LLMConfig
	cfg      types.ScraperConfig
}

// NewScraper builds a Scraper that delegates HTTP fetching to the shared
// *renderer.HTTPFetcher (so /v1/scrape and /v1/scrape-core share the same
// HTTP path) and uses chromedp for browser rendering.
//
// llmConfig is optional. When non-nil, requests that include "json" in their
// formats list and a jsonSchema (or extract.schema) will trigger LLM-based
// structured extraction and the result will be placed in data.JSON.
func NewScraper(cfg types.ScraperConfig, httpFetcher *HTTPFetcher, llmConfig *types.LLMConfig) (*Scraper, *QuickCrawlError) {
	r, err := NewRenderer(cfg, httpFetcher)
	if err != nil {
		return nil, err
	}

	return &Scraper{
		renderer: r,
		llm:      newLLMExtractor(),
		llmCfg:   llmConfig,
		cfg:      cfg,
	}, nil
}

// Config returns the scraper's resolved configuration. Exposed so
// callers (and tests in other packages, e.g. internal/config) can
// inspect the values the constructor set without poking unexported
// fields.
func (s *Scraper) Config() types.ScraperConfig {
	return s.cfg
}

func (s *Scraper) Scrape(ctx context.Context, req *types.ScrapeRequest) (*types.ScrapeData, *QuickCrawlError) {
	start := time.Now()

	if err := validateRequest(req); err != nil {
		return nil, err
	}

	//If the renderMode field is not specified in the request, leave it nil
	//so the orchestrator inherits the server-wide default. If it is set,
	//the orchestrator honors it as the per-request override.
	mode := req.RenderMode

	//If the wait is not specified, default to 0 (no extra wait). The page is considered ready as soon as the browser's load event fires, which the renderer already waits for. Callers who want extra hydration time should pass an explicit waitFor.
	waitMs := resolveWaitMs(req.WaitFor, 0)

	//Call the fetcher
	result, err := s.renderer.FetchOrchestrator(ctx, req.URL, req.Headers, mode, waitMs)
	if err != nil {
		return nil, err
	}

	if warning := buildFetchWarning(result); warning != nil {
		if result.Warning != nil {
			combined := *result.Warning + "; " + *warning
			result.Warning = &combined
		} else {
			result.Warning = warning
		}
	}

	formats := req.Formats
	if len(formats) == 0 {
		formats = []types.OutputFormat{types.FormatMarkdown}
	}
	extractOpts := extractor.ExtractOptions{
		RawHTML:       result.HTML,
		RawBytes:      result.RawBytes,
		SourceURL:     result.URL,
		StatusCode:    int(result.StatusCode),
		RenderedMode:  &result.RenderedWith,
		Formats:       formats,
		IncludeTags:   req.IncludeTags,
		ExcludeTags:   req.ExcludeTags,
		CSSSelector:   req.CSSSelector,
		ExtractorType: extractor.ExtractorTrafilatura,
	}

	data := extractor.Extract(extractOpts)

	data.Metadata.SourceURL = result.URL
	data.Metadata.StatusCode = result.StatusCode
	data.Metadata.RenderedMode = &result.RenderedWith

	// LLM-based structured extraction runs only when the caller asks for the
	// "json" output format AND has supplied a schema (top-level or under
	// extract). Behavior matches /v1/scrape.
	if includesJSONFormat(formats) {
		if llmErr := s.runLLMExtraction(ctx, req, data); llmErr != nil {
			return nil, llmErr
		}
	}

	elapsed := time.Since(start)
	blockedStr := strings.Join(result.BlockedURLs, ", ")
	utils.Log.Info("scrape completed", "url", req.URL, "duration", elapsed, "status", result.StatusCode, "blocked", blockedStr)

	return data, nil
}

// runLLMExtraction resolves the effective schema/prompt/format, calls the LLM
// extractor, and stores the result in data.JSON. It returns an error only
// when a schema is provided without a configured LLM, or when the LLM call
// itself fails.
func (s *Scraper) runLLMExtraction(_ context.Context, req *types.ScrapeRequest, data *types.ScrapeData) *QuickCrawlError {
	schema, effectiveLLM := resolveLLMInputs(req, s.llmCfg)

	if len(schema) == 0 {
		return ErrInvalidRequest.New("Structured extraction (formats: json/extract) requires a 'jsonSchema' field. Provide a JSON Schema object.")
	}
	if effectiveLLM == nil {
		return ErrExtraction.New("json extraction requested but no LLM configured. Set [extraction.llm] in server config.")
	}
	if data.Markdown == nil {
		// No content to extract from; skip rather than fail.
		return nil
	}

	extracted, llmErr := s.llm.run(*data.Markdown, schema, effectiveLLM)
	if llmErr != nil {
		return llmErr
	}
	if len(extracted) > 0 {
		data.JSON = extracted
	}
	return nil
}

func validateRequest(req *types.ScrapeRequest) *QuickCrawlError {
	if req.URL == "" {
		return ErrInvalidRequest.New("url is required")
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		return ErrInvalidURL.New("url must start with http:// or https://")
	}
	return nil
}

// this method  checks if the renderMode is set, if not then just do http
// REMOVED: resolveRenderMode is no longer used — renderMode is passed as *types.RenderMode directly

func resolveWaitMs(waitFor *int64, defaultVal int64) int64 {
	if waitFor != nil && *waitFor > 0 {
		return *waitFor
	}
	return defaultVal
}

func resolveFormats(formats []string) []types.OutputFormat {
	if len(formats) == 0 {
		return []types.OutputFormat{types.FormatMarkdown}
	}
	out := make([]types.OutputFormat, len(formats))
	for i, f := range formats {
		out[i] = types.OutputFormat(f)
	}
	return out
}

func (s *Scraper) Close() error {
	return s.renderer.Close()
}

// Renderer exposes the underlying *Renderer so callers (crawl, map) can do
// their own raw fetch + extract without going through the full LLM-aware
// Scrape pipeline. This is the single seam between the scraper and the
// fetch-only paths that need HTML for link discovery or per-page markdown
// generation.
func (s *Scraper) Renderer() *Renderer {
	return s.renderer
}

// CheckHealth reports fetcher availability. Returns one entry for "http"
// (always true) and one for "chrome" (true when a browser WS URL is
// configured). The shape mirrors the legacy FallbackRenderer.CheckHealth
// so the Health handler can render it unchanged.
func (s *Scraper) CheckHealth() map[string]bool {
	return map[string]bool{
		"http":   true,
		"chrome": s.renderer != nil && s.renderer.allocCtx != nil,
	}
}

// JSRendererNames returns the names of configured JS renderers. The new
// model has a single backend (chromedp → Chrome), so the slice is either
// empty (no browser configured) or ["chrome"].
func (s *Scraper) JSRendererNames() []string {
	if s.renderer == nil || s.renderer.allocCtx == nil {
		return nil
	}
	return []string{"chrome"}
}

// BrowsersInfo returns information about configured browser instances.
// In the new model there is at most one Chrome (connected via the
// RemoteAllocator's WebSocket URL), so the slice has length 0 or 1.
func (s *Scraper) BrowsersInfo() []types.BrowserInfo {
	if s.renderer == nil || s.renderer.cfg.WSURL == "" {
		return nil
	}
	return []types.BrowserInfo{
		{
			Name:  "chrome",
			WSURL: s.renderer.cfg.WSURL,
		},
	}
}

// FetchHTML is a thin wrapper around the underlying *Renderer that returns
// a *types.FetchResult. It is used by the crawl and map pipelines which
// need a types.FetchResult shape to feed into the shared extractor.
//
// mode=nil      → inherit server default
// mode=auto     → HTTP first, then check and escalate to browser if needed
// mode=browser  → chromedp-based browser fetch (full JavaScript).
// mode=http     → HTTP-only fetch via the shared *renderer.HTTPFetcher.
//
func (s *Scraper) FetchHTML(
	ctx context.Context,
	rawURL string,
	headers map[string]string,
	mode *types.RenderMode,
	waitMs int64,
) (*types.FetchResult, *QuickCrawlError) {
	result, err := s.renderer.FetchOrchestrator(ctx, rawURL, headers, mode, waitMs)
	if err != nil {
		return nil, err
	}
	return toTypesFetchResult(result), nil
}

func (s *Scraper) FetchForBrand(ctx context.Context, rawURL string) (*FetchResult, *QuickCrawlError) {
	return s.renderer.FetchOrchestrator(ctx, rawURL, nil, nil, 0)
}

// FetchBrand returns the rendered HTML for a URL plus, when a browser is
// available, the raw JSON design-token payload (fonts + styleguide) extracted
// from the live DOM.
//
// When no browser is configured the call falls back to HTTP fetching: HTML is
// still returned, but Tokens is empty. This lets the brand handler always
// receive a usable HTML payload even on lightweight deployments, while
// surfacing the rich tokens when a browser is reachable.
func (s *Scraper) FetchBrand(ctx context.Context, rawURL string) (*BrandDesignTokens, *QuickCrawlError) {
	if s.renderer.allocCtx != nil {
		tokens, err := s.renderer.fetchBrandDesignTokens(ctx, rawURL)
		if err == nil {
			return tokens, nil
		}
		// Browser failed (timeout, navigation error, etc.). Fall through
		// to HTTP so the caller still gets usable HTML.
	}

	result, ferr := s.renderer.FetchOrchestrator(ctx, rawURL, nil, nil, 0)
	if ferr != nil {
		return nil, ferr
	}
	return &BrandDesignTokens{HTML: result.HTML}, nil
}
