package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/cache"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/search"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/gin-gonic/gin"
)

// Handler is the single HTTP entry point. It owns the application state
// and implements every endpoint in the API: /health, /v1/scrape, /v1/crawl,
// /v1/crawl/:id, /v1/map, /v1/search. All endpoints route through the
// shared *core.Scraper so there is exactly one render path.
//
// @title           QuickCrawl API
// @version         1.0
// @description     Web Scraping API for AI Agents — Scrape, crawl, and map websites with a single binary.
// @termsOfService  https://github.com/MabudAlam/QuickCrawl

// @contact.name   Mabud Alam
// @contact.url    https://github.com/MabudAlam/QuickCrawl
// @contact.email  mabudalam1234@gmail.com

// @license.name    MIT
// @license.url    https://opensource.org/licenses/MIT

// @host      localhost:3000
// @BasePath  /

// @schemes http https
type Handler struct {
	State *api.AppState
}

// NewHandler creates a new Handler bound to the given application state.
func NewHandler(state *api.AppState) *Handler {
	return &Handler{State: state}
}

// Health godoc
// @Summary      Health check
// @Description  Returns service health status including available renderers, running browser instances, and active crawl job count
// @Tags         health
// @Produce      json
// @Success      200 {object} map[string]interface{}
// @Router       /health [get]
func (h *Handler) Health(c *gin.Context) {
	renderers := h.State.CheckHealth()
	browsersInfo := []map[string]string{}
	for _, b := range h.State.RendererBrowsersInfo() {
		browsersInfo = append(browsersInfo, map[string]string{
			"name":  b.Name,
			"wsUrl": b.WSURL,
		})
	}

	activeJobs := 0
	if h.State != nil {
		activeJobs = h.State.ActiveCrawlJobCount()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":            "ok",
		"version":           "quickcrawl",
		"renderers":         renderers,
		"browsers":          browsersInfo,
		"active_crawl_jobs": activeJobs,
	})
}

// Scrape godoc
// @Summary      Scrape a single URL
// @Description  Scrape a single URL with one or more output formats. Supports HTTP/browser rendering, content filters, and LLM-based structured extraction.
// @Tags         scraping
// @Accept       json
// @Produce      json
// @Param        request body types.ScrapeRequest true "Scrape request"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      403 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /v1/scrape [post]
func (h *Handler) Scrape(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("failed to read request body"))
		return
	}

	var req types.ScrapeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("invalid JSON request"))
		return
	}

	req.Defaults()
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr(err.Error()),
			ErrorCode: stringPtr(string(types.CodeInvalidRequest)),
		})
		return
	}

	//if the config says be polite with robots.txt then follow it
	//if it says not allowed then fail fast
	if h.State.Config.Crawler.RespectRobotsTxt {
		if err := crawler.CheckRobotsTxt(req.URL, h.State.Config.Crawler.UserAgent); err != nil {
			c.JSON(http.StatusForbidden, types.APIResponse[struct{}]{
				Success:   false,
				Error:     &err.Message,
				ErrorCode: stringPtr(string(err.Code)),
			})
			return
		}
	}

	scraper := h.State.CoreScraper
	if scraper == nil {
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}]("core scraper not initialized"))
		return
	}

	ctx := c.Request.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	ttl := req.TTL
	if ttl != nil && *ttl == 0 {
		// TTL=0 means bypass cache, do fresh fetch
	} else if h.State.Cache != nil && h.State.Cache.Enabled() {
		normalizedFormats := cache.NormalizeFormats(formatsToStrings(req.Formats))
		effectiveTTL := h.State.Config.Cache.TTLDefaultSecs
		if ttl != nil && *ttl > 0 {
			effectiveTTL = *ttl
		}
		if cachedData, found, _ := h.State.Cache.Get(ctx, req.URL, normalizedFormats, req.RenderMode, effectiveTTL); found {
			var data types.ScrapeData
			if err := json.Unmarshal(cachedData, &data); err == nil {
				c.JSON(http.StatusOK, types.APIResponse[types.ScrapeData]{
					Success: true,
					Data:    &data,
					Warning: stringPtr("cache hit"),
				})
				return
			}
		}
	}

	data, scrapeErr := scraper.Scrape(ctx, &req)
	if scrapeErr != nil {
		status, code := mapScrapeError(scrapeErr)
		c.JSON(status, types.APIResponse[struct{}]{
			Success:   false,
			Error:     &scrapeErr.Message,
			ErrorCode: &code,
		})
		return
	}

	if data == nil {
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}]("internal error"))
		return
	}

	resp := types.APIResponse[types.ScrapeData]{
		Success: true,
		Data:    data,
		Warning: data.Warning,
	}

	// If the scraped data indicates an HTTP error status code (e.g., 4xx or 5xx),
	// and the body is small enough to be an error page, surface it as a failure.
	if data.Metadata.StatusCode >= 400 {
		bodyLen := 0
		if data.Markdown != nil {
			bodyLen = len(*data.Markdown)
		}
		if data.PlainText != nil && len(*data.PlainText) > bodyLen {
			bodyLen = len(*data.PlainText)
		}
		if bodyLen < 200 {
			errorMsg := "target returned HTTP " + http.StatusText(int(data.Metadata.StatusCode))
			if data.Warning != nil && *data.Warning != "" {
				errorMsg = *data.Warning
			}
			resp.Success = false
			resp.Error = &errorMsg
			code := string(types.CodeHttp)
			resp.ErrorCode = &code
			resp.Warning = nil
		}
	}

	if resp.Success && h.State.Cache != nil && h.State.Cache.Enabled() {
		if ttl == nil || *ttl > 0 {
			normalizedFormats := cache.NormalizeFormats(formatsToStrings(req.Formats))
			if dataBytes, err := json.Marshal(data); err == nil {
				_ = h.State.Cache.Set(ctx, req.URL, normalizedFormats, req.RenderMode, dataBytes)
			}
		}
	}
	c.JSON(http.StatusOK, resp)
}

