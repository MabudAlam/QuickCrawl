package quickcrawl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/search"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
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
	URL         string   `json:"url"`
	Formats     []string `json:"formats,omitempty"`
	RenderJS    *bool    `json:"renderJs,omitempty"`
	WaitFor     *int64   `json:"waitFor,omitempty"`
	IncludeTags []string `json:"includeTags,omitempty"`
	ExcludeTags []string `json:"excludeTags,omitempty"`
	CSSSelector *string  `json:"cssSelector,omitempty"`
	// Renderer is deprecated: the new scraper uses chromedp only.
	Renderer *string `json:"renderer,omitempty"`
	// Browser is deprecated: the new scraper uses chromedp only.
	Browser *string `json:"browser,omitempty"`
}

type SearchArgs struct {
	Query      string   `json:"query"`
	Region     string   `json:"region,omitempty"`
	Safesearch string   `json:"safesearch,omitempty"`
	Timelimit  string   `json:"timelimit,omitempty"`
	RenderJS   *bool    `json:"renderJs,omitempty"`
	Formats    []string `json:"formats,omitempty"`
	Scrape     bool     `json:"scrape,omitempty"`
}

func (s *Server) HandleScrape(ctx context.Context, req *mcp.CallToolRequest, args ScrapeArgs) (*mcp.CallToolResult, any, error) {
	if args.URL == "" {
		return errorResult("url is required"), nil, nil
	}

	if _, urlErr := types.ValidateURL(args.URL); urlErr != nil {
		return errorResult(fmt.Sprintf("invalid URL: %v", urlErr)), nil, nil
	}

	// Log deprecation warning if the caller pinned a renderer/browser.
	if (args.Renderer != nil && *args.Renderer != "" && *args.Renderer != "auto") ||
		(args.Browser != nil && *args.Browser != "" && *args.Browser != "auto") {
		utils.Log.Warn("deprecated renderer/browser fields ignored for scrape", "renderer", strVal(args.Renderer), "browser", strVal(args.Browser))
	}

	scraper := s.state.CoreScraper
	if scraper == nil {
		return errorResult("scraper is not initialized"), nil, nil
	}

	coreReq := &core.ScrapeRequest{
		URL:          args.URL,
		Formats:      args.Formats,
		RenderJS:     args.RenderJS,
		WaitFor:      args.WaitFor,
		IncludeTags:  args.IncludeTags,
		ExcludeTags:  args.ExcludeTags,
		CSSSelector:  args.CSSSelector,
	}
	if coreReq.RenderJS != nil && *coreReq.RenderJS && coreReq.WaitFor == nil {
		defaultWait := int64(2000)
		coreReq.WaitFor = &defaultWait
	}
	if coreReq.Formats == nil {
		coreReq.Formats = []string{"markdown"}
	}

	// Check robots.txt if respect_robots_txt is enabled
	if s.config.Crawler.RespectRobotsTxt {
		if err := crawler.CheckRobotsTxt(args.URL, s.config.Crawler.UserAgent); err != nil {
			return errorResult(err.Message), nil, nil
		}
	}

	data, scrapeErr := scraper.Scrape(ctx, coreReq)
	if scrapeErr != nil {
		return errorResult(fmt.Sprintf("scrape error: %v", scrapeErr)), nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: formatCoreScrapeData(data)},
		},
	}, nil, nil
}

type CrawlArgs struct {
	URL      string   `json:"url"`
	MaxDepth *uint32  `json:"maxDepth,omitempty"`
	MaxPages *uint32  `json:"maxPages,omitempty"`
	Formats  []string `json:"formats,omitempty"`
	RenderJS *bool    `json:"renderJs,omitempty"`
	WaitFor  *int64   `json:"waitFor,omitempty"`
	// Renderer/Browser are deprecated: the new scraper uses chromedp only.
	Renderer *string `json:"renderer,omitempty"`
	Browser  *string `json:"browser,omitempty"`
}

