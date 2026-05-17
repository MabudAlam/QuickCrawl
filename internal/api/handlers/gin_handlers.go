package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/search"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/gin-gonic/gin"
)

// Handler holds the application state and implements HTTP handlers for all API endpoints.
type Handler struct {
	State *api.AppState
}

// NewHandler creates a new Handler with the given application state.
func NewHandler(state *api.AppState) *Handler {
	return &Handler{State: state}
}

// Health returns the service health status including available renderers,
// running browser instances, and count of active crawl jobs.
func (h *Handler) Health(c *gin.Context) {
	// Build map of renderer availability (http always available)
	renderers := map[string]bool{"http": true}
	browsersInfo := []map[string]string{}
	if h.State != nil && h.State.Renderer != nil {
		for name, ok := range h.State.Renderer.CheckHealth() {
			renderers[name] = ok
		}
		for _, b := range h.State.RendererBrowsersInfo() {
			browsersInfo = append(browsersInfo, map[string]string{
				"name":  b.Name,
				"wsUrl": b.WSURL,
			})
		}
	}

	// Count active crawl jobs
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

// Scrape handles POST /v1/scrape - scrapes a single URL and returns content.
// It accepts a ScrapeRequest JSON body and returns ScrapeData in the requested formats.
func (h *Handler) Scrape(c *gin.Context) {
	// Parse request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Failed to read request body"))
		return
	}

	req, parseErr := parseScrapeRequest(body)
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Invalid JSON request"))
		return
	}

	// Validate URL is provided
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("URL is required"))
		return
	}

	// Validate URL format
	if _, urlErr := types.ValidateURL(req.URL); urlErr != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr(urlErr.Error()),
			ErrorCode: stringPtr(string(types.CodeInvalidRequest)),
		})
		return
	}

	// Apply default settings if not provided in request
	if len(req.Formats) == 0 {
		req.Formats = []types.OutputFormat{types.OutputFormat(h.State.Config.Extraction.DefaultFormat)}
	}
	if !req.OnlyMainContent {
		req.OnlyMainContent = h.State.Config.Extraction.OnlyMainContent
	}

	// Ensure renderer is initialized
	rend := h.State.Renderer
	if rend == nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr("renderer is not initialized"),
			ErrorCode: stringPtr(string(types.CodeRendererError)),
		})
		return
	}

	// Check robots.txt if respect_robots_txt is enabled
	if h.State.Config.Crawler.RespectRobotsTxt {
		parsedURL, _ := url.Parse(req.URL)
		if parsedURL != nil {
			origin := parsedURL.Scheme + "://" + parsedURL.Host
			log.Printf("[robots] checking robots.txt for origin=%s with userAgent=%q", origin, h.State.Config.Crawler.UserAgent)
			robots := crawler.FetchRobotsTxt(origin, h.State.Config.Crawler.UserAgent, nil)
			if robots != nil && !robots.IsAllowed(parsedURL.Path) {
				log.Printf("[robots] denied: path=%s", parsedURL.Path)
				c.JSON(http.StatusForbidden, types.APIResponse[struct{}]{
					Success:   false,
					Error:     stringPtr("access denied by robots.txt"),
					ErrorCode: stringPtr(string(types.CodeForbidden)),
				})
				return
			}
			log.Printf("[robots] allowed: path=%s", parsedURL.Path)
		}
	}

	llmConfig := h.State.Config.Extraction.LLM

	// Perform the scrape using the renderer (HTTP + optional browser fallback)
	stealthStrategy := utils.HeaderStrategy(h.State.Config.Crawler.Stealth.Strategy)
	data, scrapeErr := crawler.ScrapeURL(
		&req,
		rend,
		llmConfig,
		h.State.Config.Crawler.UserAgent,
		h.State.Config.Crawler.Stealth.Enabled,
		h.State.Config.Renderer.RenderJSDefault,
		stealthStrategy,
	)

	// Handle scrape errors
	if scrapeErr != nil && hasErrorOccurred(scrapeErr) {
		log.Printf("scrape error type=%T value=%v", scrapeErr, scrapeErr)
		writequickcrawlErrorGin(c, scrapeErr)
		return
	}
	if data == nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr("internal error"),
			ErrorCode: stringPtr(string(types.CodeInternalErr)),
		})
		return
	}

	// Build response
	resp := types.APIResponse[types.ScrapeData]{
		Success: true,
		Data:    data,
		Warning: data.Warning,
	}

	// If target returned HTTP 4xx with small body, treat as error rather than success
	if statusCode := data.Metadata.StatusCode; statusCode >= 400 {
		bodyLen := calculateMaxResponseLength(data)
		if bodyLen < 200 {
			errorMsg := "Target returned HTTP " + http.StatusText(int(statusCode))
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

	c.JSON(http.StatusOK, resp)
}

// StartCrawl handles POST /v1/crawl - starts an async BFS crawl of a website.
// Returns immediately with a job ID that can be used to track progress via GetCrawlStatus.
func (h *Handler) StartCrawl(c *gin.Context) {
	// Parse request body
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

	// Validate URL is provided
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("URL is required"))
		return
	}

	// Validate URL format
	if _, urlErr := types.ValidateURL(req.URL); urlErr != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr(urlErr.Error()),
			ErrorCode: stringPtr(string(types.CodeInvalidRequest)),
		})
		return
	}

	// Apply default crawl settings if not provided
	if req.MaxDepth == nil {
		depth := uint32(h.State.Config.Crawler.DefaultMaxDepth)
		req.MaxDepth = &depth
	}
	if req.MaxPages == nil {
		pages := uint32(h.State.Config.Crawler.DefaultMaxPages)
		req.MaxPages = &pages
	}

	// Register the job in AppState and get a unique ID
	id := h.State.StartCrawlJob(&req)

	// Ensure renderer is initialized
	rend := h.State.Renderer
	if rend == nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr("renderer is not initialized"),
			ErrorCode: stringPtr(string(types.CodeRendererError)),
		})
		return
	}

	// Channel for receiving state updates from crawler
	stateCh := make(chan types.CrawlState, 100)

	// Goroutine: forward state updates from crawler to AppState
	go func() {
		for state := range stateCh {
			h.State.UpdateCrawlJob(id, state)
		}
	}()

	// Goroutine: run the actual crawl job asynchronously
	go func() {
		defer close(stateCh)
		stealthStrategy := utils.HeaderStrategy(h.State.Config.Crawler.Stealth.Strategy)
		opts := crawler.CrawlOptions{
			ID:                id,
			Req:               &req,
			Renderer:          rend,
			MaxConcurrency:    h.State.Config.Crawler.MaxConcurrency,
			RespectRobots:     h.State.Config.Crawler.RespectRobotsTxt,
			RequestsPerSecond: h.State.Config.Crawler.RequestsPerSecond,
			UserAgent:         h.State.Config.Crawler.UserAgent,
			StateCh:           stateCh,
			LLMConfig:         h.State.Config.Extraction.LLM,
			Proxy:             h.State.Config.Crawler.Proxy,
			JitterFactor:      h.State.Config.Crawler.Stealth.JitterFactor,
			StealthStrategy:   stealthStrategy,
		}
		crawler.RunCrawl(opts)
	}()

	// Return immediately with job ID
	c.JSON(http.StatusOK, types.CrawlStartResponse{
		Success: true,
		ID:      id,
	})
}