// StartCrawl godoc
// @Summary      Start an async BFS crawl
// @Description  Start an async BFS crawl of a website. Returns immediately with a job ID for tracking progress via GET /v1/crawl/:id
// @Tags         crawling
// @Accept       json
// @Produce      json
// @Param        request body types.CrawlRequest true "Crawl request"
// @Success      200 {object} types.CrawlStartResponse
// @Failure      400 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /v1/crawl [post]
func (h *Handler) StartCrawl(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Failed to read request body"))
		return
	}

	req, parseErr := parseCrawlRequest(body)
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Invalid JSON request"))
		return
	}

	req.Defaults()
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr(err.Error()),
			ErrorCode: stringPtr(string(types.CodeInvalidRequest)),
		})
		return
	}

	if req.MaxDepth == nil {
		depth := uint32(h.State.Config.Crawler.DefaultMaxDepth)
		req.MaxDepth = &depth
	}
	if req.MaxPages == nil {
		pages := uint32(h.State.Config.Crawler.DefaultMaxPages)
		req.MaxPages = &pages
	}

	id := h.State.StartCrawlJob(&req)

	scraper := h.State.CoreScraper
	if scraper == nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr("scraper is not initialized"),
			ErrorCode: stringPtr(string(types.CodeInternalErr)),
		})
		return
	}

	stateCh := make(chan types.CrawlState, 100)

	go func() {
		for state := range stateCh {
			h.State.UpdateCrawlJob(id, state)
		}
	}()

	go func() {
		defer close(stateCh)
		stealthStrategy := utils.HeaderStrategy(h.State.Config.Crawler.Stealth.Strategy)
		opts := crawler.CrawlOptions{
			ID:                id,
			Req:               &req,
			Scraper:           scraper,
			MaxConcurrency:    h.State.Config.Crawler.MaxConcurrency,
			RespectRobots:     h.State.Config.Crawler.RespectRobotsTxt,
			RequestsPerSecond: h.State.Config.Crawler.RequestsPerSecond,
			UserAgent:         h.State.Config.Crawler.UserAgent,
			StateCh:           stateCh,
			JitterFactor:      h.State.Config.Crawler.Stealth.JitterFactor,
			StealthStrategy:   stealthStrategy,
		}
		crawler.RunCrawl(opts)
	}()

	c.JSON(http.StatusOK, types.CrawlStartResponse{
		Success: true,
		ID:      id,
	})
}