func (s *Server) HandleCrawl(ctx context.Context, req *mcp.CallToolRequest, args CrawlArgs) (*mcp.CallToolResult, any, error) {
	if args.URL == "" {
		return errorResult("url is required"), nil, nil
	}

	if _, urlErr := types.ValidateURL(args.URL); urlErr != nil {
		return errorResult(fmt.Sprintf("invalid URL: %v", urlErr)), nil, nil
	}

	if (args.Renderer != nil && *args.Renderer != "" && *args.Renderer != "auto") ||
		(args.Browser != nil && *args.Browser != "" && *args.Browser != "auto") {
		utils.Log.Warn("deprecated renderer/browser fields ignored for crawl", "renderer", strVal(args.Renderer), "browser", strVal(args.Browser))
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

	for _, f := range formats {
		if f == types.FormatJson {
			return errorResult("'json' format is not supported on quickcrawl_crawl. Use quickcrawl_scrape for LLM-based JSON extraction."), nil, nil
		}
	}

	crawlReq := &types.CrawlRequest{
		URL:      args.URL,
		MaxDepth: &maxDepth,
		MaxPages: &maxPages,
		Formats:  formats,
		RenderJS: args.RenderJS,
		WaitFor:  args.WaitFor,
	}

	scraper := s.state.CoreScraper
	if scraper == nil {
		return errorResult("scraper is not initialized"), nil, nil
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
			Scraper:           scraper,
			MaxConcurrency:    s.config.Crawler.MaxConcurrency,
			RespectRobots:     s.config.Crawler.RespectRobotsTxt,
			RequestsPerSecond: s.config.Crawler.RequestsPerSecond,
			UserAgent:         s.config.Crawler.UserAgent,
			StateCh:           stateCh,
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

	scraper := s.state.CoreScraper
	if scraper == nil {
		return errorResult("scraper is not initialized"), nil, nil
	}

	urls, discoverErr := crawler.DiscoverUrls(
		args.URL,
		maxDepth,
		useSitemap,
		scraper,
		s.config.Crawler.RespectRobotsTxt,
		s.config.Crawler.MaxConcurrency,
		s.config.Crawler.RequestsPerSecond,
		s.config.Crawler.UserAgent,
		crawlCtx,
	)

	if discoverErr != nil {
		if discoverErr.Code == types.CodeForbidden {
			return errorResult("access denied by robots.txt"), nil, nil
		}
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

	scraper := s.state.CoreScraper
	if scraper == nil {
		return errorResult("scraper is not initialized"), nil, nil
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

	if !args.Scrape {
		orderedResults := make([]types.SearchResult, 0, len(results))
		for _, r := range results {
			orderedResults = append(orderedResults, types.SearchResult{
				Title:       r.Title,
				Description: r.Body,
				URL:         r.Href,
			})
		}
		response := map[string]interface{}{
			"results": orderedResults,
			"count":   len(orderedResults),
		}
		data, _ := json.Marshal(response)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(data)},
			},
		}, nil, nil
	}

	formats := []string{"markdown"}
	if len(args.Formats) > 0 {
		formats = args.Formats
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
					utils.Log.Warn("search MCP panic recovered while scraping", "url", result.Href, "error", rec)
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
				utils.Log.Warn("search MCP skipping invalid URL", "url", result.Href, "error", urlErr)
				mu.Lock()
				resultMap[index] = types.SearchResult{
					Title:       result.Title,
					Description: result.Body,
					URL:         result.Href,
				}
				mu.Unlock()
				return
			}

			scrapeReq := &core.ScrapeRequest{
				URL:      result.Href,
				Formats:  formats,
				RenderJS: args.RenderJS,
			}

			searchResult := types.SearchResult{
				Title:       result.Title,
				Description: result.Body,
				URL:         result.Href,
			}

			data, scrapeErr := scraper.Scrape(ctx, scrapeReq)
			if scrapeErr != nil {
				utils.Log.Warn("search MCP failed to scrape", "url", result.Href, "error", scrapeErr)
			} else if data != nil {
				if data.Markdown != nil {
					s := *data.Markdown
					searchResult.Markdown = &s
				}
				if data.HTML != nil {
					s := *data.HTML
					searchResult.HTML = &s
				}
				if data.PlainText != nil {
					s := *data.PlainText
					searchResult.PlainText = &s
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

// formatCoreScrapeData serializes a *core.ScrapeData into a JSON string
// suitable for the MCP text content. Field selection mirrors the
// legacy formatScrapeData helper so existing MCP clients see the same
// payload shape.
func formatCoreScrapeData(data *core.ScrapeData) string {
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

	utils.Log.Info("MCP tools registered", "tools", "quickcrawl_scrape, quickcrawl_crawl, quickcrawl_check_crawl_status, quickcrawl_map, quickcrawl_search")
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
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
			"renderJs": map[string]any{
				"type":        "boolean",
				"description": "Render JavaScript before extracting (true = force JS, false = HTTP only, omit = auto-detect/default)",
			},
			"waitFor": map[string]any{
				"type":        "integer",
				"description": "Milliseconds to wait after JS rendering for late content or XHRs",
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
			"formats": map[string]any{
				"type":        "array",
				"description": "Output formats for each crawled page",
				"items": map[string]any{
					"type": "string",
					"enum": []string{"markdown", "html", "links", "json"},
				},
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
				"description": "Enable JavaScript rendering (true = force JS, false = HTTP only, omit = auto-detect/default)",
			},
			"scrape": map[string]any{
				"type":        "boolean",
				"description": "Scrape each result URL and include extracted content (default: false; when false, only metadata is returned)",
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
