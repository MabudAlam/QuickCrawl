package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

type CoreHandler struct {
	State *api.AppState
}

func NewCoreHandler(state *api.AppState) *CoreHandler {
	return &CoreHandler{State: state}
}

func (h *CoreHandler) ScrapeHandler(c *gin.Context) {

	//Deserialize the request body into a ScrapeRequest struct, validating the URL and formats.
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("failed to read request body"))
		return
	}

	// Basic validation of the request body and URL format.
	var req core.ScrapeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("invalid JSON request"))
		return
	}

	// Validate URL format and required fields.
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("url is required"))
		return
	}
	// Ensure URL starts with http:// or https://
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		c.JSON(http.StatusBadRequest, types.APIErr[struct{}]("url must start with http:// or https://"))
		return
	}

	// If no formats specified, default to markdown.
	if len(req.Formats) == 0 {
		req.Formats = []string{"markdown"}
	}

	//if the user has enabled respecting robots.txt, check the target URL against the site's robots.txt rules before scraping.
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

	// Call the core scraper's Scrape method with the validated request data, handling any errors that occur during scraping.
	data, scrapeErr := scraper.Scrape(ctx, &req)

	// If the scrape operation returns an error, determine the appropriate HTTP status code based on the error type and return a structured error response to the client.
	if scrapeErr != nil {
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

	// If the scrape is successful, return the scraped data in a structured response format, including any warnings if applicable.
	resp := types.APIResponse[core.ScrapeData]{
		Success: true,
		Data:    data,
		Warning: data.Warning,
	}

	// If the scraped data indicates an HTTP error status code (e.g., 4xx or 5xx), modify the response to indicate a failure and include an appropriate error message and code.
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

	c.JSON(http.StatusOK, resp)
}
