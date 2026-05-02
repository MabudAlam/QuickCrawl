package quickcrawl

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/search"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	state  *api.AppState
	config *types.AppConfig
}

func NewServer(state *api.AppState, cfg *types.AppConfig) *Server {
	return &Server{
		state:  state,
		config: cfg,
	}
}

type ScrapeArgs struct {
	URL             string               `json:"url"`
	Formats         []string             `json:"formats,omitempty"`
	OnlyMainContent *bool                `json:"onlyMainContent,omitempty"`
	RenderJS        *bool                `json:"renderJs,omitempty"`
	WaitFor         *int64               `json:"waitFor,omitempty"`
	IncludeTags     []string             `json:"includeTags,omitempty"`
	ExcludeTags     []string             `json:"excludeTags,omitempty"`
	XPath           *string              `json:"xpath,omitempty"`
	CSSSelector     *string              `json:"cssSelector,omitempty"`
	Query           *string              `json:"query,omitempty"`
	TopK            *int                 `json:"topK,omitempty"`
	ChunkStrategy   *types.ChunkStrategy `json:"chunkStrategy,omitempty"`
	FilterMode      *types.FilterMode    `json:"filterMode,omitempty"`
	Renderer        *string              `json:"renderer,omitempty"`
	Browser         *string              `json:"browser,omitempty"`
}

type SearchArgs struct {
	Query      string   `json:"query"`
	Region     string   `json:"region,omitempty"`
	Safesearch string   `json:"safesearch,omitempty"`
	Timelimit  string   `json:"timelimit,omitempty"`
	RenderJS   bool     `json:"renderJs,omitempty"`
	Formats    []string `json:"formats,omitempty"`
}

func (s *Server) HandleScrape(ctx context.Context, req *mcp.CallToolRequest, args ScrapeArgs) (*mcp.CallToolResult, any, error) {
	if args.URL == "" {
		return errorResult("url is required"), nil, nil
	}

	if _, urlErr := types.ValidateURL(args.URL); urlErr != nil {
		return errorResult(fmt.Sprintf("invalid URL: %v", urlErr)), nil, nil
	}

	preferredRenderer, normalizeErr := normalizePinnedRenderer(args.Renderer, args.Browser)
	if normalizeErr != nil {
		return errorResult(normalizeErr.Error()), nil, nil
	}
	if validateErr := validatePinnedRenderer(s.state.Renderer, preferredRenderer, args.RenderJS); validateErr != nil {
		return errorResult(validateErr.Error()), nil, nil
	}

	formats := []types.OutputFormat{types.FormatMarkdown}
	if len(args.Formats) > 0 {
		formats = make([]types.OutputFormat, len(args.Formats))
		for i, f := range args.Formats {
			formats[i] = types.OutputFormat(f)
		}
	}

	onlyMain := true
	if args.OnlyMainContent != nil {
		onlyMain = *args.OnlyMainContent
	}

	scrapeReq := &types.ScrapeRequest{
		URL:             args.URL,
		Formats:         formats,
		OnlyMainContent: onlyMain,
		RenderJS:        args.RenderJS,
		WaitFor:         args.WaitFor,
		IncludeTags:     args.IncludeTags,
		ExcludeTags:     args.ExcludeTags,
		XPath:           args.XPath,
		CSSSelector:     args.CSSSelector,
		Query:           args.Query,
		TopK:            args.TopK,
		ChunkStrategy:   args.ChunkStrategy,
		FilterMode:      args.FilterMode,
		Browser:         preferredRenderer,
	}
	if scrapeReq.RenderJS == nil && preferredRenderer != nil {
		forceJS := true
		scrapeReq.RenderJS = &forceJS
	}
	if scrapeReq.Headers == nil {
		scrapeReq.Headers = make(map[string]string)
	}

	data, scrapeErr := crawler.ScrapeURL(
		scrapeReq,
		s.state.Renderer,
		s.config.Extraction.LLM,
		s.config.Crawler.UserAgent,
		s.config.Crawler.Stealth.Enabled,
		s.config.Renderer.RenderJSDefault,
	)

	if scrapeErr != nil {
		return errorResult(fmt.Sprintf("scrape error: %v", scrapeErr)), nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatScrapeData(data)},
		},
	}, nil, nil
}