// GetCrawlStatus handles GET /v1/crawl/:id - returns the current state of a crawl job.
// Includes status (in_progress, completed, failed), progress (total/completed),
// and the scraped data if completed.
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

// CancelCrawl handles DELETE /v1/crawl/:id - cancels a running crawl job.
// Removes the job from tracking; the running crawler goroutine will stop on next iteration.
func (h *Handler) CancelCrawl(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Crawl ID is required"))
		return
	}

	h.State.DeleteCrawlJob(id)

	c.Status(http.StatusNoContent)
}

// Map handles POST /v1/map - discovers all URLs on a website without scraping content.
// Uses BFS traversal and optionally sitemap.xml to find URLs.
// Returns a list of discovered links.
func (h *Handler) Map(c *gin.Context) {
	// Parse request body
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

	// Validate URL is provided
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("URL is required"))
		return
	}

	// Validate URL format
	if _, urlErr := types.ValidateURL(req.URL); urlErr != nil {
		c.JSON(http.StatusBadRequest, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr(urlErr.Error()),
			ErrorCode: stringPtr(string(types.CodeInvalidRequest)),
		})
		return
	}

	// Apply request settings with defaults
	useSitemap := true
	if req.UseSitemap != nil {
		useSitemap = *req.UseSitemap
	}

	maxDepth := uint32(h.State.Config.Crawler.DefaultMaxDepth)
	if req.MaxDepth != nil {
		maxDepth = uint32(*req.MaxDepth)
	}

	// Set timeout for URL discovery
	timeout := 30 * time.Second
	if req.Timeout != nil && *req.Timeout > 0 {
		timeout = time.Duration(*req.Timeout) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Ensure renderer is initialized
	rend := h.State.Renderer
	if rend == nil {
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}]("renderer is not initialized"))
		return
	}

	// Discover URLs via BFS and optional sitemap parsing
	urls, discoverErr := crawler.DiscoverUrls(
		req.URL,
		maxDepth,
		useSitemap,
		rend,
		h.State.Config.Crawler.MaxConcurrency,
		h.State.Config.Crawler.RequestsPerSecond,
		h.State.Config.Crawler.UserAgent,
		h.State.Config.Crawler.Proxy,
		ctx,
	)

	if discoverErr != nil {
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

// Search handles POST /v1/search - searches DuckDuckGo and returns results with scraped content.
func (h *Handler) Search(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Failed to read request body"))
		return
	}

	req, parseErr := parseSearchRequest(body)
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Invalid JSON request"))
		return
	}

	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("Query is required"))
		return
	}

	safesearch := "moderate"
	if req.Safesearch != "" {
		safesearch = req.Safesearch
	}

	engine := search.New()
	results, searchErr := engine.Search(req.Query, req.Region, safesearch, req.Timelimit)
	if searchErr != nil {
		log.Printf("search: DuckDuckGo search failed: %v", searchErr)
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}](searchErr.Error()))
		return
	}

	rend := h.State.Renderer
	if rend == nil {
		c.JSON(http.StatusInternalServerError, types.APIErr[struct{}]("renderer is not initialized"))
		return
	}
	llmConfig := h.State.Config.Extraction.LLM

	const maxWorkers = 10
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	resultMap := make(map[int]types.SearchResult, len(results))

	for i, r := range results {
		wg.Add(1)
		go func(index int, result search.TextResult) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("search: panic recovered while scraping %s: %v", result.Href, r)
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
				log.Printf("search: skipping invalid URL %s: %v", result.Href, urlErr)
				mu.Lock()
				resultMap[index] = types.SearchResult{
					Title:       result.Title,
					Description: result.Body,
					URL:         result.Href,
				}
				mu.Unlock()
				return
			}

			renderJS := req.RenderJS
			scrapeReq := &types.ScrapeRequest{
				URL:             result.Href,
				Formats:         req.Formats,
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
				h.State.Config.Crawler.UserAgent,
				h.State.Config.Crawler.Stealth.Enabled,
				&renderJS,
				utils.HeaderStrategy(h.State.Config.Crawler.Stealth.Strategy),
			)

			if scrapeErr != nil {
				log.Printf("search: failed to scrape %s: %v (type=%T)", result.Href, scrapeErr, scrapeErr)
			} else if data != nil {
				if data.Markdown != nil {
					searchResult.Markdown = data.Markdown
				}
				if data.HTML != nil {
					searchResult.HTML = data.HTML
				}
				if data.RawHTML != nil {
					searchResult.RawHTML = data.RawHTML
				}
				if data.PlainText != nil {
					searchResult.PlainText = data.PlainText
				}
				if data.Links != nil {
					searchResult.Links = data.Links
				}
			}

			mu.Lock()
			resultMap[index] = searchResult
			mu.Unlock()
		}(i, r)
	}

	wg.Wait()

	searchResults := make([]types.SearchResult, 0, len(results))
	for i := 0; i < len(results); i++ {
		searchResults = append(searchResults, resultMap[i])
	}

	c.JSON(http.StatusOK, types.SearchResponse{
		Success: true,
		Data: &types.SearchData{
			Results: searchResults,
		},
	})
}

