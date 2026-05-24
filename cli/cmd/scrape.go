package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/config"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/spf13/cobra"
)

// scrapeCmd represents the scrape subcommand.
// It fetches a single URL and outputs its content in the requested format.
var scrapeCmd = &cobra.Command{
	Use:   "scrape",
	Short: "Scrape a single URL and extract content",
	Long: `Scrape a single URL and return its content in various formats.

This command fetches a URL and extracts content using the configured
rendering strategy (HTTP or JavaScript rendering as needed).

Example:
  quickcrawl scrape https://example.com
  quickcrawl scrape https://example.com --formats html --formats markdown
  quickcrawl scrape https://example.com --render-js --wait-for 3000`,
	RunE: runScrape,
}

// scrapeFlags holds the flags for the scrape command.
// We use a separate struct to keep the command setup clean and to allow
// sharing these values with other commands if needed.
var scrapeFlags = struct {
	// formats is a comma-separated list of output formats.
	formats string
	// renderJS forces JavaScript rendering.
	renderJS bool
	// waitFor is milliseconds to wait after page load.
	waitFor int64
	// includeTags is CSS selectors to include.
	includeTags string
	// excludeTags is CSS selectors to exclude.
	excludeTags string
	// cssSelector extracts a specific element.
	cssSelector string
	// jsonSchema for structured extraction.
	jsonSchema string
	// renderer forces a specific renderer (auto, lightpanda, chrome).
	renderer string
}{}

func init() {
	// Bind flags to the scrape command.
	// These match the flags available in the Python SDK for consistency.

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
		"Renderer to use: auto, lightpanda, chrome")
}

// runScrape executes the scrape command.
// It validates the URL, sets up the renderer, calls the crawler, and outputs results.
func runScrape(cmd *cobra.Command, args []string) error {
	// Require exactly one argument: the URL to scrape.
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	// Validate the URL format before doing any work.
	// This gives immediate feedback rather than cryptic errors later.
	parsedURL, urlErr := url.Parse(targetURL)
	if urlErr != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("invalid URL: %s (must include scheme like https://)", targetURL)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (only http/https supported)", parsedURL.Scheme)
	}

	// Build the ScrapeRequest using existing types.
	// The crawler package already has all the logic for scraping.
	formats := parseFormats(scrapeFlags.formats)

	// Parse optional flags.
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

	var browser *string
	if scrapeFlags.renderer != "" && scrapeFlags.renderer != "auto" {
		browser = &scrapeFlags.renderer
	}

	scrapeReq := &types.ScrapeRequest{
		URL:          targetURL,
		Formats:      formats,
		RenderJS:     renderJS,
		WaitFor:      waitFor,
		IncludeTags:  includeTags,
		ExcludeTags:  excludeTags,
		CSSSelector:  cssSelector,
		Browser:      browser,
	}
	scrapeReq.Defaults()

	// Load configuration to set up the renderer.
	// The config package respects CONFIG env var and quickcrawl.toml.
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check robots.txt if respect_robots_txt is enabled
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

	// Create the renderer with configured settings.
	rend, rendErr := renderer.NewFallbackRendererWithConfig(
		&cfg.Renderer,
		cfg.Crawler.UserAgent,
		&cfg.Crawler.Stealth,
		cfg.Renderer.RenderJSDefault,
	)
	if rendErr != nil {
		return fmt.Errorf("failed to initialize renderer: %w", rendErr)
	}
	defer rend.Close()

	// Note: timeout handling is done internally by the crawler.
	// The config timeout is used for HTTP server mode, not direct scraping.
	data, scrapeErr := crawler.ScrapeURL(
		scrapeReq,
		rend,
		cfg.Extraction.LLM,
		cfg.Crawler.Stealth.Enabled,
		cfg.Renderer.RenderJSDefault,
		utils.HeaderStrategy(cfg.Crawler.Stealth.Strategy),
	)

	if scrapeErr != nil {
		return fmt.Errorf("scrape failed: %w", scrapeErr)
	}

	// Output the result as JSON.
	// The output may be piped to other tools, so JSON is the most versatile format.
	result := formatScrapeData(data)
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
			// A warning could be printed but would be noisy for valid future formats.
		}
	}

	// Ensure at least one format.
	if len(result) == 0 {
		result = append(result, types.FormatMarkdown)
	}

	return result
}

// formatScrapeData converts ScrapeData to a JSON-serializable map.
// This allows us to include/exclude fields based on what was requested.
func formatScrapeData(data *types.ScrapeData) string {
	if data == nil {
		return `{"error": "no data returned"}`
	}

	result := make(map[string]interface{})

	// Only include fields that have data.
	// This keeps the output clean and piping to jq easier.
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
		// Parse and re-serialize to ensure valid JSON output.
		var raw any
		if err := json.Unmarshal(data.JSON, &raw); err == nil {
			result["json"] = raw
		}
	}

	// Always include metadata even if it's empty.
	result["metadata"] = data.Metadata

	// Include warning if present (non-fatal issues like anti-bot detection).
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
	return config.LoadAppConfig()
}