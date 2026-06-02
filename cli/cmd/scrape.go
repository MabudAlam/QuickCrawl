package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/spf13/cobra"
)

// scrapeCmd represents the scrape subcommand.
// It fetches a single URL and outputs its content in the requested format.
var scrapeCmd = &cobra.Command{
	Use:   "scrape",
	Short: "Scrape a single URL and extract content",
	Long: `Scrape a single URL and return its content in various formats.

This command fetches a URL and extracts content using the in-process
chromedp-based scraper. HTTP-only and JavaScript rendering are both
supported.

Example:
  quickcrawl scrape https://example.com
  quickcrawl scrape https://example.com --formats html --formats markdown
  quickcrawl scrape https://example.com --render-js --wait-for 3000`,
	RunE: runScrape,
}

var scrapeFlags = struct {
	formats     string
	renderJS    bool
	waitFor     int64
	includeTags string
	excludeTags string
	cssSelector string
	jsonSchema  string
	// renderer is deprecated and ignored; kept for backward-compat with old CLI flags.
	renderer string
}{}

func init() {
	rootCmd.AddCommand(scrapeCmd)

	scrapeCmd.Flags().StringVarP(&scrapeFlags.formats, "formats", "f", "markdown",
		"Output formats (comma-separated): markdown,html,links,json")
	scrapeCmd.Flags().BoolVar(&scrapeFlags.renderJS, "render-js", false,
		"Force JavaScript rendering")
	scrapeCmd.Flags().Int64Var(&scrapeFlags.waitFor, "wait-for", 0,
		"Milliseconds to wait after page load for late content")
	scrapeCmd.Flags().StringVar(&scrapeFlags.includeTags, "include-tags", "",
		"CSS selectors to include (comma-separated)")
	scrapeCmd.Flags().StringVar(&scrapeFlags.excludeTags, "exclude-tags", "",
		"CSS selectors to exclude (comma-separated)")
	scrapeCmd.Flags().StringVar(&scrapeFlags.cssSelector, "css-selector", "",
		"Extract content from specific CSS selector")
	scrapeCmd.Flags().StringVar(&scrapeFlags.jsonSchema, "json-schema", "",
		"JSON Schema for structured data extraction")
	scrapeCmd.Flags().StringVar(&scrapeFlags.renderer, "renderer", "auto",
		"Deprecated: ignored. The scraper uses chromedp only.")
}

func runScrape(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	parsedURL, urlErr := url.Parse(targetURL)
	if urlErr != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("invalid URL: %s (must include scheme like https://)", targetURL)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (only http/https supported)", parsedURL.Scheme)
	}

	formats := parseFormats(scrapeFlags.formats)

	var renderJS *bool
	if scrapeFlags.renderJS {
		renderJS = &scrapeFlags.renderJS
	}

	var waitFor *int64
	if scrapeFlags.waitFor > 0 {
		waitFor = &scrapeFlags.waitFor
	}

	var includeTags, excludeTags []string
	if scrapeFlags.includeTags != "" {
		includeTags = strings.Split(scrapeFlags.includeTags, ",")
	}
	if scrapeFlags.excludeTags != "" {
		excludeTags = strings.Split(scrapeFlags.excludeTags, ",")
	}

	var cssSelector *string
	if scrapeFlags.cssSelector != "" {
		cssSelector = &scrapeFlags.cssSelector
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Crawler.RespectRobotsTxt {
		parsedURL, _ := url.Parse(targetURL)
		if parsedURL != nil {
			origin := parsedURL.Scheme + "://" + parsedURL.Host
			robots := crawler.FetchRobotsTxt(origin, cfg.Crawler.UserAgent)
			if robots != nil && !robots.IsAllowed(parsedURL.Path) {
				return fmt.Errorf("access denied by robots.txt")
			}
		}
	}

	scraper, qErr := core.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if qErr != nil {
		return fmt.Errorf("failed to initialize scraper: %w", qErr)
	}
	defer scraper.Close()

	coreReq := &core.ScrapeRequest{
		URL:          targetURL,
		Formats:      formatsToStrings(formats),
		RenderJS:     renderJS,
		WaitFor:      waitFor,
		IncludeTags:  includeTags,
		ExcludeTags:  excludeTags,
		CSSSelector:  cssSelector,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	data, scrapeErr := scraper.Scrape(ctx, coreReq)
	cancel()

	if scrapeErr != nil {
		return fmt.Errorf("scrape failed: %w", scrapeErr)
	}

	result := formatCoreScrapeData(data)
	output("%s\n", result)

	return nil
}

// parseFormats converts a comma-separated format string to a slice of OutputFormat.
// Valid formats are: markdown, html, rawHtml, links, json, plainText.
func parseFormats(formats string) []types.OutputFormat {
	if formats == "" {
		return []types.OutputFormat{types.FormatMarkdown}
	}

	parts := strings.Split(formats, ",")
	result := make([]types.OutputFormat, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch strings.ToLower(part) {
		case "markdown":
			result = append(result, types.FormatMarkdown)
		case "html":
			result = append(result, types.FormatHtml)
		case "rawhtml", "raw":
			result = append(result, types.FormatRawHtml)
		case "links":
			result = append(result, types.FormatLinks)
		case "json":
			result = append(result, types.FormatJson)
		case "plaintext", "plain":
			result = append(result, types.FormatPlainText)
		default:
			// Ignore unknown formats to allow forward compatibility.
		}
	}

	if len(result) == 0 {
		result = append(result, types.FormatMarkdown)
	}

	return result
}

// formatsToStrings converts []types.OutputFormat to []string for the
// core.ScrapeRequest API.
func formatsToStrings(formats []types.OutputFormat) []string {
	out := make([]string, len(formats))
	for i, f := range formats {
		out[i] = string(f)
	}
	return out
}

// formatCoreScrapeData converts a *core.ScrapeData to a JSON-serializable
// map and emits it as an indented JSON string. Mirrors the legacy
// types.ScrapeData formatter so existing CLI consumers see the same
// output shape.
func formatCoreScrapeData(data *core.ScrapeData) string {
	if data == nil {
		return `{"error": "no data returned"}`
	}

	result := make(map[string]interface{})

	if data.Markdown != nil && *data.Markdown != "" {
		result["markdown"] = *data.Markdown
	}
	if data.HTML != nil && *data.HTML != "" {
		result["html"] = *data.HTML
	}
	if data.PlainText != nil && *data.PlainText != "" {
		result["plainText"] = *data.PlainText
	}
	if data.Links != nil && len(data.Links) > 0 {
		result["links"] = data.Links
	}
	if len(data.JSON) > 0 {
		var raw any
		if err := json.Unmarshal(data.JSON, &raw); err == nil {
			result["json"] = raw
		}
	}

	result["metadata"] = data.Metadata

	if data.Warning != nil && *data.Warning != "" {
		result["warning"] = *data.Warning
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to format result: %v"}`, err)
	}

	return string(encoded)
}

// loadConfig loads application configuration from file and environment variables.
// It's a thin wrapper around the internal config loader that handles errors.
func loadConfig() (*types.AppConfig, error) {
	return configLoadAppConfig()
}
