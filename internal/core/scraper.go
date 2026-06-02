package core

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

type Scraper struct {
	renderer  *Renderer
	extractor *Extractor
	llm       *llmExtractor
	llmCfg    *types.LLMConfig
	cfg       Config
}

type ScrapeRequest struct {
	URL                 string
	Formats             []string
	RenderJS            *bool
	WaitFor             *int64
	IncludeTags         []string
	ExcludeTags         []string
	JSONSchema          *json.RawMessage
	Headers             map[string]string
	CSSSelector         *string
	Extract             *types.ExtractOptions
	LLMExtractionPrompt *string
	LLMResponseFormat   *string
	Browser             *string
}

// NewScraper builds a Scraper that delegates HTTP fetching to the shared
// *renderer.HTTPFetcher (so /v1/scrape and /v1/scrape-core share the same
// HTTP path) and uses chromedp for browser rendering.
//
// llmConfig is optional. When non-nil, requests that include "json" in their
// formats list and a jsonSchema (or extract.schema) will trigger LLM-based
// structured extraction and the result will be placed in data.JSON.
func NewScraper(cfg Config, httpFetcher *renderer.HTTPFetcher, llmConfig *types.LLMConfig) (*Scraper, *QuickCrawlError) {
	r, err := NewRenderer(cfg, httpFetcher)
	if err != nil {
		return nil, err
	}

	return &Scraper{
		renderer:  r,
		extractor: NewExtractor(),
		llm:       newLLMExtractor(),
		llmCfg:    llmConfig,
		cfg:       cfg,
	}, nil
}

func (s *Scraper) Scrape(ctx context.Context, req *ScrapeRequest) (*ScrapeData, *QuickCrawlError) {
	start := time.Now()

	if err := validateRequest(req); err != nil {
		return nil, err
	}

	//If the renderJS field is not specified in the request, default to false (no JavaScript rendering). If waitFor is not specified, default to 0 (no extra wait beyond what the load event signals).
	renderJS := resolveRenderJS(req.RenderJS, false)

	//If the wait is not specified, default to 0 (no extra wait). The page is considered ready as soon as the browser's load event fires, which the renderer already waits for. Callers who want extra hydration time should pass an explicit waitFor.
	waitMs := resolveWaitMs(req.WaitFor, 0)

	//Call the fetcher
	result, err := s.renderer.FetchOrchestrator(ctx, req.URL, req.Headers, renderJS, waitMs)
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

	formats := resolveFormats(req.Formats)
	extractOpts := ExtractOptions{
		RawHTML:      result.HTML,
		RawBytes:     result.RawBytes,
		SourceURL:    result.URL,
		StatusCode:   int(result.StatusCode),
		RenderedMode: &result.RenderedWith,
		TimeTaken:    result.TimeTakenMs,
		Formats:      formats,
		IncludeTags:  req.IncludeTags,
		ExcludeTags:  req.ExcludeTags,
		CSSSelector:  req.CSSSelector,
	}

	data := s.extractor.Extract(extractOpts)

	data.Metadata.SourceURL = result.URL
	data.Metadata.StatusCode = result.StatusCode
	data.Metadata.RenderedMode = &result.RenderedWith
	data.Metadata.TimeTaken = result.TimeTakenMs

	// LLM-based structured extraction runs only when the caller asks for the
	// "json" output format AND has supplied a schema (top-level or under
	// extract). Behavior matches /v1/scrape.
	if includesJSONFormat(formats) {
		if llmErr := s.runLLMExtraction(ctx, req, data); llmErr != nil {
			return nil, llmErr
		}
	}

	elapsed := time.Since(start)
	log.Printf("[core] scrape completed: url=%s duration=%v status=%d", req.URL, elapsed, result.StatusCode)

	return data, nil
}

// runLLMExtraction resolves the effective schema/prompt/format, calls the LLM
// extractor, and stores the result in data.JSON. It returns an error only
// when a schema is provided without a configured LLM, or when the LLM call
// itself fails.
func (s *Scraper) runLLMExtraction(_ context.Context, req *ScrapeRequest, data *ScrapeData) *QuickCrawlError {
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

func validateRequest(req *ScrapeRequest) *QuickCrawlError {
	if req.URL == "" {
		return ErrInvalidRequest.New("url is required")
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		return ErrInvalidURL.New("url must start with http:// or https://")
	}
	return nil
}

// this method  checks if the renderJS is enabled if not then just do http
func resolveRenderJS(reqRenderJS *bool, defaultVal bool) bool {
	if reqRenderJS != nil {
		return *reqRenderJS
	}
	return defaultVal
}

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
		"http":  true,
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
// renderJS=false → HTTP-only fetch via the shared *renderer.HTTPFetcher.
// renderJS=true  → chromedp-based browser fetch (full JavaScript).
//
// The preferredBrowser parameter is accepted for backwards compatibility
// with the legacy FallbackRenderer.Fetch signature. The new model has a
// single browser backend, so this argument is ignored.
func (s *Scraper) FetchHTML(
	ctx context.Context,
	rawURL string,
	headers map[string]string,
	renderJS bool,
	waitMs int64,
	preferredBrowser *string,
) (*types.FetchResult, *QuickCrawlError) {
	result, err := s.renderer.FetchOrchestrator(ctx, rawURL, headers, renderJS, waitMs)
	if err != nil {
		return nil, err
	}
	return toTypesFetchResult(result), nil
}
