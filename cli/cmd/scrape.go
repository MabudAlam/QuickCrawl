package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/config"
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
  quickcrawl scrape https://example.com --render browser --wait-for 3000`,
	RunE: runScrape,
}

var scrapeFlags = struct {
	formats     string
	renderMode  string
	waitFor     int64
	includeTags string
	excludeTags string
	cssSelector string
	jsonSchema  string
}{}

func init() {
	rootCmd.AddCommand(scrapeCmd)

	scrapeCmd.Flags().StringVarP(&scrapeFlags.formats, "formats", "f", "markdown",
		"Output formats (comma-separated): markdown,html,rawHtml,plainText,links,imageLinks,json")
	scrapeCmd.Flags().StringVar(&scrapeFlags.renderMode, "render", "auto",
		"Render mode: auto (default), http, browser")
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
}

func runScrape(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	formats, err := parseFormats(scrapeFlags.formats)
	if err != nil {
		return err
	}

	mode, err := types.ParseRenderMode(scrapeFlags.renderMode)
	if err != nil {
		return fmt.Errorf("invalid --render: %w", err)
	}
	var modePtr *types.RenderMode
	if mode != "" {
		m := mode
		modePtr = &m
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

	coreReq := &types.ScrapeRequest{
		URL:         targetURL,
		Formats:     formats,
		RenderMode:  modePtr,
		WaitFor:     waitFor,
		IncludeTags: includeTags,
		ExcludeTags: excludeTags,
		CSSSelector: cssSelector,
	}
	coreReq.Defaults()
	if err := coreReq.Validate(); err != nil {
		return err
	}

	cfg, teardown, err := loadConfigWithRenderer()
	if err != nil {
		return err
	}
	if teardown != nil {
		defer teardown()
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

	scraper, qErr := config.NewScraperFromConfig(cfg, cfg.Extraction.LLM)
	if qErr != nil {
		return fmt.Errorf("failed to initialize scraper: %w", qErr)
	}
	defer scraper.Close()

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

// parseFormats converts a comma-separated format string to a slice of
// OutputFormat. Valid formats are: markdown, html, rawHtml, plainText,
// links, imageLinks, json. Unknown values return an error so the user
// is told their --formats flag was wrong.
func parseFormats(formats string) ([]types.OutputFormat, error) {
	if formats == "" {
		return []types.OutputFormat{types.FormatMarkdown}, nil
	}

	parts := strings.Split(formats, ",")
	result := make([]types.OutputFormat, 0, len(parts))
	seen := map[types.OutputFormat]struct{}{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var f types.OutputFormat
		if err := f.UnmarshalJSON([]byte(`"` + part + `"`)); err != nil {
			return nil, fmt.Errorf("invalid --formats value %q: allowed: markdown, html, rawHtml, plainText, links, imageLinks, json", part)
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		result = append(result, f)
	}

	if len(result) == 0 {
		result = append(result, types.FormatMarkdown)
	}

	return result, nil
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
func formatCoreScrapeData(data *types.ScrapeData) string {
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
