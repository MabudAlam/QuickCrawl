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
	OnlyMain            bool
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

	//If the renderJS field is not specified in the request, default to false (no JavaScript rendering). If waitFor is not specified, default to 2000 milliseconds (2 seconds) to wait after page load before scraping.
	renderJS := resolveRenderJS(req.RenderJS, false)

	//if the wait is not specified, default to 2000 milliseconds (2 seconds) to wait after page load before scraping.
	waitMs := resolveWaitMs(req.WaitFor, 2000)

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
		OnlyMain:     req.OnlyMain,
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