// parseScrapeRequest deserializes a ScrapeRequest from JSON.
// Also applies default values and defaults OnlyMainContent to true if not specified.
func parseScrapeRequest(body []byte) (types.ScrapeRequest, error) {
	var req types.ScrapeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}

	req.Defaults()
	if !hasJSONField(body, "onlyMainContent") {
		req.OnlyMainContent = true
	}

	return req, nil
}

// parseCrawlRequest deserializes a CrawlRequest from JSON.
// Also applies default values and defaults OnlyMainContent to true if not specified.
func parseCrawlRequest(body []byte) (types.CrawlRequest, error) {
	var req types.CrawlRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}

	req.Defaults()
	if !hasJSONField(body, "onlyMainContent") {
		req.OnlyMainContent = true
	}

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

// parseSearchRequest deserializes a SearchRequest from JSON and applies defaults.
func parseSearchRequest(body []byte) (types.SearchRequest, error) {
	var req types.SearchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}

	req.Defaults()
	return req, nil
}

// hasJSONField checks if the JSON body contains a specific field.
// Used to detect if a boolean field was explicitly set (vs defaulted).
func hasJSONField(body []byte, field string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	_, ok := raw[field]
	return ok
}

// stringPtr is a helper to create a pointer to a string.
func stringPtr(s string) *string {
	return &s
}