type CrawlArgs struct {
	URL             string                `json:"url"`
	MaxDepth        *uint32               `json:"maxDepth,omitempty"`
	MaxPages        *uint32               `json:"maxPages,omitempty"`
	Formats         []string              `json:"formats,omitempty"`
	OnlyMainContent *bool                 `json:"onlyMainContent,omitempty"`
	RenderJS        *bool                 `json:"renderJs,omitempty"`
	WaitFor         *int64                `json:"waitFor,omitempty"`
	Query           *string               `json:"query,omitempty"`
	TopK            *int                  `json:"topK,omitempty"`
	ChunkStrategy   *types.ChunkStrategy  `json:"chunkStrategy,omitempty"`
	FilterMode      *types.FilterMode     `json:"filterMode,omitempty"`
	Extract         *types.ExtractOptions `json:"extract,omitempty"`
	Renderer        *string               `json:"renderer,omitempty"`
	Browser         *string               `json:"browser,omitempty"`
}

func (s *Server) HandleCrawl(ctx context.Context, req *mcp.CallToolRequest, args CrawlArgs) (*mcp.CallToolResult, any, error) {
	if args.URL == "" {
		return errorResult("url is required"), nil, nil
	}

	if _, urlErr := types.ValidateURL(args.URL); urlErr != nil {
		return errorResult(fmt.Sprintf("invalid URL: %v", urlErr)), nil, nil
	}

	preferredRenderer, normalizeErr := normalizePinnedRenderer(args.Renderer, args.Browser)
	if normalizeErr != nil {
		return errorResult(normalizeErr.Error()), nil, nil
	}
	if validateErr := validatePinnedRenderer(s.state.Renderer, preferredRenderer, args.RenderJS); validateErr != nil {
		return errorResult(validateErr.Error()), nil, nil
	}

	maxDepth := uint32(s.config.Crawler.DefaultMaxDepth)
	if args.MaxDepth != nil {
		maxDepth = *args.MaxDepth
	}
	maxPages := uint32(s.config.Crawler.DefaultMaxPages)
	if args.MaxPages != nil {
		maxPages = *args.MaxPages
	}

	formats := []types.OutputFormat{types.FormatMarkdown}
	if len(args.Formats) > 0 {
		formats = make([]types.OutputFormat, len(args.Formats))
		for i, f := range args.Formats {
			formats[i] = types.OutputFormat(f)
		}
	}

	onlyMain := true
	if args.OnlyMainContent != nil {
		onlyMain = *args.OnlyMainContent
	}

	scrapeReq := &types.ScrapeRequest{
		Formats:         formats,
		OnlyMainContent: onlyMain,
		Query:           args.Query,
		TopK:            args.TopK,
		ChunkStrategy:   args.ChunkStrategy,
		FilterMode:      args.FilterMode,
	}

	crawlReq := &types.CrawlRequest{
		URL:             args.URL,
		MaxDepth:        &maxDepth,
		MaxPages:        &maxPages,
		Formats:         scrapeReq.Formats,
		OnlyMainContent: scrapeReq.OnlyMainContent,
		RenderJS:        args.RenderJS,
		WaitFor:         args.WaitFor,
		Browser:         preferredRenderer,
		Query:           args.Query,
		TopK:            args.TopK,
		ChunkStrategy:   args.ChunkStrategy,
		FilterMode:      args.FilterMode,
		Extract:         args.Extract,
	}
	if crawlReq.RenderJS == nil && preferredRenderer != nil {
		forceJS := true
		crawlReq.RenderJS = &forceJS
	}

	id := s.state.StartCrawlJob(crawlReq)

	stateCh := make(chan types.CrawlState, 100)
	go func() {
		for state := range stateCh {
			s.state.UpdateCrawlJob(id, state)
		}
	}()

	go func() {
		defer close(stateCh)
		opts := crawler.CrawlOptions{
			ID:                id,
			Req:               crawlReq,
			Renderer:          s.state.Renderer,
			MaxConcurrency:    s.config.Crawler.MaxConcurrency,
			RespectRobots:     s.config.Crawler.RespectRobotsTxt,
			RequestsPerSecond: s.config.Crawler.RequestsPerSecond,
			UserAgent:         s.config.Crawler.UserAgent,
			StateCh:           stateCh,
			LLMConfig:         s.config.Extraction.LLM,
			Proxy:             s.config.Crawler.Proxy,
			JitterFactor:      s.config.Crawler.Stealth.JitterFactor,
		}
		crawler.RunCrawl(opts)
	}()

	result := map[string]interface{}{
		"id":      id,
		"status":  "scraping",
		"message": "Crawl job started. Use quickcrawl_check_crawl_status with the id to check progress.",
	}

	data, _ := json.Marshal(result)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

type CrawlStatusArgs struct {
	ID string `json:"id"`
}

func (s *Server) HandleCrawlStatus(ctx context.Context, req *mcp.CallToolRequest, args CrawlStatusArgs) (*mcp.CallToolResult, any, error) {
	if args.ID == "" {
		return errorResult("id is required"), nil, nil
	}

	state := s.state.GetCrawlJob(args.ID)
	if state == nil {
		return errorResult("crawl job not found"), nil, nil
	}

	data, _ := json.Marshal(state)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

type MapArgs struct {
	URL        string `json:"url"`
	MaxDepth   *int   `json:"maxDepth,omitempty"`
	UseSitemap *bool  `json:"useSitemap,omitempty"`
	Timeout    *int   `json:"timeout,omitempty"`
}

func (s *Server) HandleMap(ctx context.Context, req *mcp.CallToolRequest, args MapArgs) (*mcp.CallToolResult, any, error) {
	if args.URL == "" {
		return errorResult("url is required"), nil, nil
	}

	if _, urlErr := types.ValidateURL(args.URL); urlErr != nil {
		return errorResult(fmt.Sprintf("invalid URL: %v", urlErr)), nil, nil
	}

	useSitemap := true
	if args.UseSitemap != nil {
		useSitemap = *args.UseSitemap
	}

	maxDepth := uint32(s.config.Crawler.DefaultMaxDepth)
	if args.MaxDepth != nil {
		maxDepth = uint32(*args.MaxDepth)
	}

	timeout := 30 * time.Second
	if args.Timeout != nil && *args.Timeout > 0 {
		timeout = time.Duration(*args.Timeout) * time.Millisecond
	}
	crawlCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	urls, discoverErr := crawler.DiscoverUrls(
		args.URL,
		maxDepth,
		useSitemap,
		s.state.Renderer,
		s.config.Crawler.MaxConcurrency,
		s.config.Crawler.RequestsPerSecond,
		s.config.Crawler.UserAgent,
		s.config.Crawler.Proxy,
		crawlCtx,
	)

	if discoverErr != nil {
		return errorResult(fmt.Sprintf("map error: %v", discoverErr.Message)), nil, nil
	}

	result := map[string]interface{}{
		"links": urls,
		"count": len(urls),
	}

	data, _ := json.Marshal(result)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func (s *Server) HandleSearch(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Query) == "" {
		return errorResult("query is required"), nil, nil
	}

	safesearch := "moderate"
	if args.Safesearch != "" {
		safesearch = args.Safesearch
	}

	region := args.Region
	if region == "" {
		region = "us-en"
	}

	engine := search.New()
	results, searchErr := engine.Search(args.Query, region, safesearch, args.Timelimit)
	if searchErr != nil {
		return errorResult(fmt.Sprintf("search failed: %v", searchErr)), nil, nil
	}

	if len(results) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: `{"results": [], "count": 0}`},
			},
		}, nil, nil
	}

	rend := s.state.Renderer
	if rend == nil {
		return errorResult("renderer is not initialized"), nil, nil
	}
	llmConfig := s.config.Extraction.LLM

	defaultFormats := []types.OutputFormat{types.FormatMarkdown}
	formats := defaultFormats
	if len(args.Formats) > 0 {
		formats = make([]types.OutputFormat, len(args.Formats))
		for i, f := range args.Formats {
			formats[i] = types.OutputFormat(f)
		}
	}

	maxWorkers := 10
	if len(results) < maxWorkers {
		maxWorkers = len(results)
	}
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	resultMap := make(map[int]types.SearchResult, len(results))

	for i, r := range results {
		wg.Add(1)
		go func(index int, result search.TextResult) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("search MCP: panic recovered while scraping %s: %v", result.Href, rec)
					mu.Lock()
					resultMap[index] = types.SearchResult{
						Title:       result.Title,
						Description: result.Body,
						URL:         result.Href,
					}
					mu.Unlock()
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if _, urlErr := types.ValidateURL(result.Href); urlErr != nil {
				log.Printf("search MCP: skipping invalid URL %s: %v", result.Href, urlErr)
				mu.Lock()
				resultMap[index] = types.SearchResult{
					Title:       result.Title,
					Description: result.Body,
					URL:         result.Href,
				}
				mu.Unlock()
				return
			}

			renderJS := args.RenderJS
			scrapeReq := &types.ScrapeRequest{
				URL:             result.Href,
				Formats:         formats,
				OnlyMainContent: true,
				RenderJS:        &renderJS,
			}

			searchResult := types.SearchResult{
				Title:       result.Title,
				Description: result.Body,
				URL:         result.Href,
			}

			data, scrapeErr := crawler.ScrapeURL(
				scrapeReq,
				rend,
				llmConfig,
				s.config.Crawler.UserAgent,
				s.config.Crawler.Stealth.Enabled,
				&renderJS,
			)

			if scrapeErr != nil {
				log.Printf("search MCP: failed to scrape %s: %v", result.Href, scrapeErr)
			} else if data != nil {
				if data.Markdown != nil {
					searchResult.Markdown = data.Markdown
				}
				if data.HTML != nil {
					searchResult.HTML = data.HTML
				}
				if data.PlainText != nil {
					searchResult.PlainText = data.PlainText
				}
				if data.Links != nil {
					searchResult.Links = data.Links
				}
				if len(data.JSON) > 0 {
					searchResult.RawJSON = data.JSON
				}
				if data.Metadata.SourceURL != "" {
					searchResult.URL = data.Metadata.SourceURL
				}
			}

			mu.Lock()
			resultMap[index] = searchResult
			mu.Unlock()
		}(i, r)
	}

	wg.Wait()

	orderedResults := make([]types.SearchResult, 0, len(results))
	for i := range results {
		orderedResults = append(orderedResults, resultMap[i])
	}

	response := map[string]interface{}{
		"results": orderedResults,
		"count":   len(orderedResults),
	}

	responseData, _ := json.Marshal(response)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(responseData)},
		},
	}, nil, nil
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(`{"error": %s}`, jsonString(msg))},
		},
		IsError: true,
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func formatScrapeData(data *types.ScrapeData) string {
	if data == nil {
		return `{"error": "no data"}`
	}

	result := make(map[string]interface{})

	if data.Markdown != nil {
		result["markdown"] = *data.Markdown
	}
	if data.HTML != nil {
		result["html"] = *data.HTML
	}
	if data.PlainText != nil {
		result["plainText"] = *data.PlainText
	}
	if data.Links != nil {
		result["links"] = data.Links
	}
	if len(data.JSON) > 0 {
		result["json"] = json.RawMessage(data.JSON)
	}
	if len(data.Chunks) > 0 {
		result["chunks"] = data.Chunks
	}

	result["metadata"] = data.Metadata

	b, _ := json.Marshal(result)
	return string(b)
}

