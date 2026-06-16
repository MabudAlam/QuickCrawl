package quickcrawl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
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
	URL         string            `json:"url"`
	Formats     []string          `json:"formats,omitempty"`
	RenderMode  *types.RenderMode `json:"renderMode,omitempty"`
	WaitFor     *int64            `json:"waitFor,omitempty"`
	IncludeTags []string `json:"includeTags,omitempty"`
	ExcludeTags []string `json:"excludeTags,omitempty"`
	CSSSelector *string  `json:"cssSelector,omitempty"`
	// Renderer is deprecated: the new scraper uses chromedp only.
	Renderer *string `json:"renderer,omitempty"`
}

type SearchArgs struct {
	Query      string            `json:"query"`
	Region     string            `json:"region,omitempty"`
	TimeRange  string            `json:"timeRange,omitempty"`
	Page       int               `json:"page,omitempty"`
	UseBM25    bool              `json:"use_bm25,omitempty"`
	RenderMode *types.RenderMode `json:"renderMode,omitempty"`
	Formats    []string          `json:"formats,omitempty"`
	Scrape     bool     `json:"scrape,omitempty"`
}

func convertFormats(formats []string) []types.OutputFormat {
	if len(formats) == 0 {
		return nil
	}
	out := make([]types.OutputFormat, len(formats))
	for i, f := range formats {
		out[i] = types.OutputFormat(f)
	}
	return out
}