// GetCrawlStatus godoc
// @Summary      Get crawl job status
// @Description  Check the current status and results of a crawl job by its ID
// @Tags         crawling
// @Produce      json
// @Param        id path string true "Crawl job ID"
// @Success      200 {object} types.CrawlState
// @Failure      400 {object} map[string]interface{}
// @Failure      404 {object} map[string]interface{}
// @Router       /v1/crawl/{id} [get]
func (h *Handler) GetCrawlStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Crawl ID is required"))
		return
	}

	state := h.State.GetCrawlJob(id)
	if state == nil {
		c.JSON(http.StatusNotFound, types.APIErr[struct{}]("Crawl job not found"))
		return
	}

	c.JSON(http.StatusOK, state)
}

// CancelCrawl godoc
// @Summary      Cancel a crawl job
// @Description  Cancel a running crawl job by its ID
// @Tags         crawling
// @Param        id path string true "Crawl job ID"
// @Success      204 "No Content"
// @Failure      400 {object} map[string]interface{}
// @Router       /v1/crawl/{id} [delete]
func (h *Handler) CancelCrawl(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Crawl ID is required"))
		return
	}

	h.State.DeleteCrawlJob(id)

	c.Status(http.StatusNoContent)
}

// Map godoc
// @Summary      Discover URLs on a website
// @Description  Discover all URLs on a website without scraping content. Uses BFS traversal and optionally sitemap.xml
// @Tags         mapping
// @Accept       json
// @Produce      json
// @Param        request body types.MapRequest true "Map request"
// @Success      200 {object} types.MapResponse
// @Failure      400 {object} map[string]interface{}
// @Failure      403 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /v1/map [post]
func (h *Handler) Map(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Failed to read request body"))
		return
	}

	req, parseErr := parseMapRequest(body)
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Invalid JSON request"))
		return
	}

	req.Defaults()
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr(err.Error()),
			ErrorCode: stringPtr(string(types.CodeInvalidRequest)),
		})
		return
	}

	useSitemap := true
	if req.UseSitemap != nil {
		useSitemap = *req.UseSitemap
	}

	respectRobots := h.State.Config.Crawler.RespectRobotsTxt
	maxDepth := uint32(h.State.Config.Crawler.DefaultMaxDepth)
	if req.MaxDepth != nil {
		maxDepth = uint32(*req.MaxDepth)
	}

	timeout := 30 * time.Second
	if req.Timeout != nil && *req.Timeout > 0 {
		timeout = time.Duration(*req.Timeout) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	scraper := h.State.CoreScraper
	if scraper == nil {
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}]("scraper is not initialized"))
		return
	}

	urls, discoverErr := crawler.DiscoverUrls(
		req.URL,
		maxDepth,
		useSitemap,
		scraper,
		respectRobots,
		h.State.Config.Crawler.MaxConcurrency,
		h.State.Config.Crawler.RequestsPerSecond,
		h.State.Config.Crawler.UserAgent,
		ctx,
	)

	if discoverErr != nil {
		if discoverErr.Code == types.CodeForbidden {
			c.JSON(http.StatusForbidden, types.APIResponse[struct{}]{
				Success:   false,
				Error:     stringPtr("access denied by robots.txt"),
				ErrorCode: stringPtr(string(types.CodeForbidden)),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}](discoverErr.Message))
		return
	}

	c.JSON(http.StatusOK, types.MapResponse{
		Success: true,
		Data: &types.MapData{
			Links: urls,
		},
	})
}