func AddTools(server *mcp.Server, s *Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "scrape",
		Description: "Scrape a single URL and return its content in various formats (markdown, html, links, json)",
		InputSchema: scrapeInputSchema(),
	}, s.HandleScrape)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crawl",
		Description: "Start an async BFS crawl of a website, discovering and scraping multiple pages",
		InputSchema: crawlInputSchema(),
	}, s.HandleCrawl)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_crawl_status",
		Description: "Check the status of a crawl job by its ID",
	}, s.HandleCrawlStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "site_map",
		Description: "Discover all URLs on a website without scraping the content",
	}, s.HandleMap)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search DuckDuckGo and scrape results in parallel with 10 concurrent workers",
		InputSchema: searchInputSchema(),
	}, s.HandleSearch)

	log.Printf("MCP tools registered: quickcrawl_scrape, quickcrawl_crawl, quickcrawl_check_crawl_status, quickcrawl_map, quickcrawl_search")
}

func normalizePinnedRenderer(rendererName, browserName *string) (*string, error) {
	if rendererName != nil && browserName != nil && *rendererName != "" && *browserName != "" && *rendererName != *browserName {
		return nil, fmt.Errorf("renderer and browser must match when both are provided")
	}

	name := firstNonEmpty(rendererName, browserName)
	if name == nil {
		return nil, nil
	}

	switch *name {
	case "auto":
		return nil, nil
	case "lightpanda", "chrome":
		return name, nil
	default:
		return nil, fmt.Errorf("invalid renderer %q; valid values: auto, lightpanda, chrome", *name)
	}
}