func (s *Server) HandleScrape(ctx context.Context, req *mcp.CallToolRequest, args ScrapeArgs) (*mcp.CallToolResult, any, error) {
	// Log deprecation warning if the caller pinned a renderer.
	if args.Renderer != nil && *args.Renderer != "" && *args.Renderer != "auto" {
		utils.Log.Warn("deprecated renderer field ignored for scrape", "renderer", strVal(args.Renderer))
	}

	coreReq := &types.ScrapeRequest{
		URL:         args.URL,
		Formats:     convertFormats(args.Formats),
		RenderMode:  args.RenderMode,
		WaitFor:     args.WaitFor,
		IncludeTags: args.IncludeTags,
		ExcludeTags: args.ExcludeTags,
		CSSSelector: args.CSSSelector,
	}
	coreReq.Defaults()
	if err := coreReq.Validate(); err != nil {
		return errorResult(err.Error()), nil, nil
	}

	if coreReq.RenderMode != nil && *coreReq.RenderMode == types.RenderModeBrowser && coreReq.WaitFor == nil {
		defaultWait := int64(2000)
		coreReq.WaitFor = &defaultWait
	}

	scraper := s.state.CoreScraper
	if scraper == nil {
		return errorResult("scraper is not initialized"), nil, nil
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
	URL        string            `json:"url"`
	MaxDepth   *uint32           `json:"maxDepth,omitempty"`
	MaxPages   *uint32           `json:"maxPages,omitempty"`
	Formats    []string          `json:"formats,omitempty"`
	RenderMode *types.RenderMode `json:"renderMode,omitempty"`
	WaitFor    *int64            `json:"waitFor,omitempty"`
	// Renderer is deprecated: the new scraper uses chromedp only.
	Renderer *string `json:"renderer,omitempty"`
}

func (s *Server) HandleCrawl(ctx context.Context, req *mcp.CallToolRequest, args CrawlArgs) (*mcp.CallToolResult, any, error) {
	if args.Renderer != nil && *args.Renderer != "" && *args.Renderer != "auto" {
		utils.Log.Warn("deprecated renderer field ignored for crawl", "renderer", strVal(args.Renderer))
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

	crawlReq := &types.CrawlRequest{
		URL:        args.URL,
		MaxDepth:   &maxDepth,
		MaxPages:   &maxPages,
		Formats:    formats,
		RenderMode: args.RenderMode,
		WaitFor:    args.WaitFor,
	}
	crawlReq.Defaults()
	if err := crawlReq.Validate(); err != nil {
		return errorResult(err.Error()), nil, nil
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
	mapReq := types.MapRequest{
		URL:        args.URL,
		MaxDepth:   args.MaxDepth,
		UseSitemap: args.UseSitemap,
		Timeout:    args.Timeout,
	}
	mapReq.Defaults()
	if err := mapReq.Validate(); err != nil {
		return errorResult(err.Error()), nil, nil
	}

	useSitemap := true
	if mapReq.UseSitemap != nil {
		useSitemap = *mapReq.UseSitemap
	}

	maxDepth := uint32(s.config.Crawler.DefaultMaxDepth)
	if mapReq.MaxDepth != nil {
		maxDepth = uint32(*mapReq.MaxDepth)
	}

	timeout := 30 * time.Second
	if mapReq.Timeout != nil && *mapReq.Timeout > 0 {
		timeout = time.Duration(*mapReq.Timeout) * time.Millisecond
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
	formats := convertFormats(args.Formats)
	if args.Scrape && len(formats) == 0 {
		formats = []types.OutputFormat{types.FormatMarkdown}
	}

	apiReq := types.SearchRequest{
		Query:      args.Query,
		Region:     args.Region,
		TimeRange:  args.TimeRange,
		Page:       args.Page,
		UseBM25:    args.UseBM25,
		RenderMode: args.RenderMode,
		Formats:    formats,
		Scrape:     args.Scrape,
	}
	apiReq.Defaults()
	if err := apiReq.Validate(); err != nil {
		return errorResult(err.Error()), nil, nil
	}

	searxng, searxngErr := s.state.GetSearXNG()
	if searxngErr != nil {
		return errorResult(fmt.Sprintf("search configuration error: %v", searxngErr)), nil, nil
	}

	scrapeFormats := make([]string, len(formats))
	for i, f := range formats {
		scrapeFormats[i] = string(f)
	}

	resp, err := search.Search(ctx, searxng, s.state.CoreScraper, search.Request{
		Query:      apiReq.Query,
		Language:   apiReq.Language,
		TimeRange:  apiReq.TimeRange,
		Categories: apiReq.Categories,
		Page:       apiReq.Page,
		UseBM25:    apiReq.UseBM25,
		BM25FWeights: search.BM25FWeights{
			Title:   s.config.Search.BM25FTitleWeight,
			Snippet: s.config.Search.BM25FSnippetWeight,
		},
		Scrape:     apiReq.Scrape,
		Formats:    scrapeFormats,
		RenderMode: apiReq.RenderMode,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}

	return mcpTextResult(searchResponseToMap(resp)), nil, nil
}

// searchResponseToMap converts the unified search.Response into the
// MCP wire format (snake_case keys, bm25_score only when present).
func searchResponseToMap(resp *search.Response) map[string]interface{} {
	results := make([]map[string]interface{}, 0, len(resp.Results))
	for _, it := range resp.Results {
		entry := map[string]interface{}{
			"position":  it.Position,
			"score":     it.Score,
			"site_name": it.SiteName,
			"snippet":   it.Snippet,
			"title":     it.Title,
			"url":       it.URL,
		}
		if it.BM25Score != 0 {
			entry["bm25_score"] = it.BM25Score
		}
		if it.Markdown != nil {
			entry["markdown"] = *it.Markdown
		}
		if it.HTML != nil {
			entry["html"] = *it.HTML
		}
		if it.PlainText != nil {
			entry["plain_text"] = *it.PlainText
		}
		if len(it.Links) > 0 {
			entry["links"] = it.Links
		}
		if len(it.RawJSON) > 0 {
			entry["raw_json"] = it.RawJSON
		}
		if it.Published != "" {
			entry["published_date"] = it.Published
		}
		results = append(results, entry)
	}
	return map[string]interface{}{
		"query":         resp.Query,
		"results":       results,
		"total_results": resp.TotalResults,
		"page":          resp.Page,
	}
}

func mcpTextResult(payload map[string]interface{}) *mcp.CallToolResult {
	data, _ := json.Marshal(payload)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
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
func formatCoreScrapeData(data *types.ScrapeData) string {
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
		InputSchema: mapInputSchema(),
	}, s.HandleMap)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search SearXNG and scrape results in parallel with 10 concurrent workers",
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
				"description": "Output formats such as markdown, html, links, or json. Defaults to [\"markdown\"] when omitted.",
				"default":     []string{"markdown"},
				"items": map[string]any{
					"type": "string",
					"enum": []string{"markdown", "html", "links", "json"},
				},
			},
			"renderMode": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "browser", "http"},
				"description": "Per-request override for fetch strategy. omit = inherit server default (render_mode in quickcrawl.toml). 'auto' = HTTP first, escalate to browser on anti-bot signals. 'browser' = always use the browser. 'http' = HTTP only, never touch the browser.",
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
				"description": "Maximum crawl depth. Defaults to 2 when omitted.",
				"default":     2,
				"minimum":     0,
				"maximum":     100,
			},
			"maxPages": map[string]any{
				"type":        "integer",
				"description": "Maximum number of pages to crawl. Defaults to 100 when omitted.",
				"default":     100,
				"minimum":     1,
				"maximum":     100,
			},
			"renderMode": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "browser", "http"},
				"description": "Per-request override for fetch strategy on every crawled page. omit = inherit server default.",
			},
			"waitFor": map[string]any{
				"type":        "integer",
				"description": "Milliseconds to wait after JS rendering on each page",
				"minimum":     0,
				"maximum":     120000,
			},
			"formats": map[string]any{
				"type":        "array",
				"description": "Output formats for each crawled page. Defaults to [\"markdown\"] when omitted.",
				"default":     []string{"markdown"},
				"items": map[string]any{
					"type": "string",
					"enum": []string{"markdown", "html", "links"},
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
			"timeRange": map[string]any{
				"type":        "string",
				"enum":        []string{"day", "week", "month", "year"},
				"description": "SearXNG time_range filter. Omit for no time filter.",
			},
			"page": map[string]any{
				"type":        "integer",
				"description": "Page number (1-based, default 1)",
				"minimum":     1,
				"maximum":     1000,
			},
			"use_bm25": map[string]any{
				"type":        "boolean",
				"description": "Use BM25 scoring algorithm instead of native score (default: false)",
			},
			"renderMode": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "browser", "http"},
				"description": "Per-request override for fetch strategy when scraping each result. omit = inherit server default.",
			},
			"scrape": map[string]any{
				"type":        "boolean",
				"description": "Scrape each result URL and include extracted content (default: false; when false, only metadata is returned)",
			},
			"formats": map[string]any{
				"type":        "array",
				"description": "Output formats for each result. Defaults to [\"markdown\"] when omitted.",
				"default":     []string{"markdown"},
				"items": map[string]any{
					"type": "string",
					"enum": []string{"markdown", "html", "links", "json"},
				},
			},
		},
		"required": []string{"query"},
	}
}

func mapInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The starting URL to crawl",
			},
			"maxDepth": map[string]any{
				"type":        "integer",
				"description": "Maximum link depth to follow. Defaults to 2 when omitted.",
				"default":     2,
				"minimum":     0,
				"maximum":     100,
			},
			"useSitemap": map[string]any{
				"type":        "boolean",
				"description": "Use sitemap.xml and robots.txt sitemaps as seed URLs. Defaults to true.",
				"default":     true,
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in milliseconds for the entire operation. Defaults to 30000.",
				"default":     30000,
				"minimum":     1,
				"maximum":     600000,
			},
		},
		"required": []string{"url"},
	}
}