// Search godoc
// @Summary      Search the web
// @Description  Query SearXNG and return search results. Optionally scrape each result URL for content
// @Tags         search
// @Accept       json
// @Produce      json
// @Param        request body types.SearchRequest true "Search request"
// @Success      200 {object} types.SearchResponse
// @Failure      400 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /v1/search [post]
func (h *Handler) Search(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Failed to read request body"))
		return
	}

	req, parseErr := parseSearchRequest(body)
	if parseErr != nil {
		if _, ok := err.(*json.SyntaxError); ok || parseErr.Error() == "invalid character" {
			c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Invalid JSON request"))
		} else {
			c.JSON(http.StatusBadRequest, types.APIErr[struct{}](parseErr.Error()))
		}
		return
	}

	searxng, searxngErr := h.State.GetSearXNG()
	if searxngErr != nil {
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}](searxngErr.Error()))
		return
	}

	formats := scrapeFormatsToStrings(req.Formats)
	if req.Scrape && len(formats) == 0 {
		formats = []string{"markdown"}
	}

	resp, err := search.Search(c.Request.Context(), searxng, h.State.CoreScraper, search.Request{
		Query:    req.Query,
		Language: req.Language,
		TimeRange: req.TimeRange,
		Categories: req.Categories,
		Page:     req.Page,
		UseBM25:  req.UseBM25,
		BM25FWeights: search.BM25FWeights{
			Title:   h.State.Config.Search.BM25FTitleWeight,
			Snippet: h.State.Config.Search.BM25FSnippetWeight,
		},
		Scrape:     req.Scrape,
		Formats:    formats,
		RenderMode: req.RenderMode,
	})
	if err != nil {
		utils.Log.Error("search service failed", "error", err)
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}](err.Error()))
		return
	}

	c.JSON(http.StatusOK, searchResponseToAPIResponse(resp))
}

// searchResponseToAPIResponse converts the unified search response
// into the HTTP API's types.SearchResponse wire format.
func searchResponseToAPIResponse(resp *search.Response) types.SearchResponse {
	results := make([]types.SearchResult, len(resp.Results))
	for i, it := range resp.Results {
		results[i] = types.SearchResult{
			Position:  it.Position,
			Score:     it.Score,
			BM25Score: it.BM25Score,
			Title:     it.Title,
			URL:       it.URL,
			SiteName:  it.SiteName,
			Snippet:   it.Snippet,
			Engine:    it.Engine,
			Published: it.Published,
			Markdown:  it.Markdown,
			HTML:      it.HTML,
			RawHTML:   it.RawHTML,
			PlainText: it.PlainText,
			Links:     it.Links,
			RawJSON:   it.RawJSON,
		}
	}
	return types.SearchResponse{
		Query:        resp.Query,
		Results:      results,
		TotalResults: resp.TotalResults,
		Page:         resp.Page,
	}
}

// mapScrapeError converts a *core.QuickCrawlError from the scraper into
// the (HTTP status, API error code) pair to return to the caller.
func mapScrapeError(scrapeErr *core.QuickCrawlError) (int, string) {
	status := http.StatusInternalServerError
	code := string(types.CodeInternalErr)
	switch scrapeErr.Code {
	case core.CodeInvalidURL, core.CodeInvalidRequest:
		status = http.StatusBadRequest
		code = string(types.CodeInvalidRequest)
	case core.CodeExtractionError:
		status = http.StatusUnprocessableEntity
		code = string(types.CodeExtractionErr)
	case core.CodeTimeout:
		status = http.StatusGatewayTimeout
		code = string(types.CodeTimeout)
	case core.CodeRateLimited:
		status = http.StatusTooManyRequests
		code = string(types.CodeRateLimited)
	}
	return status, code
}

// parseCrawlRequest deserializes a CrawlRequest from JSON.
func parseCrawlRequest(body []byte) (types.CrawlRequest, error) {
	var req types.CrawlRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}
	req.Defaults()
	return req, nil
}

// parseMapRequest deserializes a MapRequest from JSON and applies defaults.
func parseMapRequest(body []byte) (types.MapRequest, error) {
	var req types.MapRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}
	req.Defaults()
	return req, nil
}

// parseSearchRequest deserializes a SearchRequest from JSON, applies defaults
// and validates every field against the SearXNG contract.
func parseSearchRequest(body []byte) (types.SearchRequest, error) {
	var req types.SearchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}
	req.Defaults()
	if err := req.Validate(); err != nil {
		return req, err
	}
	return req, nil
}

// scrapeFormatsToStrings converts []types.OutputFormat into []string for
// the core.ScrapeRequest. Format values are already strings, this is just
// a type conversion.
func scrapeFormatsToStrings(formats []types.OutputFormat) []string {
	out := make([]string, len(formats))
	for i, f := range formats {
		out[i] = string(f)
	}
	return out
}

// stringPtr is a helper to create a pointer to a string.
func stringPtr(s string) *string {
	return &s
}

func formatsToStrings(formats []types.OutputFormat) []string {
	if len(formats) == 0 {
		return nil
	}
	out := make([]string, len(formats))
	for i, f := range formats {
		out[i] = string(f)
	}
	return out
}