func validatePinnedRenderer(rend interface{ JSRendererNames() []string }, preferredRenderer *string, renderJS *bool) error {
	if preferredRenderer == nil || *preferredRenderer == "" {
		return nil
	}
	if renderJS != nil && !*renderJS {
		return nil
	}

	for _, name := range rend.JSRendererNames() {
		if name == *preferredRenderer {
			return nil
		}
	}

	return fmt.Errorf("renderer %q not available; configured renderers: [%s]", *preferredRenderer, strings.Join(rend.JSRendererNames(), ", "))
}

func firstNonEmpty(values ...*string) *string {
	for _, v := range values {
		if v != nil && *v != "" {
			return v
		}
	}
	return nil
}

func scrapeInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to scrape",
			},
			"formats": map[string]any{
				"type":        "array",
				"description": "Output formats such as markdown, html, links, or json",
				"items": map[string]any{
					"type": "string",
					"enum": []string{"markdown", "html", "links", "json"},
				},
			},
			"onlyMainContent": map[string]any{
				"type":        "boolean",
				"description": "Extract only the main content, removing nav/footer/etc",
			},
			"renderJs": map[string]any{
				"type":        "boolean",
				"description": "Render JavaScript before extracting (true = force JS, false = HTTP only, omit = auto-detect/default)",
			},
			"waitFor": map[string]any{
				"type":        "integer",
				"description": "Milliseconds to wait after JS rendering for late content or XHRs",
			},
			"renderer": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "lightpanda", "chrome"},
				"description": "Pin this request to a specific renderer. auto uses the configured fallback chain. Other values hard-pin to a single renderer with no fallback. Pinning a non-auto value implies renderJs:true unless renderJs:false is set explicitly.",
			},
			"includeTags": map[string]any{
				"type":        "array",
				"description": "CSS selectors to include",
				"items":       map[string]any{"type": "string"},
			},
			"excludeTags": map[string]any{
				"type":        "array",
				"description": "CSS selectors to exclude",
				"items":       map[string]any{"type": "string"},
			},
			"cssSelector": map[string]any{
				"type":        "string",
				"description": "Extract content from a specific CSS selector",
			},
			"xpath": map[string]any{
				"type":        "string",
				"description": "Extract content from a specific XPath expression",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Query for chunk filtering",
			},
			"topK": map[string]any{
				"type":        "integer",
				"description": "Return top K filtered chunks",
			},
		},
		"required": []string{"url"},
	}
}

func crawlInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The starting URL to crawl",
			},
			"maxDepth": map[string]any{
				"type":        "integer",
				"description": "Maximum crawl depth",
			},
			"maxPages": map[string]any{
				"type":        "integer",
				"description": "Maximum number of pages to crawl",
			},
			"renderJs": map[string]any{
				"type":        "boolean",
				"description": "Render JavaScript on every crawled page (true = force JS, false = HTTP only, omit = auto-detect/default)",
			},
			"waitFor": map[string]any{
				"type":        "integer",
				"description": "Milliseconds to wait after JS rendering on each page",
			},
			"renderer": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "lightpanda", "chrome"},
				"description": "Pin every crawled page to a specific renderer. auto uses the configured fallback chain. Other values hard-pin to a single renderer with no fallback. Pinning a non-auto value implies renderJs:true unless renderJs:false is set explicitly.",
			},
			"formats": map[string]any{
				"type":        "array",
				"description": "Output formats for each crawled page",
				"items": map[string]any{
					"type": "string",
					"enum": []string{"markdown", "html", "links", "json"},
				},
			},
			"onlyMainContent": map[string]any{
				"type":        "boolean",
				"description": "Extract only the main content for each crawled page",
			},
		},
		"required": []string{"url"},
	}
}

func searchInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query",
			},
			"region": map[string]any{
				"type":        "string",
				"description": "Region code (e.g., us-en)",
			},
			"safesearch": map[string]any{
				"type":        "string",
				"description": "SafeSearch mode: moderate, strict, off",
			},
			"renderJs": map[string]any{
				"type":        "boolean",
				"description": "Enable JavaScript rendering",
			},
			"formats": map[string]any{
				"type":        "array",
				"description": "Output formats for each result",
				"items": map[string]any{
					"type": "string",
					"enum": []string{"markdown", "html", "links", "json"},
				},
			},
		},
		"required": []string{"query"},
	}
}