// calculateMaxResponseLength finds the largest content length among all output formats.
// Used to determine if a 4xx response is actually an error or just small content.
func calculateMaxResponseLength(data *types.ScrapeData) int {
	maxLen := 0
	if data.Markdown != nil && len(*data.Markdown) > maxLen {
		maxLen = len(*data.Markdown)
	}
	if data.PlainText != nil && len(*data.PlainText) > maxLen {
		maxLen = len(*data.PlainText)
	}
	if data.HTML != nil && len(*data.HTML) > maxLen {
		maxLen = len(*data.HTML)
	}
	if data.RawHTML != nil && len(*data.RawHTML) > maxLen {
		maxLen = len(*data.RawHTML)
	}
	return maxLen
}

// hasErrorOccurred checks if an interface/slice/map/pointer error is non-nil.
// More thorough than err != nil for interface error types.
func hasErrorOccurred(err error) bool {
	if err == nil {
		return false
	}
	v := reflect.ValueOf(err)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	default:
		return true
	}
}

// getErrorMessageSafe extracts error message, returning "internal error" on panic.
func getErrorMessageSafe(err error) (message string) {
	message = "internal error"
	defer func() {
		if recover() != nil {
			message = "internal error"
		}
	}()
	if err != nil {
		message = err.Error()
	}
	return message
}

// writequickcrawlErrorGin writes an error response, mapping QuickCrawlError codes to HTTP status codes.
func writequickcrawlErrorGin(c *gin.Context, err error) {
	if err == nil {
		c.JSON(http.StatusInternalServerError, types.APIResponse[struct{}]{
			Success:   false,
			Error:     stringPtr("internal error"),
			ErrorCode: stringPtr(string(types.CodeInternalErr)),
		})
		return
	}

	status := http.StatusInternalServerError
	message := "internal error"
	code := types.CodeInternalErr

	// Handle QuickCrawlError with structured error codes
	if quickcrawlErr, ok := err.(*types.QuickCrawlError); ok && quickcrawlErr != nil {
		message = quickcrawlErr.Message
		code = quickcrawlErr.Code
		switch quickcrawlErr.Code {
		case types.CodeInvalidURL, types.CodeInvalidRequest:
			status = http.StatusBadRequest
		case types.CodeTimeout:
			status = http.StatusGatewayTimeout
		case types.CodeRateLimited:
			status = http.StatusTooManyRequests
		}
	} else if hasErrorOccurred(err) {
		message = getErrorMessageSafe(err)
	}

	c.JSON(status, types.APIResponse[struct{}]{
		Success:   false,
		Error:     &message,
		ErrorCode: stringPtr(string(code)),
	})
}
