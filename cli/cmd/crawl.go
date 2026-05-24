package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/crawler"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/spf13/cobra"
)

// crawlCmd represents the crawl subcommand.
// It discovers and scrapes multiple pages starting from a seed URL.
var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Crawl a website and scrape multiple pages",
	Long: `Crawl a website using breadth-first search, scraping discovered pages.

The crawl starts from a seed URL and follows links up to maxDepth levels
deep, respecting robots.txt and rate limits. Results are collected and
returned when the crawl completes or maxPages is reached.

Example:
  quickcrawl crawl https://example.com
  quickcrawl crawl https://example.com --max-depth 3 --max-pages 50
  quickcrawl crawl https://example.com --formats html --render-js`,
	RunE: runCrawl,
}

// crawlFlags holds the configuration for the crawl command.
var crawlFlags = struct {
	formats  string
	renderJS bool
	waitFor        int64
	maxDepth       int
	maxPages       int
	query          string
	topK           int
	renderer       string
}{}

func init() {
	rootCmd.AddCommand(crawlCmd)

	crawlCmd.Flags().StringVarP(&crawlFlags.formats, "formats", "f", "markdown",
		"Output formats (comma-separated): markdown,html,links,json")
	crawlCmd.Flags().BoolVar(&crawlFlags.renderJS, "render-js", false,
		"Force JavaScript rendering on all pages")
	crawlCmd.Flags().Int64Var(&crawlFlags.waitFor, "wait-for", 0,
		"Milliseconds to wait after page load")
	crawlCmd.Flags().IntVar(&crawlFlags.maxDepth, "max-depth", 2,
		"Maximum link depth to follow (0-10)")
	crawlCmd.Flags().IntVar(&crawlFlags.maxPages, "max-pages", 10,
		"Maximum number of pages to scrape")
	crawlCmd.Flags().StringVar(&crawlFlags.renderer, "renderer", "auto",
		"Renderer: auto, lightpanda, chrome")
}

// runCrawl executes the crawl command.
// It starts an async crawl job, polls for completion, and outputs results.
func runCrawl(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("a URL argument is required")
	}

	targetURL := args[0]

	// Validate URL.
	parsedURL, urlErr := url.Parse(targetURL)
	if urlErr != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("invalid URL: %s", targetURL)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: %s (only http/https)", parsedURL.Scheme)
	}

	// Parse formats.
	formats := parseFormats(crawlFlags.formats)

	// Build request options.
	maxDepth := uint32(crawlFlags.maxDepth)
	if maxDepth > 10 {
		maxDepth = 10
	}
	maxPages := uint32(crawlFlags.maxPages)
	if maxPages > 1000 {
		maxPages = 1000
	}

	var renderJS *bool
	if crawlFlags.renderJS {
		renderJS = &crawlFlags.renderJS
	}

	var waitFor *int64
	if crawlFlags.waitFor > 0 {
		waitFor = &crawlFlags.waitFor
	}

	var browser *string
	if crawlFlags.renderer != "" && crawlFlags.renderer != "auto" {
		browser = &crawlFlags.renderer
	}

	crawlReq := &types.CrawlRequest{
		URL:          targetURL,
		MaxDepth:  &maxDepth,
		MaxPages:  &maxPages,
		Formats:   formats,
		RenderJS:  renderJS,
		WaitFor:   waitFor,
		Browser:   browser,
	}
	crawlReq.Defaults()

	// Load configuration.
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create renderer.
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

	// Create app state to manage crawl jobs.
	state, stateErr := api.NewAppState(cfg)
	if stateErr != nil {
		return fmt.Errorf("failed to create app state: %w", stateErr)
	}
	state.Renderer = rend
	defer state.Close()

	// Register the crawl job and get an ID for tracking.
	jobID := state.StartCrawlJob(crawlReq)

	// Channel to receive progress updates.
	stateCh := make(chan types.CrawlState, 100)

	// Start the crawl in a goroutine so we can track progress.
	go func() {
		opts := crawler.CrawlOptions{
			ID:                jobID,
			Req:               crawlReq,
			Renderer:          rend,
			MaxConcurrency:    cfg.Crawler.MaxConcurrency,
			RespectRobots:     cfg.Crawler.RespectRobotsTxt,
			RequestsPerSecond: cfg.Crawler.RequestsPerSecond,
			UserAgent:         cfg.Crawler.UserAgent,
			StateCh:           stateCh,
			LLMConfig:         cfg.Extraction.LLM,
			JitterFactor:      cfg.Crawler.Stealth.JitterFactor,
		}
		crawler.RunCrawl(opts)
	}()

	// Poll for completion with timeout.
	// The request_timeout_secs from config serves as our overall timeout.
	timeout := time.Duration(cfg.Server.RequestTimeoutSecs) * time.Second
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("crawl timed out after %v", timeout)
		}

		select {
		case state := <-stateCh:
			if state.Status == types.CrawlStatusCompleted {
				// Crawl finished - output results.
				if state.Error != nil {
					return fmt.Errorf("crawl failed: %s", *state.Error)
				}
				outputCrawlResults(state)
				return nil
			}
			if state.Status == types.CrawlStatusFailed {
				errMsg := "unknown error"
				if state.Error != nil {
					errMsg = *state.Error
				}
				return fmt.Errorf("crawl failed: %s", errMsg)
			}
			// In progress - could print progress indicator with verbose flag.
			if verbose {
				errPrint("crawl progress: %d/%d pages\n", state.Completed, state.Total)
			}

		case <-time.After(2 * time.Second):
			// Timeout waiting for state update - check current status directly.
			current := state.GetCrawlJob(jobID)
			if current != nil {
				if current.Status == types.CrawlStatusCompleted {
					outputCrawlResults(*current)
					return nil
				}
				if current.Status == types.CrawlStatusFailed {
					errMsg := "unknown error"
					if current.Error != nil {
						errMsg = *current.Error
					}
					return fmt.Errorf("crawl failed: %s", errMsg)
				}
			}
		}
	}
}

// outputCrawlResults formats and outputs the final crawl results.
// Each scraped page is output on its own line as JSON for easy processing.
func outputCrawlResults(state types.CrawlState) {
	// For each page result, output a JSON object with the scraped data.
	for _, data := range state.Data {
		result := formatScrapeData(&data)
		output("%s\n", result)
	}

	// If there's an aggregated LLM answer, output it as well.
	if state.Answer != nil {
		var answer any
		if err := decodeJSON(state.Answer, &answer); err == nil {
			output("\n--- Aggregated Answer ---\n")
			output("%s\n", formatJSON(answer))
		}
	}

	if verbose {
		errPrint("crawl completed: %d pages scraped, %d URLs discovered\n",
			state.Completed, state.Total)
	}
}

// decodeJSON parses JSON bytes into a generic value.
// It's a helper to avoid repeating json.Unmarshal calls.
func decodeJSON(data []byte, out interface{}) error {
	return json.Unmarshal(data, out)
}

// formatJSON formats a generic value as indented JSON.
// Used for displaying structured data like LLM answers.
func formatJSON(v interface{}) string {
	encoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to format: %v"}`, err)
	}
	return string(encoded)
}